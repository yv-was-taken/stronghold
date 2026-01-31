package handlers

import (
	"time"

	"stronghold/internal/db"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
)

// AccountHandler handles account management endpoints
type AccountHandler struct {
	db         *db.DB
	authConfig *AuthConfig
}

// NewAccountHandler creates a new account handler
func NewAccountHandler(database *db.DB, authConfig *AuthConfig) *AccountHandler {
	return &AccountHandler{
		db:         database,
		authConfig: authConfig,
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
	group.Put("/wallet", authHandler.AuthMiddleware(), h.LinkWallet)
}

// GetAccount returns the current account details
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
	DepositID     string  `json:"deposit_id"`
	AmountUSDC    float64 `json:"amount_usdc"`
	Provider      string  `json:"provider"`
	Status        string  `json:"status"`
	CheckoutURL   *string `json:"checkout_url,omitempty"`
	WalletAddress *string `json:"wallet_address,omitempty"`
	Instructions  string  `json:"instructions"`
}

// InitiateDeposit initiates a new deposit
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
		// In production, this would create a Stripe Checkout Session
		// and return the checkout URL
		checkoutURL := "https://checkout.stripe.com/pay/placeholder"
		resp.CheckoutURL = &checkoutURL
		resp.Instructions = "Complete your payment using the provided checkout URL. Funds will be credited to your account upon completion."

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
		return (amount * 0.029) + 0.30
	case db.DepositProviderDirect:
		// No fee for direct deposits
		return 0
	default:
		return 0
	}
}
