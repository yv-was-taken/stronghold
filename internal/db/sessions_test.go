package db

import (
	"context"
	"net"
	"testing"
	"time"

	"stronghold/internal/db/testutil"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateSession_HashesToken(t *testing.T) {
	testDB := testutil.NewTestDB(t)
	defer testDB.Close(t)

	db := &DB{pool: testDB.Pool}
	ctx := context.Background()

	// Create an account
	account, err := db.CreateAccount(ctx, nil)
	require.NoError(t, err)

	// Create session
	ip := net.ParseIP("192.168.1.1")
	userAgent := "Test-Agent/1.0"
	duration := 24 * time.Hour

	session, refreshToken, err := db.CreateSession(ctx, account.ID, ip, userAgent, duration)
	require.NoError(t, err)
	require.NotNil(t, session)
	require.NotEmpty(t, refreshToken)

	// Verify that stored hash is NOT the plaintext token
	assert.NotEqual(t, refreshToken, session.RefreshTokenHash,
		"Stored hash should not equal plaintext refresh token")

	// Verify hash is SHA-256 (64 hex characters)
	assert.Len(t, session.RefreshTokenHash, 64,
		"Hash should be 64 characters (SHA-256 hex)")

	// Verify we can find the session by the original token
	found, err := db.GetSessionByRefreshToken(ctx, refreshToken)
	require.NoError(t, err)
	assert.Equal(t, session.ID, found.ID)
}

func TestGetSession_RejectsExpired(t *testing.T) {
	testDB := testutil.NewTestDB(t)
	defer testDB.Close(t)

	db := &DB{pool: testDB.Pool}
	ctx := context.Background()

	account, err := db.CreateAccount(ctx, nil)
	require.NoError(t, err)

	// Create session with very short duration
	ip := net.ParseIP("127.0.0.1")
	_, refreshToken, err := db.CreateSession(ctx, account.ID, ip, "", 1*time.Millisecond)
	require.NoError(t, err)

	// Wait for expiry
	time.Sleep(10 * time.Millisecond)

	// Should fail with expired error
	_, err = db.GetSessionByRefreshToken(ctx, refreshToken)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "expired")
}

func TestRotateRefreshToken_InvalidatesOld(t *testing.T) {
	testDB := testutil.NewTestDB(t)
	defer testDB.Close(t)

	db := &DB{pool: testDB.Pool}
	ctx := context.Background()

	account, err := db.CreateAccount(ctx, nil)
	require.NoError(t, err)

	// Create session
	ip := net.ParseIP("127.0.0.1")
	session, oldToken, err := db.CreateSession(ctx, account.ID, ip, "", 24*time.Hour)
	require.NoError(t, err)

	// Rotate token
	newSession, newToken, err := db.RotateRefreshToken(ctx, oldToken, 24*time.Hour)
	require.NoError(t, err)
	require.NotEmpty(t, newToken)

	// Verify same session was updated
	assert.Equal(t, session.ID, newSession.ID)

	// Verify old token no longer works
	_, err = db.GetSessionByRefreshToken(ctx, oldToken)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")

	// Verify new token works
	found, err := db.GetSessionByRefreshToken(ctx, newToken)
	require.NoError(t, err)
	assert.Equal(t, session.ID, found.ID)
}

func TestDeleteAllSessions_MultiDevice(t *testing.T) {
	testDB := testutil.NewTestDB(t)
	defer testDB.Close(t)

	db := &DB{pool: testDB.Pool}
	ctx := context.Background()

	account, err := db.CreateAccount(ctx, nil)
	require.NoError(t, err)

	// Create multiple sessions (simulating multiple devices)
	var tokens []string
	for i := 0; i < 3; i++ {
		ip := net.ParseIP("127.0.0.1")
		userAgent := "Device-" + string(rune('A'+i))
		_, token, err := db.CreateSession(ctx, account.ID, ip, userAgent, 24*time.Hour)
		require.NoError(t, err)
		tokens = append(tokens, token)
	}

	// Verify all sessions exist
	sessions, err := db.GetAccountSessions(ctx, account.ID)
	require.NoError(t, err)
	assert.Len(t, sessions, 3)

	// Delete all sessions
	err = db.DeleteAllAccountSessions(ctx, account.ID)
	require.NoError(t, err)

	// Verify all sessions are gone
	sessions, err = db.GetAccountSessions(ctx, account.ID)
	require.NoError(t, err)
	assert.Len(t, sessions, 0)

	// Verify all tokens are invalidated
	for _, token := range tokens {
		_, err := db.GetSessionByRefreshToken(ctx, token)
		require.Error(t, err)
	}
}

func TestDeleteSession_ByID(t *testing.T) {
	testDB := testutil.NewTestDB(t)
	defer testDB.Close(t)

	db := &DB{pool: testDB.Pool}
	ctx := context.Background()

	account, err := db.CreateAccount(ctx, nil)
	require.NoError(t, err)

	ip := net.ParseIP("127.0.0.1")
	session, token, err := db.CreateSession(ctx, account.ID, ip, "", 24*time.Hour)
	require.NoError(t, err)

	// Delete by ID
	err = db.DeleteSession(ctx, session.ID)
	require.NoError(t, err)

	// Verify session is gone
	_, err = db.GetSessionByRefreshToken(ctx, token)
	require.Error(t, err)
}

func TestDeleteSessionByRefreshToken(t *testing.T) {
	testDB := testutil.NewTestDB(t)
	defer testDB.Close(t)

	db := &DB{pool: testDB.Pool}
	ctx := context.Background()

	account, err := db.CreateAccount(ctx, nil)
	require.NoError(t, err)

	ip := net.ParseIP("127.0.0.1")
	_, token, err := db.CreateSession(ctx, account.ID, ip, "", 24*time.Hour)
	require.NoError(t, err)

	// Delete by token
	err = db.DeleteSessionByRefreshToken(ctx, token)
	require.NoError(t, err)

	// Verify session is gone
	_, err = db.GetSessionByRefreshToken(ctx, token)
	require.Error(t, err)
}

func TestUpdateSessionLastUsed(t *testing.T) {
	testDB := testutil.NewTestDB(t)
	defer testDB.Close(t)

	db := &DB{pool: testDB.Pool}
	ctx := context.Background()

	account, err := db.CreateAccount(ctx, nil)
	require.NoError(t, err)

	ip := net.ParseIP("127.0.0.1")
	session, token, err := db.CreateSession(ctx, account.ID, ip, "", 24*time.Hour)
	require.NoError(t, err)
	originalLastUsed := session.LastUsedAt

	// Wait a bit to ensure timestamp difference
	time.Sleep(50 * time.Millisecond)

	// Update last used
	err = db.UpdateSessionLastUsed(ctx, session.ID)
	require.NoError(t, err)

	// Verify update
	found, err := db.GetSessionByRefreshToken(ctx, token)
	require.NoError(t, err)
	require.NotNil(t, found.LastUsedAt)
	assert.True(t, found.LastUsedAt.After(*originalLastUsed),
		"LastUsedAt should be updated to a later time")
}

func TestCleanupExpiredSessions(t *testing.T) {
	testDB := testutil.NewTestDB(t)
	defer testDB.Close(t)

	db := &DB{pool: testDB.Pool}
	ctx := context.Background()

	account, err := db.CreateAccount(ctx, nil)
	require.NoError(t, err)

	ip := net.ParseIP("127.0.0.1")

	// Create expired session
	_, _, err = db.CreateSession(ctx, account.ID, ip, "expired", 1*time.Millisecond)
	require.NoError(t, err)

	// Create active session
	_, activeToken, err := db.CreateSession(ctx, account.ID, ip, "active", 24*time.Hour)
	require.NoError(t, err)

	// Wait for first session to expire
	time.Sleep(10 * time.Millisecond)

	// Cleanup expired sessions
	count, err := db.CleanupExpiredSessions(ctx)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, count, int64(1))

	// Active session should still work
	_, err = db.GetSessionByRefreshToken(ctx, activeToken)
	require.NoError(t, err)
}

func TestGetAccountSessions_OrderedByLastUsed(t *testing.T) {
	testDB := testutil.NewTestDB(t)
	defer testDB.Close(t)

	db := &DB{pool: testDB.Pool}
	ctx := context.Background()

	account, err := db.CreateAccount(ctx, nil)
	require.NoError(t, err)

	ip := net.ParseIP("127.0.0.1")

	// Create sessions with small delay between them
	var sessions []*Session
	for i := 0; i < 3; i++ {
		session, _, err := db.CreateSession(ctx, account.ID, ip, "device-"+string(rune('0'+i)), 24*time.Hour)
		require.NoError(t, err)
		sessions = append(sessions, session)
		time.Sleep(10 * time.Millisecond)
	}

	// Update middle session to make it most recently used
	err = db.UpdateSessionLastUsed(ctx, sessions[1].ID)
	require.NoError(t, err)

	// Get all sessions
	found, err := db.GetAccountSessions(ctx, account.ID)
	require.NoError(t, err)
	assert.Len(t, found, 3)

	// Most recently used should be first
	assert.Equal(t, sessions[1].ID, found[0].ID)
}

func TestSession_StoresIPAndUserAgent(t *testing.T) {
	testDB := testutil.NewTestDB(t)
	defer testDB.Close(t)

	db := &DB{pool: testDB.Pool}
	ctx := context.Background()

	account, err := db.CreateAccount(ctx, nil)
	require.NoError(t, err)

	ip := net.ParseIP("10.0.0.1")
	userAgent := "Mozilla/5.0 (Windows NT 10.0; Win64; x64) Chrome/120.0.0.0"

	session, token, err := db.CreateSession(ctx, account.ID, ip, userAgent, 24*time.Hour)
	require.NoError(t, err)
	require.NotNil(t, session.IPAddress)
	require.NotNil(t, session.UserAgent)
	assert.Equal(t, ip.String(), session.IPAddress.String())
	assert.Equal(t, userAgent, *session.UserAgent)

	// Verify persisted correctly
	found, err := db.GetSessionByRefreshToken(ctx, token)
	require.NoError(t, err)
	require.NotNil(t, found.IPAddress)
	require.NotNil(t, found.UserAgent)
	assert.Equal(t, ip.String(), found.IPAddress.String())
	assert.Equal(t, userAgent, *found.UserAgent)
}

func TestRotateRefreshToken_ExtendsExpiry(t *testing.T) {
	testDB := testutil.NewTestDB(t)
	defer testDB.Close(t)

	db := &DB{pool: testDB.Pool}
	ctx := context.Background()

	account, err := db.CreateAccount(ctx, nil)
	require.NoError(t, err)

	ip := net.ParseIP("127.0.0.1")
	session, token, err := db.CreateSession(ctx, account.ID, ip, "", 1*time.Hour)
	require.NoError(t, err)
	originalExpiry := session.ExpiresAt

	// Rotate with longer duration
	newSession, _, err := db.RotateRefreshToken(ctx, token, 7*24*time.Hour)
	require.NoError(t, err)

	// New expiry should be later
	assert.True(t, newSession.ExpiresAt.After(originalExpiry),
		"New expiry should be later than original")
}

func TestGetSessionByRefreshToken_InvalidToken(t *testing.T) {
	testDB := testutil.NewTestDB(t)
	defer testDB.Close(t)

	db := &DB{pool: testDB.Pool}
	ctx := context.Background()

	_, err := db.GetSessionByRefreshToken(ctx, "invalid-token-that-does-not-exist")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestRotateRefreshToken_ExpiredSession(t *testing.T) {
	testDB := testutil.NewTestDB(t)
	defer testDB.Close(t)

	db := &DB{pool: testDB.Pool}
	ctx := context.Background()

	account, err := db.CreateAccount(ctx, nil)
	require.NoError(t, err)

	ip := net.ParseIP("127.0.0.1")
	_, token, err := db.CreateSession(ctx, account.ID, ip, "", 1*time.Millisecond)
	require.NoError(t, err)

	// Wait for expiry
	time.Sleep(10 * time.Millisecond)

	// Should fail to rotate expired session
	_, _, err = db.RotateRefreshToken(ctx, token, 24*time.Hour)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "expired")
}
