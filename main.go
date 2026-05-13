package main

import (
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
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

// tunnelWebSocket forwards WebSocket connections by hijacking and piping data bidirectionally.
func tunnelWebSocket(cfg Config, w http.ResponseWriter, r *http.Request) {
	log.Printf("[WS] New WebSocket request: %s %s from %s", r.Method, r.URL.Path, r.RemoteAddr)
	log.Printf("[WS] Connection: %s, Upgrade: %s", r.Header.Get("Connection"), r.Header.Get("Upgrade"))
	log.Printf("[WS] Host header: %s", r.Header.Get("Host"))

	hijacker, ok := w.(http.Hijacker)
	if !ok {
		log.Printf("[WS] ERROR: hijacking not supported")
		http.Error(w, "hijacking unsupported", http.StatusInternalServerError)
		return
	}

	targetAddr := net.JoinHostPort(cfg.TargetHost, cfg.TargetPort)
	log.Printf("[WS] Dialing target: %s", targetAddr)

	targetConn, err := net.DialTimeout("tcp", targetAddr, 15*time.Second)
	if err != nil {
		log.Printf("[WS] ERROR: failed to dial target: %v", err)
		http.Error(w, "bad gateway", http.StatusBadGateway)
		return
	}

	clientConn, rw, err := hijacker.Hijack()
	if err != nil {
		_ = targetConn.Close()
		log.Printf("[WS] ERROR: hijack failed: %v", err)
		return
	}

	// Write the incoming HTTP request to the target connection, preserving the original Host.
	// RequestURI must be empty for proxy requests.
	if err := r.Write(targetConn); err != nil {
		_ = clientConn.Close()
		_ = targetConn.Close()
		log.Printf("[WS] ERROR: failed to write upgrade request to target: %v", err)
		return
	}

	// Copy any buffered data that hasn't been sent yet.
	if rw.Reader != nil && rw.Reader.Buffered() > 0 {
		if n, err := io.CopyN(targetConn, rw.Reader, int64(rw.Reader.Buffered())); err != nil && err != io.EOF {
			log.Printf("[WS] WARNING: buffered copy error: %v", err)
		} else if n > 0 {
			log.Printf("[WS] Copied %d buffered bytes to target", n)
		}
	}

	log.Printf("[WS] Tunnel established: %s -> %s", r.RemoteAddr, targetAddr)

	// Pipe data bidirectionally until connection closes.
	done := make(chan struct{}, 2)

	go func() {
		_, _ = io.Copy(targetConn, clientConn)
		_ = targetConn.Close()
		_ = clientConn.Close()
		select {
		case done <- struct{}{}:
		default:
		}
	}()

	go func() {
		_, _ = io.Copy(clientConn, targetConn)
		_ = clientConn.Close()
		_ = targetConn.Close()
		select {
		case done <- struct{}{}:
		default:
		}
	}()

	// Wait for at least one goroutine to finish (indicates connection closed).
	<-done
	log.Printf("[WS] Tunnel closed: %s", r.RemoteAddr)
}

// proxyHTTP forwards HTTP requests using ReverseProxy.
func proxyHTTP(cfg Config, targetURL *url.URL) http.Handler {
	return &httputil.ReverseProxy{
		Director: func(req *http.Request) {
			log.Printf("[HTTP] Incoming request: %s %s from %s", req.Method, req.URL.Path, req.RemoteAddr)
			log.Printf("[HTTP] Host header: %s", req.Header.Get("Host"))

			// Update the request to point to the target.
			req.URL.Scheme = targetURL.Scheme
			req.URL.Host = targetURL.Host
			// Preserve the original path and query.

			// Preserve the original Host header from the incoming request.
			// req.Host = req.URL.Host  (already set by ReverseProxy)

			log.Printf("[HTTP] Target URL: %s", req.URL.String())
		},
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			log.Printf("[HTTP] Proxy error: %v", err)
			http.Error(w, "bad gateway", http.StatusBadGateway)
		},
		Transport: &http.Transport{
			Proxy:                 http.ProxyFromEnvironment,
			MaxIdleConns:          4096,
			MaxIdleConnsPerHost:   4096,
			IdleConnTimeout:       90 * time.Second,
			TLSHandshakeTimeout:   10 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
		},
	}
}

func main() {
	cfg := loadConfig()

	publicHost := codespacesPublicHost(cfg.ListenAddr)

	targetURL := &url.URL{
		Scheme: cfg.TargetScheme,
		Host:   net.JoinHostPort(cfg.TargetHost, cfg.TargetPort),
	}

	target := fmt.Sprintf("%s://%s", cfg.TargetScheme, net.JoinHostPort(cfg.TargetHost, cfg.TargetPort))

	log.Println("============================================================")
	log.Println("g2ray-lite-forwarder-go started")
	log.Printf("Listen:      %s", cfg.ListenAddr)
	log.Printf("Target:      %s", target)
	log.Printf("Target Host: %s", cfg.TargetHost)
	log.Printf("Target Port: %s", cfg.TargetPort)

	if publicHost != "" {
		log.Printf("Codespaces Host: %s", publicHost)
		log.Printf("Public URL:      https://%s", publicHost)
	} else {
		log.Println("Codespaces Host: not detected (running locally)")
	}

	if cfg.VlessUUID == "" {
		log.Println("")
		log.Println("⚠️  VLESS_UUID is empty.")
		log.Println("Set VLESS_UUID environment variable or GitHub Codespaces Secret.")
	} else if publicHost != "" {
		log.Println("")
		log.Println("📝 Final VLESS link (Codespaces domain as address):")
		log.Println(buildVlessLink(cfg, publicHost, publicHost))

		if cfg.ClientAddressOverride != "" {
			log.Println("")
			log.Println("📝 Final VLESS link (CLIENT_ADDRESS_OVERRIDE as address):")
			log.Println(buildVlessLink(cfg, publicHost, cfg.ClientAddressOverride))
		}
	}

	log.Println("")
	log.Println("🔍 Troubleshooting:")
	log.Println("  curl -I http://127.0.0.1:3000/health")
	if publicHost != "" {
		log.Printf("  curl -I https://%s/health", publicHost)
		log.Printf("  curl -I --resolve %s:443:94.130.50.12 https://%s/health", publicHost, publicHost)
	}

	log.Println("============================================================")

	mux := http.NewServeMux()

	// Health check endpoint - no proxying.
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		log.Printf("[HEALTH] %s from %s", r.Method, r.RemoteAddr)
		w.Header().Set("content-type", "text/plain; charset=utf-8")
		w.Header().Set("cache-control", "no-store")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok\n"))
	})

	// Main handler: route WebSocket or HTTP requests.
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if isWebSocket(r) {
			tunnelWebSocket(cfg, w, r)
		} else {
			proxyHTTP(cfg, targetURL).ServeHTTP(w, r)
		}
	})

	server := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           mux,
		ReadHeaderTimeout: 20 * time.Second,
	}

	log.Printf("Listening on %s...", cfg.ListenAddr)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("server error: %v", err)
	}
}
