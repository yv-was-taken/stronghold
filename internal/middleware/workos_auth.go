package middleware

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"stronghold/internal/config"
	"stronghold/internal/db"

	"github.com/MicahParks/keyfunc/v3"
	"github.com/gofiber/fiber/v3"
	"github.com/golang-jwt/jwt/v5"
	"github.com/stripe/stripe-go/v82"
	"github.com/stripe/stripe-go/v82/customer"
)

// WorkOSAuthMiddleware validates WorkOS AuthKit JWTs and provisions B2B accounts.
type WorkOSAuthMiddleware struct {
	db           *db.DB
	workosConfig *config.WorkOSConfig
	stripeConfig *config.StripeConfig

	httpClient *http.Client

	mu   sync.Mutex
	jwks keyfunc.Keyfunc
}

// NewWorkOSAuthMiddleware creates a new WorkOS auth middleware.
func NewWorkOSAuthMiddleware(database *db.DB, workosConfig *config.WorkOSConfig, stripeConfig *config.StripeConfig) *WorkOSAuthMiddleware {
	return &WorkOSAuthMiddleware{
		db:           database,
		workosConfig: workosConfig,
		stripeConfig: stripeConfig,
		httpClient:   &http.Client{Timeout: 10 * time.Second},
	}
}

// initJWKS lazily initializes the JWKS keyfunc on first use.
func (m *WorkOSAuthMiddleware) initJWKS(ctx context.Context) (keyfunc.Keyfunc, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.jwks != nil {
		return m.jwks, nil
	}

	jwksURL := fmt.Sprintf("https://api.workos.com/sso/jwks/%s", m.workosConfig.ClientID)
	k, err := keyfunc.NewDefault([]string{jwksURL})
	if err != nil {
		return nil, fmt.Errorf("failed to create JWKS keyfunc: %w", err)
	}

	m.jwks = k
	return k, nil
}

// workOSClaims represents the JWT claims from a WorkOS access token.
type workOSClaims struct {
	jwt.RegisteredClaims
}

// Handler returns a Fiber middleware handler that validates WorkOS JWTs.
//
// If the request uses an API key (sk_live_* prefix) or a cookie-based token,
// this middleware skips and lets the next handler deal with it.
// Only Bearer tokens that don't match API key format are treated as WorkOS JWTs.
func (m *WorkOSAuthMiddleware) Handler() fiber.Handler {
	return func(c fiber.Ctx) error {
		// Skip if there's a cookie-based token (B2C flow handled by AuthMiddleware)
		if c.Cookies("stronghold_access") != "" {
			return c.Next()
		}

		authHeader := string(c.Request().Header.Peek("Authorization"))
		if authHeader == "" {
			return c.Next() // No auth header — let downstream middleware handle
		}

		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
			return c.Next()
		}

		token := parts[1]

		// API keys use sk_live_ prefix — skip, let API key middleware handle
		if strings.HasPrefix(token, "sk_live_") {
			return c.Next()
		}

		// Treat as WorkOS JWT
		k, err := m.initJWKS(c.Context())
		if err != nil {
			slog.Error("failed to initialize JWKS", "error", err)
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "Authentication service unavailable",
			})
		}

		claims := &workOSClaims{}
		parsed, err := jwt.ParseWithClaims(token, claims, k.Keyfunc,
			jwt.WithExpirationRequired(),
			jwt.WithIssuer("https://api.workos.com"),
			jwt.WithAudience(m.workosConfig.ClientID),
		)
		if err != nil || !parsed.Valid {
			slog.Debug("WorkOS JWT validation failed", "error", err)
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": "Invalid or expired token",
			})
		}

		workosUserID := claims.Subject
		if workosUserID == "" {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": "Token missing subject",
			})
		}

		// Look up existing account
		account, err := m.db.GetAccountByWorkOSUserID(c.Context(), workosUserID)
		if err != nil {
			if !errors.Is(err, db.ErrAccountNotFound) {
				slog.Error("failed to look up account by WorkOS user ID",
					"workos_user_id", workosUserID, "error", err)
				return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
					"error": "Internal server error",
				})
			}

			// JIT provision: create new B2B account
			account, err = m.jitProvision(c.Context(), workosUserID)
			if err != nil {
				slog.Error("JIT provisioning failed",
					"workos_user_id", workosUserID, "error", err)
				return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
					"error": "Failed to provision account",
				})
			}

			slog.Info("JIT provisioned B2B account",
				"account_id", account.ID,
				"workos_user_id", workosUserID,
			)
		}

		if account.Status != db.AccountStatusActive {
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
				"error": "Account is not active",
			})
		}

		// Set context — same keys as existing AuthMiddleware
		c.Locals("account_id", account.ID.String())
		c.Locals("account_number", account.AccountNumber)

		return c.Next()
	}
}

// jitProvision creates a new B2B account + Stripe customer for a WorkOS user.
func (m *WorkOSAuthMiddleware) jitProvision(ctx context.Context, workosUserID string) (*db.Account, error) {
	// Fetch user details from WorkOS to get email
	email, err := m.fetchWorkOSUserEmail(ctx, workosUserID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch WorkOS user: %w", err)
	}

	// Create B2B account (no company name yet — collected during onboarding)
	account, err := m.db.CreateB2BAccount(ctx, workosUserID, email, "")
	if err != nil {
		return nil, fmt.Errorf("failed to create account: %w", err)
	}

	// Create Stripe customer if Stripe is configured
	if m.stripeConfig.SecretKey != "" {
		params := &stripe.CustomerParams{
			Email: stripe.String(email),
		}
		params.AddMetadata("account_id", account.ID.String())
		params.AddMetadata("account_type", "b2b")

		cust, err := customer.New(params)
		if err != nil {
			slog.Error("failed to create Stripe customer during JIT provision, rolling back",
				"account_id", account.ID, "error", err)
			if delErr := m.db.DeleteAccount(ctx, account.ID); delErr != nil {
				slog.Error("failed to delete orphaned account", "account_id", account.ID, "error", delErr)
			}
			return nil, fmt.Errorf("failed to create Stripe customer: %w", err)
		}

		if err := m.db.UpdateStripeCustomerID(ctx, account.ID, cust.ID); err != nil {
			slog.Error("failed to store Stripe customer ID, rolling back",
				"account_id", account.ID, "error", err)
			if _, delErr := customer.Del(cust.ID, nil); delErr != nil {
				slog.Error("failed to delete Stripe customer during rollback",
					"stripe_customer_id", cust.ID, "error", delErr)
			}
			if delErr := m.db.DeleteAccount(ctx, account.ID); delErr != nil {
				slog.Error("failed to delete orphaned account", "account_id", account.ID, "error", delErr)
			}
			return nil, fmt.Errorf("failed to store Stripe customer ID: %w", err)
		}
		account.StripeCustomerID = &cust.ID
	}

	return account, nil
}

// workOSUserResponse is the subset of WorkOS User Management API response we need.
type workOSUserResponse struct {
	ID    string `json:"id"`
	Email string `json:"email"`
}

// fetchWorkOSUserEmail calls the WorkOS User Management API to get the user's email.
func (m *WorkOSAuthMiddleware) fetchWorkOSUserEmail(ctx context.Context, userID string) (string, error) {
	url := fmt.Sprintf("https://api.workos.com/user_management/users/%s", userID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+m.workosConfig.APIKey)

	resp, err := m.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("WorkOS API request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("WorkOS API returned %d: %s", resp.StatusCode, string(body))
	}

	var user workOSUserResponse
	if err := json.NewDecoder(resp.Body).Decode(&user); err != nil {
		return "", fmt.Errorf("failed to decode WorkOS user response: %w", err)
	}

	if user.Email == "" {
		return "", fmt.Errorf("WorkOS user %s has no email", userID)
	}

	return user.Email, nil
}
