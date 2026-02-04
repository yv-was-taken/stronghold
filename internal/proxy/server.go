package proxy

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
	"stronghold/internal/wallet"
)

// Config holds the proxy configuration
type Config struct {
	Proxy     ProxyConfig     `yaml:"proxy"`
	API       APIConfig       `yaml:"api"`
	Auth      AuthConfig      `yaml:"auth"`
	Wallet    WalletConfig    `yaml:"wallet"`
	Scanning  ScanningConfig  `yaml:"scanning"`
	Logging   LoggingConfig   `yaml:"logging"`
}

// WalletConfig holds wallet configuration
type WalletConfig struct {
	Address string `yaml:"address"`
	Network string `yaml:"network"`
}

// ProxyConfig holds proxy-specific configuration
type ProxyConfig struct {
	Port int    `yaml:"port"`
	Bind string `yaml:"bind"`
}

// APIConfig holds API configuration
type APIConfig struct {
	Endpoint string        `yaml:"endpoint"`
	Timeout  time.Duration `yaml:"timeout"`
}

// AuthConfig holds authentication configuration
type AuthConfig struct {
	Token    string `yaml:"token"`
	Email    string `yaml:"email"`
	UserID   string `yaml:"user_id"`
	LoggedIn bool   `yaml:"logged_in"`
}

// ScanningConfig holds scanning configuration
type ScanningConfig struct {
	Mode           string  `yaml:"mode"`
	BlockThreshold float64 `yaml:"block_threshold"`
	FailOpen       bool    `yaml:"fail_open"`
}

// LoggingConfig holds logging configuration
type LoggingConfig struct {
	Level string `yaml:"level"`
	File  string `yaml:"file"`
}

// GetProxyAddr returns the proxy address
func (c *Config) GetProxyAddr() string {
	return fmt.Sprintf("%s:%d", c.Proxy.Bind, c.Proxy.Port)
}

// Server is the HTTP/HTTPS proxy server
type Server struct {
	config         *Config
	scanner        *ScannerClient
	wallet         *wallet.Wallet
	httpServer     *http.Server
	listener       net.Listener
	logger         *slog.Logger
	requestCount   int64
	blockedCount   int64
	warnedCount    int64
	mu             sync.RWMutex
}

// NewServer creates a new proxy server
func NewServer(config *Config) (*Server, error) {
	// Setup logging
	var handler slog.Handler
	output := os.Stdout

	if config.Logging.File != "" {
		if f, err := os.OpenFile(config.Logging.File, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644); err == nil {
			output = f
		}
	}

	handler = slog.NewTextHandler(output, &slog.HandlerOptions{
		Level:     slog.LevelDebug,
		AddSource: true,
	})
	logger := slog.New(handler)

	// Create scanner client
	scanner := NewScannerClient(config.API.Endpoint, config.Auth.Token)

	s := &Server{
		config:  config,
		scanner: scanner,
		logger:  logger,
	}

	// Load wallet if configured
	if config.Auth.UserID != "" && config.Wallet.Address != "" {
		w, err := wallet.New(wallet.Config{
			UserID:  config.Auth.UserID,
			Network: config.Wallet.Network,
		})
		if err != nil {
			logger.Warn("failed to load wallet", "error", err)
		} else if w.Exists() {
			s.wallet = w
			scanner.SetWallet(w)
			logger.Info("wallet loaded", "address", config.Wallet.Address)
		}
	}

	// Setup HTTP server
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleRequest)
	mux.HandleFunc("/health", s.handleHealth)

	s.httpServer = &http.Server{
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	return s, nil
}

// LoadConfig loads configuration from file or environment
func LoadConfig() (*Config, error) {
	config := &Config{
		Proxy: ProxyConfig{
			Port: 8402,
			Bind: "127.0.0.1",
		},
		API: APIConfig{
			Endpoint: "https://api.getstronghold.xyz",
			Timeout:  30 * time.Second,
		},
		Scanning: ScanningConfig{
			Mode:           "smart",
			BlockThreshold: 0.55,
			FailOpen:       true,
		},
		Logging: LoggingConfig{
			Level: "info",
		},
	}

	// Try to load from config file
	configPath := os.Getenv("STRONGHOLD_CONFIG")
	if configPath == "" {
		homeDir, _ := os.UserHomeDir()
		configPath = homeDir + "/.stronghold/config.yaml"
	}

	if data, err := os.ReadFile(configPath); err == nil {
		if err := yaml.Unmarshal(data, config); err != nil {
			return nil, fmt.Errorf("failed to parse config file: %w", err)
		}
	}

	// Override with environment variables
	if port := os.Getenv("STRONGHOLD_PROXY_PORT"); port != "" {
		if p, err := strconv.Atoi(port); err == nil {
			config.Proxy.Port = p
		}
	}

	if bind := os.Getenv("STRONGHOLD_PROXY_BIND"); bind != "" {
		config.Proxy.Bind = bind
	}

	if endpoint := os.Getenv("STRONGHOLD_API_ENDPOINT"); endpoint != "" {
		config.API.Endpoint = endpoint
	}

	return config, nil
}

// Start starts the proxy server
func (s *Server) Start(ctx context.Context) error {
	addr := s.config.GetProxyAddr()

	listener, err := net.Listen("tcp", addr)
	if err != nil {
		// Port in use - try to find an available one
		s.logger.Warn("configured port unavailable, searching for alternative",
			"original_port", s.config.Proxy.Port, "error", err)

		newPort := s.findAvailablePort(s.config.Proxy.Port + 1)
		if newPort == 0 {
			return fmt.Errorf("failed to listen on %s and no available ports found: %w", addr, err)
		}

		s.config.Proxy.Port = newPort
		addr = s.config.GetProxyAddr()

		listener, err = net.Listen("tcp", addr)
		if err != nil {
			return fmt.Errorf("failed to listen on fallback port %s: %w", addr, err)
		}

		s.logger.Info("using fallback port", "port", newPort)
	}

	s.listener = listener
	s.logger.Info("proxy listening", "addr", addr)

	// Start accepting connections
	go func() {
		if err := s.httpServer.Serve(listener); err != nil && err != http.ErrServerClosed {
			s.logger.Error("server error", "error", err)
		}
	}()

	// Wait for context cancellation
	<-ctx.Done()
	return nil
}

// Shutdown gracefully shuts down the server
func (s *Server) Shutdown(ctx context.Context) error {
	if s.listener != nil {
		return s.httpServer.Shutdown(ctx)
	}
	return nil
}

// findAvailablePort searches for an available port starting from startPort
func (s *Server) findAvailablePort(startPort int) int {
	for port := startPort; port < startPort+100; port++ {
		addr := fmt.Sprintf("%s:%d", s.config.Proxy.Bind, port)
		listener, err := net.Listen("tcp", addr)
		if err == nil {
			listener.Close()
			return port
		}
	}
	return 0
}

// handleRequest handles incoming HTTP requests
func (s *Server) handleRequest(w http.ResponseWriter, r *http.Request) {
	start := time.Now()

	// Log the request
	s.logger.Debug("incoming request", "method", r.Method, "url", r.URL.String(), "proto", r.Proto)

	// Increment request count
	s.mu.Lock()
	s.requestCount++
	s.mu.Unlock()

	// Handle CONNECT method for HTTPS proxying
	if r.Method == http.MethodConnect {
		s.handleConnect(w, r)
		return
	}

	// Handle regular HTTP requests
	s.handleHTTP(w, r, start)
}

// handleHTTP handles regular HTTP requests
func (s *Server) handleHTTP(w http.ResponseWriter, r *http.Request, start time.Time) {
	// Parse the target URL
	targetURL := r.URL.String()
	if !strings.HasPrefix(targetURL, "http") {
		targetURL = "http://" + r.Host + r.URL.String()
	}

	_, err := url.Parse(targetURL)
	if err != nil {
		s.logger.Error("error parsing URL", "error", err)
		http.Error(w, "Invalid URL", http.StatusBadRequest)
		return
	}

	// Create the outgoing request
	outReq, err := http.NewRequest(r.Method, targetURL, r.Body)
	if err != nil {
		s.logger.Error("error creating request", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// Copy headers
	for key, values := range r.Header {
		for _, value := range values {
			outReq.Header.Add(key, value)
		}
	}

	// Remove proxy-related headers
	outReq.Header.Del("Proxy-Connection")
	outReq.Header.Del("Proxy-Authenticate")

	// Perform the request
	client := &http.Client{
		Timeout: 30 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse // Don't follow redirects automatically
		},
	}

	resp, err := client.Do(outReq)
	if err != nil {
		s.logger.Error("error forwarding request", "error", err)
		http.Error(w, "Bad Gateway", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	// Read the response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		s.logger.Error("error reading response body", "error", err)
		http.Error(w, "Bad Gateway", http.StatusBadGateway)
		return
	}

	// Scan the response if it's a scannable content type
	contentType := resp.Header.Get("Content-Type")
	scanResult := s.scanResponse(body, targetURL, contentType)

	// Add Stronghold headers
	w.Header().Set("X-Stronghold-Request-ID", generateRequestID())
	w.Header().Set("X-Stronghold-Scan-Latency", fmt.Sprintf("%dms", time.Since(start).Milliseconds()))

	if scanResult != nil {
		w.Header().Set("X-Stronghold-Decision", string(scanResult.Decision))

		// Handle block decision
		if scanResult.Decision == DecisionBlock {
			s.mu.Lock()
			s.blockedCount++
			s.mu.Unlock()

			s.logger.Warn("content blocked", "url", targetURL, "reason", scanResult.Reason)

			w.WriteHeader(http.StatusForbidden)
			w.Write([]byte(fmt.Sprintf(`{
	"error": "Content blocked by Stronghold security scan",
	"reason": "%s",
	"request_id": "%s",
	"recommended_action": "%s"
}`, scanResult.Reason, generateRequestID(), scanResult.RecommendedAction)))
			return
		}

		// Handle warn decision
		if scanResult.Decision == DecisionWarn {
			s.mu.Lock()
			s.warnedCount++
			s.mu.Unlock()

			s.logger.Warn("content warned", "url", targetURL, "reason", scanResult.Reason)
			w.Header().Set("X-Stronghold-Warning", scanResult.Reason)
		}
	} else {
		w.Header().Set("X-Stronghold-Decision", "ALLOW")
	}

	// Copy response headers
	for key, values := range resp.Header {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}

	// Write response
	w.WriteHeader(resp.StatusCode)
	w.Write(body)
}

// handleConnect handles HTTPS CONNECT requests
func (s *Server) handleConnect(w http.ResponseWriter, r *http.Request) {
	destConn, err := net.DialTimeout("tcp", r.Host, 10*time.Second)
	if err != nil {
		s.logger.Error("error connecting to host", "host", r.Host, "error", err)
		http.Error(w, "Bad Gateway", http.StatusBadGateway)
		return
	}
	defer destConn.Close()

	w.WriteHeader(http.StatusOK)

	hijacker, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "Hijacking not supported", http.StatusInternalServerError)
		return
	}

	clientConn, _, err := hijacker.Hijack()
	if err != nil {
		s.logger.Error("error hijacking connection", "error", err)
		return
	}
	defer clientConn.Close()

	// Bidirectional copy with panic recovery and error logging
	go func() {
		defer func() {
			if r := recover(); r != nil {
				s.logger.Error("panic in HTTPS tunnel goroutine (client->dest)", "panic", r)
			}
		}()
		if _, err := io.Copy(destConn, clientConn); err != nil {
			s.logger.Debug("HTTPS tunnel copy error (client->dest)", "error", err)
		}
	}()

	defer func() {
		if r := recover(); r != nil {
			s.logger.Error("panic in HTTPS tunnel (dest->client)", "panic", r)
		}
	}()
	if _, err := io.Copy(clientConn, destConn); err != nil {
		s.logger.Debug("HTTPS tunnel copy error (dest->client)", "error", err)
	}
}

// scanResponse scans the response content
func (s *Server) scanResponse(body []byte, sourceURL, contentType string) *ScanResult {
	// Skip binary content
	if IsBinaryContentType(contentType) {
		return nil
	}

	// Skip if content is too large (> 1MB)
	if len(body) > 1024*1024 {
		s.logger.Debug("skipping scan: content too large", "bytes", len(body))
		return nil
	}

	// Check if we should scan this content type
	if !ShouldScanContentType(contentType) {
		return nil
	}

	// Perform the scan
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	result, err := s.scanner.ScanContent(ctx, body, sourceURL, contentType)
	if err != nil {
		s.logger.Error("scan error", "error", err)

		// Fail open or closed based on configuration
		if s.config.Scanning.FailOpen {
			return nil // Allow through
		}

		// Fail closed - block the request
		return &ScanResult{
			Decision:          DecisionBlock,
			Reason:            "Scan failed - blocking for safety",
			RecommendedAction: "Retry the request",
		}
	}

	return result
}

// handleHealth handles health check requests
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	stats := map[string]interface{}{
		"status":         "healthy",
		"requests_total": s.requestCount,
		"blocked":        s.blockedCount,
		"warned":         s.warnedCount,
	}
	s.mu.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	// Simple JSON encoding
	fmt.Fprintf(w, `{"status":"%s","requests_total":%d,"blocked":%d,"warned":%d}`,
		stats["status"], stats["requests_total"], stats["blocked"], stats["warned"])
}

// generateRequestID generates a simple request ID
func generateRequestID() string {
	return fmt.Sprintf("req-%d", time.Now().UnixNano())
}
