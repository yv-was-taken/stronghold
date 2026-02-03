package middleware

import (
	"github.com/gofiber/fiber/v3"
)

// SecurityHeaders returns middleware that sets security-related HTTP headers
// to protect against common web vulnerabilities like XSS, clickjacking, and MIME-sniffing.
func SecurityHeaders() fiber.Handler {
	// Build CSP policy
	// Note: unpkg.com and unsafe-inline needed for Swagger UI docs page
	csp := "default-src 'self'; " +
		"script-src 'self' 'unsafe-inline' https://unpkg.com; " +
		"style-src 'self' 'unsafe-inline' https://unpkg.com; " +
		"img-src 'self' data: https:; " +
		"font-src 'self'; " +
		"connect-src 'self'; " +
		"frame-ancestors 'none'; " +
		"base-uri 'self'; " +
		"form-action 'self'"

	return func(c fiber.Ctx) error {
		// HTTP Strict Transport Security - force HTTPS, prevent downgrade attacks
		c.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")

		// Content Security Policy - controls which resources can be loaded
		c.Set("Content-Security-Policy", csp)

		// Prevent MIME type sniffing
		c.Set("X-Content-Type-Options", "nosniff")

		// Prevent clickjacking by denying iframe embedding
		c.Set("X-Frame-Options", "DENY")

		// Enable browser XSS filter (legacy, but still useful for older browsers)
		c.Set("X-XSS-Protection", "1; mode=block")

		// Control referrer information sent with requests
		c.Set("Referrer-Policy", "strict-origin-when-cross-origin")

		// Restrict browser features/APIs
		c.Set("Permissions-Policy", "geolocation=(), microphone=(), camera=()")

		return c.Next()
	}
}
