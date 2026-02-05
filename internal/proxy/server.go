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
	CA        CAConfig        `yaml:"ca"`
}

// CAConfig holds CA certificate configuration for MITM
type CAConfig struct {
	CertPath string `yaml:"cert_path"`
	KeyPath  string `yaml:"key_path"`
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

// ScanTypeConfig configures behavior for a specific scan type
type ScanTypeConfig struct {
	Enabled       bool   `yaml:"enabled"`         // Whether this scan type is active
	ActionOnWarn  string `yaml:"action_on_warn"`  // "allow", "warn", "block"
	ActionOnBlock string `yaml:"action_on_block"` // "allow", "warn", "block"
}

// ScanningConfig holds scanning configuration
type ScanningConfig struct {
	Mode           string         `yaml:"mode"`
	BlockThreshold float64        `yaml:"block_threshold"`
	FailOpen       bool           `yaml:"fail_open"`
	Content        ScanTypeConfig `yaml:"content"` // Prompt injection scanning (incoming)
	Output         ScanTypeConfig `yaml:"output"`  // Credential leak scanning (outgoing)
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

// applyDefaultScanTypeConfig sets default values for ScanTypeConfig if not already set
func applyDefaultScanTypeConfig(cfg *ScanTypeConfig) {
	// If ActionOnWarn is empty, this is an old config without these fields
	if cfg.ActionOnWarn == "" {
		cfg.Enabled = true
		cfg.ActionOnWarn = "warn"
		cfg.ActionOnBlock = "block"
	}
}

// getAction determines what action to take based on scan decision and config
func getAction(decision Decision, cfg ScanTypeConfig) string {
	switch decision {
	case DecisionWarn:
		if cfg.ActionOnWarn != "" {
			return cfg.ActionOnWarn
		}
		return "warn" // Default
	case DecisionBlock:
		if cfg.ActionOnBlock != "" {
			return cfg.ActionOnBlock
		}
		return "block" // Default
	default:
		return "allow" // ALLOW decision always passes
	}
}

// Server is the HTTP/HTTPS proxy server
type Server struct {
	config         *Config
	scanner        *ScannerClient
	wallet         *wallet.Wallet
	httpServer     *http.Server
	listener       net.Listener
	logger         *slog.Logger
	httpClient     *http.Client
	ca             *CA
	certCache      *CertCache
	mitm           *MITMHandler
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

	// Create standard HTTP client (no socket marks needed - we use user-based filtering)
	httpClient := &http.Client{
		Timeout: 30 * time.Second,
		// Don't follow redirects to prevent payment headers from being sent
		// to attacker-controlled URLs via redirect chains
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	s := &Server{
		config:     config,
		scanner:    scanner,
		logger:     logger,
		httpClient: httpClient,
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

	// Load or create CA for MITM
	if config.CA.CertPath != "" && config.CA.KeyPath != "" {
		ca, err := LoadCA(config.CA.CertPath, config.CA.KeyPath)
		if err != nil {
			logger.Warn("failed to load CA, MITM disabled", "error", err)
		} else {
			s.ca = ca
			s.certCache = NewCertCache(ca)
			s.mitm = NewMITMHandler(s.certCache, scanner, config, logger)
			logger.Info("MITM enabled with CA certificate")
		}
	} else {
		// Try default CA location
		homeDir, _ := os.UserHomeDir()
		caDir := homeDir + "/.stronghold/ca"
		ca, err := LoadOrCreateCA(caDir)
		if err != nil {
			logger.Warn("failed to load/create CA, MITM disabled", "error", err)
		} else {
			s.ca = ca
			s.certCache = NewCertCache(ca)
			s.mitm = NewMITMHandler(s.certCache, scanner, config, logger)
			logger.Info("MITM enabled with CA certificate", "ca_dir", caDir)
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
			Content: ScanTypeConfig{
				Enabled:       true,
				ActionOnWarn:  "warn",
				ActionOnBlock: "block",
			},
			Output: ScanTypeConfig{
				Enabled:       true,
				ActionOnWarn:  "warn",
				ActionOnBlock: "block",
			},
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
		// Apply defaults for new ScanTypeConfig fields if not set (migration)
		applyDefaultScanTypeConfig(&config.Scanning.Content)
		applyDefaultScanTypeConfig(&config.Scanning.Output)
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
	s.logger.Info("proxy listening", "addr", addr, "mitm_enabled", s.mitm != nil)

	// Start accepting raw connections for transparent proxy mode
	go s.acceptConnections(ctx)

	// Wait for context cancellation
	<-ctx.Done()
	return nil
}

// acceptConnections handles incoming TCP connections
func (s *Server) acceptConnections(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		conn, err := s.listener.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return // Context cancelled
			}
			s.logger.Error("accept error", "error", err)
			continue
		}

		go s.handleConnection(conn)
	}
}

// handleConnection handles a single incoming connection
// It detects whether the traffic is TLS or HTTP and routes accordingly
func (s *Server) handleConnection(conn net.Conn) {
	// Set initial read deadline for protocol detection
	conn.SetReadDeadline(time.Now().Add(10 * time.Second))

	// Peek at first byte to detect protocol
	buf := make([]byte, 1)
	n, err := conn.Read(buf)
	if err != nil || n == 0 {
		conn.Close()
		return
	}

	// Reset deadline
	conn.SetReadDeadline(time.Time{})

	// Create a connection that includes the peeked byte
	prefixedConn := newPrefixedConn(conn, buf[:n])

	// Check if this is TLS (ClientHello starts with 0x16)
	if buf[0] == 0x16 {
		// TLS connection - handle with MITM if available
		if s.mitm != nil {
			// Get original destination for transparent mode
			// First try SO_ORIGINAL_DST (Linux only)
			originalDst, err := GetOriginalDst(conn)
			if err != nil {
				// SO_ORIGINAL_DST failed (macOS or error) - extract SNI from ClientHello
				s.logger.Debug("SO_ORIGINAL_DST failed, extracting SNI", "error", err)

				sni, fullClientHello, sniErr := ExtractSNI(conn, buf[:n])
				if sniErr != nil {
					s.logger.Error("failed to extract SNI", "error", sniErr)
					conn.Close()
					return
				}

				// Use SNI hostname with default HTTPS port
				originalDst = sni + ":443"
				s.logger.Debug("extracted SNI for destination", "sni", sni, "dst", originalDst)

				// Create new prefixed connection with the full ClientHello we read
				prefixedConn = newPrefixedConn(conn, fullClientHello)
			}
			s.mitm.HandleTLS(prefixedConn, originalDst)
		} else {
			// No MITM - just tunnel the connection
			s.logger.Debug("TLS connection but MITM not available, tunneling")
			s.tunnelConnection(prefixedConn)
		}
	} else {
		// HTTP connection - handle with HTTP server
		s.handleHTTPConnection(prefixedConn)
	}
}

// handleHTTPConnection handles an HTTP connection
func (s *Server) handleHTTPConnection(conn net.Conn) {
	defer conn.Close()

	// Serve HTTP using the standard library server
	s.httpServer.ConnContext = func(ctx context.Context, c net.Conn) context.Context {
		return ctx
	}

	// Create a single-connection listener
	singleConnListener := &singleConnListener{conn: conn, done: make(chan struct{})}
	s.httpServer.Serve(singleConnListener)
}

// tunnelConnection tunnels a TLS connection without MITM
func (s *Server) tunnelConnection(conn net.Conn) {
	defer conn.Close()

	var originalDst string
	var tunnelConn net.Conn = conn

	// Try to get underlying TCP connection for SO_ORIGINAL_DST
	underlyingConn := conn
	if pc, ok := conn.(*prefixedConn); ok {
		underlyingConn = pc.Conn
	}

	// Try SO_ORIGINAL_DST first (Linux)
	var err error
	originalDst, err = GetOriginalDst(underlyingConn)
	if err != nil {
		// SO_ORIGINAL_DST failed - try SNI extraction
		s.logger.Debug("SO_ORIGINAL_DST failed for tunnel, extracting SNI", "error", err)

		// Get the prefix data if this is a prefixedConn
		var prefix []byte
		if pc, ok := conn.(*prefixedConn); ok {
			prefix = pc.prefix
		}

		// We need to read the TLS ClientHello for SNI
		sni, fullClientHello, sniErr := ExtractSNI(underlyingConn, prefix)
		if sniErr != nil {
			s.logger.Error("failed to extract SNI for tunnel", "error", sniErr)
			return
		}

		originalDst = sni + ":443"
		s.logger.Debug("extracted SNI for tunnel", "sni", sni, "dst", originalDst)

		// Create new prefixed connection with full ClientHello for tunneling
		tunnelConn = newPrefixedConn(underlyingConn, fullClientHello)
	}

	// Connect to destination
	destConn, err := net.DialTimeout("tcp", originalDst, 10*time.Second)
	if err != nil {
		s.logger.Error("failed to connect to destination", "dest", originalDst, "error", err)
		return
	}
	defer destConn.Close()

	// Bidirectional copy
	done := make(chan struct{})
	go func() {
		io.Copy(destConn, tunnelConn)
		close(done)
	}()
	io.Copy(tunnelConn, destConn)
	<-done
}

// prefixedConn wraps a connection with a prefix that was already read
type prefixedConn struct {
	net.Conn
	prefix []byte
	read   bool
}

func newPrefixedConn(conn net.Conn, prefix []byte) *prefixedConn {
	return &prefixedConn{
		Conn:   conn,
		prefix: prefix,
		read:   false,
	}
}

func (c *prefixedConn) Read(b []byte) (int, error) {
	if !c.read && len(c.prefix) > 0 {
		c.read = true
		n := copy(b, c.prefix)
		if n < len(c.prefix) {
			// Partial read of prefix - shouldn't happen with single byte
			c.prefix = c.prefix[n:]
			c.read = false
		}
		return n, nil
	}
	return c.Conn.Read(b)
}

// singleConnListener is a net.Listener that serves a single connection
type singleConnListener struct {
	conn net.Conn
	done chan struct{}
	once sync.Once
}

func (l *singleConnListener) Accept() (net.Conn, error) {
	select {
	case <-l.done:
		return nil, net.ErrClosed
	default:
	}

	var conn net.Conn
	l.once.Do(func() {
		conn = l.conn
		close(l.done)
	})

	if conn == nil {
		return nil, net.ErrClosed
	}
	return conn, nil
}

func (l *singleConnListener) Close() error {
	return nil
}

func (l *singleConnListener) Addr() net.Addr {
	return l.conn.LocalAddr()
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

	// Perform the request using standard client
	// (no socket marks needed - we use user-based filtering via nftables/pf)
	resp, err := s.httpClient.Do(outReq)
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

	// Add base Stronghold headers
	requestID := generateRequestID()
	w.Header().Set("X-Stronghold-Request-ID", requestID)
	w.Header().Set("X-Stronghold-Scan-Latency", fmt.Sprintf("%dms", time.Since(start).Milliseconds()))

	// Scan the response if content scanning is enabled
	contentType := resp.Header.Get("Content-Type")
	var scanResult *ScanResult
	if s.config.Scanning.Content.Enabled {
		scanResult = s.scanResponse(body, targetURL, contentType)
	}

	// Determine action based on scan result and config
	var action string
	if scanResult != nil {
		action = getAction(scanResult.Decision, s.config.Scanning.Content)

		// Always add scan result headers (even when not blocking)
		w.Header().Set("X-Stronghold-Decision", string(scanResult.Decision))
		w.Header().Set("X-Stronghold-Reason", scanResult.Reason)
		w.Header().Set("X-Stronghold-Action", action)
		w.Header().Set("X-Stronghold-Scan-Type", "content")
		if score, ok := scanResult.Scores["combined"]; ok {
			w.Header().Set("X-Stronghold-Score", fmt.Sprintf("%.2f", score))
		} else if score, ok := scanResult.Scores["heuristic"]; ok {
			w.Header().Set("X-Stronghold-Score", fmt.Sprintf("%.2f", score))
		}

		// Update counters based on original decision
		if scanResult.Decision == DecisionBlock {
			s.mu.Lock()
			s.blockedCount++
			s.mu.Unlock()
		} else if scanResult.Decision == DecisionWarn {
			s.mu.Lock()
			s.warnedCount++
			s.mu.Unlock()
		}

		// Handle action
		switch action {
		case "block":
			s.logger.Warn("content blocked", "url", targetURL, "reason", scanResult.Reason, "decision", scanResult.Decision)
			w.WriteHeader(http.StatusForbidden)
			w.Write([]byte(fmt.Sprintf(`{
	"error": "Content blocked by Stronghold security scan",
	"reason": "%s",
	"request_id": "%s",
	"recommended_action": "%s"
}`, scanResult.Reason, requestID, scanResult.RecommendedAction)))
			return
		case "warn":
			s.logger.Warn("content warned", "url", targetURL, "reason", scanResult.Reason, "decision", scanResult.Decision)
			w.Header().Set("X-Stronghold-Warning", scanResult.Reason)
			// Continue to forward response
		default: // "allow"
			s.logger.Debug("content allowed despite scan result", "url", targetURL, "decision", scanResult.Decision)
			// Continue to forward response (headers still present)
		}
	} else {
		// No scan performed or content not scannable
		w.Header().Set("X-Stronghold-Decision", "ALLOW")
		w.Header().Set("X-Stronghold-Action", "allow")
		if !s.config.Scanning.Content.Enabled {
			w.Header().Set("X-Stronghold-Scan-Type", "disabled")
		}
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

// handleConnect handles HTTPS CONNECT requests (explicit proxy mode)
func (s *Server) handleConnect(w http.ResponseWriter, r *http.Request) {
	// Use standard dialer (no socket marks needed - we use user-based filtering)
	dialer := &net.Dialer{Timeout: 10 * time.Second}
	destConn, err := dialer.Dial("tcp", r.Host)
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

	// For CONNECT requests with MITM enabled, intercept TLS
	if s.mitm != nil {
		s.mitm.HandleTLS(clientConn, r.Host)
		return
	}

	// No MITM - bidirectional tunnel
	done := make(chan struct{})
	go func() {
		defer func() {
			if r := recover(); r != nil {
				s.logger.Error("panic in HTTPS tunnel goroutine (client->dest)", "panic", r)
			}
		}()
		io.Copy(destConn, clientConn)
		close(done)
	}()

	defer func() {
		if r := recover(); r != nil {
			s.logger.Error("panic in HTTPS tunnel (dest->client)", "panic", r)
		}
	}()
	io.Copy(clientConn, destConn)
	<-done
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
