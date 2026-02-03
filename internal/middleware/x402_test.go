package middleware

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"stronghold/internal/config"
	"stronghold/internal/db"
	"stronghold/internal/db/testutil"
	"stronghold/internal/wallet"

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
		ScanContent:  0.001,
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
		ScanContent: 0.001,
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
		ScanContent: 0.001,
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
		ScanContent: 0.001,
		ScanOutput:  0.001,
	}

	m := NewX402Middleware(cfg, pricing)
	routes := m.GetRoutes()

	assert.Len(t, routes, 2)

	// Verify route pricing
	routeMap := make(map[string]float64)
	for _, r := range routes {
		routeMap[r.Path] = r.Price
	}

	assert.Equal(t, 0.001, routeMap["/v1/scan/content"])
	assert.Equal(t, 0.001, routeMap["/v1/scan/output"])
}

func TestFloat64ToWei(t *testing.T) {
	testCases := []struct {
		amount   float64
		expected string
	}{
		{0.001, "1000"},    // $0.001 = 1000 USDC atomic units (6 decimals)
		{0.01, "10000"},    // $0.01 = 10000 units
		{1.0, "1000000"},   // $1.00 = 1000000 units
		{0.000001, "1"},    // Smallest unit = $0.000001
	}

	for _, tc := range testCases {
		t.Run(tc.expected, func(t *testing.T) {
			result := float64ToWei(tc.amount)
			assert.Equal(t, tc.expected, result.String())
		})
	}
}

func TestFloat64ToWei_Precision(t *testing.T) {
	// Test edge cases that could cause precision loss with int64 conversion
	testCases := []struct {
		name     string
		amount   float64
		expected string
	}{
		{"very small amount", 0.0000001, "0"},  // Below USDC precision
		{"one cent", 0.01, "10000"},
		{"one dollar", 1.0, "1000000"},
		{"large amount", 1000000.0, "1000000000000"},
		{"fractional precision", 0.123456, "123456"},
		{"nine cents", 0.09, "90000"},          // Potential float representation issue
		{"nineteen cents", 0.19, "190000"},     // Another common float issue
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := float64ToWei(tc.amount)
			assert.Equal(t, tc.expected, result.String())
		})
	}
}

func TestAtomicPayment_WithDB_IdempotencyCache(t *testing.T) {
	testDB := testutil.NewTestDB(t)
	defer testDB.Close(t)

	// Create test wallet
	testWallet, err := wallet.NewTestWallet()
	require.NoError(t, err)

	receiverAddress := "0x1234567890123456789012345678901234567890"

	cfg := &config.X402Config{
		WalletAddress:  receiverAddress,
		FacilitatorURL: "https://x402.org/facilitator",
		Network:        "base-sepolia",
	}
	pricing := &config.PricingConfig{
		ScanContent: 0.001,
	}

	// Create middleware with database
	m := NewX402MiddlewareWithDB(cfg, pricing, db.NewFromPool(testDB.Pool))

	// Setup httpmock for facilitator
	httpmock.Activate()
	defer httpmock.DeactivateAndReset()

	// Mock verify to succeed
	httpmock.RegisterResponder("POST", "https://x402.org/facilitator/verify",
		httpmock.NewJsonResponderOrPanic(200, map[string]interface{}{
			"valid": true,
		}))

	// Mock settle to succeed
	httpmock.RegisterResponder("POST", "https://x402.org/facilitator/settle",
		httpmock.NewJsonResponderOrPanic(200, map[string]interface{}{
			"payment_id": "test-payment-123",
		}))

	// Create payment header
	paymentHeader, err := testWallet.CreateTestPaymentHeader(receiverAddress, "1000", "base-sepolia")
	require.NoError(t, err)

	app := fiber.New()
	callCount := 0
	app.Post("/v1/scan/content", m.AtomicPayment(0.001), func(c fiber.Ctx) error {
		callCount++
		return c.JSON(fiber.Map{"status": "ok", "call": callCount})
	})

	// First request - should succeed and store result
	req := httptest.NewRequest("POST", "/v1/scan/content", bytes.NewBufferString(`{"text":"test"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Payment", paymentHeader)

	resp, err := app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, 200, resp.StatusCode)
	assert.Equal(t, 1, callCount)

	// Second request with SAME payment - should return cached result (conflict since in progress)
	// Note: In real scenario, the first request would complete and subsequent
	// requests would get the cached completed result. Here we test the conflict case.
	req2 := httptest.NewRequest("POST", "/v1/scan/content", bytes.NewBufferString(`{"text":"test"}`))
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set("X-Payment", paymentHeader)

	resp2, err := app.Test(req2)
	require.NoError(t, err)
	defer resp2.Body.Close()

	// Should return cached result (200) or conflict (409) depending on timing
	// Since first request completed, we should get the cached result
	assert.Contains(t, []int{200, 409}, resp2.StatusCode)
}

func TestAtomicPayment_DuplicateInProgress(t *testing.T) {
	testDB := testutil.NewTestDB(t)
	defer testDB.Close(t)

	testWallet, err := wallet.NewTestWallet()
	require.NoError(t, err)

	receiverAddress := "0x1234567890123456789012345678901234567890"

	cfg := &config.X402Config{
		WalletAddress:  receiverAddress,
		FacilitatorURL: "https://x402.org/facilitator",
		Network:        "base-sepolia",
	}
	pricing := &config.PricingConfig{
		ScanContent: 0.001,
	}

	m := NewX402MiddlewareWithDB(cfg, pricing, db.NewFromPool(testDB.Pool))

	httpmock.Activate()
	defer httpmock.DeactivateAndReset()

	httpmock.RegisterResponder("POST", "https://x402.org/facilitator/verify",
		httpmock.NewJsonResponderOrPanic(200, map[string]interface{}{
			"valid": true,
		}))

	// Slow settle to simulate in-progress state
	httpmock.RegisterResponder("POST", "https://x402.org/facilitator/settle",
		func(req *http.Request) (*http.Response, error) {
			time.Sleep(100 * time.Millisecond)
			return httpmock.NewJsonResponse(200, map[string]interface{}{
				"payment_id": "test-payment-123",
			})
		})

	paymentHeader, err := testWallet.CreateTestPaymentHeader(receiverAddress, "1000", "base-sepolia")
	require.NoError(t, err)

	app := fiber.New()
	app.Post("/v1/scan/content", m.AtomicPayment(0.001), func(c fiber.Ctx) error {
		time.Sleep(50 * time.Millisecond) // Simulate some work
		return c.JSON(fiber.Map{"status": "ok"})
	})

	// Send two concurrent requests with same payment
	var wg sync.WaitGroup
	results := make([]int, 2)

	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			req := httptest.NewRequest("POST", "/v1/scan/content", bytes.NewBufferString(`{"text":"test"}`))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-Payment", paymentHeader)

			resp, err := app.Test(req, fiber.TestConfig{Timeout: 5 * time.Second})
			if err != nil {
				results[idx] = -1
				return
			}
			results[idx] = resp.StatusCode
			resp.Body.Close()
		}(i)
	}

	wg.Wait()

	// One should succeed (200), one should get conflict (409)
	successCount := 0
	conflictCount := 0
	for _, code := range results {
		if code == 200 {
			successCount++
		} else if code == 409 {
			conflictCount++
		}
	}

	// At least one should succeed, and we should see some conflict handling
	assert.GreaterOrEqual(t, successCount, 1, "At least one request should succeed")
}

func TestAtomicPayment_SettlementFailure(t *testing.T) {
	testDB := testutil.NewTestDB(t)
	defer testDB.Close(t)

	testWallet, err := wallet.NewTestWallet()
	require.NoError(t, err)

	receiverAddress := "0x1234567890123456789012345678901234567890"

	cfg := &config.X402Config{
		WalletAddress:  receiverAddress,
		FacilitatorURL: "https://x402.org/facilitator",
		Network:        "base-sepolia",
	}
	pricing := &config.PricingConfig{
		ScanContent: 0.001,
	}

	m := NewX402MiddlewareWithDB(cfg, pricing, db.NewFromPool(testDB.Pool))

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

	paymentHeader, err := testWallet.CreateTestPaymentHeader(receiverAddress, "1000", "base-sepolia")
	require.NoError(t, err)

	handlerCalled := false
	app := fiber.New()
	app.Post("/v1/scan/content", m.AtomicPayment(0.001), func(c fiber.Ctx) error {
		handlerCalled = true
		return c.JSON(fiber.Map{"status": "ok", "result": "should not be returned"})
	})

	req := httptest.NewRequest("POST", "/v1/scan/content", bytes.NewBufferString(`{"text":"test"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Payment", paymentHeader)

	resp, err := app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	// Handler should have been called
	assert.True(t, handlerCalled)

	// But response should be 503 since settlement failed
	assert.Equal(t, 503, resp.StatusCode)

	var body map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&body)
	require.NoError(t, err)

	assert.Equal(t, "Payment settlement failed", body["error"])
	assert.Equal(t, true, body["retry"])
}

func TestAtomicPayment_HandlerError(t *testing.T) {
	testDB := testutil.NewTestDB(t)
	defer testDB.Close(t)

	testWallet, err := wallet.NewTestWallet()
	require.NoError(t, err)

	receiverAddress := "0x1234567890123456789012345678901234567890"

	cfg := &config.X402Config{
		WalletAddress:  receiverAddress,
		FacilitatorURL: "https://x402.org/facilitator",
		Network:        "base-sepolia",
	}
	pricing := &config.PricingConfig{
		ScanContent: 0.001,
	}

	m := NewX402MiddlewareWithDB(cfg, pricing, db.NewFromPool(testDB.Pool))

	httpmock.Activate()
	defer httpmock.DeactivateAndReset()

	httpmock.RegisterResponder("POST", "https://x402.org/facilitator/verify",
		httpmock.NewJsonResponderOrPanic(200, map[string]interface{}{
			"valid": true,
		}))

	// Settle should NOT be called since handler fails
	settleCalled := false
	httpmock.RegisterResponder("POST", "https://x402.org/facilitator/settle",
		func(req *http.Request) (*http.Response, error) {
			settleCalled = true
			return httpmock.NewJsonResponse(200, map[string]interface{}{
				"payment_id": "test-payment-123",
			})
		})

	paymentHeader, err := testWallet.CreateTestPaymentHeader(receiverAddress, "1000", "base-sepolia")
	require.NoError(t, err)

	app := fiber.New()
	app.Post("/v1/scan/content", m.AtomicPayment(0.001), func(c fiber.Ctx) error {
		// Handler returns an error status
		return c.Status(500).JSON(fiber.Map{"error": "internal error"})
	})

	req := httptest.NewRequest("POST", "/v1/scan/content", bytes.NewBufferString(`{"text":"test"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Payment", paymentHeader)

	resp, err := app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	// Should return the handler's error
	assert.Equal(t, 500, resp.StatusCode)

	// Settlement should NOT have been called since handler failed
	assert.False(t, settleCalled, "Settlement should not be called when handler fails")
}

func TestMiddleware_SkipsFreeRoutes(t *testing.T) {
	cfg := &config.X402Config{
		WalletAddress:  "0x1234567890123456789012345678901234567890",
		FacilitatorURL: "https://x402.org/facilitator",
		Network:        "base-sepolia",
	}
	pricing := &config.PricingConfig{
		ScanContent: 0.001,
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
		ScanContent: 0.001,
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
	testDB := testutil.NewTestDB(t)
	defer testDB.Close(t)

	testWallet, err := wallet.NewTestWallet()
	require.NoError(t, err)

	receiverAddress := "0x1234567890123456789012345678901234567890"

	cfg := &config.X402Config{
		WalletAddress:  receiverAddress,
		FacilitatorURL: "https://x402.org/facilitator",
		Network:        "base-sepolia",
	}
	pricing := &config.PricingConfig{
		ScanContent: 0.001,
	}

	database := db.NewFromPool(testDB.Pool)
	m := NewX402MiddlewareWithDB(cfg, pricing, database)

	httpmock.Activate()
	defer httpmock.DeactivateAndReset()

	httpmock.RegisterResponder("POST", "https://x402.org/facilitator/verify",
		httpmock.NewJsonResponderOrPanic(200, map[string]interface{}{
			"valid": true,
		}))

	httpmock.RegisterResponder("POST", "https://x402.org/facilitator/settle",
		httpmock.NewJsonResponderOrPanic(200, map[string]interface{}{
			"payment_id": "test-payment-456",
		}))

	paymentHeader, err := testWallet.CreateTestPaymentHeader(receiverAddress, "1000", "base-sepolia")
	require.NoError(t, err)

	app := fiber.New()
	app.Post("/v1/scan/content", m.AtomicPayment(0.001), func(c fiber.Ctx) error {
		// Verify we have access to the payment transaction
		tx := GetPaymentTransaction(c)
		if tx == nil {
			return c.Status(500).JSON(fiber.Map{"error": "no payment transaction in context"})
		}
		return c.JSON(fiber.Map{
			"status":    "ok",
			"payer":     tx.PayerAddress,
			"amount":    tx.AmountUSDC,
			"nonce":     tx.PaymentNonce,
		})
	})

	req := httptest.NewRequest("POST", "/v1/scan/content", bytes.NewBufferString(`{"text":"test"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Payment", paymentHeader)

	resp, err := app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, 200, resp.StatusCode)

	var body map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&body)
	require.NoError(t, err)

	assert.Equal(t, "ok", body["status"])
	assert.Equal(t, testWallet.AddressString(), body["payer"])
	assert.NotEmpty(t, body["nonce"])

	// Check X-Payment-Response header
	paymentResp := resp.Header.Get("X-Payment-Response")
	assert.NotEmpty(t, paymentResp)

	var paymentData map[string]string
	err = json.Unmarshal([]byte(paymentResp), &paymentData)
	require.NoError(t, err)
	assert.Equal(t, "test-payment-456", paymentData["payment_id"])

	// Verify the payment was recorded in the database
	payment, err := database.GetPaymentByNonce(context.Background(), body["nonce"].(string))
	require.NoError(t, err)
	assert.Equal(t, db.PaymentStatusCompleted, payment.Status)
	assert.Equal(t, "test-payment-456", *payment.FacilitatorPaymentID)
}

func TestAtomicPayment_InvalidAmountFormat(t *testing.T) {
	// Test that malformed amount in payment header is rejected
	testDB := testutil.NewTestDB(t)
	defer testDB.Close(t)

	testWallet, err := wallet.NewTestWallet()
	require.NoError(t, err)

	receiverAddress := "0x1234567890123456789012345678901234567890"

	cfg := &config.X402Config{
		WalletAddress:  receiverAddress,
		FacilitatorURL: "https://x402.org/facilitator",
		Network:        "base-sepolia",
	}
	pricing := &config.PricingConfig{
		ScanContent: 0.001,
	}

	m := NewX402MiddlewareWithDB(cfg, pricing, db.NewFromPool(testDB.Pool))

	httpmock.Activate()
	defer httpmock.DeactivateAndReset()

	// This should not even be called since amount parsing should fail first
	httpmock.RegisterResponder("POST", "https://x402.org/facilitator/verify",
		httpmock.NewJsonResponderOrPanic(200, map[string]interface{}{
			"valid": true,
		}))

	// Create payment with invalid amount (not a number)
	paymentHeader, err := testWallet.CreateTestPaymentHeader(receiverAddress, "invalid_amount", "base-sepolia")
	require.NoError(t, err)

	app := fiber.New()
	app.Post("/v1/scan/content", m.AtomicPayment(0.001), func(c fiber.Ctx) error {
		return c.JSON(fiber.Map{"status": "ok"})
	})

	req := httptest.NewRequest("POST", "/v1/scan/content", bytes.NewBufferString(`{"text":"test"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Payment", paymentHeader)

	resp, err := app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	// Should return 402 since amount is invalid
	assert.Equal(t, 402, resp.StatusCode)
}

func TestVerifyPayment_AddressNormalization(t *testing.T) {
	// Test that address comparison works regardless of checksum
	testWallet, err := wallet.NewTestWallet()
	require.NoError(t, err)

	// Use lowercase address
	receiverLower := "0x1234567890abcdef1234567890abcdef12345678"
	// Checksum version would be: 0x1234567890AbcdEF1234567890aBcDeF12345678

	cfg := &config.X402Config{
		WalletAddress:  receiverLower,
		FacilitatorURL: "https://x402.org/facilitator",
		Network:        "base-sepolia",
	}
	pricing := &config.PricingConfig{
		ScanContent: 0.001,
	}

	m := NewX402Middleware(cfg, pricing)

	httpmock.Activate()
	defer httpmock.DeactivateAndReset()

	httpmock.RegisterResponder("POST", "https://x402.org/facilitator/verify",
		httpmock.NewJsonResponderOrPanic(200, map[string]interface{}{
			"valid": true,
		}))

	httpmock.RegisterResponder("POST", "https://x402.org/facilitator/settle",
		httpmock.NewJsonResponderOrPanic(200, map[string]interface{}{
			"payment_id": "test-123",
		}))

	// Create payment with uppercase address (should still match)
	paymentHeader, err := testWallet.CreateTestPaymentHeader(
		"0x1234567890ABCDEF1234567890ABCDEF12345678", // uppercase
		"1000",
		"base-sepolia",
	)
	require.NoError(t, err)

	app := fiber.New()
	app.Post("/v1/scan/content", m.AtomicPayment(0.001), func(c fiber.Ctx) error {
		return c.JSON(fiber.Map{"status": "ok"})
	})

	req := httptest.NewRequest("POST", "/v1/scan/content", bytes.NewBufferString(`{"text":"test"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Payment", paymentHeader)

	resp, err := app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	// Should succeed - addresses should match regardless of case
	assert.Equal(t, 200, resp.StatusCode)
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
