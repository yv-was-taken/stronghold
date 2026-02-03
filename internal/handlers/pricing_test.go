package handlers

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"stronghold/internal/config"
	"stronghold/internal/middleware"

	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetPricing_ReturnsAllRoutes(t *testing.T) {
	x402cfg := &config.X402Config{
		WalletAddress:  "0x1234567890123456789012345678901234567890",
		FacilitatorURL: "https://x402.org/facilitator",
		Network:        "base-sepolia",
	}
	pricingCfg := &config.PricingConfig{
		ScanContent:  0.001,
		ScanOutput: 0.001,
	}

	x402 := middleware.NewX402Middleware(x402cfg, pricingCfg)
	handler := NewPricingHandler(x402)

	app := fiber.New()
	handler.RegisterRoutes(app)

	req := httptest.NewRequest("GET", "/v1/pricing", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, 200, resp.StatusCode)

	var body PricingResponse
	err = json.NewDecoder(resp.Body).Decode(&body)
	require.NoError(t, err)

	// Verify response structure
	assert.Equal(t, "USDC", body.Currency)
	assert.Equal(t, "base-sepolia", body.Network)
	assert.NotEmpty(t, body.Routes)

	// Verify routes contain expected endpoints
	routePaths := make(map[string]bool)
	for _, route := range body.Routes {
		routePaths[route.Path] = true
		assert.NotEmpty(t, route.Method)
		assert.GreaterOrEqual(t, route.Price, 0.0)
	}

	// These endpoints should be in the pricing
	expectedPaths := []string{
		"/v1/scan/content",
		"/v1/scan/output",
	}

	for _, path := range expectedPaths {
		assert.True(t, routePaths[path], "Expected route %s to be in pricing", path)
	}
}

func TestGetPricing_HasDescriptions(t *testing.T) {
	x402cfg := &config.X402Config{
		WalletAddress:  "0x1234567890123456789012345678901234567890",
		FacilitatorURL: "https://x402.org/facilitator",
		Network:        "base-sepolia",
	}
	pricingCfg := &config.PricingConfig{
		ScanContent:  0.001,
		ScanOutput: 0.001,
	}

	x402 := middleware.NewX402Middleware(x402cfg, pricingCfg)
	handler := NewPricingHandler(x402)

	app := fiber.New()
	handler.RegisterRoutes(app)

	req := httptest.NewRequest("GET", "/v1/pricing", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	var body PricingResponse
	err = json.NewDecoder(resp.Body).Decode(&body)
	require.NoError(t, err)

	// Verify descriptions are present for scan endpoints
	descriptionsByPath := make(map[string]string)
	for _, route := range body.Routes {
		descriptionsByPath[route.Path] = route.Description
	}

	assert.Contains(t, descriptionsByPath["/v1/scan/content"], "prompt injection")
	assert.Contains(t, descriptionsByPath["/v1/scan/output"], "credential leak")
}

func TestGetPricing_CorrectPrices(t *testing.T) {
	x402cfg := &config.X402Config{
		WalletAddress:  "0x1234567890123456789012345678901234567890",
		FacilitatorURL: "https://x402.org/facilitator",
		Network:        "base-sepolia",
	}
	pricingCfg := &config.PricingConfig{
		ScanContent:  0.001,
		ScanOutput: 0.001,
	}

	x402 := middleware.NewX402Middleware(x402cfg, pricingCfg)
	handler := NewPricingHandler(x402)

	app := fiber.New()
	handler.RegisterRoutes(app)

	req := httptest.NewRequest("GET", "/v1/pricing", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	var body PricingResponse
	err = json.NewDecoder(resp.Body).Decode(&body)
	require.NoError(t, err)

	// Map prices by path
	pricesByPath := make(map[string]float64)
	for _, route := range body.Routes {
		pricesByPath[route.Path] = route.Price
	}

	// Verify expected prices - all endpoints are $0.001
	assert.Equal(t, 0.001, pricesByPath["/v1/scan/content"])
	assert.Equal(t, 0.001, pricesByPath["/v1/scan/output"])
}

func TestGetPricing_JSONContentType(t *testing.T) {
	x402cfg := &config.X402Config{
		WalletAddress:  "0x1234567890123456789012345678901234567890",
		FacilitatorURL: "https://x402.org/facilitator",
		Network:        "base-sepolia",
	}
	pricingCfg := &config.PricingConfig{
		ScanContent:  0.001,
		ScanOutput: 0.001,
	}

	x402 := middleware.NewX402Middleware(x402cfg, pricingCfg)
	handler := NewPricingHandler(x402)

	app := fiber.New()
	handler.RegisterRoutes(app)

	req := httptest.NewRequest("GET", "/v1/pricing", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Contains(t, resp.Header.Get("Content-Type"), "application/json")
}

func TestGetPricing_AllRoutesHaveMethod(t *testing.T) {
	x402cfg := &config.X402Config{
		WalletAddress:  "0x1234567890123456789012345678901234567890",
		FacilitatorURL: "https://x402.org/facilitator",
		Network:        "base-sepolia",
	}
	pricingCfg := &config.PricingConfig{
		ScanContent:  0.001,
		ScanOutput: 0.001,
	}

	x402 := middleware.NewX402Middleware(x402cfg, pricingCfg)
	handler := NewPricingHandler(x402)

	app := fiber.New()
	handler.RegisterRoutes(app)

	req := httptest.NewRequest("GET", "/v1/pricing", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	var body PricingResponse
	err = json.NewDecoder(resp.Body).Decode(&body)
	require.NoError(t, err)

	for _, route := range body.Routes {
		assert.NotEmpty(t, route.Method, "Route %s should have a method", route.Path)
		assert.Equal(t, "POST", route.Method, "Scan routes should be POST")
	}
}
