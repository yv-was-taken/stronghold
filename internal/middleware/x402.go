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
	"stronghold/internal/usdc"
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

// NewX402Middleware creates a new x402 middleware instance without database support.
// Use NewX402MiddlewareWithDB for routes that require AtomicPayment.
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

// createFacilitatorRequest creates an HTTP request to the facilitator
func (m *X402Middleware) createFacilitatorRequest(method, url string, body []byte) (*http.Request, error) {
	req, err := http.NewRequest(method, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")

	return req, nil
}

// PriceRoute represents a route with its price
type PriceRoute struct {
	Path   string
	Method string
	Price  usdc.MicroUSDC
}

// GetRoutes returns all priced routes
func (m *X402Middleware) GetRoutes() []PriceRoute {
	return []PriceRoute{
		{Path: "/v1/scan/content", Method: "POST", Price: m.pricing.ScanContent},
		{Path: "/v1/scan/output", Method: "POST", Price: m.pricing.ScanOutput},
	}
}

// GetNetwork returns the first configured payment network (backward compat for pricing API)
func (m *X402Middleware) GetNetwork() string {
	if len(m.config.Networks) > 0 {
		return m.config.Networks[0]
	}
	return ""
}

// GetNetworks returns all configured payment networks
func (m *X402Middleware) GetNetworks() []string {
	return m.config.Networks
}

// AtomicPayment returns middleware that implements the reserve-commit pattern for atomic payments.
// It ensures that either both service execution and payment settlement succeed, or neither does.
// If settlement fails, a 503 is returned and the service result is not delivered.
func (m *X402Middleware) AtomicPayment(price usdc.MicroUSDC) fiber.Handler {
	return func(c fiber.Ctx) error {
		// Skip if no payment networks configured (allow all in dev mode)
		if !m.config.HasPayments() {
			return c.Next()
		}
		// Atomic payments require a database-backed nonce reservation.
		if m.db == nil {
			slog.Error("atomic payment middleware misconfigured: missing database", "path", c.Path())
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "Payment middleware misconfigured",
			})
		}

		// Check for payment header
		paymentHeader := c.Get("X-Payment")
		if paymentHeader == "" {
			return m.requirePaymentResponse(c, price)
		}

		// Parse payment header to get nonce
		payload, err := wallet.ParseX402Payment(paymentHeader)
		if err != nil {
			return m.requirePaymentResponse(c, price)
		}

		// Verify payment with facilitator first (before database operations)
		valid, err := m.verifyPayment(paymentHeader, price)
		if err != nil || !valid {
			return m.requirePaymentResponse(c, price)
		}

		// Reserve payment in database using atomic upsert to prevent TOCTOU race conditions
		newTx := &db.PaymentTransaction{
			PaymentNonce:    payload.Nonce,
			PaymentHeader:   paymentHeader,
			PayerAddress:    payload.Payer,
			ReceiverAddress: m.config.WalletForNetwork(payload.Network),
			Endpoint:        c.Path(),
			AmountUSDC:      price,
			Network:         payload.Network,
			ExpiresAt:       time.Now().Add(5 * time.Minute),
		}

		// Atomic upsert - either creates new or returns existing transaction
		paymentTx, wasCreated, err := m.db.CreateOrGetPaymentTransaction(c.Context(), newTx)
		if err != nil {
			slog.Error("failed to create/get payment transaction", "error", err)
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "Payment processing error",
			})
		}

		if !wasCreated {
			// Transaction already exists - handle based on status
			if paymentTx.Status == db.PaymentStatusCompleted {
				// Return cached result (idempotent replay)
				if paymentTx.FacilitatorPaymentID != nil {
					m.PaymentResponse(c, *paymentTx.FacilitatorPaymentID)
				}
				if paymentTx.ServiceResult != nil {
					return c.JSON(paymentTx.ServiceResult)
				}
				// Settlement completed but service result was not stored;
				// return a minimal success response so the caller is not charged twice.
				return c.JSON(fiber.Map{"status": "already_settled"})
			}
			// If payment is in another state (reserved, executing, settling, failed),
			// treat as conflict - another request is processing this payment
			if paymentTx.Status != db.PaymentStatusExpired {
				slog.Warn("duplicate payment nonce with in-progress status",
					"nonce", paymentTx.PaymentNonce, "status", string(paymentTx.Status))
				return c.Status(fiber.StatusConflict).JSON(fiber.Map{
					"error": "Payment already in progress",
				})
			}
			// Expired payment - allow retry with new transaction
			// This shouldn't happen often as we just tried to create one
			return c.Status(fiber.StatusConflict).JSON(fiber.Map{
				"error": "Payment nonce expired, please generate a new payment",
			})
		}

		// Transition to executing
		if err := m.db.TransitionStatus(c.Context(), paymentTx.ID, db.PaymentStatusReserved, db.PaymentStatusExecuting); err != nil {
			slog.Error("failed to transition payment to executing", "error", err)
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "Payment processing error",
			})
		}

		// Store payment transaction in context for handler to access
		c.Locals("payment_tx", paymentTx)

		// Execute the handler
		if err := c.Next(); err != nil {
			// Handler failed - expire the reservation
			if expireErr := m.db.TransitionStatus(c.Context(), paymentTx.ID, db.PaymentStatusExecuting, db.PaymentStatusExpired); expireErr != nil {
				slog.Warn("failed to expire payment after handler error", "payment_id", paymentTx.ID, "error", expireErr)
			}
			return err
		}

		// Check if handler returned an error status
		if c.Response().StatusCode() >= 400 {
			// Service returned an error - expire the reservation
			if expireErr := m.db.TransitionStatus(c.Context(), paymentTx.ID, db.PaymentStatusExecuting, db.PaymentStatusExpired); expireErr != nil {
				slog.Warn("failed to expire payment after error response", "payment_id", paymentTx.ID, "error", expireErr)
			}
			return nil
		}

		// Transition to settling and attempt settlement
		if err := m.db.TransitionStatus(c.Context(), paymentTx.ID, db.PaymentStatusExecuting, db.PaymentStatusSettling); err != nil {
			slog.Error("failed to transition payment to settling", "error", err)
		}

		// Settle payment (blocking)
		paymentID, err := m.settlePayment(paymentHeader)
		if err != nil {
			slog.Error("failed to settle payment", "error", err)
			if failErr := m.db.FailSettlement(c.Context(), paymentTx.ID, err.Error()); failErr != nil {
				slog.Warn("failed to record settlement failure", "payment_id", paymentTx.ID, "error", failErr)
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
		if err := m.db.CompleteSettlement(c.Context(), paymentTx.ID, paymentID); err != nil {
			slog.Error("failed to mark payment as completed", "error", err)
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

func (m *X402Middleware) priceToAtomicUnits(price usdc.MicroUSDC, network string) *big.Int {
	if network == "" {
		network = m.GetNetwork()
	}
	if network == "" {
		network = "base"
	}
	return price.ToBigInt(network)
}

// requirePaymentResponse returns a 402 Payment Required response
func (m *X402Middleware) requirePaymentResponse(c fiber.Ctx, price usdc.MicroUSDC) error {
	c.Status(fiber.StatusPaymentRequired)

	accepts := []map[string]interface{}{}

	// Build accepts array from configured networks
	for _, network := range m.config.Networks {
		recipient := m.config.WalletForNetwork(network)
		if recipient == "" {
			continue // skip networks without a configured wallet
		}
		amount := m.priceToAtomicUnits(price, network)
		option := map[string]interface{}{
			"scheme":          "x402",
			"network":         network,
			"recipient":       recipient,
			"amount":          amount.String(),
			"currency":        "USDC",
			"facilitator_url": m.config.FacilitatorURL,
			"description":     "Citadel security scan",
		}
		if wallet.IsSolanaNetwork(network) && m.config.SolanaFeePayer != "" {
			option["fee_payer"] = m.config.SolanaFeePayer
		}
		accepts = append(accepts, option)
	}

	if len(accepts) == 0 {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "No payment networks configured",
		})
	}

	response := map[string]interface{}{
		"error":                "Payment required",
		"payment_requirements": accepts[0], // backward compat: primary option
		"accepts":              accepts,    // multi-chain: all options
	}

	return c.JSON(response)
}

// verifyPayment verifies the x402 payment header via the facilitator.
func (m *X402Middleware) verifyPayment(paymentHeader string, price usdc.MicroUSDC) (bool, error) {
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
	expectedAmount := m.priceToAtomicUnits(price, payload.Network)
	if amount.Cmp(expectedAmount) != 0 {
		return false, fmt.Errorf("amount mismatch: expected %s, got %s", expectedAmount.String(), payload.Amount)
	}

	// Verify the payment network is one we support
	networkConfigured := false
	for _, n := range m.config.Networks {
		if n == payload.Network {
			networkConfigured = true
			break
		}
	}
	if !networkConfigured {
		return false, fmt.Errorf("unsupported payment network: %s (configured: %v)", payload.Network, m.config.Networks)
	}

	// Look up the wallet address for this network
	expectedWallet := m.config.WalletForNetwork(payload.Network)
	if expectedWallet == "" {
		return false, fmt.Errorf("no wallet configured for network: %s", payload.Network)
	}

	// Verify the recipient and signature based on network type
	if wallet.IsSolanaNetwork(payload.Network) {
		// Solana: simple string comparison for base58 addresses
		if payload.Receiver != expectedWallet {
			return false, fmt.Errorf("recipient mismatch: expected %s, got %s", expectedWallet, payload.Receiver)
		}
		// Solana signature verification is handled by the facilitator
	} else {
		// EVM: use Ethereum address normalization for checksummed addresses
		expectedAddr := common.HexToAddress(expectedWallet)
		receivedAddr := common.HexToAddress(payload.Receiver)
		if expectedAddr != receivedAddr {
			return false, fmt.Errorf("recipient mismatch: expected %s, got %s", expectedWallet, payload.Receiver)
		}

		// Verify EIP-3009 signature locally
		if err := wallet.VerifyPaymentSignature(payload, payload.Payer); err != nil {
			return false, fmt.Errorf("invalid signature: %w", err)
		}
	}

	// Build the original payment requirements for facilitator
	originalReq := &wallet.PaymentRequirements{
		Scheme:    "x402",
		Network:   payload.Network,
		Recipient: expectedWallet,
		Amount:    expectedAmount.String(),
		Currency:  "USDC",
	}

	// Call facilitator to verify payment is valid and not already spent
	// Use x402 v2 format with paymentPayload and paymentRequirements
	facilitatorReq := wallet.BuildFacilitatorRequest(payload, originalReq)

	verifyBody, err := json.Marshal(facilitatorReq)
	if err != nil {
		return false, fmt.Errorf("failed to marshal verify request: %w", err)
	}

	facilitatorURL := m.config.FacilitatorURL

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

	// x402-rs VerifyResponseWire uses rename_all = "camelCase"
	var verifyResult struct {
		IsValid       bool   `json:"isValid"`
		InvalidReason string `json:"invalidReason,omitempty"`
		Payer         string `json:"payer,omitempty"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&verifyResult); err != nil {
		return false, fmt.Errorf("failed to decode verify response: %w", err)
	}

	if !verifyResult.IsValid {
		return false, fmt.Errorf("payment invalid: %s", verifyResult.InvalidReason)
	}

	return true, nil
}

// settlePayment settles the payment with the facilitator
func (m *X402Middleware) settlePayment(paymentHeader string) (string, error) {
	payload, err := wallet.ParseX402Payment(paymentHeader)
	if err != nil {
		return "", fmt.Errorf("failed to parse payment: %w", err)
	}

	// Look up the wallet address for this payment's network
	recipientAddr := m.config.WalletForNetwork(payload.Network)
	if recipientAddr == "" {
		return "", fmt.Errorf("no wallet configured for network: %s", payload.Network)
	}

	// Build the original payment requirements for facilitator
	originalReq := &wallet.PaymentRequirements{
		Scheme:    "x402",
		Network:   payload.Network,
		Recipient: recipientAddr,
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

	req, err := m.createFacilitatorRequest("POST", facilitatorURL+"/settle", settleBody)
	if err != nil {
		return "", fmt.Errorf("failed to create settle request: %w", err)
	}
	resp, err := m.httpClient.Do(req)

	// Retry once on transient errors (connection errors, timeouts, 5xx)
	if err != nil || (resp != nil && resp.StatusCode >= 500) {
		if resp != nil {
			resp.Body.Close()
		}
		if err != nil {
			slog.Warn("facilitator settle failed, retrying once", "error", err)
		} else {
			slog.Warn("facilitator settle returned 5xx, retrying once", "status", resp.StatusCode)
		}
		time.Sleep(500 * time.Millisecond)
		retryReq, retryErr := m.createFacilitatorRequest("POST", facilitatorURL+"/settle", settleBody)
		if retryErr != nil {
			return "", fmt.Errorf("failed to create settle retry request: %w", retryErr)
		}
		resp, err = m.httpClient.Do(retryReq)
	}

	if err != nil {
		return "", fmt.Errorf("failed to call facilitator: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("facilitator settlement failed: %s", resp.Status)
	}

	// x402-rs SettleResponseWire does NOT use rename_all, so fields are snake_case
	var settleResult struct {
		Success     bool   `json:"success"`
		Transaction string `json:"transaction,omitempty"`
		Network     string `json:"network,omitempty"`
		Payer       string `json:"payer,omitempty"`
		ErrorReason string `json:"error_reason,omitempty"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&settleResult); err != nil {
		return "", fmt.Errorf("failed to decode settle response: %w", err)
	}

	if !settleResult.Success {
		reason := settleResult.ErrorReason
		if reason == "" {
			reason = "unknown"
		}
		return "", fmt.Errorf("facilitator returned success=false: %s", reason)
	}

	return settleResult.Transaction, nil
}

// PaymentResponse adds payment response header after successful processing
func (m *X402Middleware) PaymentResponse(c fiber.Ctx, paymentID string) {
	if !m.config.HasPayments() {
		return
	}

	response := map[string]string{
		"payment_id": paymentID,
		"status":     "settled",
	}

	responseJSON, err := json.Marshal(response)
	if err != nil {
		slog.Error("failed to marshal payment response", "error", err)
		return
	}
	c.Set("X-Payment-Response", string(responseJSON))
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

