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
	app.Get("/docs", h.SwaggerUI)
	app.Get("/docs/swagger.json", h.SwaggerJSON)
}

// SwaggerUI serves the Swagger UI page
// @Summary API Documentation
// @Description Interactive API documentation using Swagger UI
// @Tags docs
// @Produce html
// @Router /docs [get]
func (h *DocsHandler) SwaggerUI(c fiber.Ctx) error {
	html := `<!DOCTYPE html>
<html>
<head>
    <title>Stronghold API Documentation</title>
    <link rel="stylesheet" type="text/css" href="https://unpkg.com/swagger-ui-dist@5/swagger-ui.css">
    <style>
        html { box-sizing: border-box; overflow-y: scroll; }
        *, *:before, *:after { box-sizing: inherit; }
        body { margin: 0; background: #fafafa; }
        .swagger-ui .topbar { display: none; }
    </style>
</head>
<body>
    <div id="swagger-ui"></div>
    <script src="https://unpkg.com/swagger-ui-dist@5/swagger-ui-bundle.js"></script>
    <script>
        window.onload = function() {
            SwaggerUIBundle({
                url: "/docs/swagger.json",
                dom_id: '#swagger-ui',
                presets: [
                    SwaggerUIBundle.presets.apis,
                    SwaggerUIBundle.SwaggerUIStandalonePreset
                ],
                layout: "BaseLayout",
                deepLinking: true,
                showExtensions: true,
                showCommonExtensions: true
            });
        };
    </script>
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
