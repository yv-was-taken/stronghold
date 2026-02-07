package handlers

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"stronghold/internal/config"
	"stronghold/internal/db"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
)

// AccountHandler handles account management endpoints
type AccountHandler struct {
	db           *db.DB
	authConfig   *AuthConfig
	stripeConfig *config.StripeConfig
}

// NewAccountHandler creates a new account handler
func NewAccountHandler(database *db.DB, authConfig *AuthConfig, stripeConfig *config.StripeConfig) *AccountHandler {
	return &AccountHandler{
		db:           database,
		authConfig:   authConfig,
		stripeConfig: stripeConfig,
	}
}

// RegisterRoutes registers account routes
func (h *AccountHandler) RegisterRoutes(app *fiber.App, authHandler *AuthHandler) {
	group := app.Group("/v1/account")

	// All account routes require authentication
	group.Get("/", authHandler.AuthMiddleware(), h.GetAccount)
	group.Get("/usage", authHandler.AuthMiddleware(), h.GetUsage)
	group.Get("/usage/stats", authHandler.AuthMiddleware(), h.GetUsageStats)
	group.Post("/deposit", authHandler.AuthMiddleware(), h.InitiateDeposit)
	group.Get("/deposits", authHandler.AuthMiddleware(), h.GetDeposits)
	// NOTE: Wallet linking removed - wallets are generated server-side with KMS encryption.
	// Changing wallet requires providing the private key (via CLI only).
}

// GetAccount returns the current account details
// @Summary Get account details
// @Description Returns the authenticated user's account details including balance and deposit stats
// @Tags account
// @Produce json
// @Success 200 {object} map[string]interface{} "Account details"
// @Failure 401 {object} map[string]string "Not authenticated"
// @Failure 404 {object} map[string]string "Account not found"
// @Security CookieAuth
// @Router /v1/account [get]
func (h *AccountHandler) GetAccount(c fiber.Ctx) error {
	accountIDStr := c.Locals("account_id")
	if accountIDStr == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "Not authenticated",
		})
	}

	accountID, err := uuid.Parse(accountIDStr.(string))
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Invalid account ID",
		})
	}

	ctx := c.Context()

	account, err := h.db.GetAccountByID(ctx, accountID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Account not found",
		})
	}

	// Get deposit stats
	depositStats, err := h.db.GetDepositStats(ctx, accountID)
	if err != nil {
		// Log but don't fail
		depositStats = &db.DepositStats{}
	}

	return c.JSON(fiber.Map{
		"id":              account.ID,
		"account_number":  account.AccountNumber,
		"wallet_address":  account.WalletAddress,
		"balance_usdc":    account.BalanceUSDC,
		"status":          account.Status,
		"created_at":      account.CreatedAt,
		"updated_at":      account.UpdatedAt,
		"last_login_at":   account.LastLoginAt,
		"deposit_stats":   depositStats,
	})
}

// GetUsageRequest represents the query parameters for usage logs
type GetUsageRequest struct {
	Limit  int `query:"limit"`
	Offset int `query:"offset"`
}

// GetUsage returns usage logs for the account
// @Summary Get usage logs
// @Description Returns paginated usage logs for the authenticated account
// @Tags account
// @Produce json
// @Param limit query int false "Number of records to return (default 50)"
// @Param offset query int false "Number of records to skip (default 0)"
// @Success 200 {object} map[string]interface{} "Usage logs with pagination"
// @Failure 401 {object} map[string]string "Not authenticated"
// @Failure 500 {object} map[string]string "Server error"
// @Security CookieAuth
// @Router /v1/account/usage [get]
func (h *AccountHandler) GetUsage(c fiber.Ctx) error {
	accountIDStr := c.Locals("account_id")
	if accountIDStr == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "Not authenticated",
		})
	}

	accountID, err := uuid.Parse(accountIDStr.(string))
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Invalid account ID",
		})
	}

	var req GetUsageRequest
	if err := c.Bind().Query(&req); err != nil {
		// Apply sensible defaults when query binding fails (e.g., invalid types)
		req.Limit = 50
		req.Offset = 0
	}

	ctx := c.Context()

	logs, err := h.db.GetUsageLogs(ctx, accountID, req.Limit, req.Offset)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to get usage logs",
		})
	}

	return c.JSON(fiber.Map{
		"logs":   logs,
		"limit":  req.Limit,
		"offset": req.Offset,
	})
}

// GetUsageStatsRequest represents the query parameters for usage stats
type GetUsageStatsRequest struct {
	Days int `query:"days"`
}

// GetUsageStats returns aggregated usage statistics for the account
// @Summary Get usage statistics
// @Description Returns aggregated usage statistics including daily breakdown and endpoint stats
// @Tags account
// @Produce json
// @Param days query int false "Number of days to include (default 30, max 365)"
// @Success 200 {object} map[string]interface{} "Usage statistics"
// @Failure 401 {object} map[string]string "Not authenticated"
// @Failure 500 {object} map[string]string "Server error"
// @Security CookieAuth
// @Router /v1/account/usage/stats [get]
func (h *AccountHandler) GetUsageStats(c fiber.Ctx) error {
	accountIDStr := c.Locals("account_id")
	if accountIDStr == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "Not authenticated",
		})
	}

	accountID, err := uuid.Parse(accountIDStr.(string))
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Invalid account ID",
		})
	}

	var req GetUsageStatsRequest
	if err := c.Bind().Query(&req); err != nil {
		req.Days = 30
	}

	if req.Days <= 0 {
		req.Days = 30
	}
	if req.Days > 365 {
		req.Days = 365
	}

	ctx := c.Context()

	// Get overall stats for the period
	end := time.Now().UTC()
	start := end.AddDate(0, 0, -req.Days)

	stats, err := h.db.GetUsageStats(ctx, accountID, start, end)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to get usage stats",
		})
	}

	// Get daily breakdown
	dailyStats, err := h.db.GetDailyUsageStats(ctx, accountID, req.Days)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to get daily usage stats",
		})
	}

	// Get endpoint breakdown
	endpointStats, err := h.db.GetEndpointUsageStats(ctx, accountID, start, end)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to get endpoint usage stats",
		})
	}

	return c.JSON(fiber.Map{
		"period_days":    req.Days,
		"total_stats":    stats,
		"daily_breakdown": dailyStats,
		"endpoint_stats": endpointStats,
	})
}

// InitiateDepositRequest represents a request to initiate a deposit
type InitiateDepositRequest struct {
	AmountUSDC float64 `json:"amount_usdc"`
	Provider   string  `json:"provider"`
}

// InitiateDepositResponse represents the response after initiating a deposit
type InitiateDepositResponse struct {
	DepositID      string  `json:"deposit_id"`
	AmountUSDC     float64 `json:"amount_usdc"`
	Provider       string  `json:"provider"`
	Status         string  `json:"status"`
	CheckoutURL    *string `json:"checkout_url,omitempty"`
	ClientSecret   *string `json:"client_secret,omitempty"`
	PublishableKey *string `json:"publishable_key,omitempty"`
	WalletAddress  *string `json:"wallet_address,omitempty"`
	Instructions   string  `json:"instructions"`
}

// InitiateDeposit initiates a new deposit
// @Summary Initiate a deposit
// @Description Start a deposit via Stripe (requires linked wallet) or direct crypto transfer
// @Tags account
// @Accept json
// @Produce json
// @Param request body InitiateDepositRequest true "Deposit details"
// @Success 201 {object} InitiateDepositResponse
// @Failure 400 {object} map[string]string "Invalid request or no wallet linked"
// @Failure 401 {object} map[string]string "Not authenticated"
// @Failure 500 {object} map[string]string "Server error"
// @Security CookieAuth
// @Router /v1/account/deposit [post]
func (h *AccountHandler) InitiateDeposit(c fiber.Ctx) error {
	accountIDStr := c.Locals("account_id")
	if accountIDStr == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "Not authenticated",
		})
	}

	accountID, err := uuid.Parse(accountIDStr.(string))
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Invalid account ID",
		})
	}

	var req InitiateDepositRequest
	if err := c.Bind().Body(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	if req.AmountUSDC <= 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Amount must be greater than 0",
		})
	}

	// Validate provider
	var provider db.DepositProvider
	switch req.Provider {
	case "stripe":
		provider = db.DepositProviderStripe
	case "direct":
		provider = db.DepositProviderDirect
	default:
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid provider. Must be 'stripe' or 'direct'",
		})
	}

	ctx := c.Context()

	// Get account to check wallet address
	account, err := h.db.GetAccountByID(ctx, accountID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Account not found",
		})
	}

	// Create deposit record
	deposit := &db.Deposit{
		AccountID:    accountID,
		Provider:     provider,
		AmountUSDC:   req.AmountUSDC,
		FeeUSDC:      calculateFee(req.AmountUSDC, provider),
		Status:       db.DepositStatusPending,
	}

	if account.WalletAddress != nil {
		deposit.WalletAddress = account.WalletAddress
	}

	if err := h.db.CreateDeposit(ctx, deposit); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to create deposit",
		})
	}

	// Build response based on provider
	resp := InitiateDepositResponse{
		DepositID:  deposit.ID.String(),
		AmountUSDC: req.AmountUSDC,
		Provider:   string(provider),
		Status:     string(db.DepositStatusPending),
	}

	switch provider {
	case db.DepositProviderStripe:
		// Require linked wallet for Stripe onramp
		if account.WalletAddress == nil || *account.WalletAddress == "" {
			// Cancel the pending deposit
			_ = h.db.FailDeposit(ctx, deposit.ID, "No wallet linked")
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "You must link a wallet address before using Stripe to purchase crypto",
			})
		}

		// Create Stripe Crypto Onramp session
		session, err := h.createStripeOnrampSession(deposit.ID.String(), *account.WalletAddress)
		if err != nil {
			// Mark deposit as failed
			_ = h.db.FailDeposit(ctx, deposit.ID, fmt.Sprintf("Failed to create Stripe session: %v", err))
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "Failed to create payment session",
			})
		}

		// Update deposit with session ID
		if err := h.db.UpdateDepositProviderTransaction(ctx, deposit.ID, session.ID); err != nil {
			// Log but don't fail - the session was created successfully
			slog.Error("failed to update deposit with provider transaction ID",
				"deposit_id", deposit.ID,
				"session_id", session.ID,
				"error", err,
			)
		}

		resp.ClientSecret = &session.ClientSecret
		if h.stripeConfig != nil {
			resp.PublishableKey = &h.stripeConfig.PublishableKey
		}
		resp.Instructions = "Complete your purchase using the Stripe Crypto Onramp. USDC will be delivered to your wallet and credited to your account."

	case db.DepositProviderDirect:
		if account.WalletAddress != nil {
			resp.WalletAddress = account.WalletAddress
		}
		resp.Instructions = "Send USDC on Base network to your wallet address. Deposits are credited automatically after blockchain confirmation."
	}

	return c.Status(fiber.StatusCreated).JSON(resp)
}

// GetDepositsRequest represents the query parameters for deposits
type GetDepositsRequest struct {
	Limit  int `query:"limit"`
	Offset int `query:"offset"`
}

// GetDeposits returns deposit history for the account
// @Summary Get deposit history
// @Description Returns paginated deposit history for the authenticated account
// @Tags account
// @Produce json
// @Param limit query int false "Number of records to return (default 50)"
// @Param offset query int false "Number of records to skip (default 0)"
// @Success 200 {object} map[string]interface{} "Deposits with pagination"
// @Failure 401 {object} map[string]string "Not authenticated"
// @Failure 500 {object} map[string]string "Server error"
// @Security CookieAuth
// @Router /v1/account/deposits [get]
func (h *AccountHandler) GetDeposits(c fiber.Ctx) error {
	accountIDStr := c.Locals("account_id")
	if accountIDStr == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "Not authenticated",
		})
	}

	accountID, err := uuid.Parse(accountIDStr.(string))
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Invalid account ID",
		})
	}

	var req GetDepositsRequest
	if err := c.Bind().Query(&req); err != nil {
		req.Limit = 50
		req.Offset = 0
	}

	ctx := c.Context()

	deposits, err := h.db.GetDepositsByAccount(ctx, accountID, req.Limit, req.Offset)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to get deposits",
		})
	}

	return c.JSON(fiber.Map{
		"deposits": deposits,
		"limit":    req.Limit,
		"offset":   req.Offset,
	})
}

// LinkWalletRequest represents a request to link a wallet
type LinkWalletRequest struct {
	WalletAddress string `json:"wallet_address"`
}

// LinkWallet links a wallet address to the account
// @Summary Link a wallet address
// @Description Links an Ethereum wallet address to the account for receiving crypto
// @Tags account
// @Accept json
// @Produce json
// @Param request body LinkWalletRequest true "Wallet address (0x...)"
// @Success 200 {object} map[string]string "Wallet linked successfully"
// @Failure 400 {object} map[string]string "Invalid wallet address or already linked"
// @Failure 401 {object} map[string]string "Not authenticated"
// @Security CookieAuth
// @Router /v1/account/wallet [put]
func (h *AccountHandler) LinkWallet(c fiber.Ctx) error {
	accountIDStr := c.Locals("account_id")
	if accountIDStr == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "Not authenticated",
		})
	}

	accountID, err := uuid.Parse(accountIDStr.(string))
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Invalid account ID",
		})
	}

	var req LinkWalletRequest
	if err := c.Bind().Body(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	if !isValidWalletAddress(req.WalletAddress) {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid wallet address format",
		})
	}

	ctx := c.Context()

	if err := h.db.LinkWallet(ctx, accountID, req.WalletAddress); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"message":        "Wallet linked successfully",
		"wallet_address": req.WalletAddress,
	})
}

// calculateFee calculates the fee for a deposit based on provider
func calculateFee(amount float64, provider db.DepositProvider) float64 {
	switch provider {
	case db.DepositProviderStripe:
		// Stripe typically charges 2.9% + $0.30
		return amount*0.029 + 0.30
	case db.DepositProviderDirect:
		// No fee for direct deposits
		return 0
	default:
		return 0
	}
}

// stripeOnrampSession represents a Stripe Crypto Onramp session response
type stripeOnrampSession struct {
	ID           string `json:"id"`
	ClientSecret string `json:"client_secret"`
	Status       string `json:"status"`
}

// createStripeOnrampSession creates a Stripe Crypto Onramp session
func (h *AccountHandler) createStripeOnrampSession(depositID, walletAddress string) (*stripeOnrampSession, error) {
	if h.stripeConfig == nil || h.stripeConfig.SecretKey == "" {
		return nil, fmt.Errorf("stripe not configured")
	}

	// Build form data for the API request
	data := url.Values{}
	data.Set("wallet_addresses[base]", walletAddress)
	data.Set("destination_currencies[]", "usdc")
	data.Set("destination_networks[]", "base")
	data.Set("lock_wallet_address", "true")
	data.Set("metadata[deposit_id]", depositID)

	req, err := http.NewRequest("POST", "https://api.stripe.com/v1/crypto/onramp_sessions", strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+h.stripeConfig.SecretKey)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	// Use beta header for crypto onramp API
	req.Header.Set("Stripe-Version", "2024-12-18.acacia;crypto_onramp_beta=v2")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to call Stripe API: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		var errResp struct {
			Error struct {
				Message string `json:"message"`
				Type    string `json:"type"`
				Code    string `json:"code"`
			} `json:"error"`
		}
		if err := json.Unmarshal(body, &errResp); err == nil && errResp.Error.Message != "" {
			return nil, fmt.Errorf("stripe error: %s", errResp.Error.Message)
		}
		return nil, fmt.Errorf("stripe API error (status %d): %s", resp.StatusCode, string(body))
	}

	var session stripeOnrampSession
	if err := json.Unmarshal(body, &session); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &session, nil
}
