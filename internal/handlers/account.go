package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"stronghold/internal/config"
	"stronghold/internal/db"
	"stronghold/internal/usdc"
	"stronghold/internal/wallet"

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
	group.Get("/", authHandler.AuthMiddleware(), authHandler.RequireTrustedDevice(), h.GetAccount)
	group.Get("/usage", authHandler.AuthMiddleware(), authHandler.RequireTrustedDevice(), h.GetUsage)
	group.Get("/usage/stats", authHandler.AuthMiddleware(), authHandler.RequireTrustedDevice(), h.GetUsageStats)
	group.Post("/deposit", authHandler.AuthMiddleware(), authHandler.RequireTrustedDevice(), h.InitiateDeposit)
	group.Get("/deposits", authHandler.AuthMiddleware(), authHandler.RequireTrustedDevice(), h.GetDeposits)
	group.Put("/wallets", authHandler.AuthMiddleware(), authHandler.RequireTrustedDevice(), h.UpdateWallets)
	group.Get("/balances", authHandler.AuthMiddleware(), authHandler.RequireTrustedDevice(), h.GetBalances)
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

	resp := fiber.Map{
		"id":                    account.ID,
		"account_number":        account.AccountNumber,
		"evm_wallet_address":    account.EVMWalletAddress,
		"solana_wallet_address": account.SolanaWalletAddress,
		"balance_usdc":          account.BalanceUSDC,
		"status":                account.Status,
		"wallet_escrow_enabled": account.WalletEscrow,
		"totp_enabled":          account.TOTPEnabled,
		"created_at":            account.CreatedAt,
		"updated_at":            account.UpdatedAt,
		"last_login_at":         account.LastLoginAt,
		"deposit_stats":         depositStats,
	}
	if account.EVMWalletAddress != nil {
		resp["wallet_address"] = account.EVMWalletAddress
	}

	return c.JSON(resp)
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
		"period_days":     req.Days,
		"total_stats":     stats,
		"daily_breakdown": dailyStats,
		"endpoint_stats":  endpointStats,
	})
}

// InitiateDepositRequest represents a request to initiate a deposit
type InitiateDepositRequest struct {
	AmountUSDC float64 `json:"amount_usdc"`
	Provider   string  `json:"provider"`
	Network    string  `json:"network"` // "base" (default) or "solana"
}

// InitiateDepositResponse represents the response after initiating a deposit
type InitiateDepositResponse struct {
	DepositID      string         `json:"deposit_id"`
	AmountUSDC     usdc.MicroUSDC `json:"amount_usdc"`
	Provider       string         `json:"provider"`
	Network        string  `json:"network"`
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

	// Convert float64 request amount to MicroUSDC immediately
	amountMicro := usdc.FromFloat(req.AmountUSDC)

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

	// Validate network - default to "base" if empty
	network := req.Network
	if network == "" {
		network = "base"
	}
	if network != "base" && network != "solana" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid network. Must be 'base' or 'solana'",
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

	// Resolve wallet address for the selected network
	var walletAddress *string
	switch network {
	case "base":
		walletAddress = account.EVMWalletAddress
	case "solana":
		walletAddress = account.SolanaWalletAddress
	}

	// Create deposit record
	deposit := &db.Deposit{
		AccountID:  accountID,
		Provider:   provider,
		AmountUSDC: amountMicro,
		FeeUSDC:    calculateFee(amountMicro, provider),
		Status:     db.DepositStatusPending,
		Metadata:   map[string]any{"network": network},
	}

	if walletAddress != nil {
		deposit.WalletAddress = walletAddress
	}

	if err := h.db.CreateDeposit(ctx, deposit); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to create deposit",
		})
	}

	// Build response based on provider
	resp := InitiateDepositResponse{
		DepositID:  deposit.ID.String(),
		AmountUSDC: amountMicro,
		Provider:   string(provider),
		Network:    network,
		Status:     string(db.DepositStatusPending),
	}

	switch provider {
	case db.DepositProviderStripe:
		// Require linked wallet for the selected network
		if walletAddress == nil || *walletAddress == "" {
			// Cancel the pending deposit
			_ = h.db.FailDeposit(ctx, deposit.ID, fmt.Sprintf("No %s wallet linked", network))
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": fmt.Sprintf("You must link a %s wallet address before using Stripe to purchase crypto", network),
			})
		}

		// Create Stripe Crypto Onramp session targeting the selected network
		session, err := h.createStripeOnrampSession(deposit.ID.String(), *walletAddress, network)
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
		networkLabel := "Base"
		if network == "solana" {
			networkLabel = "Solana"
		}
		resp.Instructions = fmt.Sprintf("Complete your purchase using the Stripe Crypto Onramp. USDC will be delivered to your %s wallet and credited to your account.", networkLabel)

	case db.DepositProviderDirect:
		if walletAddress != nil {
			resp.WalletAddress = walletAddress
		}
		switch network {
		case "base":
			resp.Instructions = "Send USDC on Base network to your wallet address. Deposits are credited automatically after blockchain confirmation."
		case "solana":
			resp.Instructions = "Send USDC on Solana network to your wallet address. Deposits are credited automatically after blockchain confirmation."
		}
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

// Validation patterns for wallet addresses
var (
	evmAddressRegex    = regexp.MustCompile(`^0x[a-fA-F0-9]{40}$`)
	solanaAddressRegex = regexp.MustCompile(`^[1-9A-HJ-NP-Za-km-z]{32,44}$`)
	queryEVMBalance    = wallet.QueryEVMBalance
	querySolanaBalance = wallet.QuerySolanaBalance
)

// UpdateWalletsRequest represents a request to update wallet addresses
type UpdateWalletsRequest struct {
	EVMAddress    *string `json:"evm_address,omitempty"`
	SolanaAddress *string `json:"solana_address,omitempty"`
}

// UpdateWalletsResponse represents the response from updating wallet addresses
type UpdateWalletsResponse struct {
	EVMWalletAddress    *string `json:"evm_wallet_address,omitempty"`
	SolanaWalletAddress *string `json:"solana_wallet_address,omitempty"`
}

// UpdateWallets updates wallet addresses for the authenticated account
// @Summary Update wallet addresses
// @Description Update EVM and/or Solana wallet addresses for the authenticated account
// @Tags account
// @Accept json
// @Produce json
// @Param request body UpdateWalletsRequest true "Wallet addresses to update (both optional)"
// @Success 200 {object} UpdateWalletsResponse
// @Failure 400 {object} map[string]string "Invalid request"
// @Failure 409 {object} map[string]string "Wallet address already linked to another account"
// @Failure 401 {object} map[string]string "Not authenticated"
// @Failure 500 {object} map[string]string "Server error"
// @Security CookieAuth
// @Router /v1/account/wallets [put]
func (h *AccountHandler) UpdateWallets(c fiber.Ctx) error {
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

	var req UpdateWalletsRequest
	if err := c.Bind().Body(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	// Validate EVM address if provided
	if req.EVMAddress != nil {
		evmAddr := strings.TrimSpace(*req.EVMAddress)
		if evmAddr == "" {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "EVM wallet address cannot be empty",
			})
		}
		if !evmAddressRegex.MatchString(evmAddr) {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "Invalid EVM wallet address format (expected 0x + 40 hex chars)",
			})
		}
		req.EVMAddress = &evmAddr
	}

	// Validate Solana address if provided
	if req.SolanaAddress != nil {
		solanaAddr := strings.TrimSpace(*req.SolanaAddress)
		if solanaAddr == "" {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "Solana wallet address cannot be empty",
			})
		}
		if !solanaAddressRegex.MatchString(solanaAddr) {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "Invalid Solana wallet address format (expected base58, 32-44 chars)",
			})
		}
		req.SolanaAddress = &solanaAddr
	}

	// Check that at least one address is provided
	if req.EVMAddress == nil && req.SolanaAddress == nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "At least one wallet address must be provided",
		})
	}

	ctx := c.Context()

	// Update wallet addresses
	if err := h.db.UpdateWalletAddresses(ctx, accountID, req.EVMAddress, req.SolanaAddress); err != nil {
		if errors.Is(err, db.ErrEVMWalletAddressConflict) {
			return c.Status(fiber.StatusConflict).JSON(fiber.Map{
				"error": "EVM wallet address is already linked to another account",
			})
		}
		if errors.Is(err, db.ErrSolanaWalletAddressConflict) {
			return c.Status(fiber.StatusConflict).JSON(fiber.Map{
				"error": "Solana wallet address is already linked to another account",
			})
		}
		if errors.Is(err, db.ErrInvalidEVMWalletAddress) {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "Invalid EVM wallet address",
			})
		}
		if errors.Is(err, db.ErrInvalidSolanaWalletAddress) {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "Invalid Solana wallet address",
			})
		}

		slog.Error("failed to update wallet addresses", "account_id", accountID, "error", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to update wallet addresses",
		})
	}

	// Fetch updated account to return current state
	account, err := h.db.GetAccountByID(ctx, accountID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to retrieve updated account",
		})
	}

	return c.JSON(UpdateWalletsResponse{
		EVMWalletAddress:    account.EVMWalletAddress,
		SolanaWalletAddress: account.SolanaWalletAddress,
	})
}

// WalletBalanceInfo represents balance information for a single wallet
type WalletBalanceInfo struct {
	Address     string         `json:"address"`
	BalanceUSDC usdc.MicroUSDC `json:"balance_usdc"`
	Network     string         `json:"network"`
	Error       string         `json:"error,omitempty"`
}

// GetBalancesResponse represents the response from GetBalances
type GetBalancesResponse struct {
	EVM       *WalletBalanceInfo `json:"evm,omitempty"`
	Solana    *WalletBalanceInfo `json:"solana,omitempty"`
	TotalUSDC usdc.MicroUSDC     `json:"total_usdc"`
}

// GetBalances returns on-chain USDC balances for the account's wallets
// @Summary Get wallet balances
// @Description Returns on-chain USDC balances for the account's EVM and Solana wallets
// @Tags account
// @Produce json
// @Success 200 {object} GetBalancesResponse
// @Failure 401 {object} map[string]string "Not authenticated"
// @Failure 404 {object} map[string]string "Account not found"
// @Security CookieAuth
// @Router /v1/account/balances [get]
func (h *AccountHandler) GetBalances(c fiber.Ctx) error {
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

	account, err := h.db.GetAccountByID(c.Context(), accountID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Account not found",
		})
	}

	resp := GetBalancesResponse{}
	var totalUSDC usdc.MicroUSDC

	// Use a single request-level timeout budget and query both chains in parallel.
	ctx, cancel := context.WithTimeout(c.Context(), 28*time.Second)
	defer cancel()

	var wg sync.WaitGroup
	var mu sync.Mutex

	// Query EVM balance if wallet is configured
	if account.EVMWalletAddress != nil && *account.EVMWalletAddress != "" {
		address := *account.EVMWalletAddress
		wg.Add(1)
		go func() {
			defer wg.Done()

			info := &WalletBalanceInfo{
				Address: address,
				Network: "base",
			}

			balance, err := queryEVMBalance(ctx, address, "base")
			if err != nil {
				slog.Error("failed to query EVM balance",
					"account_id", accountID,
					"address", address,
					"error", err,
				)
				info.Error = "Failed to query balance"
			} else {
				info.BalanceUSDC = usdc.FromFloat(balance)
			}

			mu.Lock()
			resp.EVM = info
			if err == nil {
				totalUSDC += info.BalanceUSDC
			}
			mu.Unlock()
		}()
	}

	// Query Solana balance if wallet is configured
	if account.SolanaWalletAddress != nil && *account.SolanaWalletAddress != "" {
		address := *account.SolanaWalletAddress
		wg.Add(1)
		go func() {
			defer wg.Done()

			info := &WalletBalanceInfo{
				Address: address,
				Network: "solana",
			}

			balance, err := querySolanaBalance(ctx, address, "solana")
			if err != nil {
				slog.Error("failed to query Solana balance",
					"account_id", accountID,
					"address", address,
					"error", err,
				)
				info.Error = "Failed to query balance"
			} else {
				info.BalanceUSDC = usdc.FromFloat(balance)
			}

			mu.Lock()
			resp.Solana = info
			if err == nil {
				totalUSDC += info.BalanceUSDC
			}
			mu.Unlock()
		}()
	}

	wg.Wait()

	resp.TotalUSDC = totalUSDC

	return c.JSON(resp)
}

// calculateFee calculates the fee for a deposit based on provider using integer arithmetic.
func calculateFee(amount usdc.MicroUSDC, provider db.DepositProvider) usdc.MicroUSDC {
	switch provider {
	case db.DepositProviderStripe:
		// Stripe typically charges 2.9% + $0.30, computed in microUSDC
		// amount * 29 / 1000 + 300_000 (where 300_000 = $0.30)
		return usdc.MicroUSDC(int64(amount)*29/1000 + 300_000)
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

// createStripeOnrampSession creates a Stripe Crypto Onramp session targeting the specified network
func (h *AccountHandler) createStripeOnrampSession(depositID, walletAddress, network string) (*stripeOnrampSession, error) {
	if h.stripeConfig == nil || h.stripeConfig.SecretKey == "" {
		return nil, fmt.Errorf("stripe not configured")
	}

	// Build form data for the API request.
	// Stripe Crypto Onramp uses "base" and "solana" as network identifiers
	// for wallet_addresses and destination_networks parameters.
	// See: https://docs.stripe.com/crypto/onramp
	data := url.Values{}
	data.Set("wallet_addresses["+network+"]", walletAddress)
	data.Set("destination_currencies[]", "usdc")
	data.Set("destination_networks[]", network)
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
