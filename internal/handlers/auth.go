package handlers

import (
	"encoding/hex"
	"fmt"
	"log/slog"
	"net"
	"regexp"
	"strings"
	"time"

	"stronghold/internal/db"
	"stronghold/internal/kms"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/gofiber/fiber/v3"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

// accountNumberRegex validates account numbers in either format:
// - 16 consecutive digits: 1234567890123456
// - With dashes: 1234-5678-9012-3456
var accountNumberRegex = regexp.MustCompile(`^(\d{16}|\d{4}-\d{4}-\d{4}-\d{4})$`)

// CookieConfig holds httpOnly cookie configuration
type CookieConfig struct {
	Domain   string
	Secure   bool
	SameSite string
}

// AuthConfig holds authentication configuration
type AuthConfig struct {
	JWTSecret       string
	AccessTokenTTL  time.Duration
	RefreshTokenTTL time.Duration
	DashboardURL    string
	AllowedOrigins  []string
	Cookie          CookieConfig
}

// AuthHandler handles authentication endpoints
type AuthHandler struct {
	db        *db.DB
	config    *AuthConfig
	kmsClient *kms.Client
}

// Cookie names
const (
	AccessTokenCookie  = "stronghold_access"
	RefreshTokenCookie = "stronghold_refresh"
)

// NewAuthHandler creates a new auth handler
func NewAuthHandler(database *db.DB, config *AuthConfig, kmsClient *kms.Client) *AuthHandler {
	return &AuthHandler{
		db:        database,
		config:    config,
		kmsClient: kmsClient,
	}
}

// Config returns the auth configuration (used by other handlers that need the same config)
func (h *AuthHandler) Config() *AuthConfig {
	return h.config
}

// setAuthCookies sets httpOnly cookies for access and refresh tokens
func (h *AuthHandler) setAuthCookies(c fiber.Ctx, accessToken, refreshToken string, accessExpiry, refreshExpiry time.Time) {
	sameSite := h.parseSameSite()

	// Set access token cookie
	c.Cookie(&fiber.Cookie{
		Name:     AccessTokenCookie,
		Value:    accessToken,
		Expires:  accessExpiry,
		HTTPOnly: true,
		Secure:   h.config.Cookie.Secure,
		SameSite: sameSite,
		Path:     "/",
		Domain:   h.config.Cookie.Domain,
	})

	// Set refresh token cookie
	c.Cookie(&fiber.Cookie{
		Name:     RefreshTokenCookie,
		Value:    refreshToken,
		Expires:  refreshExpiry,
		HTTPOnly: true,
		Secure:   h.config.Cookie.Secure,
		SameSite: sameSite,
		Path:     "/v1/auth", // Restrict refresh token to auth endpoints
		Domain:   h.config.Cookie.Domain,
	})
}

// clearAuthCookies clears the auth cookies
func (h *AuthHandler) clearAuthCookies(c fiber.Ctx) {
	sameSite := h.parseSameSite()

	c.Cookie(&fiber.Cookie{
		Name:     AccessTokenCookie,
		Value:    "",
		Expires:  time.Now().Add(-1 * time.Hour),
		HTTPOnly: true,
		Secure:   h.config.Cookie.Secure,
		SameSite: sameSite,
		Path:     "/",
		Domain:   h.config.Cookie.Domain,
	})

	c.Cookie(&fiber.Cookie{
		Name:     RefreshTokenCookie,
		Value:    "",
		Expires:  time.Now().Add(-1 * time.Hour),
		HTTPOnly: true,
		Secure:   h.config.Cookie.Secure,
		SameSite: sameSite,
		Path:     "/v1/auth",
		Domain:   h.config.Cookie.Domain,
	})
}

// parseSameSite converts string config to fiber SameSite constant
func (h *AuthHandler) parseSameSite() string {
	switch strings.ToLower(h.config.Cookie.SameSite) {
	case "lax":
		return "Lax"
	case "none":
		return "None"
	default:
		return "Strict"
	}
}

// RegisterRoutes registers auth routes
func (h *AuthHandler) RegisterRoutes(app *fiber.App) {
	h.RegisterRoutesWithMiddleware(app)
}

// RegisterRoutesWithMiddleware registers auth routes with optional middleware
func (h *AuthHandler) RegisterRoutesWithMiddleware(app *fiber.App, middlewares ...fiber.Handler) {
	group := app.Group("/v1/auth")

	// Apply any provided middleware to the group
	for _, mw := range middlewares {
		group.Use(mw)
	}

	group.Post("/account", h.CreateAccount)
	group.Post("/login", h.Login)
	group.Post("/refresh", h.RefreshToken)
	group.Post("/logout", h.AuthMiddleware(), h.Logout)
	group.Get("/me", h.AuthMiddleware(), h.RequireTrustedDevice(), h.GetMe)
	group.Get("/wallet-key", h.AuthMiddleware(), h.RequireTrustedDevice(), h.GetWalletKey)
	group.Put("/wallet", h.AuthMiddleware(), h.UpdateWallet)
	group.Post("/totp/setup", h.AuthMiddleware(), h.SetupTOTP)
	group.Post("/totp/verify", h.AuthMiddleware(), h.VerifyTOTP)
	group.Get("/devices", h.AuthMiddleware(), h.RequireTrustedDevice(), h.ListDevices)
	group.Post("/devices/revoke", h.AuthMiddleware(), h.RequireTrustedDevice(), h.RevokeDevice)
}

// JWTClaims represents JWT claims
type JWTClaims struct {
	AccountID     string `json:"account_id"`
	AccountNumber string `json:"account_number"`
	TokenType     string `json:"token_type"`
	jwt.RegisteredClaims
}

// CreateAccountRequest represents a request to create a new account
type CreateAccountRequest struct {
	WalletAddress *string `json:"wallet_address,omitempty"`
	PrivateKey    *string `json:"private_key,omitempty"` // hex-encoded, used to import existing wallet
}

// CreateAccountResponse represents the response after creating an account
type CreateAccountResponse struct {
	AccountNumber    string    `json:"account_number"`
	WalletAddress    string    `json:"wallet_address,omitempty"`
	EVMWalletAddress string    `json:"evm_wallet_address,omitempty"`
	ExpiresAt        time.Time `json:"expires_at"`
	RecoveryFile     string    `json:"recovery_file"`
}

// CreateAccount creates a new account with a generated account number
// @Summary Create a new account
// @Description Creates a new account with a generated account number and server-side wallet. Optionally accepts a private key to import an existing wallet.
// @Tags auth
// @Accept json
// @Produce json
// @Param request body CreateAccountRequest false "Optional wallet address or private key"
// @Success 201 {object} CreateAccountResponse
// @Failure 400 {object} map[string]string "Invalid request"
// @Failure 500 {object} map[string]string "Server error"
// @Router /v1/auth/account [post]
func (h *AuthHandler) CreateAccount(c fiber.Ctx) error {
	var req CreateAccountRequest
	if err := c.Bind().Body(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	ctx := c.Context()

	if req.PrivateKey != nil && *req.PrivateKey != "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Wallet uploads require TOTP setup. Create the account first, then upload the wallet.",
		})
	}

	// Validate wallet address if provided (only EVM addresses supported via this endpoint)
	if req.WalletAddress != nil && !isValidWalletAddress(*req.WalletAddress) {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid wallet address format",
		})
	}

	// Create account - wallet address provided here is always EVM
	account, err := h.db.CreateAccount(ctx, req.WalletAddress, nil)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to create account",
		})
	}

	// Create session
	ip := c.IP()
	userAgent := string(c.Request().Header.UserAgent())
	_, refreshToken, err := h.db.CreateSession(ctx, account.ID, net.ParseIP(ip), userAgent, h.config.RefreshTokenTTL)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to create session",
		})
	}

	// Generate access token
	accessToken, expiresAt, err := h.generateAccessToken(account.ID.String(), account.AccountNumber)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to generate access token",
		})
	}

	// Generate recovery file content
	recoveryFile := generateRecoveryFile(account.AccountNumber, account.ID.String())

	// Set httpOnly cookies
	refreshExpiry := time.Now().UTC().Add(h.config.RefreshTokenTTL)
	h.setAuthCookies(c, accessToken, refreshToken, expiresAt, refreshExpiry)

	response := CreateAccountResponse{
		AccountNumber: account.AccountNumber,
		ExpiresAt:     expiresAt,
		RecoveryFile:  recoveryFile,
	}
	if req.WalletAddress != nil {
		response.WalletAddress = *req.WalletAddress
		response.EVMWalletAddress = *req.WalletAddress
	}
	return c.Status(fiber.StatusCreated).JSON(response)
}

// LoginRequest represents a login request
type LoginRequest struct {
	AccountNumber string `json:"account_number"`
}

// LoginResponse represents the login response
type LoginResponse struct {
	AccountNumber       string    `json:"account_number"`
	ExpiresAt           time.Time `json:"expires_at"`
	WalletAddress       *string   `json:"wallet_address,omitempty"`
	EVMWalletAddress    *string   `json:"evm_wallet_address,omitempty"`
	SolanaWalletAddress *string   `json:"solana_wallet_address,omitempty"`
	TOTPRequired        bool      `json:"totp_required"`
	DeviceTrusted       bool      `json:"device_trusted"`
	EscrowEnabled       bool      `json:"wallet_escrow_enabled"`
}

// Login authenticates an account by account number
// @Summary Login to an account
// @Description Authenticates using account number and sets httpOnly auth cookies. Returns decrypted wallet key if KMS-encrypted key exists.
// @Tags auth
// @Accept json
// @Produce json
// @Param request body LoginRequest true "Account number"
// @Success 200 {object} LoginResponse
// @Failure 400 {object} map[string]string "Invalid request"
// @Failure 401 {object} map[string]string "Invalid credentials"
// @Failure 403 {object} map[string]string "Account not active"
// @Router /v1/auth/login [post]
func (h *AuthHandler) Login(c fiber.Ctx) error {
	var req LoginRequest
	if err := c.Bind().Body(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	if req.AccountNumber == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Account number is required",
		})
	}

	// Validate account number format (16 digits)
	if !accountNumberRegex.MatchString(req.AccountNumber) {
		slog.Warn("login attempt with invalid account number format",
			"ip", c.IP(),
			"user_agent", string(c.Request().Header.UserAgent()),
		)
		// Use constant time to prevent timing attacks
		time.Sleep(100 * time.Millisecond)
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "Invalid account number",
		})
	}

	ctx := c.Context()

	// Get account by account number
	account, err := h.db.GetAccountByNumber(ctx, req.AccountNumber)
	if err != nil {
		slog.Warn("login failed: account not found",
			"ip", c.IP(),
			"user_agent", string(c.Request().Header.UserAgent()),
		)
		// Use constant time comparison to prevent timing attacks
		time.Sleep(100 * time.Millisecond)
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "Invalid account number",
		})
	}

	// Check if account is active
	if account.Status != db.AccountStatusActive {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
			"error": "Account is not active",
		})
	}

	// Update last login
	if err := h.db.UpdateLastLogin(ctx, account.ID); err != nil {
		slog.Error("failed to update last login", "account_id", account.ID, "error", err)
	}

	// Log successful login
	slog.Info("login successful",
		"account_id", account.ID,
		"ip", c.IP(),
		"user_agent", string(c.Request().Header.UserAgent()),
	)

	// Create session
	ip := c.IP()
	userAgent := string(c.Request().Header.UserAgent())
	_, refreshToken, err := h.db.CreateSession(ctx, account.ID, net.ParseIP(ip), userAgent, h.config.RefreshTokenTTL)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to create session",
		})
	}

	// Generate access token
	accessToken, expiresAt, err := h.generateAccessToken(account.ID.String(), account.AccountNumber)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to generate access token",
		})
	}

	// Set httpOnly cookies
	refreshExpiry := time.Now().UTC().Add(h.config.RefreshTokenTTL)
	h.setAuthCookies(c, accessToken, refreshToken, expiresAt, refreshExpiry)

	// Build response
	deviceToken := getDeviceToken(c)
	deviceTrusted := false
	totpRequired := false
	if account.WalletEscrow {
		if deviceToken != "" {
			if _, err := h.db.GetDeviceByToken(ctx, account.ID, deviceToken); err == nil {
				deviceTrusted = true
			}
		}
		totpRequired = !deviceTrusted
	}

	response := LoginResponse{
		AccountNumber:       account.AccountNumber,
		ExpiresAt:           expiresAt,
		WalletAddress:       account.EVMWalletAddress,
		EVMWalletAddress:    account.EVMWalletAddress,
		SolanaWalletAddress: account.SolanaWalletAddress,
		TOTPRequired:        totpRequired,
		DeviceTrusted:       deviceTrusted,
		EscrowEnabled:       account.WalletEscrow,
	}

	return c.JSON(response)
}

// RefreshTokenRequest represents a token refresh request
type RefreshTokenRequest struct {
	RefreshToken string `json:"refresh_token"`
}

// RefreshTokenResponse represents the token refresh response
type RefreshTokenResponse struct {
	ExpiresAt time.Time `json:"expires_at"`
}

// RefreshToken refreshes an access token using a refresh token
// @Summary Refresh access token
// @Description Uses the refresh token cookie to get a new access token
// @Tags auth
// @Produce json
// @Success 200 {object} RefreshTokenResponse
// @Failure 401 {object} map[string]string "Invalid or expired refresh token"
// @Failure 403 {object} map[string]string "Account not active"
// @Router /v1/auth/refresh [post]
func (h *AuthHandler) RefreshToken(c fiber.Ctx) error {
	// Read refresh token from httpOnly cookie
	refreshToken := c.Cookies(RefreshTokenCookie)
	if refreshToken == "" {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "Refresh token is required",
		})
	}

	ctx := c.Context()

	// Rotate refresh token
	session, newRefreshToken, err := h.db.RotateRefreshToken(ctx, refreshToken, h.config.RefreshTokenTTL)
	if err != nil {
		slog.Warn("token refresh failed: invalid or expired refresh token",
			"ip", c.IP(),
			"user_agent", string(c.Request().Header.UserAgent()),
		)
		// Clear invalid cookies
		h.clearAuthCookies(c)
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "Invalid or expired refresh token",
		})
	}

	slog.Info("token refresh successful",
		"account_id", session.AccountID,
		"ip", c.IP(),
	)

	// Get account
	account, err := h.db.GetAccountByID(ctx, session.AccountID)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "Account not found",
		})
	}

	// Check if account is active
	if account.Status != db.AccountStatusActive {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
			"error": "Account is not active",
		})
	}

	// Generate new access token
	accessToken, expiresAt, err := h.generateAccessToken(account.ID.String(), account.AccountNumber)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to generate access token",
		})
	}

	// Set new httpOnly cookies
	refreshExpiry := time.Now().UTC().Add(h.config.RefreshTokenTTL)
	h.setAuthCookies(c, accessToken, newRefreshToken, expiresAt, refreshExpiry)

	return c.JSON(RefreshTokenResponse{
		ExpiresAt: expiresAt,
	})
}

// Logout logs out the current session
// @Summary Logout
// @Description Logs out from all sessions and clears auth cookies
// @Tags auth
// @Produce json
// @Success 200 {object} map[string]string "Logged out successfully"
// @Failure 401 {object} map[string]string "Not authenticated"
// @Security CookieAuth
// @Router /v1/auth/logout [post]
func (h *AuthHandler) Logout(c fiber.Ctx) error {
	// Get account ID from context (set by AuthMiddleware)
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

	// Delete all sessions for this account (logout from all devices)
	// Alternatively, we could only delete the current session using the refresh token
	if err := h.db.DeleteAllAccountSessions(ctx, accountID); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to logout",
		})
	}

	// Clear auth cookies
	h.clearAuthCookies(c)

	return c.JSON(fiber.Map{
		"message": "Logged out successfully",
	})
}

// GetMe returns the current account information
// @Summary Get current user
// @Description Returns the authenticated user's account information
// @Tags auth
// @Produce json
// @Success 200 {object} map[string]interface{} "Account info with id, account_number, evm_wallet_address, solana_wallet_address, balance_usdc, status"
// @Failure 401 {object} map[string]string "Not authenticated"
// @Security CookieAuth
// @Router /v1/auth/me [get]
func (h *AuthHandler) GetMe(c fiber.Ctx) error {
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

	resp := fiber.Map{
		"id":                    account.ID,
		"account_number":        account.AccountNumber,
		"account_type":          account.AccountType,
		"evm_wallet_address":    account.EVMWalletAddress,
		"solana_wallet_address": account.SolanaWalletAddress,
		"balance_usdc":          account.BalanceUSDC,
		"status":                account.Status,
		"wallet_escrow_enabled": account.WalletEscrow,
		"totp_enabled":          account.TOTPEnabled,
		"created_at":            account.CreatedAt,
		"last_login_at":         account.LastLoginAt,
	}
	if account.EVMWalletAddress != nil {
		resp["wallet_address"] = account.EVMWalletAddress
	}
	if account.AccountType == db.AccountTypeB2B {
		resp["email"] = account.Email
		resp["company_name"] = account.CompanyName
	}

	return c.JSON(resp)
}

// GetWalletKeyResponse represents the response from the wallet key endpoint
type GetWalletKeyResponse struct {
	PrivateKey string `json:"private_key"`
}

// GetWalletKey returns the decrypted wallet private key for the authenticated account.
// This is a sensitive endpoint that should be called over TLS only.
// @Summary Get wallet private key
// @Description Returns the KMS-decrypted wallet private key for the authenticated account
// @Tags auth
// @Produce json
// @Success 200 {object} GetWalletKeyResponse
// @Failure 401 {object} map[string]string "Not authenticated"
// @Failure 404 {object} map[string]string "No encrypted key found"
// @Failure 500 {object} map[string]string "Server error"
// @Security CookieAuth
// @Router /v1/auth/wallet-key [get]
func (h *AuthHandler) GetWalletKey(c fiber.Ctx) error {
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

	if h.kmsClient == nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "KMS not configured",
		})
	}

	ctx := c.Context()

	account, err := h.db.GetAccountByID(ctx, accountID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Account not found",
		})
	}
	if !account.WalletEscrow {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "No wallet stored for this account",
		})
	}

	// Require a fresh session (logged in within the last 5 minutes) to retrieve wallet key.
	// This protects against stolen session cookies while allowing multi-device use.
	tokenIssuedAt := c.Locals("token_issued_at")
	if tokenIssuedAt == nil {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
			"error": "Session information unavailable. Please log in again.",
		})
	}
	issuedAt, ok := tokenIssuedAt.(time.Time)
	if !ok || time.Since(issuedAt) > 5*time.Minute {
		slog.Warn("wallet key retrieval with stale session",
			"account_id", accountID,
			"ip", c.IP(),
		)
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
			"error": "Session too old. Please log in again to retrieve your wallet key.",
		})
	}

	hasKey, err := h.db.HasEncryptedKey(ctx, accountID)
	if err != nil {
		slog.Error("failed to check encrypted key", "account_id", accountID, "error", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to check wallet key",
		})
	}

	if !hasKey {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "No encrypted key found for this account",
		})
	}

	encryptedKey, err := h.db.GetEncryptedKey(ctx, accountID)
	if err != nil {
		slog.Error("failed to get encrypted key", "account_id", accountID, "error", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to retrieve wallet key",
		})
	}

	privateKeyHex, err := h.kmsClient.Decrypt(ctx, encryptedKey)
	if err != nil {
		slog.Error("failed to decrypt wallet key", "account_id", accountID, "error", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to decrypt wallet key",
		})
	}

	slog.Info("wallet key decrypted and retrieved",
		"account_id", accountID,
		"ip", c.IP(),
	)

	return c.JSON(GetWalletKeyResponse{
		PrivateKey: privateKeyHex,
	})
}

// generateAccessToken generates a new JWT access token
func (h *AuthHandler) generateAccessToken(accountID, accountNumber string) (string, time.Time, error) {
	expiresAt := time.Now().UTC().Add(h.config.AccessTokenTTL)

	claims := JWTClaims{
		AccountID:     accountID,
		AccountNumber: accountNumber,
		TokenType:     "access",
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expiresAt),
			IssuedAt:  jwt.NewNumericDate(time.Now().UTC()),
			NotBefore: jwt.NewNumericDate(time.Now().UTC()),
			Subject:   accountID,
			Issuer:    "stronghold-api",
			Audience:  jwt.ClaimStrings{"stronghold-api"},
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString([]byte(h.config.JWTSecret))
	if err != nil {
		return "", time.Time{}, err
	}

	return tokenString, expiresAt, nil
}

// AuthMiddleware returns a middleware that validates JWT tokens
func (h *AuthHandler) AuthMiddleware() fiber.Handler {
	return func(c fiber.Ctx) error {
		// If account_id is already set (e.g., by WorkOS B2B middleware), skip
		if c.Locals("account_id") != nil {
			return c.Next()
		}

		var tokenString string

		// First, try to get token from httpOnly cookie
		tokenString = c.Cookies(AccessTokenCookie)

		// Fall back to Authorization header for API clients
		if tokenString == "" {
			authHeader := string(c.Request().Header.Peek("Authorization"))
			if authHeader != "" {
				parts := strings.SplitN(authHeader, " ", 2)
				if len(parts) == 2 && strings.EqualFold(parts[0], "Bearer") {
					tokenString = parts[1]
				}
			}
		}

		if tokenString == "" {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": "Authentication required",
			})
		}

		// Parse and validate token with issuer and audience checks
		token, err := jwt.ParseWithClaims(tokenString, &JWTClaims{}, func(token *jwt.Token) (interface{}, error) {
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
			}
			return []byte(h.config.JWTSecret), nil
		},
			jwt.WithIssuer("stronghold-api"),
			jwt.WithAudience("stronghold-api"),
		)

		if err != nil {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": "Invalid token",
			})
		}

		claims, ok := token.Claims.(*JWTClaims)
		if !ok || !token.Valid {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": "Invalid token claims",
			})
		}

		// Verify token type
		if claims.TokenType != "access" {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": "Invalid token type",
			})
		}

		// Store account info in context
		c.Locals("account_id", claims.AccountID)
		c.Locals("account_number", claims.AccountNumber)
		if claims.IssuedAt != nil {
			c.Locals("token_issued_at", claims.IssuedAt.Time)
		}

		return c.Next()
	}
}

// UpdateWalletRequest represents a request to update wallet
type UpdateWalletRequest struct {
	PrivateKey string `json:"private_key"` // hex-encoded
}

// UpdateWalletResponse represents the response from updating wallet
type UpdateWalletResponse struct {
	WalletAddress    string `json:"wallet_address"`
	EVMWalletAddress string `json:"evm_wallet_address"`
}

// UpdateWallet updates the wallet for the authenticated account
// @Summary Update wallet
// @Description Replace the wallet for the authenticated account with a new private key
// @Tags auth
// @Accept json
// @Produce json
// @Param request body UpdateWalletRequest true "New private key (hex-encoded)"
// @Success 200 {object} UpdateWalletResponse
// @Failure 400 {object} map[string]string "Invalid request"
// @Failure 401 {object} map[string]string "Not authenticated"
// @Failure 500 {object} map[string]string "Server error"
// @Security CookieAuth
// @Router /v1/auth/wallet [put]
func (h *AuthHandler) UpdateWallet(c fiber.Ctx) error {
	var req UpdateWalletRequest
	if err := c.Bind().Body(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	if req.PrivateKey == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Private key is required",
		})
	}

	// Get account ID from context (set by AuthMiddleware)
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

	if h.kmsClient == nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "KMS not configured",
		})
	}
	if !account.TOTPEnabled {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
			"error": "TOTP is required before uploading a wallet",
		})
	}
	deviceToken := getDeviceToken(c)
	if deviceToken == "" {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
			"error":         "TOTP required",
			"totp_required": true,
		})
	}
	if _, err := h.db.GetDeviceByToken(ctx, accountID, deviceToken); err != nil {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
			"error":         "TOTP required",
			"totp_required": true,
		})
	}
	_ = h.db.TouchDevice(ctx, accountID, deviceToken)

	// Validate and parse the private key
	privateKey, err := crypto.HexToECDSA(strings.TrimPrefix(req.PrivateKey, "0x"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid private key format",
		})
	}

	// Get wallet address from public key
	address := crypto.PubkeyToAddress(privateKey.PublicKey).Hex()

	// If KMS is configured, encrypt and store the key
	if h.kmsClient != nil {
		privateKeyHex := hex.EncodeToString(crypto.FromECDSA(privateKey))

		// Encrypt via KMS
		encryptedKey, err := h.kmsClient.Encrypt(ctx, privateKeyHex)
		if err != nil {
			slog.Error("failed to encrypt wallet key", "error", err)
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "Failed to secure wallet",
			})
		}

		// Store encrypted key in DB
		if err := h.db.StoreEncryptedKey(ctx, accountID, encryptedKey, h.kmsClient.KeyID()); err != nil {
			slog.Error("failed to store encrypted key", "error", err, "account_id", accountID)
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "Failed to store wallet key",
			})
		}

		if err := h.db.SetWalletEscrowEnabled(ctx, accountID, true); err != nil {
			slog.Error("failed to update wallet escrow status", "error", err, "account_id", accountID)
		}
	}

	// Zero out the private key from memory
	privateKey.D.SetUint64(0)

	// Update wallet address in DB
	if err := h.db.UpdateWalletAddress(ctx, accountID, address); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to update wallet address",
		})
	}

	slog.Info("wallet updated",
		"account_id", accountID,
		"wallet_address", address,
	)

	return c.JSON(UpdateWalletResponse{
		WalletAddress:    address,
		EVMWalletAddress: address,
	})
}

// isValidWalletAddress checks if a wallet address is valid
func isValidWalletAddress(address string) bool {
	if len(address) != 42 {
		return false
	}
	if !strings.HasPrefix(address, "0x") {
		return false
	}
	for _, c := range address[2:] {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
}

// generateRecoveryFile generates a recovery file content for the account
func generateRecoveryFile(accountNumber, accountID string) string {
	return fmt.Sprintf(`STRONGHOLD ACCOUNT RECOVERY FILE
================================

Account Number: %s
Account ID: %s
Generated: %s

IMPORTANT: Store this file securely. Anyone with access to your account number
can access your account. Treat it like a password.

To recover your account:
1. Visit https://getstronghold.xyz/dashboard
2. Enter your account number: %s
3. Download your wallet recovery file separately if needed

This file was generated automatically. Do not modify its contents.
`, accountNumber, accountID, time.Now().UTC().Format(time.RFC3339), accountNumber)
}
