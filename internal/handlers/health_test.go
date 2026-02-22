package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"stronghold/internal/config"
	"stronghold/internal/db"
	"stronghold/internal/db/testutil"

	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHealth_AllUp(t *testing.T) {
	testDB := testutil.NewTestDB(t)
	defer testDB.Close(t)

	database := &db.DB{}
	// Access pool through test setup
	ctx := context.Background()
	err := testDB.Pool.Ping(ctx)
	require.NoError(t, err)

	// Create a mock facilitator server
	facilitatorServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer facilitatorServer.Close()

	cfg := &config.Config{
		X402: config.X402Config{
			FacilitatorURL: facilitatorServer.URL,
		},
	}

	// We need to create a proper DB with the pool
	database = createTestDBWrapper(testDB)
	handler := NewHealthHandler(database, cfg)

	app := fiber.New()
	handler.RegisterRoutes(app)

	req := httptest.NewRequest("GET", "/health", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, 200, resp.StatusCode)

	var body HealthResponse
	err = json.NewDecoder(resp.Body).Decode(&body)
	require.NoError(t, err)

	assert.Equal(t, "healthy", body.Status)
	assert.Equal(t, "dev", body.Version)
	assert.Equal(t, "up", body.Services["database"])
	assert.Equal(t, "up", body.Services["api"])
	assert.Equal(t, "up", body.Services["x402"])
	assert.NotZero(t, body.Timestamp)
}

func TestHealth_DBDown(t *testing.T) {
	// Create handler with nil DB
	cfg := &config.Config{
		X402: config.X402Config{
			FacilitatorURL: "", // Not configured
		},
	}

	handler := NewHealthHandler(nil, cfg)

	app := fiber.New()
	handler.RegisterRoutes(app)

	req := httptest.NewRequest("GET", "/health", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, 200, resp.StatusCode)

	var body HealthResponse
	err = json.NewDecoder(resp.Body).Decode(&body)
	require.NoError(t, err)

	assert.Equal(t, "degraded", body.Status)
	assert.Equal(t, "not_configured", body.Services["database"])
}

func TestHealthReady_DBDown(t *testing.T) {
	cfg := &config.Config{
		X402: config.X402Config{
			FacilitatorURL: "",
		},
	}

	handler := NewHealthHandler(nil, cfg)

	app := fiber.New()
	handler.RegisterRoutes(app)

	req := httptest.NewRequest("GET", "/health/ready", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, 503, resp.StatusCode)

	var body map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&body)
	require.NoError(t, err)

	assert.Equal(t, "not_ready", body["status"])
	assert.Equal(t, "database_unavailable", body["reason"])
}

func TestHealthLive_Always200(t *testing.T) {
	// Liveness probe should always return 200 if the process is running
	handler := NewHealthHandler(nil, nil)

	app := fiber.New()
	handler.RegisterRoutes(app)

	req := httptest.NewRequest("GET", "/health/live", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, 200, resp.StatusCode)

	var body map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&body)
	require.NoError(t, err)

	assert.Equal(t, "alive", body["status"])
}

func TestHealth_X402Down(t *testing.T) {
	testDB := testutil.NewTestDB(t)
	defer testDB.Close(t)

	database := createTestDBWrapper(testDB)

	// Clear cached facilitator status from previous tests
	resetFacilitatorCache()

	// Use a non-existent URL for the facilitator
	cfg := &config.Config{
		X402: config.X402Config{
			FacilitatorURL: "http://127.0.0.1:59999", // Non-existent
		},
	}

	handler := NewHealthHandler(database, cfg)

	app := fiber.New()
	handler.RegisterRoutes(app)

	req := httptest.NewRequest("GET", "/health", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	var body HealthResponse
	err = json.NewDecoder(resp.Body).Decode(&body)
	require.NoError(t, err)

	assert.Equal(t, "degraded", body.Status)
	assert.Equal(t, "up", body.Services["database"])
	assert.Contains(t, []string{"unreachable", "error"}, body.Services["x402"])
}

func TestHealthReady_X402Down(t *testing.T) {
	testDB := testutil.NewTestDB(t)
	defer testDB.Close(t)

	database := createTestDBWrapper(testDB)

	// Clear cached facilitator status from previous tests
	resetFacilitatorCache()

	cfg := &config.Config{
		X402: config.X402Config{
			FacilitatorURL: "http://127.0.0.1:59999",
		},
	}

	handler := NewHealthHandler(database, cfg)

	app := fiber.New()
	handler.RegisterRoutes(app)

	req := httptest.NewRequest("GET", "/health/ready", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, 503, resp.StatusCode)

	var body map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&body)
	require.NoError(t, err)

	assert.Equal(t, "not_ready", body["status"])
	assert.Equal(t, "x402_unavailable", body["reason"])
}

func TestHealthReady_ProductionPaymentsNotConfigured(t *testing.T) {
	testDB := testutil.NewTestDB(t)
	defer testDB.Close(t)

	database := createTestDBWrapper(testDB)

	resetFacilitatorCache()
	facilitatorServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer facilitatorServer.Close()

	cfg := &config.Config{
		Environment: config.EnvProduction,
		X402: config.X402Config{
			FacilitatorURL: facilitatorServer.URL,
			Networks:       []string{"base"},
		},
	}

	handler := NewHealthHandler(database, cfg)

	app := fiber.New()
	handler.RegisterRoutes(app)

	req := httptest.NewRequest("GET", "/health/ready", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, 503, resp.StatusCode)

	var body map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&body)
	require.NoError(t, err)

	assert.Equal(t, "not_ready", body["status"])
	assert.Equal(t, "payment_not_configured", body["reason"])
}

func TestHealthReady_DevModeNoPayments(t *testing.T) {
	testDB := testutil.NewTestDB(t)
	defer testDB.Close(t)

	database := createTestDBWrapper(testDB)

	resetFacilitatorCache()
	facilitatorServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer facilitatorServer.Close()

	cfg := &config.Config{
		Environment: config.EnvDevelopment,
		X402: config.X402Config{
			FacilitatorURL: facilitatorServer.URL,
			// No wallet addresses configured
		},
	}

	handler := NewHealthHandler(database, cfg)

	app := fiber.New()
	handler.RegisterRoutes(app)

	req := httptest.NewRequest("GET", "/health/ready", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, 200, resp.StatusCode)

	var body map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&body)
	require.NoError(t, err)

	assert.Equal(t, "ready", body["status"])
}

func TestHealth_Timeout(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping timeout test in short mode")
	}

	// Clear any cached facilitator result from previous tests
	resetFacilitatorCache()

	// Create a slow facilitator server
	slowServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(5 * time.Second) // Sleep longer than timeout
		w.WriteHeader(http.StatusOK)
	}))
	defer slowServer.Close()

	testDB := testutil.NewTestDB(t)
	defer testDB.Close(t)

	database := createTestDBWrapper(testDB)

	cfg := &config.Config{
		X402: config.X402Config{
			FacilitatorURL: slowServer.URL,
		},
	}

	handler := NewHealthHandler(database, cfg)

	app := fiber.New()
	handler.RegisterRoutes(app)

	start := time.Now()
	req := httptest.NewRequest("GET", "/health", nil)
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 10 * time.Second})
	require.NoError(t, err)
	defer resp.Body.Close()
	elapsed := time.Since(start)

	// Should timeout within ~3 seconds (the health check timeout)
	assert.Less(t, elapsed, 5*time.Second, "Health check should timeout before 5 seconds")

	var body HealthResponse
	err = json.NewDecoder(resp.Body).Decode(&body)
	require.NoError(t, err)

	// X402 should show as unreachable due to timeout
	assert.Contains(t, []string{"unreachable", "error"}, body.Services["x402"])
}

func TestHealth_X402NotConfigured(t *testing.T) {
	testDB := testutil.NewTestDB(t)
	defer testDB.Close(t)

	database := createTestDBWrapper(testDB)

	cfg := &config.Config{
		X402: config.X402Config{
			FacilitatorURL: "", // Not configured
		},
	}

	handler := NewHealthHandler(database, cfg)

	app := fiber.New()
	handler.RegisterRoutes(app)

	req := httptest.NewRequest("GET", "/health", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	var body HealthResponse
	err = json.NewDecoder(resp.Body).Decode(&body)
	require.NoError(t, err)

	// When not configured, it should be marked as "not_configured"
	assert.Equal(t, "not_configured", body.Services["x402"])
}

func TestHealth_NoConfig(t *testing.T) {
	handler := NewHealthHandler(nil, nil)

	app := fiber.New()
	handler.RegisterRoutes(app)

	req := httptest.NewRequest("GET", "/health", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, 200, resp.StatusCode)

	var body HealthResponse
	err = json.NewDecoder(resp.Body).Decode(&body)
	require.NoError(t, err)

	assert.Equal(t, "degraded", body.Status)
}

// Helper function to create a DB wrapper from testutil.TestDB
func createTestDBWrapper(testDB *testutil.TestDB) *db.DB {
	// We need to access the internal pool field
	// This is a testing-only helper
	type dbWithPool struct {
		pool interface{}
	}

	// Create a new DB that wraps the test pool
	cfg := &db.Config{
		Host:     testDB.Host,
		Port:     testDB.Port,
		User:     testDB.User,
		Password: testDB.Password,
		Name:     testDB.Database,
		SSLMode:  "disable",
	}

	database, err := db.New(cfg)
	if err != nil {
		panic(err)
	}
	return database
}
