package middleware

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"stronghold/internal/config"
	"stronghold/internal/db"
	"stronghold/internal/db/testutil"

	"github.com/MicahParks/jwkset"
	"github.com/MicahParks/keyfunc/v3"
	"github.com/gofiber/fiber/v3"
	"github.com/golang-jwt/jwt/v5"
	"github.com/jarcoal/httpmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testJWTSetup creates a test ECDSA key pair and a keyfunc that validates tokens
// signed with the private key.
type testJWTSetup struct {
	privateKey *ecdsa.PrivateKey
	kf         keyfunc.Keyfunc
}

func newTestJWTSetup(t *testing.T) *testJWTSetup {
	t.Helper()
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	// Create in-memory JWKS storage with the public key
	storage := jwkset.NewMemoryStorage()
	jwk, err := jwkset.NewJWKFromKey(privateKey.Public(), jwkset.JWKOptions{
		Metadata: jwkset.JWKMetadataOptions{
			KID: "test-key",
		},
	})
	require.NoError(t, err)
	err = storage.KeyWrite(context.Background(), jwk)
	require.NoError(t, err)

	kf, err := keyfunc.New(keyfunc.Options{
		Storage: storage,
	})
	require.NoError(t, err)

	return &testJWTSetup{
		privateKey: privateKey,
		kf:         kf,
	}
}

func (s *testJWTSetup) signToken(sub string, exp time.Time) string {
	claims := jwt.RegisteredClaims{
		Subject:   sub,
		ExpiresAt: jwt.NewNumericDate(exp),
		IssuedAt:  jwt.NewNumericDate(time.Now()),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodES256, claims)
	token.Header["kid"] = "test-key"
	signed, err := token.SignedString(s.privateKey)
	if err != nil {
		panic(err)
	}
	return signed
}

// signTokenFull signs a JWT with issuer and audience claims set, matching what
// a real WorkOS access token contains. Use this for tests that exercise the
// full middleware path (DB lookup, JIT provisioning, etc.) where the JWT must
// pass all validation checks including issuer and audience.
func (s *testJWTSetup) signTokenFull(sub, issuer, audience string, exp time.Time) string {
	claims := jwt.RegisteredClaims{
		Subject:   sub,
		Issuer:    issuer,
		Audience:  jwt.ClaimStrings{audience},
		ExpiresAt: jwt.NewNumericDate(exp),
		IssuedAt:  jwt.NewNumericDate(time.Now()),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodES256, claims)
	token.Header["kid"] = "test-key"
	signed, err := token.SignedString(s.privateKey)
	if err != nil {
		panic(err)
	}
	return signed
}

func openTestDB(t *testing.T, testDB *testutil.TestDB) *db.DB {
	t.Helper()
	database, err := db.New(&db.Config{
		Host:     testDB.Host,
		Port:     testDB.Port,
		User:     testDB.User,
		Password: testDB.Password,
		Name:     testDB.Database,
		SSLMode:  "disable",
	})
	require.NoError(t, err)
	return database
}

func setupTestMiddleware(t *testing.T, database *db.DB, jwtSetup *testJWTSetup) *fiber.App {
	t.Helper()

	workosConfig := &config.WorkOSConfig{
		APIKey:   "sk_test_fake",
		ClientID: "client_01TEST",
	}
	stripeConfig := &config.StripeConfig{} // No Stripe in tests

	m := NewWorkOSAuthMiddleware(database, workosConfig, stripeConfig)
	// Inject the test keyfunc directly (skip real JWKS fetch)
	m.jwks = jwtSetup.kf

	app := fiber.New()
	app.Use(m.Handler())
	app.Get("/test", func(c fiber.Ctx) error {
		accountID := c.Locals("account_id")
		if accountID == nil {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "no account"})
		}
		return c.JSON(fiber.Map{"account_id": accountID})
	})

	return app
}

// setupSkipApp creates a Fiber app with WorkOS middleware but no real DB,
// used for tests where the middleware is expected to skip (not hit DB).
func setupSkipApp(t *testing.T, jwtSetup *testJWTSetup) *fiber.App {
	t.Helper()
	m := NewWorkOSAuthMiddleware(nil, &config.WorkOSConfig{ClientID: "c"}, &config.StripeConfig{})
	m.jwks = jwtSetup.kf

	app := fiber.New()
	app.Use(m.Handler())
	app.Get("/test", func(c fiber.Ctx) error {
		if c.Locals("account_id") == nil {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "no account"})
		}
		return c.JSON(fiber.Map{"ok": true})
	})
	return app
}

func TestWorkOSAuth_SkipsAPIKeys(t *testing.T) {
	jwtSetup := newTestJWTSetup(t)
	app := setupSkipApp(t, jwtSetup)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Authorization", "Bearer sk_live_testkey123")

	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

func TestWorkOSAuth_SkipsCookieAuth(t *testing.T) {
	jwtSetup := newTestJWTSetup(t)
	app := setupSkipApp(t, jwtSetup)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.AddCookie(&http.Cookie{Name: "stronghold_access", Value: "some-cookie-token"})

	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

func TestWorkOSAuth_RejectsExpiredToken(t *testing.T) {
	jwtSetup := newTestJWTSetup(t)
	app := setupSkipApp(t, jwtSetup)

	token := jwtSetup.signToken("user_01EXPIRED", time.Now().Add(-1*time.Hour))
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

func TestWorkOSAuth_RejectsInvalidSignature(t *testing.T) {
	jwtSetup := newTestJWTSetup(t)
	app := setupSkipApp(t, jwtSetup)

	// Token signed by a different key
	otherSetup := newTestJWTSetup(t)
	token := otherSetup.signToken("user_01INVALID", time.Now().Add(1*time.Hour))
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

func TestWorkOSAuth_RejectsEmptySubject(t *testing.T) {
	jwtSetup := newTestJWTSetup(t)
	app := setupSkipApp(t, jwtSetup)

	token := jwtSetup.signToken("", time.Now().Add(1*time.Hour))
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

func TestWorkOSAuth_MissingAuthHeader(t *testing.T) {
	jwtSetup := newTestJWTSetup(t)
	app := setupSkipApp(t, jwtSetup)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)

	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

func TestWorkOSAuth_ValidToken_ExistingAccount(t *testing.T) {
	testDB := testutil.NewTestDB(t)
	defer testDB.Close(t)
	database := openTestDB(t, testDB)
	defer database.Close()

	jwtSetup := newTestJWTSetup(t)
	app := setupTestMiddleware(t, database, jwtSetup)

	// Create an account first
	account, err := database.CreateB2BAccount(t.Context(), "user_01EXIST", "exist@example.com", "Existing Corp")
	require.NoError(t, err)

	token := jwtSetup.signToken("user_01EXIST", time.Now().Add(1*time.Hour))
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var body map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&body)
	require.NoError(t, err)
	assert.Equal(t, account.ID.String(), body["account_id"])
}

func TestWorkOSAuth_SuspendedAccount(t *testing.T) {
	testDB := testutil.NewTestDB(t)
	defer testDB.Close(t)
	database := openTestDB(t, testDB)
	defer database.Close()

	jwtSetup := newTestJWTSetup(t)
	app := setupTestMiddleware(t, database, jwtSetup)

	// Create and suspend account
	account, err := database.CreateB2BAccount(t.Context(), "user_01SUSP", "susp@example.com", "Suspended Corp")
	require.NoError(t, err)
	err = database.SuspendAccount(t.Context(), account.ID)
	require.NoError(t, err)

	token := jwtSetup.signToken("user_01SUSP", time.Now().Add(1*time.Hour))
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusForbidden, resp.StatusCode)
}

func TestWorkOSAuth_JITProvisioning(t *testing.T) {
	testDB := testutil.NewTestDB(t)
	defer testDB.Close(t)
	database := openTestDB(t, testDB)
	defer database.Close()

	jwtSetup := newTestJWTSetup(t)

	workosConfig := &config.WorkOSConfig{
		APIKey:   "sk_test_fake",
		ClientID: "client_01TEST",
	}
	stripeConfig := &config.StripeConfig{} // No Stripe in tests

	m := NewWorkOSAuthMiddleware(database, workosConfig, stripeConfig)
	m.jwks = jwtSetup.kf

	// Mock WorkOS User Management API using httpmock.
	// The middleware's httpClient uses http.DefaultTransport (no custom transport),
	// so httpmock.Activate() intercepts it.
	httpmock.Activate()
	defer httpmock.DeactivateAndReset()

	httpmock.RegisterResponder("GET", "https://api.workos.com/user_management/users/user_01NEW",
		httpmock.NewJsonResponderOrPanic(200, map[string]interface{}{
			"id":    "user_01NEW",
			"email": "newuser@example.com",
		}))

	app := fiber.New()
	app.Use(m.Handler())
	app.Get("/test", func(c fiber.Ctx) error {
		accountID := c.Locals("account_id")
		if accountID == nil {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "no account"})
		}
		return c.JSON(fiber.Map{"account_id": accountID})
	})

	// Sign a token with issuer and audience so it passes all JWT validation checks
	token := jwtSetup.signTokenFull(
		"user_01NEW",
		"https://api.workos.com",
		"client_01TEST",
		time.Now().Add(1*time.Hour),
	)
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Verify the response contains an account ID
	var body map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&body)
	require.NoError(t, err)
	assert.NotEmpty(t, body["account_id"])

	// Verify the account was actually created in the database
	account, err := database.GetAccountByWorkOSUserID(t.Context(), "user_01NEW")
	require.NoError(t, err)
	assert.Equal(t, "newuser@example.com", *account.Email)
	assert.Equal(t, db.AccountTypeB2B, account.AccountType)
	assert.Equal(t, db.AccountStatusActive, account.Status)

	// Verify the WorkOS API was called exactly once
	assert.Equal(t, 1, httpmock.GetTotalCallCount())
}
