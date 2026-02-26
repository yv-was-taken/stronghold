package db

import (
	"context"
	"testing"

	"stronghold/internal/db/testutil"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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

	key, err := db.CreateAPIKey(ctx, account.ID, "sk_live_abc1", "sha256hashvalue", "My Key", 100)
	require.NoError(t, err)
	require.NotNil(t, key)

	// Verify fields
	assert.NotEqual(t, uuid.Nil, key.ID)
	assert.Equal(t, account.ID, key.AccountID)
	assert.Equal(t, "sk_live_abc1", key.KeyPrefix)
	assert.Equal(t, "sha256hashvalue", key.KeyHash)
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

	created, err := db.CreateAPIKey(ctx, account.ID, "sk_live_xyz", "hash_lookup_test", "Lookup Key", 100)
	require.NoError(t, err)

	// Retrieve by hash
	found, err := db.GetAPIKeyByHash(ctx, "hash_lookup_test")
	require.NoError(t, err)
	require.NotNil(t, found)
	assert.Equal(t, created.ID, found.ID)
	assert.Equal(t, account.ID, found.AccountID)
	assert.Equal(t, "sk_live_xyz", found.KeyPrefix)
	assert.Equal(t, "hash_lookup_test", found.KeyHash)
	assert.Equal(t, "Lookup Key", found.Name)
}

func TestGetAPIKeyByHash_NotFound(t *testing.T) {
	testDB := testutil.NewTestDB(t)
	defer testDB.Close(t)

	db := &DB{pool: testDB.Pool}
	ctx := context.Background()

	_, err := db.GetAPIKeyByHash(ctx, "nonexistent_hash")
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
	key, err := db.CreateAPIKey(ctx, account.ID, "sk_live_rev", "hash_revoked_test", "Revoked Key", 100)
	require.NoError(t, err)

	err = db.RevokeAPIKey(ctx, key.ID, account.ID)
	require.NoError(t, err)

	// Revoked key should not be found via GetAPIKeyByHash
	_, err = db.GetAPIKeyByHash(ctx, "hash_revoked_test")
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
	_, err := db.CreateAPIKey(ctx, account.ID, "sk_live_a", "hash_a", "Key A", 100)
	require.NoError(t, err)

	_, err = db.CreateAPIKey(ctx, account.ID, "sk_live_b", "hash_b", "Key B", 100)
	require.NoError(t, err)

	keyToRevoke, err := db.CreateAPIKey(ctx, account.ID, "sk_live_c", "hash_c", "Key C (revoked)", 100)
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

	key, err := db.CreateAPIKey(ctx, account.ID, "sk_live_del", "hash_del", "Key To Revoke", 100)
	require.NoError(t, err)
	assert.Nil(t, key.RevokedAt)

	// Revoke the key
	err = db.RevokeAPIKey(ctx, key.ID, account.ID)
	require.NoError(t, err)

	// Verify the key is no longer returned by GetAPIKeyByHash
	_, err = db.GetAPIKeyByHash(ctx, "hash_del")
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
	key, err := db.CreateAPIKey(ctx, account1.ID, "sk_live_own", "hash_own", "Owner's Key", 100)
	require.NoError(t, err)

	// Attempt to revoke with account2's ID — should fail
	err = db.RevokeAPIKey(ctx, key.ID, account2.ID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found or already revoked")

	// Verify the key is still active
	found, err := db.GetAPIKeyByHash(ctx, "hash_own")
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

	key, err := db.CreateAPIKey(ctx, account.ID, "sk_live_lu", "hash_lu", "Last Used Key", 100)
	require.NoError(t, err)
	assert.Nil(t, key.LastUsedAt)

	// Update last used
	err = db.UpdateAPIKeyLastUsed(ctx, key.ID)
	require.NoError(t, err)

	// Retrieve and verify last_used_at is set
	found, err := db.GetAPIKeyByHash(ctx, "hash_lu")
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
	_, err = db.CreateAPIKey(ctx, account.ID, "sk_live_c1", "hash_c1", "Key 1", 100)
	require.NoError(t, err)
	_, err = db.CreateAPIKey(ctx, account.ID, "sk_live_c2", "hash_c2", "Key 2", 100)
	require.NoError(t, err)
	keyToRevoke, err := db.CreateAPIKey(ctx, account.ID, "sk_live_c3", "hash_c3", "Key 3", 100)
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
