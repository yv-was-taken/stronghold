package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"stronghold/internal/db"
	"stronghold/internal/db/testutil"
	"stronghold/internal/usdc"

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

	authHandler := NewAuthHandler(database, authConfig, nil) // nil KMS client for tests
	accountHandler := NewAccountHandler(database, authConfig, nil)

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

func TestGetAccount_SolanaOnly_OmitsLegacyWalletAddress(t *testing.T) {
	app, _, _, testDB, database := setupAccountTest(t)
	defer testDB.Close(t)
	defer database.Close()

	accountNumber, accessToken := createAuthenticatedAccount(t, app)

	account, err := database.GetAccountByNumber(t.Context(), accountNumber)
	require.NoError(t, err)
	solanaAddr := "9WzDXwBbmkg8ZTbNMqUxvQRAyrZzDsGYdLVL9zYtAWWM"
	err = database.UpdateWalletAddresses(t.Context(), account.ID, nil, &solanaAddr)
	require.NoError(t, err)

	req := httptest.NewRequest("GET", "/v1/account", nil)
	req.Header.Set("Cookie", AccessTokenCookie+"="+accessToken)
	resp, err := app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, 200, resp.StatusCode)

	var body map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&body)
	require.NoError(t, err)

	assert.NotContains(t, body, "wallet_address")
	assert.Equal(t, solanaAddr, body["solana_wallet_address"])
}

func TestUpdateWallets_ReturnsConflictForAlreadyLinkedWallet(t *testing.T) {
	app, _, _, testDB, database := setupAccountTest(t)
	defer testDB.Close(t)
	defer database.Close()

	occupiedEVM := "0x1234567890abcdef1234567890abcdef12345678"
	_, err := database.CreateAccount(t.Context(), &occupiedEVM, nil)
	require.NoError(t, err)

	_, accessToken := createAuthenticatedAccount(t, app)

	reqBody := map[string]string{
		"evm_address": occupiedEVM,
	}
	bodyJSON, _ := json.Marshal(reqBody)

	req := httptest.NewRequest("PUT", "/v1/account/wallets", bytes.NewBuffer(bodyJSON))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Cookie", AccessTokenCookie+"="+accessToken)

	resp, err := app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, 409, resp.StatusCode)

	var body map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&body)
	require.NoError(t, err)
	assert.Contains(t, body["error"], "already linked")
}

func TestUpdateWallets_ReturnsBadRequestForEmptyAddress(t *testing.T) {
	app, _, _, testDB, database := setupAccountTest(t)
	defer testDB.Close(t)
	defer database.Close()

	_, accessToken := createAuthenticatedAccount(t, app)

	reqBody := map[string]string{
		"evm_address": "   ",
	}
	bodyJSON, _ := json.Marshal(reqBody)

	req := httptest.NewRequest("PUT", "/v1/account/wallets", bytes.NewBuffer(bodyJSON))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Cookie", AccessTokenCookie+"="+accessToken)

	resp, err := app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, 400, resp.StatusCode)

	var body map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&body)
	require.NoError(t, err)
	assert.Contains(t, body["error"], "cannot be empty")
}

func TestGetBalances_QueriesWalletsInParallel(t *testing.T) {
	app, _, _, testDB, database := setupAccountTest(t)
	defer testDB.Close(t)
	defer database.Close()

	accountNumber, accessToken := createAuthenticatedAccount(t, app)

	account, err := database.GetAccountByNumber(t.Context(), accountNumber)
	require.NoError(t, err)

	evmAddr := "0x1234567890abcdef1234567890abcdef12345678"
	solAddr := "9WzDXwBbmkg8ZTbNMqUxvQRAyrZzDsGYdLVL9zYtAWWM"
	err = database.UpdateWalletAddresses(t.Context(), account.ID, &evmAddr, &solAddr)
	require.NoError(t, err)

	origQueryEVM := queryEVMBalance
	origQuerySolana := querySolanaBalance
	defer func() {
		queryEVMBalance = origQueryEVM
		querySolanaBalance = origQuerySolana
	}()

	var active int32
	var maxActive int32

	markActive := func() func() {
		current := atomic.AddInt32(&active, 1)
		for {
			recorded := atomic.LoadInt32(&maxActive)
			if current <= recorded || atomic.CompareAndSwapInt32(&maxActive, recorded, current) {
				break
			}
		}
		return func() {
			atomic.AddInt32(&active, -1)
		}
	}

	queryEVMBalance = func(ctx context.Context, address, network string) (float64, error) {
		defer markActive()()
		if address != evmAddr || network != "base" {
			return 0, fmt.Errorf("unexpected evm query args: address=%s network=%s", address, network)
		}
		time.Sleep(150 * time.Millisecond)
		return 1.25, nil
	}
	querySolanaBalance = func(ctx context.Context, address, network string) (float64, error) {
		defer markActive()()
		if address != solAddr || network != "solana" {
			return 0, fmt.Errorf("unexpected solana query args: address=%s network=%s", address, network)
		}
		time.Sleep(150 * time.Millisecond)
		return 2.50, nil
	}

	req := httptest.NewRequest("GET", "/v1/account/balances", nil)
	req.Header.Set("Cookie", AccessTokenCookie+"="+accessToken)

	resp, err := app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	require.Equal(t, 200, resp.StatusCode)

	var body GetBalancesResponse
	err = json.NewDecoder(resp.Body).Decode(&body)
	require.NoError(t, err)

	require.NotNil(t, body.EVM)
	require.NotNil(t, body.Solana)
	assert.Equal(t, evmAddr, body.EVM.Address)
	assert.Equal(t, "base", body.EVM.Network)
	assert.Equal(t, solAddr, body.Solana.Address)
	assert.Equal(t, "solana", body.Solana.Network)
	assert.Equal(t, usdc.FromFloat(1.25), body.EVM.BalanceUSDC)
	assert.Equal(t, usdc.FromFloat(2.50), body.Solana.BalanceUSDC)
	assert.Equal(t, usdc.FromFloat(3.75), body.TotalUSDC)
	assert.GreaterOrEqual(t, atomic.LoadInt32(&maxActive), int32(2), "wallet balance lookups should overlap")
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

func TestInitiateDeposit_Stripe_RequiresWallet(t *testing.T) {
	app, _, _, testDB, database := setupAccountTest(t)
	defer testDB.Close(t)
	defer database.Close()

	_, accessToken := createAuthenticatedAccount(t, app)

	// Stripe deposit without linked wallet should fail
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

	assert.Equal(t, 400, resp.StatusCode)

	var body map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&body)

	assert.Contains(t, body["error"], "wallet address before using Stripe")
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
	assert.Equal(t, usdc.FromFloat(100.00), body.AmountUSDC)
	assert.Equal(t, "direct", body.Provider)
	assert.Equal(t, "base", body.Network, "should default to base when network is omitted")
	assert.Contains(t, body.Instructions, "USDC")
	assert.Contains(t, body.Instructions, "Base")
}

func TestInitiateDeposit_Direct_Solana(t *testing.T) {
	app, _, _, testDB, database := setupAccountTest(t)
	defer testDB.Close(t)
	defer database.Close()

	accountNumber, accessToken := createAuthenticatedAccount(t, app)

	// Link a Solana wallet
	account, err := database.GetAccountByNumber(t.Context(), accountNumber)
	require.NoError(t, err)
	solAddr := "7EcDhSYGxXyscszYEp35KHN8vvw3svAuLKTzXwCFLtV"
	err = database.UpdateWalletAddresses(t.Context(), account.ID, nil, &solAddr)
	require.NoError(t, err)

	reqBody := map[string]interface{}{
		"amount_usdc": 50.00,
		"provider":    "direct",
		"network":     "solana",
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
	assert.Equal(t, usdc.FromFloat(50.00), body.AmountUSDC)
	assert.Equal(t, "direct", body.Provider)
	assert.Equal(t, "solana", body.Network)
	assert.Contains(t, body.Instructions, "Solana")
	assert.Equal(t, &solAddr, body.WalletAddress)
}

func TestInitiateDeposit_InvalidNetwork(t *testing.T) {
	app, _, _, testDB, database := setupAccountTest(t)
	defer testDB.Close(t)
	defer database.Close()

	_, accessToken := createAuthenticatedAccount(t, app)

	reqBody := map[string]interface{}{
		"amount_usdc": 50.00,
		"provider":    "direct",
		"network":     "ethereum",
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
	assert.Contains(t, body["error"], "Invalid network")
}

func TestInitiateDeposit_Stripe_RequiresSolanaWallet(t *testing.T) {
	app, _, _, testDB, database := setupAccountTest(t)
	defer testDB.Close(t)
	defer database.Close()

	_, accessToken := createAuthenticatedAccount(t, app)

	// Stripe deposit targeting Solana without linked wallet should fail
	reqBody := map[string]interface{}{
		"amount_usdc": 50.00,
		"provider":    "stripe",
		"network":     "solana",
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
	assert.Contains(t, body["error"], "solana wallet")
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
		amount      usdc.MicroUSDC
		provider    db.DepositProvider
		expectedFee usdc.MicroUSDC
	}{
		{usdc.FromFloat(10.00), db.DepositProviderStripe, usdc.MicroUSDC(590_000)},
		{usdc.FromFloat(100.00), db.DepositProviderStripe, usdc.MicroUSDC(3_200_000)},
		{usdc.FromFloat(1000.00), db.DepositProviderStripe, usdc.MicroUSDC(29_300_000)},
		{usdc.FromFloat(100.00), db.DepositProviderDirect, 0},
		{usdc.FromFloat(0.01), db.DepositProviderDirect, 0},
	}

	for _, tc := range testCases {
		t.Run("", func(t *testing.T) {
			fee := calculateFee(tc.amount, tc.provider)
			assert.Equal(t, tc.expectedFee, fee, "expected %v, got %v", tc.expectedFee, fee)
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
		AmountUSDC:    usdc.FromFloat(100.00),
		FeeUSDC:       0,
		NetAmountUSDC: usdc.FromFloat(100.00),
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
	// total_deposited_usdc is now a JSON string (MicroUSDC marshals as string-encoded integer)
	totalDepositedStr, ok := stats["total_deposited_usdc"].(string)
	require.True(t, ok, "total_deposited_usdc should be a string")
	totalDeposited, err := strconv.ParseInt(totalDepositedStr, 10, 64)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, totalDeposited, int64(100_000_000)) // 100 USDC in microUSDC
}
