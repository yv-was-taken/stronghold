package middleware

import (
	"regexp"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
)

const (
	// RequestIDHeader is the header name for the request ID
	RequestIDHeader = "X-Request-ID"
	// RequestIDKey is the key used to store the request ID in Fiber's Locals
	RequestIDKey = "request_id"
)

// validRequestIDPattern matches UUIDs or alphanumeric+hyphen strings up to 64 chars
var validRequestIDPattern = regexp.MustCompile(`^[0-9a-zA-Z-]{1,64}$`)

// RequestID returns middleware that generates a unique request ID for each request.
// The request ID is stored in c.Locals("request_id") and added to the response header.
// If the client provides a valid X-Request-ID header, that value is used instead.
// Invalid request IDs (non-alphanumeric, too long) are replaced with server-generated UUIDs.
func RequestID() fiber.Handler {
	return func(c fiber.Ctx) error {
		// Check if client provided a request ID
		requestID := c.Get(RequestIDHeader)
		if requestID == "" || !validRequestIDPattern.MatchString(requestID) {
			// Generate a new UUID if missing or invalid format
			requestID = uuid.New().String()
		}

		// Store in Locals for use by handlers and error handler
		c.Locals(RequestIDKey, requestID)

		// Add to response header
		c.Set(RequestIDHeader, requestID)

		return c.Next()
	}
}

// GetRequestID retrieves the request ID from the Fiber context.
// Returns an empty string if no request ID is set.
func GetRequestID(c fiber.Ctx) string {
	if id, ok := c.Locals(RequestIDKey).(string); ok {
		return id
	}
	return ""
}
