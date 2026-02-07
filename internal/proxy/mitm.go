package proxy

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"
)

// MITMHandler handles transparent HTTPS interception (Man-In-The-Middle)
type MITMHandler struct {
	certCache *CertCache
	scanner   *ScannerClient
	config    *Config
	logger    *slog.Logger
}

// NewMITMHandler creates a new MITM handler
func NewMITMHandler(certCache *CertCache, scanner *ScannerClient, config *Config, logger *slog.Logger) *MITMHandler {
	return &MITMHandler{
		certCache: certCache,
		scanner:   scanner,
		config:    config,
		logger:    logger,
	}
}

// HandleTLS intercepts a TLS connection for content inspection
func (m *MITMHandler) HandleTLS(clientConn net.Conn, originalDst string) error {
	defer clientConn.Close()

	// Parse host from original destination
	host, port, err := net.SplitHostPort(originalDst)
	if err != nil {
		host = originalDst
		port = "443"
	}

	m.logger.Debug("MITM intercepting TLS connection", "host", host, "port", port)

	// Get certificate for this domain
	cert, err := m.certCache.GetCert(host)
	if err != nil {
		m.logger.Error("failed to get certificate for host", "host", host, "error", err)
		return fmt.Errorf("failed to get certificate: %w", err)
	}

	// Create TLS config with our certificate
	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{*cert},
		MinVersion:   tls.VersionTLS12,
	}

	// Wrap client connection in TLS (we're the server to the client)
	// Set deadline for TLS handshake to prevent slow clients from tying up resources
	clientConn.SetDeadline(time.Now().Add(10 * time.Second))
	tlsClientConn := tls.Server(clientConn, tlsConfig)
	if err := tlsClientConn.Handshake(); err != nil {
		m.logger.Debug("TLS handshake with client failed", "host", host, "error", err)
		return fmt.Errorf("TLS handshake failed: %w", err)
	}
	// Clear deadline after successful handshake
	tlsClientConn.SetDeadline(time.Time{})
	defer tlsClientConn.Close()

	// Connect to actual server with TLS (with connection timeout)
	serverConn, err := tls.DialWithDialer(&net.Dialer{Timeout: 10 * time.Second}, "tcp", originalDst, &tls.Config{
		ServerName: host,
		MinVersion: tls.VersionTLS12,
	})
	if err != nil {
		m.logger.Error("failed to connect to server", "host", host, "error", err)
		return fmt.Errorf("failed to connect to server: %w", err)
	}
	defer serverConn.Close()

	// Handle HTTP requests over the TLS connection
	return m.proxyHTTPS(tlsClientConn, serverConn, host)
}

// proxyHTTPS proxies HTTP requests over established TLS connections
func (m *MITMHandler) proxyHTTPS(clientConn, serverConn net.Conn, host string) error {
	clientReader := bufio.NewReader(clientConn)
	serverReader := bufio.NewReader(serverConn)

	for {
		// Set read deadline to detect closed connections
		clientConn.SetReadDeadline(time.Now().Add(30 * time.Second))

		// Read HTTP request from client
		req, err := http.ReadRequest(clientReader)
		if err != nil {
			if err == io.EOF || strings.Contains(err.Error(), "connection reset") {
				return nil // Normal connection close
			}
			return fmt.Errorf("failed to read request: %w", err)
		}

		// Fix up the request URL for proxying
		req.URL.Scheme = "https"
		req.URL.Host = host
		req.RequestURI = "" // Must be empty for client requests

		m.logger.Debug("MITM request", "method", req.Method, "url", req.URL.String())

		// Scan request body if it exists (for prompt injection in POST data)
		var requestBody []byte
		if req.Body != nil && req.ContentLength != 0 && m.config.Scanning.Content.Enabled {
			requestBody, _ = io.ReadAll(io.LimitReader(req.Body, 1024*1024+1))
			req.Body.Close()

			// Scan the request content (skip if over 1MB)
			if len(requestBody) > 0 && len(requestBody) <= 1024*1024 {
				result := m.scanContent(requestBody, req.URL.String(), req.Header.Get("Content-Type"))
				if result != nil && result.Decision == DecisionBlock {
					// Block the request
					m.sendBlockResponse(clientConn, result, req)
					continue
				}
			}

			// Restore body for forwarding
			req.Body = io.NopCloser(strings.NewReader(string(requestBody)))
		}

		// Forward request to server
		if err := req.Write(serverConn); err != nil {
			return fmt.Errorf("failed to forward request: %w", err)
		}

		// Read response from server
		resp, err := http.ReadResponse(serverReader, req)
		if err != nil {
			return fmt.Errorf("failed to read response: %w", err)
		}

		// Check if response should be scanned before reading the full body
		contentType := resp.Header.Get("Content-Type")
		shouldScan := m.config.Scanning.Content.Enabled &&
			ShouldScanContentType(contentType) && !IsBinaryContentType(contentType)

		if shouldScan {
			// Read body for scanning (with 1MB limit + 1 byte to detect oversized)
			responseBody, err := io.ReadAll(io.LimitReader(resp.Body, 1024*1024+1))
			resp.Body.Close()
			if err != nil {
				return fmt.Errorf("failed to read response body: %w", err)
			}

			// Scan if within size limit
			var scanResult *ScanResult
			if len(responseBody) > 0 && len(responseBody) <= 1024*1024 {
				scanResult = m.scanContent(responseBody, req.URL.String(), contentType)
			}

			// Add Stronghold headers
			resp.Header.Set("X-Stronghold-Proxy", "mitm")
			if scanResult != nil {
				resp.Header.Set("X-Stronghold-Decision", string(scanResult.Decision))
				resp.Header.Set("X-Stronghold-Reason", scanResult.Reason)

				// Block if needed
				action := getAction(scanResult.Decision, m.config.Scanning.Content)
				if action == "block" {
					m.sendBlockResponse(clientConn, scanResult, req)
					continue
				}
			}

			// Forward response to client with the read body
			resp.Body = io.NopCloser(bytes.NewReader(responseBody))
			resp.ContentLength = int64(len(responseBody))

			if err := resp.Write(clientConn); err != nil {
				return fmt.Errorf("failed to forward response: %w", err)
			}
		} else {
			// Non-scannable content: stream directly without buffering
			resp.Header.Set("X-Stronghold-Proxy", "mitm")
			if err := resp.Write(clientConn); err != nil {
				resp.Body.Close()
				return fmt.Errorf("failed to forward response: %w", err)
			}
			resp.Body.Close()
		}
	}
}

// scanContent scans content for threats
func (m *MITMHandler) scanContent(body []byte, sourceURL, contentType string) *ScanResult {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	result, err := m.scanner.ScanContent(ctx, body, sourceURL, contentType)
	if err != nil {
		m.logger.Error("scan error", "error", err)
		if m.config.Scanning.FailOpen {
			return nil
		}
		return &ScanResult{
			Decision: DecisionBlock,
			Reason:   "Scan failed - blocking for safety",
		}
	}

	return result
}

// sendBlockResponse sends a block response to the client
func (m *MITMHandler) sendBlockResponse(conn net.Conn, result *ScanResult, req *http.Request) {
	m.logger.Warn("content blocked", "url", req.URL.String(), "reason", result.Reason)

	bodyBytes, _ := json.Marshal(struct {
		Error  string `json:"error"`
		Reason string `json:"reason"`
		URL    string `json:"url"`
	}{
		Error:  "Content blocked by Stronghold security scan",
		Reason: result.Reason,
		URL:    req.URL.String(),
	})
	body := string(bodyBytes)

	resp := &http.Response{
		StatusCode:    http.StatusForbidden,
		Status:        "403 Forbidden",
		Proto:         "HTTP/1.1",
		ProtoMajor:    1,
		ProtoMinor:    1,
		Header:        make(http.Header),
		Body:          io.NopCloser(strings.NewReader(body)),
		ContentLength: int64(len(body)),
		Request:       req,
	}

	resp.Header.Set("Content-Type", "application/json")
	resp.Header.Set("X-Stronghold-Decision", string(result.Decision))
	resp.Header.Set("X-Stronghold-Reason", result.Reason)

	resp.Write(conn)
}
