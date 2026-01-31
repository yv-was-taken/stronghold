package middleware

import (
	"encoding/json"
	"fmt"
	"math/big"
	"strings"

	"citadel-api/internal/config"

	"github.com/gofiber/fiber/v3"
)

// X402Middleware creates x402 payment verification middleware
type X402Middleware struct {
	config  *config.X402Config
	pricing *config.PricingConfig
}

// NewX402Middleware creates a new x402 middleware instance
func NewX402Middleware(cfg *config.X402Config, pricing *config.PricingConfig) *X402Middleware {
	return &X402Middleware{
		config:  cfg,
		pricing: pricing,
	}
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
		{Path: "/v1/scan/input", Method: "POST", Price: m.pricing.ScanInput},
		{Path: "/v1/scan/output", Method: "POST", Price: m.pricing.ScanOutput},
		{Path: "/v1/scan", Method: "POST", Price: m.pricing.ScanUnified},
		{Path: "/v1/scan/multiturn", Method: "POST", Price: m.pricing.ScanMultiturn},
	}
}

// RequirePayment returns middleware that requires x402 payment
func (m *X402Middleware) RequirePayment(price float64) fiber.Handler {
	return func(c fiber.Ctx) error {
		// Skip if wallet address not configured (allow all in dev mode)
		if m.config.WalletAddress == "" {
			return c.Next()
		}

		// Convert price to wei (assuming 6 decimal places for USDC)
		priceWei := float64ToWei(price)

		// Check for payment header
		paymentHeader := c.Get("X-PAYMENT")
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

// requirePaymentResponse returns a 402 Payment Required response
func (m *X402Middleware) requirePaymentResponse(c fiber.Ctx, amount *big.Int) error {
	c.Status(fiber.StatusPaymentRequired)

	response := map[string]interface{}{
		"error": "Payment required",
		"payment_requirements": map[string]interface{}{
			"scheme":           "x402",
			"network":          m.config.Network,
			"recipient":        m.config.WalletAddress,
			"amount":           amount.String(),
			"currency":         "USDC",
			"facilitator_url":  m.config.FacilitatorURL,
			"description":      "Citadel security scan",
		},
	}

	return c.JSON(response)
}

// verifyPayment verifies the x402 payment header
func (m *X402Middleware) verifyPayment(paymentHeader string, expectedAmount *big.Int) (bool, error) {
	// TODO: Integrate with actual x402-go verification
	// For now, do basic validation

	// Parse payment header (format: "x402 scheme;payload")
	parts := strings.SplitN(paymentHeader, ";", 2)
	if len(parts) != 2 {
		return false, fmt.Errorf("invalid payment header format")
	}

	scheme := strings.TrimSpace(parts[0])
	if scheme != "x402" {
		return false, fmt.Errorf("unsupported payment scheme: %s", scheme)
	}

	// TODO: Call x402 facilitator to verify payment
	// This would:
	// 1. Parse the payment payload
	// 2. Call facilitator to verify
	// 3. Check amount matches expected
	// 4. Verify payment is not already spent

	// For now, accept any well-formed header in dev mode
	return true, nil
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
	c.Set("X-PAYMENT-RESPONSE", string(responseJSON))
}

// float64ToWei converts a dollar amount to wei (6 decimals for USDC)
func float64ToWei(amount float64) *big.Int {
	// USDC has 6 decimals
	multiplier := big.NewInt(1_000_000)
	amountInt := big.NewInt(int64(amount * 1_000_000))
	return new(big.Int).Mul(amountInt, multiplier)
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
		paymentHeader := c.Get("X-PAYMENT")
		if paymentHeader == "" {
			return m.requirePaymentResponse(c, float64ToWei(price))
		}

		valid, err := m.verifyPayment(paymentHeader, float64ToWei(price))
		if err != nil || !valid {
			return m.requirePaymentResponse(c, float64ToWei(price))
		}

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

// X402Client is a client for interacting with x402 payments
type X402Client struct {
	FacilitatorURL string
	Network        string
}

// NewX402Client creates a new x402 client
func NewX402Client(facilitatorURL, network string) *X402Client {
	return &X402Client{
		FacilitatorURL: facilitatorURL,
		Network:        network,
	}
}

// VerifyPayment verifies a payment with the facilitator
func (c *X402Client) VerifyPayment(payment string, amount *big.Int) (bool, error) {
	// TODO: Implement actual facilitator call
	// This would make an HTTP request to the facilitator
	return true, nil
}

// SettlePayment settles a payment with the facilitator
func (c *X402Client) SettlePayment(payment string) (string, error) {
	// TODO: Implement actual settlement
	// Returns payment ID
	return "payment-id", nil
}
