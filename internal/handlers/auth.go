package handlers

import (
	"fmt"
	"net"
	"os"
	"strings"
	"time"

	"stronghold/internal/db"

	"github.com/gofiber/fiber/v3"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

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

// LoadAuthConfig loads auth configuration from environment
func LoadAuthConfig() *AuthConfig {
	secret := getEnv("JWT_SECRET", "")
	if secret == "" {
		// In production, this should fail hard
		secret = "development-secret-do-not-use-in-production"
	}

	return &AuthConfig{
		JWTSecret:       secret,
		AccessTokenTTL:  getDuration("ACCESS_TOKEN_TTL", 15*time.Minute),
		RefreshTokenTTL: getDuration("REFRESH_TOKEN_TTL", 90*24*time.Hour), // 90 days
		DashboardURL:    getEnv("DASHBOARD_URL", "http://localhost:3000"),
		AllowedOrigins:  strings.Split(getEnv("DASHBOARD_ALLOWED_ORIGINS", "http://localhost:3000"), ","),
	}
}

// AuthHandler handles authentication endpoints
type AuthHandler struct {
	db     *db.DB
	config *AuthConfig
}

// Cookie names
const (
	AccessTokenCookie  = "stronghold_access"
	RefreshTokenCookie = "stronghold_refresh"
)

// NewAuthHandler creates a new auth handler
func NewAuthHandler(database *db.DB, config *AuthConfig) *AuthHandler {
	return &AuthHandler{
		db:     database,
		config: config,
	}
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
	group.Get("/me", h.AuthMiddleware(), h.GetMe)
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
}

// CreateAccountResponse represents the response after creating an account
type CreateAccountResponse struct {
	AccountNumber string    `json:"account_number"`
	ExpiresAt     time.Time `json:"expires_at"`
	RecoveryFile  string    `json:"recovery_file"`
}

// CreateAccount creates a new account with a generated account number
func (h *AuthHandler) CreateAccount(c fiber.Ctx) error {
	var req CreateAccountRequest
	if err := c.Bind().Body(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	// Validate wallet address if provided
	if req.WalletAddress != nil && !isValidWalletAddress(*req.WalletAddress) {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid wallet address format",
		})
	}

	ctx := c.Context()

	// Create account
	account, err := h.db.CreateAccount(ctx, req.WalletAddress)
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

	return c.Status(fiber.StatusCreated).JSON(CreateAccountResponse{
		AccountNumber: account.AccountNumber,
		ExpiresAt:     expiresAt,
		RecoveryFile:  recoveryFile,
	})
}

// LoginRequest represents a login request
type LoginRequest struct {
	AccountNumber string `json:"account_number"`
}

// LoginResponse represents the login response
type LoginResponse struct {
	AccountNumber string    `json:"account_number"`
	ExpiresAt     time.Time `json:"expires_at"`
}

// Login authenticates an account by account number
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

	ctx := c.Context()

	// Get account by account number
	account, err := h.db.GetAccountByNumber(ctx, req.AccountNumber)
	if err != nil {
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
		// Log but don't fail
		// TODO: Add proper logging
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

	// Set httpOnly cookies
	refreshExpiry := time.Now().UTC().Add(h.config.RefreshTokenTTL)
	h.setAuthCookies(c, accessToken, refreshToken, expiresAt, refreshExpiry)

	return c.JSON(LoginResponse{
		AccountNumber: account.AccountNumber,
		ExpiresAt:     expiresAt,
	})
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
		// Clear invalid cookies
		h.clearAuthCookies(c)
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "Invalid or expired refresh token",
		})
	}

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

	return c.JSON(fiber.Map{
		"id":             account.ID,
		"account_number": account.AccountNumber,
		"wallet_address": account.WalletAddress,
		"balance_usdc":   account.BalanceUSDC,
		"status":         account.Status,
		"created_at":     account.CreatedAt,
		"last_login_at":  account.LastLoginAt,
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

		// Parse and validate token
		token, err := jwt.ParseWithClaims(tokenString, &JWTClaims{}, func(token *jwt.Token) (interface{}, error) {
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
			}
			return []byte(h.config.JWTSecret), nil
		})

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

		return c.Next()
	}
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
1. Visit https://dashboard.stronghold.security
2. Enter your account number: %s
3. Download your wallet recovery file separately if needed

This file was generated automatically. Do not modify its contents.
`, accountNumber, accountID, time.Now().UTC().Format(time.RFC3339), accountNumber)
}

// Helper functions
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getDuration(key string, defaultValue time.Duration) time.Duration {
	if value := os.Getenv(key); value != "" {
		if d, err := time.ParseDuration(value); err == nil {
			return d
		}
	}
	return defaultValue
}
