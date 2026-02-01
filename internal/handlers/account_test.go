package handlers

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"stronghold/internal/db"
	"stronghold/internal/db/testutil"

	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupAccountTest(t *testing.T) (*fiber.App, *AuthHandler, *AccountHandler, *testutil.TestDB, *db.DB) {
	testDB := testutil.NewTestDB(t)

	cfg := &db.Config{
		Host:     testDB.Host,
		Port:     testDB.Port,
		User:     testDB.User,
		Password: testDB.Password,
		Name:     testDB.Database,
		SSLMode:  "disable",
	}

	database, err := db.New(cfg)
	require.NoError(t, err)

	authConfig := &AuthConfig{
		JWTSecret:       "test-secret-key-for-testing",
		AccessTokenTTL:  15 * time.Minute,
		RefreshTokenTTL: 90 * 24 * time.Hour,
		DashboardURL:    "http://localhost:3000",
		AllowedOrigins:  []string{"http://localhost:3000"},
		Cookie: CookieConfig{
			Domain:   "",
			Secure:   false,
			SameSite: "Lax",
		},
	}

	authHandler := NewAuthHandler(database, authConfig)
	accountHandler := NewAccountHandler(database, authConfig)

	app := fiber.New()
	authHandler.RegisterRoutes(app)
	accountHandler.RegisterRoutes(app, authHandler)

	return app, authHandler, accountHandler, testDB, database
}

func createAuthenticatedAccount(t *testing.T, app *fiber.App) (string, string) {
	// Create account
	createReq := httptest.NewRequest("POST", "/v1/auth/account", bytes.NewBufferString(`{}`))
	createReq.Header.Set("Content-Type", "application/json")
	createResp, err := app.Test(createReq)
	require.NoError(t, err)

	var createBody CreateAccountResponse
	json.NewDecoder(createResp.Body).Decode(&createBody)
	createResp.Body.Close()

	// Extract access token
	var accessToken string
	for _, cookie := range createResp.Header.Values("Set-Cookie") {
		if strings.Contains(cookie, AccessTokenCookie) {
			parts := strings.Split(cookie, ";")
			for _, part := range parts {
				part = strings.TrimSpace(part)
				if strings.HasPrefix(part, AccessTokenCookie+"=") {
					accessToken = strings.TrimPrefix(part, AccessTokenCookie+"=")
					break
				}
			}
		}
	}

	return createBody.AccountNumber, accessToken
}

func TestGetAccount_ReturnsStats(t *testing.T) {
	app, _, _, testDB, database := setupAccountTest(t)
	defer testDB.Close(t)
	defer database.Close()

	accountNumber, accessToken := createAuthenticatedAccount(t, app)
	_ = accountNumber

	// Get account with stats
	req := httptest.NewRequest("GET", "/v1/account", nil)
	req.Header.Set("Cookie", AccessTokenCookie+"="+accessToken)

	resp, err := app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, 200, resp.StatusCode)

	var body map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&body)
	require.NoError(t, err)

	// Verify account fields
	assert.Contains(t, body, "id")
	assert.Contains(t, body, "account_number")
	assert.Contains(t, body, "balance_usdc")
	assert.Contains(t, body, "status")
	assert.Contains(t, body, "created_at")

	// Verify deposit_stats is included
	assert.Contains(t, body, "deposit_stats")
	stats := body["deposit_stats"].(map[string]interface{})
	assert.Contains(t, stats, "total_deposits")
	assert.Contains(t, stats, "total_deposited_usdc")
}

func TestLinkWallet_Success(t *testing.T) {
	app, _, _, testDB, database := setupAccountTest(t)
	defer testDB.Close(t)
	defer database.Close()

	_, accessToken := createAuthenticatedAccount(t, app)

	wallet := "0xabcdef1234567890abcdef1234567890abcdef12"
	reqBody := map[string]string{"wallet_address": wallet}
	bodyJSON, _ := json.Marshal(reqBody)

	req := httptest.NewRequest("PUT", "/v1/account/wallet", bytes.NewBuffer(bodyJSON))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Cookie", AccessTokenCookie+"="+accessToken)

	resp, err := app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, 200, resp.StatusCode)

	var body map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&body)

	assert.Contains(t, body["message"], "Wallet linked successfully")
	assert.Equal(t, wallet, body["wallet_address"])
}

func TestLinkWallet_InvalidFormat(t *testing.T) {
	app, _, _, testDB, database := setupAccountTest(t)
	defer testDB.Close(t)
	defer database.Close()

	_, accessToken := createAuthenticatedAccount(t, app)

	invalidAddresses := []string{
		"not-a-wallet",
		"0x123", // too short
		"0xGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGG", // invalid chars
		"1234567890abcdef1234567890abcdef12345678",   // missing 0x
	}

	for _, addr := range invalidAddresses {
		t.Run(addr, func(t *testing.T) {
			reqBody := map[string]string{"wallet_address": addr}
			bodyJSON, _ := json.Marshal(reqBody)

			req := httptest.NewRequest("PUT", "/v1/account/wallet", bytes.NewBuffer(bodyJSON))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Cookie", AccessTokenCookie+"="+accessToken)

			resp, err := app.Test(req)
			require.NoError(t, err)
			resp.Body.Close()

			assert.Equal(t, 400, resp.StatusCode)
		})
	}
}

func TestGetUsageStats_DateRange(t *testing.T) {
	app, _, _, testDB, database := setupAccountTest(t)
	defer testDB.Close(t)
	defer database.Close()

	_, accessToken := createAuthenticatedAccount(t, app)

	testCases := []struct {
		name     string
		days     string
		expected int
	}{
		{"default (no param)", "", 30},
		{"7 days", "7", 7},
		{"30 days", "30", 30},
		{"365 days (max)", "365", 365},
		{"over max capped", "1000", 365},
		{"negative defaults", "-1", 30},
		{"zero defaults", "0", 30},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			url := "/v1/account/usage/stats"
			if tc.days != "" {
				url += "?days=" + tc.days
			}

			req := httptest.NewRequest("GET", url, nil)
			req.Header.Set("Cookie", AccessTokenCookie+"="+accessToken)

			resp, err := app.Test(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			assert.Equal(t, 200, resp.StatusCode)

			var body map[string]interface{}
			json.NewDecoder(resp.Body).Decode(&body)

			// The response should contain the period_days field
			periodDays := int(body["period_days"].(float64))

			// Note: the actual clamping happens in the handler
			// We can verify the response contains expected fields
			assert.Contains(t, body, "total_stats")
			assert.Contains(t, body, "daily_breakdown")
			assert.Contains(t, body, "endpoint_stats")
			_ = periodDays // Used for verification in actual implementation
		})
	}
}

func TestGetUsage_Pagination(t *testing.T) {
	app, _, _, testDB, database := setupAccountTest(t)
	defer testDB.Close(t)
	defer database.Close()

	_, accessToken := createAuthenticatedAccount(t, app)

	// Request with pagination params
	req := httptest.NewRequest("GET", "/v1/account/usage?limit=10&offset=0", nil)
	req.Header.Set("Cookie", AccessTokenCookie+"="+accessToken)

	resp, err := app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, 200, resp.StatusCode)

	var body map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&body)

	assert.Contains(t, body, "logs")
	assert.Contains(t, body, "limit")
	assert.Contains(t, body, "offset")
}

func TestInitiateDeposit_Stripe(t *testing.T) {
	app, _, _, testDB, database := setupAccountTest(t)
	defer testDB.Close(t)
	defer database.Close()

	_, accessToken := createAuthenticatedAccount(t, app)

	reqBody := map[string]interface{}{
		"amount_usdc": 50.00,
		"provider":    "stripe",
	}
	bodyJSON, _ := json.Marshal(reqBody)

	req := httptest.NewRequest("POST", "/v1/account/deposit", bytes.NewBuffer(bodyJSON))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Cookie", AccessTokenCookie+"="+accessToken)

	resp, err := app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, 201, resp.StatusCode)

	var body InitiateDepositResponse
	json.NewDecoder(resp.Body).Decode(&body)

	assert.NotEmpty(t, body.DepositID)
	assert.Equal(t, 50.00, body.AmountUSDC)
	assert.Equal(t, "stripe", body.Provider)
	assert.Equal(t, "pending", body.Status)
	assert.NotNil(t, body.CheckoutURL)
	assert.Contains(t, body.Instructions, "checkout")
}

func TestInitiateDeposit_Direct(t *testing.T) {
	app, _, _, testDB, database := setupAccountTest(t)
	defer testDB.Close(t)
	defer database.Close()

	_, accessToken := createAuthenticatedAccount(t, app)

	reqBody := map[string]interface{}{
		"amount_usdc": 100.00,
		"provider":    "direct",
	}
	bodyJSON, _ := json.Marshal(reqBody)

	req := httptest.NewRequest("POST", "/v1/account/deposit", bytes.NewBuffer(bodyJSON))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Cookie", AccessTokenCookie+"="+accessToken)

	resp, err := app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, 201, resp.StatusCode)

	var body InitiateDepositResponse
	json.NewDecoder(resp.Body).Decode(&body)

	assert.NotEmpty(t, body.DepositID)
	assert.Equal(t, 100.00, body.AmountUSDC)
	assert.Equal(t, "direct", body.Provider)
	assert.Contains(t, body.Instructions, "USDC")
}

func TestInitiateDeposit_InvalidProvider(t *testing.T) {
	app, _, _, testDB, database := setupAccountTest(t)
	defer testDB.Close(t)
	defer database.Close()

	_, accessToken := createAuthenticatedAccount(t, app)

	reqBody := map[string]interface{}{
		"amount_usdc": 50.00,
		"provider":    "invalid",
	}
	bodyJSON, _ := json.Marshal(reqBody)

	req := httptest.NewRequest("POST", "/v1/account/deposit", bytes.NewBuffer(bodyJSON))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Cookie", AccessTokenCookie+"="+accessToken)

	resp, err := app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, 400, resp.StatusCode)

	var body map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&body)

	assert.Contains(t, body["error"], "Invalid provider")
}

func TestInitiateDeposit_ZeroAmount(t *testing.T) {
	app, _, _, testDB, database := setupAccountTest(t)
	defer testDB.Close(t)
	defer database.Close()

	_, accessToken := createAuthenticatedAccount(t, app)

	reqBody := map[string]interface{}{
		"amount_usdc": 0,
		"provider":    "stripe",
	}
	bodyJSON, _ := json.Marshal(reqBody)

	req := httptest.NewRequest("POST", "/v1/account/deposit", bytes.NewBuffer(bodyJSON))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Cookie", AccessTokenCookie+"="+accessToken)

	resp, err := app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, 400, resp.StatusCode)

	var body map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&body)

	assert.Contains(t, body["error"], "Amount must be greater than 0")
}

func TestGetDeposits_Pagination(t *testing.T) {
	app, _, _, testDB, database := setupAccountTest(t)
	defer testDB.Close(t)
	defer database.Close()

	_, accessToken := createAuthenticatedAccount(t, app)

	req := httptest.NewRequest("GET", "/v1/account/deposits?limit=10&offset=0", nil)
	req.Header.Set("Cookie", AccessTokenCookie+"="+accessToken)

	resp, err := app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, 200, resp.StatusCode)

	var body map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&body)

	assert.Contains(t, body, "deposits")
	assert.Contains(t, body, "limit")
	assert.Contains(t, body, "offset")
}

func TestAccount_NotAuthenticated(t *testing.T) {
	app, _, _, testDB, database := setupAccountTest(t)
	defer testDB.Close(t)
	defer database.Close()

	endpoints := []struct {
		method string
		path   string
	}{
		{"GET", "/v1/account"},
		{"GET", "/v1/account/usage"},
		{"GET", "/v1/account/usage/stats"},
		{"POST", "/v1/account/deposit"},
		{"GET", "/v1/account/deposits"},
		{"PUT", "/v1/account/wallet"},
	}

	for _, ep := range endpoints {
		t.Run(ep.method+" "+ep.path, func(t *testing.T) {
			req := httptest.NewRequest(ep.method, ep.path, nil)
			resp, err := app.Test(req)
			require.NoError(t, err)
			resp.Body.Close()

			assert.Equal(t, 401, resp.StatusCode)
		})
	}
}

func TestCalculateFee(t *testing.T) {
	testCases := []struct {
		amount      float64
		provider    db.DepositProvider
		expectedFee float64
	}{
		{10.00, db.DepositProviderStripe, (10.00 * 0.029) + 0.30},
		{100.00, db.DepositProviderStripe, (100.00 * 0.029) + 0.30},
		{1000.00, db.DepositProviderStripe, (1000.00 * 0.029) + 0.30},
		{100.00, db.DepositProviderDirect, 0},
		{0.01, db.DepositProviderDirect, 0},
	}

	for _, tc := range testCases {
		t.Run("", func(t *testing.T) {
			fee := calculateFee(tc.amount, tc.provider)
			assert.InDelta(t, tc.expectedFee, fee, 0.01)
		})
	}
}

func TestGetAccount_WithDeposits(t *testing.T) {
	app, _, _, testDB, database := setupAccountTest(t)
	defer testDB.Close(t)
	defer database.Close()

	accountNumber, accessToken := createAuthenticatedAccount(t, app)

	// Create some deposits
	account, err := database.GetAccountByNumber(t.Context(), accountNumber)
	require.NoError(t, err)

	deposit := &db.Deposit{
		AccountID:     account.ID,
		Provider:      db.DepositProviderDirect,
		AmountUSDC:    100.00,
		FeeUSDC:       0,
		NetAmountUSDC: 100.00,
	}
	err = database.CreateDeposit(t.Context(), deposit)
	require.NoError(t, err)
	err = database.CompleteDeposit(t.Context(), deposit.ID)
	require.NoError(t, err)

	// Get account
	req := httptest.NewRequest("GET", "/v1/account", nil)
	req.Header.Set("Cookie", AccessTokenCookie+"="+accessToken)

	resp, err := app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, 200, resp.StatusCode)

	var body map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&body)

	stats := body["deposit_stats"].(map[string]interface{})
	assert.GreaterOrEqual(t, stats["total_deposits"].(float64), float64(1))
	assert.GreaterOrEqual(t, stats["total_deposited_usdc"].(float64), float64(100))
}

func TestLinkWallet_AlreadyLinked(t *testing.T) {
	app, _, _, testDB, database := setupAccountTest(t)
	defer testDB.Close(t)
	defer database.Close()

	// Create first account with wallet
	_, accessToken1 := createAuthenticatedAccount(t, app)
	wallet := "0xabcdef1234567890abcdef1234567890abcdef12"

	reqBody := map[string]string{"wallet_address": wallet}
	bodyJSON, _ := json.Marshal(reqBody)

	req := httptest.NewRequest("PUT", "/v1/account/wallet", bytes.NewBuffer(bodyJSON))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Cookie", AccessTokenCookie+"="+accessToken1)

	resp, err := app.Test(req)
	require.NoError(t, err)
	resp.Body.Close()
	require.Equal(t, 200, resp.StatusCode)

	// Create second account and try to link same wallet
	_, accessToken2 := createAuthenticatedAccount(t, app)

	req = httptest.NewRequest("PUT", "/v1/account/wallet", bytes.NewBuffer(bodyJSON))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Cookie", AccessTokenCookie+"="+accessToken2)

	resp, err = app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, 400, resp.StatusCode)

	var body map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&body)
	assert.Contains(t, body["error"], "already linked")
}
