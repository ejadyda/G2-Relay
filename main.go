package main

import (
	"encoding/json"
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
	ListenAddr   string `json:"listen_addr"`
	TargetHost   string `json:"target_host"`
	TargetPort   string `json:"target_port"`
	TargetScheme string `json:"target_scheme"`
}

func loadConfigFromFile(filePath string) (Config, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return Config{}, fmt.Errorf("failed to read config file: %w", err)
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("failed to parse config file: %w", err)
	}

	// Validate required fields
	if cfg.ListenAddr == "" {
		return Config{}, fmt.Errorf("listen_addr is required in config.json")
	}
	if cfg.TargetHost == "" {
		return Config{}, fmt.Errorf("target_host is required in config.json")
	}
	if cfg.TargetPort == "" {
		return Config{}, fmt.Errorf("target_port is required in config.json")
	}
	if cfg.TargetScheme == "" {
		return Config{}, fmt.Errorf("target_scheme is required in config.json")
	}

	return cfg, nil
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
	cfg, err := loadConfigFromFile("config.json")
	if err != nil {
		log.Fatalf("configuration error: %v", err)
	}

	targetURL := &url.URL{
		Scheme: cfg.TargetScheme,
		Host:   net.JoinHostPort(cfg.TargetHost, cfg.TargetPort),
	}

	target := fmt.Sprintf("%s://%s", cfg.TargetScheme, net.JoinHostPort(cfg.TargetHost, cfg.TargetPort))

	log.Println("============================================================")
	log.Println("g2ray-lite-forwarder-go started")
	log.Printf("Loaded config from: config.json")
	log.Printf("Listen:  %s", cfg.ListenAddr)
	log.Printf("Target:  %s", target)
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
