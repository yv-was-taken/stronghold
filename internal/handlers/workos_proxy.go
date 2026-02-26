package handlers

import (
	"bytes"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/gofiber/fiber/v3"
)

const workosAPIBase = "https://api.workos.com"

// WorkOSProxyHandler proxies WorkOS user management endpoints through our API.
// This works around a WorkOS CORS bug where OPTIONS preflight responses include
// Access-Control-Allow-Origin but actual POST/GET responses do not, breaking
// the client-only AuthKit SDK's code exchange and token refresh flows.
type WorkOSProxyHandler struct {
	client *http.Client
}

// NewWorkOSProxyHandler creates a new WorkOS proxy handler.
func NewWorkOSProxyHandler() *WorkOSProxyHandler {
	return &WorkOSProxyHandler{
		client: &http.Client{
			Timeout: 15 * time.Second,
			// Don't follow redirects — return them to the caller
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}
}

// RegisterRoutes registers the WorkOS proxy routes. These must be registered
// BEFORE the WorkOS auth middleware so they are not intercepted.
func (h *WorkOSProxyHandler) RegisterRoutes(app *fiber.App) {
	// POST /user_management/authenticate — code exchange and token refresh
	app.Post("/user_management/authenticate", h.ProxyAuthenticate)

	// GET /user_management/authorize — redirect to WorkOS authorization
	app.Get("/user_management/authorize", h.RedirectAuthorize)

	// GET /user_management/sessions/logout — redirect to WorkOS logout
	app.Get("/user_management/sessions/logout", h.RedirectLogout)
}

// ProxyAuthenticate forwards the code exchange / token refresh POST to WorkOS
// and returns the response. Our CORS middleware handles the cross-origin headers.
func (h *WorkOSProxyHandler) ProxyAuthenticate(c fiber.Ctx) error {
	target := workosAPIBase + "/user_management/authenticate"

	req, err := http.NewRequestWithContext(c.Context(), http.MethodPost, target, bytes.NewReader(c.Body()))
	if err != nil {
		slog.Error("workos proxy: failed to create request", "error", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "proxy error",
		})
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := h.client.Do(req)
	if err != nil {
		slog.Error("workos proxy: request failed", "error", err)
		return c.Status(fiber.StatusBadGateway).JSON(fiber.Map{
			"error": "upstream error",
		})
	}
	defer resp.Body.Close()

	// Copy response content-type and status
	c.Set("Content-Type", resp.Header.Get("Content-Type"))
	c.Status(resp.StatusCode)

	// Stream the response body back
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		slog.Error("workos proxy: failed to read response", "error", err)
		return c.Status(fiber.StatusBadGateway).JSON(fiber.Map{
			"error": "upstream read error",
		})
	}

	return c.Send(body)
}

// RedirectAuthorize redirects the browser to WorkOS's authorization endpoint.
// This is a browser navigation (not fetch), so CORS doesn't apply.
func (h *WorkOSProxyHandler) RedirectAuthorize(c fiber.Ctx) error {
	target := workosAPIBase + "/user_management/authorize"
	queryString := string(c.Request().URI().QueryString())
	if queryString != "" {
		target += "?" + queryString
	}
	return c.Redirect().Status(fiber.StatusFound).To(target)
}

// RedirectLogout redirects the browser to WorkOS's logout endpoint.
func (h *WorkOSProxyHandler) RedirectLogout(c fiber.Ctx) error {
	target := workosAPIBase + "/user_management/sessions/logout"
	queryString := string(c.Request().URI().QueryString())
	if queryString != "" {
		target += "?" + queryString
	}
	return c.Redirect().Status(fiber.StatusFound).To(target)
}
