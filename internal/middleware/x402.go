package middleware

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"math/big"
	"net/http"
	"strings"
	"time"

	"stronghold/internal/config"
	"stronghold/internal/db"
	"stronghold/internal/wallet"

	"github.com/ethereum/go-ethereum/common"
	"github.com/gofiber/fiber/v3"
)

// X402Middleware creates x402 payment verification middleware
type X402Middleware struct {
	config     *config.X402Config
	pricing    *config.PricingConfig
	httpClient *http.Client
	db         *db.DB
}

// NewX402Middleware creates a new x402 middleware instance
func NewX402Middleware(cfg *config.X402Config, pricing *config.PricingConfig) *X402Middleware {
	return &X402Middleware{
		config:  cfg,
		pricing: pricing,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// NewX402MiddlewareWithDB creates a new x402 middleware instance with database support for atomic payments
func NewX402MiddlewareWithDB(cfg *config.X402Config, pricing *config.PricingConfig, database *db.DB) *X402Middleware {
	return &X402Middleware{
		config:  cfg,
		pricing: pricing,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
		db: database,
	}
}

// createFacilitatorRequest creates an HTTP request with CDP authentication headers
func (m *X402Middleware) createFacilitatorRequest(method, url string, body []byte) (*http.Request, error) {
	req, err := http.NewRequest(method, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")

	// Add CDP API key authentication if configured
	if m.config.CDPAPIKeyID != "" && m.config.CDPAPIKeySecret != "" {
		req.Header.Set("X-Api-Key-Id", m.config.CDPAPIKeyID)
		req.Header.Set("X-Api-Key-Secret", m.config.CDPAPIKeySecret)
	}

	return req, nil
}

// PriceRoute represents a route with its price
type PriceRoute struct {
	Path   string
	Method string
	Price  float64
}

// GetRoutes returns all priced routes
func (m *X402Middleware) GetRoutes() []PriceRoute {
	return []PriceRoute{
		{Path: "/v1/scan/content", Method: "POST", Price: m.pricing.ScanContent},
		{Path: "/v1/scan/output", Method: "POST", Price: m.pricing.ScanOutput},
	}
}

// GetNetwork returns the configured payment network
func (m *X402Middleware) GetNetwork() string {
	return m.config.Network
}

// RequirePayment returns middleware that requires x402 payment
func (m *X402Middleware) RequirePayment(price float64) fiber.Handler {
	return func(c fiber.Ctx) error {
		// Skip if wallet address not configured (allow all in dev mode)
		if m.config.WalletAddress == "" {
			return c.Next()
		}

		// Convert price to wei (6 decimal places for USDC)
		priceWei := decimalToWei(price)

		// Check for payment header
		paymentHeader := c.Get("X-Payment")
		if paymentHeader == "" {
			// Return 402 with payment requirements
			return m.requirePaymentResponse(c, priceWei)
		}

		// Verify payment
		valid, err := m.verifyPayment(paymentHeader, priceWei)
		if err != nil || !valid {
			return m.requirePaymentResponse(c, priceWei)
		}

		// Payment valid, continue
		return c.Next()
	}
}

// AtomicPayment returns middleware that implements the reserve-commit pattern for atomic payments.
// It ensures that either both service execution and payment settlement succeed, or neither does.
// If settlement fails, a 503 is returned and the service result is not delivered.
func (m *X402Middleware) AtomicPayment(price float64) fiber.Handler {
	return func(c fiber.Ctx) error {
		// Skip if wallet address not configured (allow all in dev mode)
		if m.config.WalletAddress == "" {
			return c.Next()
		}

		// Convert price to wei (6 decimal places for USDC)
		priceWei := decimalToWei(price)

		// Check for payment header
		paymentHeader := c.Get("X-Payment")
		if paymentHeader == "" {
			return m.requirePaymentResponse(c, priceWei)
		}

		// Parse payment header to get nonce
		payload, err := wallet.ParseX402Payment(paymentHeader)
		if err != nil {
			return m.requirePaymentResponse(c, priceWei)
		}

		// Verify payment with facilitator first (before database operations)
		valid, err := m.verifyPayment(paymentHeader, priceWei)
		if err != nil || !valid {
			return m.requirePaymentResponse(c, priceWei)
		}

		// Reserve payment in database using atomic upsert to prevent TOCTOU race conditions
		var paymentTx *db.PaymentTransaction
		if m.db != nil {
			newTx := &db.PaymentTransaction{
				PaymentNonce:    payload.Nonce,
				PaymentHeader:   paymentHeader,
				PayerAddress:    payload.Payer,
				ReceiverAddress: m.config.WalletAddress,
				Endpoint:        c.Path(),
				AmountUSDC:      price,
				Network:         payload.Network,
				ExpiresAt:       time.Now().Add(5 * time.Minute),
			}

			// Atomic upsert - either creates new or returns existing transaction
			existing, wasCreated, err := m.db.CreateOrGetPaymentTransaction(c.Context(), newTx)
			if err != nil {
				slog.Error("failed to create/get payment transaction", "error", err)
				return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
					"error": "Payment processing error",
				})
			}

			if !wasCreated {
				// Transaction already exists - handle based on status
				if existing.Status == db.PaymentStatusCompleted && existing.ServiceResult != nil {
					// Return cached result (idempotent replay)
					if existing.FacilitatorPaymentID != nil {
						m.PaymentResponse(c, *existing.FacilitatorPaymentID)
					}
					return c.JSON(existing.ServiceResult)
				}
				// If payment is in another state (reserved, executing, settling, failed),
				// treat as conflict - another request is processing this payment
				if existing.Status != db.PaymentStatusExpired {
					return c.Status(fiber.StatusConflict).JSON(fiber.Map{
						"error":  "Payment already in progress",
						"status": string(existing.Status),
					})
				}
				// Expired payment - allow retry with new transaction
				// This shouldn't happen often as we just tried to create one
				return c.Status(fiber.StatusConflict).JSON(fiber.Map{
					"error": "Payment nonce expired, please generate a new payment",
				})
			}

			paymentTx = existing

			// Transition to executing
			if err := m.db.TransitionStatus(c.Context(), paymentTx.ID, db.PaymentStatusReserved, db.PaymentStatusExecuting); err != nil {
				slog.Error("failed to transition payment to executing", "error", err)
				return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
					"error": "Payment processing error",
				})
			}

			// Store payment transaction in context for handler to access
			c.Locals("payment_tx", paymentTx)
		}

		// Execute the handler
		if err := c.Next(); err != nil {
			// Handler failed - expire the reservation
			if m.db != nil && paymentTx != nil {
				_ = m.db.TransitionStatus(c.Context(), paymentTx.ID, db.PaymentStatusExecuting, db.PaymentStatusExpired)
			}
			return err
		}

		// Check if handler returned an error status
		if c.Response().StatusCode() >= 400 {
			// Service returned an error - expire the reservation
			if m.db != nil && paymentTx != nil {
				_ = m.db.TransitionStatus(c.Context(), paymentTx.ID, db.PaymentStatusExecuting, db.PaymentStatusExpired)
			}
			return nil
		}

		// Transition to settling and attempt settlement
		if m.db != nil && paymentTx != nil {
			if err := m.db.TransitionStatus(c.Context(), paymentTx.ID, db.PaymentStatusExecuting, db.PaymentStatusSettling); err != nil {
				slog.Error("failed to transition payment to settling", "error", err)
			}
		}

		// Settle payment (blocking)
		paymentID, err := m.settlePayment(paymentHeader)
		if err != nil {
			slog.Error("failed to settle payment", "error", err)
			if m.db != nil && paymentTx != nil {
				_ = m.db.FailSettlement(c.Context(), paymentTx.ID, err.Error())
			}
			// Return 503 - payment not settled, service result not returned
			// Clear the response body that was set by the handler
			c.Response().ResetBody()
			return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
				"error":   "Payment settlement failed",
				"retry":   true,
				"message": "Please retry with the same payment. Your payment was not charged.",
			})
		}

		// Mark completed
		if m.db != nil && paymentTx != nil {
			if err := m.db.CompleteSettlement(c.Context(), paymentTx.ID, paymentID); err != nil {
				slog.Error("failed to mark payment as completed", "error", err)
			}
		}

		m.PaymentResponse(c, paymentID)
		return nil
	}
}

// GetPaymentTransaction retrieves the payment transaction from the request context
func GetPaymentTransaction(c fiber.Ctx) *db.PaymentTransaction {
	if tx, ok := c.Locals("payment_tx").(*db.PaymentTransaction); ok {
		return tx
	}
	return nil
}

// requirePaymentResponse returns a 402 Payment Required response
func (m *X402Middleware) requirePaymentResponse(c fiber.Ctx, amount *big.Int) error {
	c.Status(fiber.StatusPaymentRequired)

	response := map[string]interface{}{
		"error": "Payment required",
		"payment_requirements": map[string]interface{}{
			"scheme":          "x402",
			"network":         m.config.Network,
			"recipient":       m.config.WalletAddress,
			"amount":          amount.String(),
			"currency":        "USDC",
			"facilitator_url": m.config.FacilitatorURL,
			"description":     "Citadel security scan",
		},
	}

	return c.JSON(response)
}

// verifyPayment verifies the x402 payment header via the facilitator
func (m *X402Middleware) verifyPayment(paymentHeader string, expectedAmount *big.Int) (bool, error) {
	// Parse payment header
	payload, err := wallet.ParseX402Payment(paymentHeader)
	if err != nil {
		return false, fmt.Errorf("failed to parse payment: %w", err)
	}

	// Verify the amount matches
	amount := new(big.Int)
	if _, ok := amount.SetString(payload.Amount, 10); !ok {
		return false, fmt.Errorf("invalid amount format: %s", payload.Amount)
	}
	if amount.Cmp(expectedAmount) != 0 {
		return false, fmt.Errorf("amount mismatch: expected %s, got %s", expectedAmount.String(), payload.Amount)
	}

	// Verify the recipient is our wallet
	// Use Ethereum address normalization rather than case-insensitive comparison
	// to properly handle checksummed addresses
	expectedAddr := common.HexToAddress(m.config.WalletAddress)
	receivedAddr := common.HexToAddress(payload.Receiver)
	if expectedAddr != receivedAddr {
		return false, fmt.Errorf("recipient mismatch: expected %s, got %s", m.config.WalletAddress, payload.Receiver)
	}

	// Verify payment signature
	if err := wallet.VerifyPaymentSignature(payload, payload.Payer); err != nil {
		return false, fmt.Errorf("invalid signature: %w", err)
	}

	// Build the original payment requirements for facilitator
	originalReq := &wallet.PaymentRequirements{
		Scheme:     "x402",
		Network:    m.config.Network,
		Recipient:  m.config.WalletAddress,
		Amount:     expectedAmount.String(),
		Currency:   "USDC",
	}

	// Call facilitator to verify payment is valid and not already spent
	// Use x402 v2 format with paymentPayload and paymentRequirements
	facilitatorReq := wallet.BuildFacilitatorRequest(payload, originalReq)

	verifyBody, err := json.Marshal(facilitatorReq)
	if err != nil {
		return false, fmt.Errorf("failed to marshal verify request: %w", err)
	}

	facilitatorURL := m.config.FacilitatorURL
	if facilitatorURL == "" {
		facilitatorURL = "https://x402.org/facilitator"
	}

	req, err := m.createFacilitatorRequest("POST", facilitatorURL+"/verify", verifyBody)
	if err != nil {
		return false, fmt.Errorf("failed to create verify request: %w", err)
	}
	resp, err := m.httpClient.Do(req)

	// Retry once on transient errors (connection errors, timeouts, 5xx)
	if err != nil || (resp != nil && resp.StatusCode >= 500) {
		if resp != nil {
			resp.Body.Close()
		}
		if err != nil {
			slog.Warn("facilitator verify failed, retrying once", "error", err)
		} else {
			slog.Warn("facilitator verify returned 5xx, retrying once", "status", resp.StatusCode)
		}
		time.Sleep(500 * time.Millisecond)
		retryReq, retryErr := m.createFacilitatorRequest("POST", facilitatorURL+"/verify", verifyBody)
		if retryErr != nil {
			return false, fmt.Errorf("failed to create verify retry request: %w", retryErr)
		}
		resp, err = m.httpClient.Do(retryReq)
	}

	if err != nil {
		return false, fmt.Errorf("failed to call facilitator: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("facilitator verification failed: %s", resp.Status)
	}

	// x402 v2 response format
	var verifyResult struct {
		IsValid        bool   `json:"isValid"`
		InvalidReason  string `json:"invalidReason,omitempty"`
		InvalidMessage string `json:"invalidMessage,omitempty"`
		Payer          string `json:"payer,omitempty"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&verifyResult); err != nil {
		return false, fmt.Errorf("failed to decode verify response: %w", err)
	}

	if !verifyResult.IsValid {
		reason := verifyResult.InvalidReason
		if verifyResult.InvalidMessage != "" {
			reason = verifyResult.InvalidMessage
		}
		return false, fmt.Errorf("payment invalid: %s", reason)
	}

	return true, nil
}

// settlePayment settles the payment with the facilitator
func (m *X402Middleware) settlePayment(paymentHeader string) (string, error) {
	payload, err := wallet.ParseX402Payment(paymentHeader)
	if err != nil {
		return "", fmt.Errorf("failed to parse payment: %w", err)
	}

	// Build the original payment requirements for facilitator
	originalReq := &wallet.PaymentRequirements{
		Scheme:    "x402",
		Network:   m.config.Network,
		Recipient: m.config.WalletAddress,
		Amount:    payload.Amount,
		Currency:  "USDC",
	}

	// Use x402 v2 format with paymentPayload and paymentRequirements
	facilitatorReq := wallet.BuildFacilitatorRequest(payload, originalReq)

	settleBody, err := json.Marshal(facilitatorReq)
	if err != nil {
		return "", fmt.Errorf("failed to marshal settle request: %w", err)
	}

	facilitatorURL := m.config.FacilitatorURL
	if facilitatorURL == "" {
		facilitatorURL = "https://x402.org/facilitator"
	}

	req, err := m.createFacilitatorRequest("POST", facilitatorURL+"/settle", settleBody)
	if err != nil {
		return "", fmt.Errorf("failed to create settle request: %w", err)
	}
	resp, err := m.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to call facilitator: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("facilitator settlement failed: %s", resp.Status)
	}

	// x402 v2 settle response format
	var settleResult struct {
		Success   bool   `json:"success"`
		TxHash    string `json:"txHash,omitempty"`
		PaymentID string `json:"paymentId,omitempty"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&settleResult); err != nil {
		return "", fmt.Errorf("failed to decode settle response: %w", err)
	}

	// Check if the facilitator reported failure despite 200 status
	if !settleResult.Success {
		return "", fmt.Errorf("facilitator returned success=false")
	}

	// Return txHash or paymentId as the payment identifier
	if settleResult.TxHash != "" {
		return settleResult.TxHash, nil
	}
	return settleResult.PaymentID, nil
}

// PaymentResponse adds payment response header after successful processing
func (m *X402Middleware) PaymentResponse(c fiber.Ctx, paymentID string) {
	if m.config.WalletAddress == "" {
		return
	}

	response := map[string]string{
		"payment_id": paymentID,
		"status":     "settled",
	}

	responseJSON, _ := json.Marshal(response)
	c.Set("X-Payment-Response", string(responseJSON))
}

// decimalToWei converts a dollar amount to USDC atomic units (6 decimals)
// For example: $0.001 -> 1000 units, $1.00 -> 1000000 units
func decimalToWei(amount float64) *big.Int {
	// USDC has 6 decimals, so multiply by 10^6
	units := int64(amount * 1e6)
	return big.NewInt(units)
}

// IsFreeRoute checks if a route doesn't require payment
func (m *X402Middleware) IsFreeRoute(path string) bool {
	freeRoutes := []string{
		"/health",
		"/v1/pricing",
	}

	for _, route := range freeRoutes {
		if strings.HasPrefix(path, route) {
			return true
		}
	}
	return false
}

// Middleware returns the main x402 middleware that handles all routes
func (m *X402Middleware) Middleware() fiber.Handler {
	return func(c fiber.Ctx) error {
		path := c.Path()

		// Skip free routes
		if m.IsFreeRoute(path) {
			return c.Next()
		}

		// Skip if wallet not configured
		if m.config.WalletAddress == "" {
			return c.Next()
		}

		// Get price for this route
		price := m.getPriceForRoute(path, c.Method())
		if price == 0 {
			// No price set, allow through
			return c.Next()
		}

		// Check payment
		paymentHeader := c.Get("X-Payment")
		if paymentHeader == "" {
			return m.requirePaymentResponse(c, decimalToWei(price))
		}

		valid, err := m.verifyPayment(paymentHeader, decimalToWei(price))
		if err != nil || !valid {
			return m.requirePaymentResponse(c, decimalToWei(price))
		}

		// Store payment header for settlement after successful handler
		c.Locals("x402_payment", paymentHeader)

		return c.Next()
	}
}

// getPriceForRoute returns the price for a given route
func (m *X402Middleware) getPriceForRoute(path, method string) float64 {
	routes := m.GetRoutes()
	for _, route := range routes {
		if strings.HasPrefix(path, route.Path) && method == route.Method {
			return route.Price
		}
	}
	return 0
}

