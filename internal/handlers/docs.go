package handlers

import (
	"github.com/gofiber/fiber/v3"
)

// DocsHandler serves API documentation
type DocsHandler struct{}

// NewDocsHandler creates a new docs handler
func NewDocsHandler() *DocsHandler {
	return &DocsHandler{}
}

// RegisterRoutes registers documentation routes
func (h *DocsHandler) RegisterRoutes(app *fiber.App) {
	app.Get("/docs", h.ScalarDocs)
	app.Get("/docs/swagger.json", h.SwaggerJSON)
}

// ScalarDocs serves the API documentation page using Scalar
// @Summary API Documentation
// @Description Interactive API documentation
// @Tags docs
// @Produce html
// @Router /docs [get]
func (h *DocsHandler) ScalarDocs(c fiber.Ctx) error {
	html := `<!DOCTYPE html>
<html>
<head>
    <title>Stronghold API Documentation</title>
    <meta charset="utf-8">
    <meta name="viewport" content="width=device-width, initial-scale=1">
</head>
<body>
    <script id="api-reference" data-url="/docs/swagger.json"
        src="https://cdn.jsdelivr.net/npm/@scalar/api-reference"></script>
</body>
</html>`

	c.Set("Content-Type", "text/html")
	return c.SendString(html)
}

// SwaggerJSON serves the OpenAPI specification
// @Summary OpenAPI Specification
// @Description Returns the OpenAPI 3.0 specification in JSON format
// @Tags docs
// @Produce json
// @Router /docs/swagger.json [get]
func (h *DocsHandler) SwaggerJSON(c fiber.Ctx) error {
	// Import the generated docs package which contains the swagger spec
	// This will be populated by swag init
	c.Set("Content-Type", "application/json")

	// Return the embedded swagger spec from the docs package
	// We'll read from the generated docs/swagger.json file
	return c.SendFile("./docs/swagger.json")
}
