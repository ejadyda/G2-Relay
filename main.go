package main

import (
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
		LinkName:  env("LINK_NAME", "g2ray-lite"),
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

func buildVlessLink(cfg Config, publicHost string) string {
	if cfg.VlessUUID == "" || publicHost == "" {
		return ""
	}

	values := url.Values{}
	values.Set("type", "ws")
	values.Set("encryption", "none")
	values.Set("security", "tls")
	values.Set("path", cfg.VlessPath)
	values.Set("host", publicHost)
	values.Set("sni", publicHost)

	return fmt.Sprintf(
		"vless://%s@%s:443?%s#%s",
		cfg.VlessUUID,
		publicHost,
		values.Encode(),
		url.QueryEscape(cfg.LinkName),
	)
}

func copyHeader(dst, src http.Header) {
	for key, values := range src {
		for _, value := range values {
			dst.Add(key, value)
		}
	}
}

func reverseProxyHTTP(cfg Config, w http.ResponseWriter, r *http.Request) {
	targetURL := &url.URL{
		Scheme:   cfg.TargetScheme,
		Host:     net.JoinHostPort(cfg.TargetHost, cfg.TargetPort),
		Path:     r.URL.Path,
		RawQuery: r.URL.RawQuery,
	}

	req, err := http.NewRequestWithContext(r.Context(), r.Method, targetURL.String(), r.Body)
	if err != nil {
		http.Error(w, "failed to build request", http.StatusBadGateway)
		return
	}

	req.Header = r.Header.Clone()
	req.Host = r.Host

	client := &http.Client{
		Timeout: 0,
		Transport: &http.Transport{
			Proxy:                 http.ProxyFromEnvironment,
			MaxIdleConns:          1024,
			MaxIdleConnsPerHost:   1024,
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

	copyHeader(w.Header(), resp.Header)
	w.WriteHeader(resp.StatusCode)

	_, _ = io.Copy(w, resp.Body)
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

	clientConn, _, err := hijacker.Hijack()
	if err != nil {
		_ = targetConn.Close()
		log.Printf("[WS] hijack error: %v", err)
		return
	}

	err = r.Write(targetConn)
	if err != nil {
		_ = clientConn.Close()
		_ = targetConn.Close()
		log.Printf("[WS] write upgrade request error: %v", err)
		return
	}

	log.Printf("[WS] %s %s -> %s", r.RemoteAddr, r.URL.Path, targetAddr)

	errc := make(chan error, 2)

	go func() {
		_, err := io.Copy(targetConn, clientConn)
		errc <- err
	}()

	go func() {
		_, err := io.Copy(clientConn, targetConn)
		errc <- err
	}()

	<-errc

	_ = clientConn.Close()
	_ = targetConn.Close()
}

func isWebSocket(r *http.Request) bool {
	connection := strings.ToLower(r.Header.Get("Connection"))
	upgrade := strings.ToLower(r.Header.Get("Upgrade"))

	return strings.Contains(connection, "upgrade") && upgrade == "websocket"
}

func main() {
	cfg := loadConfig()

	publicHost := codespacesPublicHost(cfg.ListenAddr)
	vlessLink := buildVlessLink(cfg, publicHost)

	target := fmt.Sprintf("%s://%s", cfg.TargetScheme, net.JoinHostPort(cfg.TargetHost, cfg.TargetPort))

	log.Println("============================================================")
	log.Println("g2ray-lite-forwarder-go started")
	log.Printf("Listen: %s", cfg.ListenAddr)
	log.Printf("Target: %s", target)

	if publicHost != "" {
		log.Printf("Public URL: https://%s", publicHost)
	} else {
		log.Println("Public URL: not running inside GitHub Codespaces")
	}

	if vlessLink != "" {
		log.Println("")
		log.Println("Final VLESS link:")
		log.Println(vlessLink)
	} else {
		log.Println("")
		log.Println("VLESS_UUID is empty.")
		log.Println("Set VLESS_UUID as a GitHub Codespaces Secret or environment variable to print the final link.")
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

		reverseProxyHTTP(cfg, w, r)
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
