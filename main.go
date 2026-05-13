package main

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

type Config struct {
	ListenAddr string

	TargetHost   string
	TargetPort   string
	TargetScheme string

	VlessUUID string
	VlessPath string
	LinkName  string

	ClientAddressOverride string
}

func env(key, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}

func loadConfig() Config {
	return Config{
		ListenAddr: env("LISTEN_ADDR", "0.0.0.0:3000"),

		TargetHost:   env("TARGET_HOST", "212.95.41.118"),
		TargetPort:   env("TARGET_PORT", "48560"),
		TargetScheme: env("TARGET_SCHEME", "http"),

		VlessUUID: env("VLESS_UUID", ""),
		VlessPath: env("VLESS_PATH", "/"),
		LinkName:  env("LINK_NAME", "g2ray-lwq4w11y"),

		// Optional.
		// Example: 94.130.50.12
		ClientAddressOverride: env("CLIENT_ADDRESS_OVERRIDE", ""),
	}
}

func codespacesPublicHost(listenAddr string) string {
	codespaceName := strings.TrimSpace(os.Getenv("CODESPACE_NAME"))
	domain := strings.TrimSpace(os.Getenv("GITHUB_CODESPACES_PORT_FORWARDING_DOMAIN"))

	if codespaceName == "" {
		return ""
	}

	if domain == "" {
		domain = "app.github.dev"
	}

	_, port, err := net.SplitHostPort(listenAddr)
	if err != nil {
		port = "3000"
	}

	return fmt.Sprintf("%s-%s.%s", codespaceName, port, domain)
}

func buildVlessLink(cfg Config, publicHost string, address string) string {
	if cfg.VlessUUID == "" || publicHost == "" || address == "" {
		return ""
	}

	values := url.Values{}
	values.Set("encryption", "none")
	values.Set("security", "tls")
	values.Set("sni", publicHost)
	values.Set("fp", "chrome")
	values.Set("type", "ws")
	values.Set("host", publicHost)
	values.Set("path", cfg.VlessPath)

	return fmt.Sprintf(
		"vless://%s@%s:443?%s#%s",
		cfg.VlessUUID,
		address,
		values.Encode(),
		url.QueryEscape(cfg.LinkName),
	)
}

func isWebSocket(r *http.Request) bool {
	connection := strings.ToLower(r.Header.Get("Connection"))
	upgrade := strings.ToLower(r.Header.Get("Upgrade"))

	return strings.Contains(connection, "upgrade") && upgrade == "websocket"
}

func pipeThenClose(dst net.Conn, src net.Conn, done chan<- struct{}) {
	_, _ = io.Copy(dst, src)
	_ = dst.Close()
	_ = src.Close()

	select {
	case done <- struct{}{}:
	default:
	}
}

func copyBufferedToTarget(targetConn net.Conn, rw *bufio.ReadWriter) {
	if rw == nil || rw.Reader == nil {
		return
	}

	if rw.Reader.Buffered() <= 0 {
		return
	}

	_, _ = io.CopyN(targetConn, rw, int64(rw.Reader.Buffered()))
}

func tunnelWebSocket(cfg Config, w http.ResponseWriter, r *http.Request) {
	hijacker, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "hijacking unsupported", http.StatusInternalServerError)
		return
	}

	targetAddr := net.JoinHostPort(cfg.TargetHost, cfg.TargetPort)

	targetConn, err := net.DialTimeout("tcp", targetAddr, 15*time.Second)
	if err != nil {
		log.Printf("[WS] dial target error: %v", err)
		http.Error(w, "bad gateway", http.StatusBadGateway)
		return
	}

	clientConn, rw, err := hijacker.Hijack()
	if err != nil {
		_ = targetConn.Close()
		log.Printf("[WS] hijack error: %v", err)
		return
	}

	// Keep original Host from Codespaces.
	// This is important because the client reaches GitHub Codespaces with that Host.
	r.RequestURI = r.URL.RequestURI()

	if err := r.Write(targetConn); err != nil {
		_ = clientConn.Close()
		_ = targetConn.Close()
		log.Printf("[WS] write upgrade request error: %v", err)
		return
	}

	copyBufferedToTarget(targetConn, rw)

	log.Printf("[WS] %s %s Host=%s -> %s", r.RemoteAddr, r.URL.Path, r.Host, targetAddr)

	done := make(chan struct{}, 2)

	go pipeThenClose(targetConn, clientConn, done)
	go pipeThenClose(clientConn, targetConn, done)

	<-done
}

func proxyHTTP(cfg Config, w http.ResponseWriter, r *http.Request) {
	targetURL := &url.URL{
		Scheme:   cfg.TargetScheme,
		Host:     net.JoinHostPort(cfg.TargetHost, cfg.TargetPort),
		Path:     r.URL.Path,
		RawQuery: r.URL.RawQuery,
	}

	req, err := http.NewRequestWithContext(r.Context(), r.Method, targetURL.String(), r.Body)
	if err != nil {
		http.Error(w, "failed to build upstream request", http.StatusBadGateway)
		return
	}

	req.Header = r.Header.Clone()
	req.Host = r.Host

	client := &http.Client{
		Timeout: 0,
		Transport: &http.Transport{
			Proxy:                 http.ProxyFromEnvironment,
			MaxIdleConns:          4096,
			MaxIdleConnsPerHost:   4096,
			IdleConnTimeout:       90 * time.Second,
			TLSHandshakeTimeout:   10 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
		},
	}

	resp, err := client.Do(req)
	if err != nil {
		log.Printf("[HTTP] proxy error: %v", err)
		http.Error(w, "bad gateway", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	for key, values := range resp.Header {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}

	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)
}

func main() {
	cfg := loadConfig()

	publicHost := codespacesPublicHost(cfg.ListenAddr)

	clientAddress := publicHost
	if cfg.ClientAddressOverride != "" {
		clientAddress = cfg.ClientAddressOverride
	}

	target := fmt.Sprintf("%s://%s", cfg.TargetScheme, net.JoinHostPort(cfg.TargetHost, cfg.TargetPort))

	log.Println("============================================================")
	log.Println("g2ray-lite-forwarder-go started")
	log.Printf("Listen: %s", cfg.ListenAddr)
	log.Printf("Target: %s", target)

	if publicHost != "" {
		log.Printf("Codespaces Host: %s", publicHost)
		log.Printf("Public URL: https://%s", publicHost)
	} else {
		log.Println("Codespaces Host: not detected")
	}

	if cfg.VlessUUID == "" {
		log.Println("")
		log.Println("VLESS_UUID is empty.")
		log.Println("Set VLESS_UUID as a GitHub Codespaces Secret.")
	} else if publicHost != "" {
		log.Println("")
		log.Println("Final VLESS link using Codespaces domain as address:")
		log.Println(buildVlessLink(cfg, publicHost, publicHost))

		if cfg.ClientAddressOverride != "" {
			log.Println("")
			log.Println("Final VLESS link using CLIENT_ADDRESS_OVERRIDE:")
			log.Println(buildVlessLink(cfg, publicHost, cfg.ClientAddressOverride))
		}
	}

	log.Println("============================================================")

	mux := http.NewServeMux()

	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("content-type", "text/plain; charset=utf-8")
		w.Header().Set("cache-control", "no-store")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok\n"))
	})

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if isWebSocket(r) {
			tunnelWebSocket(cfg, w, r)
			return
		}

		proxyHTTP(cfg, w, r)
	})

	server := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           mux,
		ReadHeaderTimeout: 20 * time.Second,
	}

	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("server error: %v", err)
	}
}
