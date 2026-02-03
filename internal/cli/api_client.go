// Package cli provides the API client for CLI operations
package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// APIClient handles communication with the Stronghold API
type APIClient struct {
	baseURL    string
	httpClient *http.Client
}

// NewAPIClient creates a new API client
func NewAPIClient(baseURL string) *APIClient {
	return &APIClient{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// CreateAccountRequest represents a request to create an account
type CreateAccountRequest struct {
	WalletAddress *string `json:"wallet_address,omitempty"`
}

// CreateAccountResponse represents the response from creating an account
type CreateAccountResponse struct {
	AccountNumber string `json:"account_number"`
	ExpiresAt     string `json:"expires_at"`
	RecoveryFile  string `json:"recovery_file"`
}

// LoginRequest represents a login request
type LoginRequest struct {
	AccountNumber string `json:"account_number"`
}

// LoginResponse represents the login response
type LoginResponse struct {
	AccountNumber string  `json:"account_number"`
	ExpiresAt     string  `json:"expires_at"`
	WalletAddress *string `json:"wallet_address,omitempty"`
	PrivateKey    *string `json:"private_key,omitempty"`
}

// ErrorResponse represents an API error response
type ErrorResponse struct {
	Error string `json:"error"`
}

// doRequest performs an HTTP request with JSON marshaling/unmarshaling
func (c *APIClient) doRequest(method, endpoint string, expectedStatus int, reqBody interface{}, respBody interface{}) error {
	var body io.Reader
	if reqBody != nil {
		data, err := json.Marshal(reqBody)
		if err != nil {
			return fmt.Errorf("failed to marshal request: %w", err)
		}
		body = bytes.NewReader(data)
	}

	req, err := http.NewRequest(method, c.baseURL+endpoint, body)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respData, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != expectedStatus {
		var errResp ErrorResponse
		if json.Unmarshal(respData, &errResp) == nil && errResp.Error != "" {
			return fmt.Errorf("API error (%d %s %s): %s",
				resp.StatusCode, method, endpoint, errResp.Error)
		}
		// Include truncated response body for debugging
		bodyPreview := string(respData)
		if len(bodyPreview) > 200 {
			bodyPreview = bodyPreview[:200] + "..."
		}
		return fmt.Errorf("unexpected status %d from %s %s: %s",
			resp.StatusCode, method, endpoint, bodyPreview)
	}

	if respBody != nil {
		if err := json.Unmarshal(respData, respBody); err != nil {
			return fmt.Errorf("failed to parse response: %w", err)
		}
	}
	return nil
}

// CreateAccount creates a new account via the API
func (c *APIClient) CreateAccount(req *CreateAccountRequest) (*CreateAccountResponse, error) {
	var result CreateAccountResponse
	if err := c.doRequest(http.MethodPost, "/v1/auth/account", http.StatusCreated, req, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// Login authenticates with the API and returns account info including decrypted wallet key
func (c *APIClient) Login(accountNumber string) (*LoginResponse, error) {
	req := LoginRequest{AccountNumber: accountNumber}
	var result LoginResponse
	if err := c.doRequest(http.MethodPost, "/v1/auth/login", http.StatusOK, req, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// UpdateWalletRequest represents a request to update wallet
type UpdateWalletRequest struct {
	PrivateKey string `json:"private_key"`
}

// UpdateWallet updates the wallet for the current account
func (c *APIClient) UpdateWallet(privateKey string) error {
	req := UpdateWalletRequest{PrivateKey: privateKey}
	return c.doRequest(http.MethodPut, "/v1/auth/wallet", http.StatusOK, req, nil)
}
