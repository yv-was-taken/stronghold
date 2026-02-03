package proxy

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"stronghold/internal/wallet"
)

// Decision represents the scan decision
type Decision string

const (
	DecisionAllow Decision = "ALLOW"
	DecisionWarn  Decision = "WARN"
	DecisionBlock Decision = "BLOCK"
)

// Threat represents a detected threat
type Threat struct {
	Category    string `json:"category"`
	Pattern     string `json:"pattern"`
	Location    string `json:"location"`
	Severity    string `json:"severity"`
	Description string `json:"description"`
}

// ScanResult represents the result of a security scan
type ScanResult struct {
	Decision          Decision               `json:"decision"`
	Scores            map[string]float64     `json:"scores"`
	Reason            string                 `json:"reason"`
	LatencyMs         int64                  `json:"latency_ms"`
	RequestID         string                 `json:"request_id"`
	Metadata          map[string]interface{} `json:"metadata,omitempty"`
	SanitizedText     string                 `json:"sanitized_text,omitempty"`
	ThreatsFound      []Threat               `json:"threats_found,omitempty"`
	RecommendedAction string                 `json:"recommended_action,omitempty"`
}

// ScanRequest represents a scan request
type ScanRequest struct {
	Text        string `json:"text"`
	SourceURL   string `json:"source_url,omitempty"`
	SourceType  string `json:"source_type,omitempty"`
	ContentType string `json:"content_type,omitempty"`
}

// ScannerClient is a client for the Stronghold scanning API
type ScannerClient struct {
	baseURL       string
	token         string
	httpClient    *http.Client
	wallet        *wallet.Wallet
	facilitatorURL string
}

// NewScannerClient creates a new scanner client
func NewScannerClient(baseURL, token string) *ScannerClient {
	return &ScannerClient{
		baseURL: baseURL,
		token:   token,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
			// Don't follow redirects to prevent payment headers from being sent
			// to attacker-controlled URLs via redirect chains
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
		facilitatorURL: "https://x402.org/facilitator",
	}
}

// SetWallet sets the wallet for x402 payments
func (c *ScannerClient) SetWallet(w *wallet.Wallet) {
	c.wallet = w
}

// ScanContent scans external content for prompt injection attacks
func (c *ScannerClient) ScanContent(ctx context.Context, content []byte, sourceURL, contentType string) (*ScanResult, error) {
	req := ScanRequest{
		Text:        string(content),
		SourceURL:   sourceURL,
		SourceType:  "http_proxy",
		ContentType: contentType,
	}

	return c.scanWithPayment(ctx, "/v1/scan/content", req)
}

// scanWithPayment performs a scan request with automatic x402 payment handling
func (c *ScannerClient) scanWithPayment(ctx context.Context, endpoint string, reqBody interface{}) (*ScanResult, error) {
	// Try the request first (might already have credit or in dev mode)
	result, statusCode, paymentReq, err := c.scan(ctx, endpoint, reqBody, "")
	
	// If successful or error other than 402, return immediately
	if err != nil || statusCode != http.StatusPaymentRequired {
		return result, err
	}

	// Handle 402 Payment Required
	if paymentReq == nil {
		return nil, fmt.Errorf("payment required but no requirements received")
	}

	// Check if we have a wallet
	if c.wallet == nil {
		return nil, fmt.Errorf("payment required but no wallet configured. Run 'stronghold wallet show' to check your balance or visit https://dashboard.stronghold.security to add funds")
	}

	// Create x402 payment
	paymentHeader, err := c.wallet.CreateX402Payment(paymentReq)
	if err != nil {
		return nil, fmt.Errorf("failed to create payment: %w", err)
	}

	// Retry with payment
	result, statusCode, _, err = c.scan(ctx, endpoint, reqBody, paymentHeader)
	if err != nil {
		return nil, err
	}

	if statusCode == http.StatusPaymentRequired {
		return nil, fmt.Errorf("payment was rejected - insufficient funds or invalid payment. Check your balance with 'stronghold wallet balance'")
	}

	return result, nil
}

// scan performs the actual scan request
// Returns: result, statusCode, paymentRequirements (if 402), error
func (c *ScannerClient) scan(ctx context.Context, endpoint string, reqBody interface{}, paymentHeader string) (*ScanResult, int, *wallet.PaymentRequirements, error) {
	url := c.baseURL + endpoint

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, 0, nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, 0, nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	if paymentHeader != "" {
		req.Header.Set("X-Payment", paymentHeader)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, 0, nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	// Handle 402 Payment Required
	if resp.StatusCode == http.StatusPaymentRequired {
		paymentReq, err := c.parsePaymentRequired(resp)
		if err != nil {
			return nil, resp.StatusCode, nil, fmt.Errorf("payment required but failed to parse requirements: %w", err)
		}
		return nil, resp.StatusCode, paymentReq, nil
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, resp.StatusCode, nil, fmt.Errorf("scan failed: %s - %s", resp.Status, string(body))
	}

	var result ScanResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, resp.StatusCode, nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &result, resp.StatusCode, nil, nil
}

// parsePaymentRequired parses a 402 response to extract payment requirements
func (c *ScannerClient) parsePaymentRequired(resp *http.Response) (*wallet.PaymentRequirements, error) {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	var response struct {
		Error               string `json:"error"`
		PaymentRequirements struct {
			Scheme          string `json:"scheme"`
			Network         string `json:"network"`
			Recipient       string `json:"recipient"`
			Amount          string `json:"amount"`
			Currency        string `json:"currency"`
			FacilitatorURL  string `json:"facilitator_url"`
			Description     string `json:"description"`
		} `json:"payment_requirements"`
	}

	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("failed to parse 402 response: %w", err)
	}

	return &wallet.PaymentRequirements{
		Scheme:         response.PaymentRequirements.Scheme,
		Network:        response.PaymentRequirements.Network,
		Recipient:      response.PaymentRequirements.Recipient,
		Amount:         response.PaymentRequirements.Amount,
		Currency:       response.PaymentRequirements.Currency,
		FacilitatorURL: response.PaymentRequirements.FacilitatorURL,
		Description:    response.PaymentRequirements.Description,
	}, nil
}

// ShouldScanContentType determines if a content type should be scanned
func ShouldScanContentType(contentType string) bool {
	// Scan these content types
	scannableTypes := []string{
		"text/html",
		"text/plain",
		"text/markdown",
		"application/json",
		"application/xml",
		"text/xml",
		"application/javascript",
		"text/javascript",
		"text/css",
	}

	for _, t := range scannableTypes {
		if contains(contentType, t) {
			return true
		}
	}

	return false
}

// IsBinaryContentType determines if a content type is binary
func IsBinaryContentType(contentType string) bool {
	binaryTypes := []string{
		"image/",
		"video/",
		"audio/",
		"application/octet-stream",
		"application/pdf",
		"application/zip",
		"application/gzip",
		"application/x-",
	}

	for _, t := range binaryTypes {
		if contains(contentType, t) {
			return true
		}
	}

	return false
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
