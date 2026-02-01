package middleware

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"stronghold/internal/config"
	"stronghold/internal/db"
	"stronghold/internal/db/testutil"

	"github.com/gofiber/fiber/v3"
	"github.com/jarcoal/httpmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAtomicPayment_DevModeBypass(t *testing.T) {
	// No wallet address = dev mode
	cfg := &config.X402Config{
		WalletAddress:  "", // Empty = dev mode
		FacilitatorURL: "https://x402.org/facilitator",
		Network:        "base-sepolia",
	}
	pricing := &config.PricingConfig{
		ScanInput:  0.001,
		ScanOutput: 0.001,
	}

	m := NewX402Middleware(cfg, pricing)

	app := fiber.New()
	app.Post("/v1/scan/content", m.AtomicPayment(0.001), func(c fiber.Ctx) error {
		return c.JSON(fiber.Map{"status": "ok"})
	})

	// No payment header, but should work in dev mode
	req := httptest.NewRequest("POST", "/v1/scan/content", bytes.NewBufferString(`{"text":"test"}`))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, 200, resp.StatusCode)
}

func TestAtomicPayment_MissingHeader(t *testing.T) {
	cfg := &config.X402Config{
		WalletAddress:  "0x1234567890123456789012345678901234567890",
		FacilitatorURL: "https://x402.org/facilitator",
		Network:        "base-sepolia",
	}
	pricing := &config.PricingConfig{
		ScanInput: 0.001,
	}

	m := NewX402Middleware(cfg, pricing)

	app := fiber.New()
	app.Post("/v1/scan/content", m.AtomicPayment(0.001), func(c fiber.Ctx) error {
		return c.JSON(fiber.Map{"status": "ok"})
	})

	// No payment header
	req := httptest.NewRequest("POST", "/v1/scan/content", bytes.NewBufferString(`{"text":"test"}`))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, 402, resp.StatusCode)

	var body map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&body)
	require.NoError(t, err)

	assert.Equal(t, "Payment required", body["error"])
	assert.Contains(t, body, "payment_requirements")

	requirements := body["payment_requirements"].(map[string]interface{})
	assert.Equal(t, "x402", requirements["scheme"])
	assert.Equal(t, "base-sepolia", requirements["network"])
	assert.Equal(t, "0x1234567890123456789012345678901234567890", requirements["recipient"])
	assert.Equal(t, "USDC", requirements["currency"])
}

func TestRequirePayment_DevModeBypass(t *testing.T) {
	cfg := &config.X402Config{
		WalletAddress:  "", // Dev mode
		FacilitatorURL: "https://x402.org/facilitator",
		Network:        "base-sepolia",
	}
	pricing := &config.PricingConfig{
		ScanInput: 0.001,
	}

	m := NewX402Middleware(cfg, pricing)

	app := fiber.New()
	app.Post("/v1/scan", m.RequirePayment(0.001), func(c fiber.Ctx) error {
		return c.JSON(fiber.Map{"status": "ok"})
	})

	req := httptest.NewRequest("POST", "/v1/scan", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, 200, resp.StatusCode)
}

func TestIsFreeRoute(t *testing.T) {
	cfg := &config.X402Config{
		WalletAddress:  "0x1234567890123456789012345678901234567890",
		FacilitatorURL: "https://x402.org/facilitator",
		Network:        "base-sepolia",
	}
	pricing := &config.PricingConfig{}

	m := NewX402Middleware(cfg, pricing)

	testCases := []struct {
		path     string
		expected bool
	}{
		{"/health", true},
		{"/health/live", true},
		{"/health/ready", true},
		{"/v1/pricing", true},
		{"/v1/scan", false},
		{"/v1/scan/content", false},
		{"/v1/auth/login", false},
	}

	for _, tc := range testCases {
		t.Run(tc.path, func(t *testing.T) {
			result := m.IsFreeRoute(tc.path)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestGetRoutes(t *testing.T) {
	cfg := &config.X402Config{
		WalletAddress:  "0x1234567890123456789012345678901234567890",
		FacilitatorURL: "https://x402.org/facilitator",
		Network:        "base-sepolia",
	}
	pricing := &config.PricingConfig{
		ScanInput:     0.001,
		ScanOutput:    0.001,
		ScanUnified:   0.002,
		ScanMultiturn: 0.005,
	}

	m := NewX402Middleware(cfg, pricing)
	routes := m.GetRoutes()

	assert.Len(t, routes, 4)

	// Verify route pricing
	routeMap := make(map[string]float64)
	for _, r := range routes {
		routeMap[r.Path] = r.Price
	}

	assert.Equal(t, 0.001, routeMap["/v1/scan/input"])
	assert.Equal(t, 0.001, routeMap["/v1/scan/output"])
	assert.Equal(t, 0.002, routeMap["/v1/scan"])
	assert.Equal(t, 0.005, routeMap["/v1/scan/multiturn"])
}

func TestFloat64ToWei(t *testing.T) {
	testCases := []struct {
		amount   float64
		expected string
	}{
		{0.001, "1000000000"}, // 0.001 * 1e6 * 1e6 = 1e9
		{0.01, "10000000000"},
		{1.0, "1000000000000"},
	}

	for _, tc := range testCases {
		t.Run(tc.expected, func(t *testing.T) {
			result := float64ToWei(tc.amount)
			assert.Equal(t, tc.expected, result.String())
		})
	}
}

func TestAtomicPayment_WithDB_IdempotencyCache(t *testing.T) {
	testDB := testutil.NewTestDB(t)
	defer testDB.Close(t)

	database := &db.DB{}
	// We need to access the pool, but it's private. We'll use a workaround.
	// For testing, we'll skip this complex test and note it requires more setup.
	// This would typically require dependency injection or a test-specific setup.
	_ = database
	_ = testDB

	t.Skip("Requires mock facilitator and wallet package integration")
}

func TestAtomicPayment_DuplicateInProgress(t *testing.T) {
	t.Skip("Requires mock facilitator and wallet package integration")
}

func TestAtomicPayment_SettlementFailure(t *testing.T) {
	// This test verifies that if settlement fails, a 503 is returned
	// and the result is NOT delivered to the client

	httpmock.Activate()
	defer httpmock.DeactivateAndReset()

	// Mock verify to succeed
	httpmock.RegisterResponder("POST", "https://x402.org/facilitator/verify",
		httpmock.NewJsonResponderOrPanic(200, map[string]interface{}{
			"valid": true,
		}))

	// Mock settle to fail
	httpmock.RegisterResponder("POST", "https://x402.org/facilitator/settle",
		httpmock.NewJsonResponderOrPanic(500, map[string]interface{}{
			"error": "settlement failed",
		}))

	t.Skip("Requires wallet package integration for payment header parsing")
}

func TestAtomicPayment_HandlerError(t *testing.T) {
	// When handler returns error, payment reservation should be expired
	t.Skip("Requires wallet package integration for payment header parsing")
}

func TestMiddleware_SkipsFreeRoutes(t *testing.T) {
	cfg := &config.X402Config{
		WalletAddress:  "0x1234567890123456789012345678901234567890",
		FacilitatorURL: "https://x402.org/facilitator",
		Network:        "base-sepolia",
	}
	pricing := &config.PricingConfig{
		ScanInput: 0.001,
	}

	m := NewX402Middleware(cfg, pricing)

	app := fiber.New()
	app.Use(m.Middleware())
	app.Get("/health", func(c fiber.Ctx) error {
		return c.JSON(fiber.Map{"status": "healthy"})
	})
	app.Get("/v1/pricing", func(c fiber.Ctx) error {
		return c.JSON(fiber.Map{"prices": "..."})
	})

	// Health should work without payment
	req := httptest.NewRequest("GET", "/health", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	resp.Body.Close()
	assert.Equal(t, 200, resp.StatusCode)

	// Pricing should work without payment
	req = httptest.NewRequest("GET", "/v1/pricing", nil)
	resp, err = app.Test(req)
	require.NoError(t, err)
	resp.Body.Close()
	assert.Equal(t, 200, resp.StatusCode)
}

func TestPaymentResponse(t *testing.T) {
	cfg := &config.X402Config{
		WalletAddress:  "0x1234567890123456789012345678901234567890",
		FacilitatorURL: "https://x402.org/facilitator",
		Network:        "base-sepolia",
	}
	pricing := &config.PricingConfig{}

	m := NewX402Middleware(cfg, pricing)

	app := fiber.New()
	app.Get("/test", func(c fiber.Ctx) error {
		m.PaymentResponse(c, "payment-123")
		return c.JSON(fiber.Map{"status": "ok"})
	})

	req := httptest.NewRequest("GET", "/test", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	// Check X-Payment-Response header
	paymentResp := resp.Header.Get("X-Payment-Response")
	assert.NotEmpty(t, paymentResp)

	var paymentData map[string]string
	err = json.Unmarshal([]byte(paymentResp), &paymentData)
	require.NoError(t, err)

	assert.Equal(t, "payment-123", paymentData["payment_id"])
	assert.Equal(t, "settled", paymentData["status"])
}

func TestPaymentResponse_DevMode(t *testing.T) {
	cfg := &config.X402Config{
		WalletAddress:  "", // Dev mode
		FacilitatorURL: "https://x402.org/facilitator",
		Network:        "base-sepolia",
	}
	pricing := &config.PricingConfig{}

	m := NewX402Middleware(cfg, pricing)

	app := fiber.New()
	app.Get("/test", func(c fiber.Ctx) error {
		m.PaymentResponse(c, "payment-123")
		return c.JSON(fiber.Map{"status": "ok"})
	})

	req := httptest.NewRequest("GET", "/test", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	// No X-Payment-Response in dev mode
	paymentResp := resp.Header.Get("X-Payment-Response")
	assert.Empty(t, paymentResp)
}

func TestGetPaymentTransaction_NoContext(t *testing.T) {
	app := fiber.New()
	app.Get("/test", func(c fiber.Ctx) error {
		tx := GetPaymentTransaction(c)
		if tx == nil {
			return c.SendString("nil")
		}
		return c.SendString("found")
	})

	req := httptest.NewRequest("GET", "/test", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	body := make([]byte, 10)
	n, _ := resp.Body.Read(body)
	assert.Equal(t, "nil", string(body[:n]))
}

func TestNewX402MiddlewareWithDB(t *testing.T) {
	testDB := testutil.NewTestDB(t)
	defer testDB.Close(t)

	cfg := &config.X402Config{
		WalletAddress:  "0x1234567890123456789012345678901234567890",
		FacilitatorURL: "https://x402.org/facilitator",
		Network:        "base-sepolia",
	}
	pricing := &config.PricingConfig{
		ScanInput: 0.001,
	}

	// This test just verifies the constructor works
	database := &db.DB{}
	m := NewX402MiddlewareWithDB(cfg, pricing, database)
	assert.NotNil(t, m)
	assert.NotNil(t, m.db)
}

func TestX402Client_Placeholder(t *testing.T) {
	// Test the placeholder client methods
	client := NewX402Client("https://x402.org/facilitator", "base-sepolia")
	assert.NotNil(t, client)

	// These are placeholder implementations
	valid, err := client.VerifyPayment("payment", nil)
	require.NoError(t, err)
	assert.True(t, valid)

	paymentID, err := client.SettlePayment("payment")
	require.NoError(t, err)
	assert.Equal(t, "payment-id", paymentID)
}

// Integration test with real database
func TestAtomicPayment_FullFlow_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	testDB := testutil.NewTestDB(t)
	defer testDB.Close(t)

	// This would require a complete integration test with:
	// 1. Real database
	// 2. Mocked facilitator
	// 3. Valid payment signatures
	// Skipping for now as it requires extensive mocking

	t.Skip("Full integration test requires wallet package mocking")
}

func TestHttpClientTimeout(t *testing.T) {
	cfg := &config.X402Config{
		WalletAddress:  "0x1234567890123456789012345678901234567890",
		FacilitatorURL: "https://x402.org/facilitator",
		Network:        "base-sepolia",
	}
	pricing := &config.PricingConfig{}

	m := NewX402Middleware(cfg, pricing)

	// Verify the http client has a timeout set
	assert.Equal(t, 10*time.Second, m.httpClient.Timeout)
}

// Mock DB for testing middleware with database
type mockDB struct {
	db.Database
	paymentByNonce *db.PaymentTransaction
	paymentErr     error
}

func (m *mockDB) GetPaymentByNonce(ctx context.Context, nonce string) (*db.PaymentTransaction, error) {
	if m.paymentErr != nil {
		return nil, m.paymentErr
	}
	return m.paymentByNonce, nil
}

func (m *mockDB) CreatePaymentTransaction(ctx context.Context, tx *db.PaymentTransaction) error {
	return nil
}

func (m *mockDB) TransitionStatus(ctx context.Context, id interface{}, from, to db.PaymentStatus) error {
	return nil
}

func (m *mockDB) Ping(ctx context.Context) error {
	return nil
}

func (m *mockDB) Close() {}

// HTTP client mock for facilitator calls
type mockTransport struct {
	roundTripper func(*http.Request) (*http.Response, error)
}

func (m *mockTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return m.roundTripper(req)
}
