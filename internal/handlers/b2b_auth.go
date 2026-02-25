package handlers

import (
	"log/slog"
	"net"
	"regexp"
	"strings"
	"time"

	"stronghold/internal/auth"
	"stronghold/internal/config"
	"stronghold/internal/db"
	"stronghold/internal/usdc"

	"github.com/gofiber/fiber/v3"
	"github.com/stripe/stripe-go/v82"
	"github.com/stripe/stripe-go/v82/customer"
)

var emailRegex = regexp.MustCompile(`^[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}$`)

// dummyBcryptHash is a pre-computed bcrypt hash used on login failure paths
// to normalize response timing and prevent email enumeration. Running a real
// bcrypt compare (~200-300ms at cost 12) instead of a fixed sleep makes the
// unknown-email path indistinguishable from the wrong-password path.
var dummyBcryptHash = func() string {
	h, _ := auth.HashPassword("timing-equalization-dummy")
	return h
}()

// B2BAuthHandler handles B2B registration and login
type B2BAuthHandler struct {
	db           *db.DB
	authHandler  *AuthHandler
	stripeConfig *config.StripeConfig
}

// NewB2BAuthHandler creates a new B2B auth handler
func NewB2BAuthHandler(database *db.DB, authHandler *AuthHandler, stripeConfig *config.StripeConfig) *B2BAuthHandler {
	return &B2BAuthHandler{
		db:           database,
		authHandler:  authHandler,
		stripeConfig: stripeConfig,
	}
}

// RegisterRoutes registers B2B auth routes with rate limiting middleware
func (h *B2BAuthHandler) RegisterRoutes(app *fiber.App, middlewares ...fiber.Handler) {
	group := app.Group("/v1/auth/b2b")
	for _, mw := range middlewares {
		group.Use(mw)
	}
	group.Post("/register", h.Register)
	group.Post("/login", h.Login)
}

// B2BRegisterRequest represents a B2B registration request
type B2BRegisterRequest struct {
	Email       string `json:"email"`
	Password    string `json:"password"`
	CompanyName string `json:"company_name"`
}

// B2BRegisterResponse represents the B2B registration response
type B2BRegisterResponse struct {
	AccountID   string    `json:"account_id"`
	Email       string    `json:"email"`
	CompanyName string    `json:"company_name"`
	AccountType string    `json:"account_type"`
	ExpiresAt   time.Time `json:"expires_at"`
}

// Register creates a new B2B account with email/password
func (h *B2BAuthHandler) Register(c fiber.Ctx) error {
	var req B2BRegisterRequest
	if err := c.Bind().Body(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	// Validate email
	req.Email = strings.TrimSpace(strings.ToLower(req.Email))
	if !emailRegex.MatchString(req.Email) {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid email address",
		})
	}

	// Validate password (bcrypt silently truncates at 72 bytes; reject early)
	if len(req.Password) < 8 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Password must be at least 8 characters",
		})
	}
	if len(req.Password) > 72 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Password must be at most 72 bytes",
		})
	}

	// Validate company name
	req.CompanyName = strings.TrimSpace(req.CompanyName)
	if req.CompanyName == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Company name is required",
		})
	}

	// Stripe is required for B2B billing — both SecretKey (API calls) and
	// WebhookSecret (checkout.session.completed verification) must be set.
	// Without WebhookSecret, credit purchases via Checkout succeed on Stripe's
	// side but the webhook is rejected, so credits never post to the account.
	if h.stripeConfig.SecretKey == "" || h.stripeConfig.WebhookSecret == "" {
		slog.Error("B2B registration rejected: Stripe not fully configured",
			"has_secret_key", h.stripeConfig.SecretKey != "",
			"has_webhook_secret", h.stripeConfig.WebhookSecret != "")
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": "Business account registration is temporarily unavailable",
		})
	}

	// Hash password
	passwordHash, err := auth.HashPassword(req.Password)
	if err != nil {
		slog.Error("failed to hash password", "error", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to create account",
		})
	}

	ctx := c.Context()

	// Create B2B account
	account, err := h.db.CreateB2BAccount(ctx, req.Email, passwordHash, req.CompanyName)
	if err != nil {
		if err == db.ErrEmailAlreadyExists {
			return c.Status(fiber.StatusConflict).JSON(fiber.Map{
				"error": "Email already registered",
			})
		}
		slog.Error("failed to create B2B account", "error", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to create account",
		})
	}
	params := &stripe.CustomerParams{
		Email: stripe.String(req.Email),
		Name:  stripe.String(req.CompanyName),
	}
	params.AddMetadata("account_id", account.ID.String())
	params.AddMetadata("account_type", "b2b")

	cust, err := customer.New(params)
	if err != nil {
		slog.Error("failed to create Stripe customer, rolling back account",
			"account_id", account.ID, "error", err)
		if delErr := h.db.DeleteAccount(ctx, account.ID); delErr != nil {
			slog.Error("failed to delete orphaned account after Stripe failure",
				"account_id", account.ID, "error", delErr)
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to set up billing. Please try again.",
		})
	}
	if err := h.db.UpdateStripeCustomerID(ctx, account.ID, cust.ID); err != nil {
		slog.Error("failed to store Stripe customer ID, rolling back account",
			"account_id", account.ID, "error", err)
		if _, delErr := customer.Del(cust.ID, nil); delErr != nil {
			slog.Error("failed to delete Stripe customer during rollback",
				"account_id", account.ID, "stripe_customer_id", cust.ID, "error", delErr)
		}
		if delErr := h.db.DeleteAccount(ctx, account.ID); delErr != nil {
			slog.Error("failed to delete orphaned account after Stripe ID update failure",
				"account_id", account.ID, "error", delErr)
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to set up billing. Please try again.",
		})
	}
	account.StripeCustomerID = &cust.ID

	// Create session
	ip := c.IP()
	userAgent := string(c.Request().Header.UserAgent())
	_, refreshToken, err := h.db.CreateSession(ctx, account.ID, net.ParseIP(ip), userAgent, h.authHandler.config.RefreshTokenTTL)
	if err != nil {
		slog.Error("failed to create session, rolling back account",
			"account_id", account.ID, "error", err)
		if _, delErr := customer.Del(cust.ID, nil); delErr != nil {
			slog.Error("failed to delete Stripe customer during rollback",
				"account_id", account.ID, "stripe_customer_id", cust.ID, "error", delErr)
		}
		if delErr := h.db.DeleteAccount(ctx, account.ID); delErr != nil {
			slog.Error("failed to delete orphaned account after session creation failure",
				"account_id", account.ID, "error", delErr)
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to create account. Please try again.",
		})
	}

	// Generate access token
	accessToken, expiresAt, err := h.authHandler.generateAccessToken(account.ID.String(), "")
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to generate access token",
		})
	}

	// Set cookies
	refreshExpiry := time.Now().UTC().Add(h.authHandler.config.RefreshTokenTTL)
	h.authHandler.setAuthCookies(c, accessToken, refreshToken, expiresAt, refreshExpiry)

	slog.Info("B2B account created",
		"account_id", account.ID,
		"email", req.Email,
		"company", req.CompanyName,
	)

	return c.Status(fiber.StatusCreated).JSON(B2BRegisterResponse{
		AccountID:   account.ID.String(),
		Email:       req.Email,
		CompanyName: req.CompanyName,
		AccountType: db.AccountTypeB2B,
		ExpiresAt:   expiresAt,
	})
}

// B2BLoginRequest represents a B2B login request
type B2BLoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// B2BLoginResponse represents the B2B login response
type B2BLoginResponse struct {
	AccountID   string    `json:"account_id"`
	Email       string    `json:"email"`
	CompanyName string    `json:"company_name"`
	AccountType string    `json:"account_type"`
	BalanceUSDC usdc.MicroUSDC `json:"balance_usdc"`
	ExpiresAt   time.Time `json:"expires_at"`
}

// Login authenticates a B2B account by email/password
func (h *B2BAuthHandler) Login(c fiber.Ctx) error {
	var req B2BLoginRequest
	if err := c.Bind().Body(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	req.Email = strings.TrimSpace(strings.ToLower(req.Email))
	if req.Email == "" || req.Password == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Email and password are required",
		})
	}

	ctx := c.Context()

	// Lookup account by email
	account, err := h.db.GetAccountByEmail(ctx, req.Email)
	if err != nil {
		// Distinguish not-found (unknown email) from infrastructure errors (DB down).
		// Not-found → 401 with dummy bcrypt to equalize timing.
		// DB error → 500 so clients can retry and monitors can alert.
		if strings.Contains(err.Error(), "not found") {
			auth.CheckPassword(dummyBcryptHash, req.Password)
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": "Invalid email or password",
			})
		}
		slog.Error("failed to look up account by email", "error", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Internal server error",
		})
	}

	// Verify password
	if account.PasswordHash == nil {
		auth.CheckPassword(dummyBcryptHash, req.Password)
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "Invalid email or password",
		})
	}
	if err := auth.CheckPassword(*account.PasswordHash, req.Password); err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "Invalid email or password",
		})
	}

	// Check account status
	if account.Status != db.AccountStatusActive {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
			"error": "Account is not active",
		})
	}

	// Update last login
	if err := h.db.UpdateLastLogin(ctx, account.ID); err != nil {
		slog.Error("failed to update last login", "account_id", account.ID, "error", err)
	}

	// Create session
	ip := c.IP()
	userAgent := string(c.Request().Header.UserAgent())
	_, refreshToken, err := h.db.CreateSession(ctx, account.ID, net.ParseIP(ip), userAgent, h.authHandler.config.RefreshTokenTTL)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to create session",
		})
	}

	// Generate access token
	accessToken, expiresAt, err := h.authHandler.generateAccessToken(account.ID.String(), "")
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to generate access token",
		})
	}

	// Set cookies
	refreshExpiry := time.Now().UTC().Add(h.authHandler.config.RefreshTokenTTL)
	h.authHandler.setAuthCookies(c, accessToken, refreshToken, expiresAt, refreshExpiry)

	slog.Info("B2B login successful",
		"account_id", account.ID,
		"email", req.Email,
		"ip", c.IP(),
	)

	companyName := ""
	if account.CompanyName != nil {
		companyName = *account.CompanyName
	}

	return c.JSON(B2BLoginResponse{
		AccountID:   account.ID.String(),
		Email:       req.Email,
		CompanyName: companyName,
		AccountType: db.AccountTypeB2B,
		BalanceUSDC: account.BalanceUSDC,
		ExpiresAt:   expiresAt,
	})
}
