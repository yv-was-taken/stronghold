package db

import (
	"context"
	"strings"
	"testing"

	"stronghold/internal/db/testutil"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testHash returns a valid 64-char hex string for use as a key_hash in tests.
// Pads the suffix with zeros to reach exactly 64 characters.
func testHash(suffix string) string {
	if len(suffix) >= 64 {
		return suffix[:64]
	}
	return suffix + strings.Repeat("0", 64-len(suffix))
}

// createTestB2BAccount is a helper that creates a B2B account for API key tests.
func createTestB2BAccount(t *testing.T, db *DB, email string) *Account {
	t.Helper()
	ctx := context.Background()
	account, err := db.CreateB2BAccount(ctx, "user_01"+email, email, "Test Company")
	require.NoError(t, err)
	return account
}

func TestCreateAPIKey(t *testing.T) {
	testDB := testutil.NewTestDB(t)
	defer testDB.Close(t)

	db := &DB{pool: testDB.Pool}
	ctx := context.Background()

	account := createTestB2BAccount(t, db, "apikey-create@example.com")

	hash := testHash("abc1")
	key, err := db.CreateAPIKey(ctx, account.ID, "sk_live_abc1", hash, "My Key", 100)
	require.NoError(t, err)
	require.NotNil(t, key)

	// Verify fields
	assert.NotEqual(t, uuid.Nil, key.ID)
	assert.Equal(t, account.ID, key.AccountID)
	assert.Equal(t, "sk_live_abc1", key.KeyPrefix)
	assert.Equal(t, hash, key.KeyHash)
	assert.Equal(t, "My Key", key.Name)
	assert.NotZero(t, key.CreatedAt)
	assert.Nil(t, key.LastUsedAt)
	assert.Nil(t, key.RevokedAt)
}

func TestGetAPIKeyByHash(t *testing.T) {
	testDB := testutil.NewTestDB(t)
	defer testDB.Close(t)

	db := &DB{pool: testDB.Pool}
	ctx := context.Background()

	account := createTestB2BAccount(t, db, "apikey-get@example.com")

	hash := testHash("1001")
	created, err := db.CreateAPIKey(ctx, account.ID, "sk_live_xyz", hash, "Lookup Key", 100)
	require.NoError(t, err)

	// Retrieve by hash
	found, err := db.GetAPIKeyByHash(ctx, hash)
	require.NoError(t, err)
	require.NotNil(t, found)
	assert.Equal(t, created.ID, found.ID)
	assert.Equal(t, account.ID, found.AccountID)
	assert.Equal(t, "sk_live_xyz", found.KeyPrefix)
	assert.Equal(t, hash, found.KeyHash)
	assert.Equal(t, "Lookup Key", found.Name)
}

func TestGetAPIKeyByHash_NotFound(t *testing.T) {
	testDB := testutil.NewTestDB(t)
	defer testDB.Close(t)

	db := &DB{pool: testDB.Pool}
	ctx := context.Background()

	_, err := db.GetAPIKeyByHash(ctx, testHash("deadbeef"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestGetAPIKeyByHash_Revoked(t *testing.T) {
	testDB := testutil.NewTestDB(t)
	defer testDB.Close(t)

	db := &DB{pool: testDB.Pool}
	ctx := context.Background()

	account := createTestB2BAccount(t, db, "apikey-revoked@example.com")

	// Create and then revoke a key
	hash := testHash("2002")
	key, err := db.CreateAPIKey(ctx, account.ID, "sk_live_rev", hash, "Revoked Key", 100)
	require.NoError(t, err)

	err = db.RevokeAPIKey(ctx, key.ID, account.ID)
	require.NoError(t, err)

	// Revoked key should not be found via GetAPIKeyByHash
	_, err = db.GetAPIKeyByHash(ctx, hash)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestListAPIKeys(t *testing.T) {
	testDB := testutil.NewTestDB(t)
	defer testDB.Close(t)

	db := &DB{pool: testDB.Pool}
	ctx := context.Background()

	account := createTestB2BAccount(t, db, "apikey-list@example.com")

	// Create multiple keys
	_, err := db.CreateAPIKey(ctx, account.ID, "sk_live_a", testHash("3001"), "Key A", 100)
	require.NoError(t, err)

	_, err = db.CreateAPIKey(ctx, account.ID, "sk_live_b", testHash("3002"), "Key B", 100)
	require.NoError(t, err)

	keyToRevoke, err := db.CreateAPIKey(ctx, account.ID, "sk_live_c", testHash("3003"), "Key C (revoked)", 100)
	require.NoError(t, err)

	// Revoke one key
	err = db.RevokeAPIKey(ctx, keyToRevoke.ID, account.ID)
	require.NoError(t, err)

	// List should only return active (non-revoked) keys
	keys, err := db.ListAPIKeys(ctx, account.ID)
	require.NoError(t, err)
	assert.Len(t, keys, 2)

	// Verify revoked key is excluded
	for _, k := range keys {
		assert.Nil(t, k.RevokedAt)
		assert.NotEqual(t, keyToRevoke.ID, k.ID)
	}
}

func TestRevokeAPIKey(t *testing.T) {
	testDB := testutil.NewTestDB(t)
	defer testDB.Close(t)

	db := &DB{pool: testDB.Pool}
	ctx := context.Background()

	account := createTestB2BAccount(t, db, "apikey-revoke@example.com")

	hash := testHash("4001")
	key, err := db.CreateAPIKey(ctx, account.ID, "sk_live_del", hash, "Key To Revoke", 100)
	require.NoError(t, err)
	assert.Nil(t, key.RevokedAt)

	// Revoke the key
	err = db.RevokeAPIKey(ctx, key.ID, account.ID)
	require.NoError(t, err)

	// Verify the key is no longer returned by GetAPIKeyByHash
	_, err = db.GetAPIKeyByHash(ctx, hash)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")

	// Trying to revoke again should fail (already revoked)
	err = db.RevokeAPIKey(ctx, key.ID, account.ID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found or already revoked")
}

func TestRevokeAPIKey_WrongAccount(t *testing.T) {
	testDB := testutil.NewTestDB(t)
	defer testDB.Close(t)

	db := &DB{pool: testDB.Pool}
	ctx := context.Background()

	account1 := createTestB2BAccount(t, db, "apikey-owner@example.com")
	account2 := createTestB2BAccount(t, db, "apikey-intruder@example.com")

	// Create key owned by account1
	hash := testHash("5001")
	key, err := db.CreateAPIKey(ctx, account1.ID, "sk_live_own", hash, "Owner's Key", 100)
	require.NoError(t, err)

	// Attempt to revoke with account2's ID — should fail
	err = db.RevokeAPIKey(ctx, key.ID, account2.ID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found or already revoked")

	// Verify the key is still active
	found, err := db.GetAPIKeyByHash(ctx, hash)
	require.NoError(t, err)
	assert.Equal(t, key.ID, found.ID)
	assert.Nil(t, found.RevokedAt)
}

func TestUpdateAPIKeyLastUsed(t *testing.T) {
	testDB := testutil.NewTestDB(t)
	defer testDB.Close(t)

	db := &DB{pool: testDB.Pool}
	ctx := context.Background()

	account := createTestB2BAccount(t, db, "apikey-lastused@example.com")

	hash := testHash("6001")
	key, err := db.CreateAPIKey(ctx, account.ID, "sk_live_lu", hash, "Last Used Key", 100)
	require.NoError(t, err)
	assert.Nil(t, key.LastUsedAt)

	// Update last used
	err = db.UpdateAPIKeyLastUsed(ctx, key.ID)
	require.NoError(t, err)

	// Retrieve and verify last_used_at is set
	found, err := db.GetAPIKeyByHash(ctx, hash)
	require.NoError(t, err)
	assert.NotNil(t, found.LastUsedAt)
}

func TestCountActiveAPIKeys(t *testing.T) {
	testDB := testutil.NewTestDB(t)
	defer testDB.Close(t)

	db := &DB{pool: testDB.Pool}
	ctx := context.Background()

	account := createTestB2BAccount(t, db, "apikey-count@example.com")

	// Initially zero
	count, err := db.CountActiveAPIKeys(ctx, account.ID)
	require.NoError(t, err)
	assert.Equal(t, 0, count)

	// Create 3 keys
	_, err = db.CreateAPIKey(ctx, account.ID, "sk_live_c1", testHash("7001"), "Key 1", 100)
	require.NoError(t, err)
	_, err = db.CreateAPIKey(ctx, account.ID, "sk_live_c2", testHash("7002"), "Key 2", 100)
	require.NoError(t, err)
	keyToRevoke, err := db.CreateAPIKey(ctx, account.ID, "sk_live_c3", testHash("7003"), "Key 3", 100)
	require.NoError(t, err)

	// Count should be 3
	count, err = db.CountActiveAPIKeys(ctx, account.ID)
	require.NoError(t, err)
	assert.Equal(t, 3, count)

	// Revoke one
	err = db.RevokeAPIKey(ctx, keyToRevoke.ID, account.ID)
	require.NoError(t, err)

	// Count should be 2 (revoked key excluded)
	count, err = db.CountActiveAPIKeys(ctx, account.ID)
	require.NoError(t, err)
	assert.Equal(t, 2, count)
}
