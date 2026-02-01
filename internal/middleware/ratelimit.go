package middleware

import (
	"strings"
	"time"

	"stronghold/internal/config"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/limiter"
)

// RateLimitMiddleware provides rate limiting for the API
type RateLimitMiddleware struct {
	config *config.RateLimitConfig
}

// NewRateLimitMiddleware creates a new rate limit middleware instance
func NewRateLimitMiddleware(cfg *config.RateLimitConfig) *RateLimitMiddleware {
	return &RateLimitMiddleware{
		config: cfg,
	}
}

// Middleware returns the general rate limiter for all endpoints
func (m *RateLimitMiddleware) Middleware() fiber.Handler {
	if !m.config.Enabled {
		return func(c fiber.Ctx) error {
			return c.Next()
		}
	}

	return limiter.New(limiter.Config{
		Max:        m.config.MaxRequests,
		Expiration: time.Duration(m.config.WindowSeconds) * time.Second,
		KeyGenerator: func(c fiber.Ctx) string {
			return c.IP()
		},
		LimitReached: rateLimitResponse,
		SkipSuccessfulRequests: false,
		SkipFailedRequests:     false,
		Next: func(c fiber.Ctx) bool {
			// Skip rate limiting for health endpoints
			return isHealthEndpoint(c.Path())
		},
	})
}

// AuthLimiter returns a stricter rate limiter for auth endpoints
func (m *RateLimitMiddleware) AuthLimiter() fiber.Handler {
	if !m.config.Enabled {
		return func(c fiber.Ctx) error {
			return c.Next()
		}
	}

	return limiter.New(limiter.Config{
		MaxFunc:    m.getAuthLimit,
		Expiration: time.Duration(m.config.WindowSeconds) * time.Second,
		KeyGenerator: func(c fiber.Ctx) string {
			// Key by IP + endpoint for per-endpoint limits
			return c.IP() + ":" + c.Path()
		},
		LimitReached:           rateLimitResponse,
		SkipSuccessfulRequests: false,
		SkipFailedRequests:     false,
	})
}

// getAuthLimit returns the appropriate limit based on the endpoint
func (m *RateLimitMiddleware) getAuthLimit(c fiber.Ctx) int {
	path := c.Path()

	switch {
	case strings.HasSuffix(path, "/login"):
		return m.config.LoginMax
	case strings.HasSuffix(path, "/account"):
		return m.config.AccountMax
	case strings.HasSuffix(path, "/refresh"):
		return m.config.RefreshMax
	default:
		return m.config.MaxRequests
	}
}

// rateLimitResponse returns a 429 Too Many Requests response
func rateLimitResponse(c fiber.Ctx) error {
	retryAfter := c.GetRespHeader("Retry-After")
	if retryAfter == "" {
		retryAfter = "60"
	}

	c.Set("Retry-After", retryAfter)
	return c.Status(fiber.StatusTooManyRequests).JSON(fiber.Map{
		"error":       "Too many requests",
		"message":     "Rate limit exceeded. Please try again later.",
		"retry_after": retryAfter,
	})
}

// isHealthEndpoint checks if the path is a health endpoint
func isHealthEndpoint(path string) bool {
	return strings.HasPrefix(path, "/health")
}
