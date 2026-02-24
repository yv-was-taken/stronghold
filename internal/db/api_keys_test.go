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

func TestCreateAPIKey_ReturnsCorrectFormat(t *testing.T) {
	testDB := testutil.NewTestDB(t)
	defer testDB.Close(t)

	db := &DB{pool: testDB.Pool}
	ctx := context.Background()

	account, err := db.CreateAccount(ctx, nil, nil)
	require.NoError(t, err)

	apiKey, rawKey, err := db.CreateAPIKey(ctx, account.ID, "test key")
	require.NoError(t, err)
	require.NotNil(t, apiKey)

	// Raw key should start with sh_live_
	assert.True(t, strings.HasPrefix(rawKey, "sh_live_"),
		"raw key should start with sh_live_, got: %s", rawKey)

	// sh_live_ (8 chars) + 64 hex chars (32 bytes) = 72 chars total
	assert.Len(t, rawKey, 72, "raw key should be 72 characters")

	// Key prefix should be first 12 chars of raw key
	assert.Equal(t, rawKey[:12], apiKey.KeyPrefix)

	// Metadata fields
	assert.Equal(t, "test key", apiKey.Label)
	assert.Equal(t, account.ID, apiKey.AccountID)
	assert.NotEqual(t, uuid.Nil, apiKey.ID)
	assert.Nil(t, apiKey.LastUsedAt)
	assert.Nil(t, apiKey.RevokedAt)
}

func TestCreateAPIKey_MultipleKeysForSameAccount(t *testing.T) {
	testDB := testutil.NewTestDB(t)
	defer testDB.Close(t)

	db := &DB{pool: testDB.Pool}
	ctx := context.Background()

	account, err := db.CreateAccount(ctx, nil, nil)
	require.NoError(t, err)

	key1, rawKey1, err := db.CreateAPIKey(ctx, account.ID, "key one")
	require.NoError(t, err)

	key2, rawKey2, err := db.CreateAPIKey(ctx, account.ID, "key two")
	require.NoError(t, err)

	// Keys should be different
	assert.NotEqual(t, key1.ID, key2.ID)
	assert.NotEqual(t, rawKey1, rawKey2)
}

func TestGetAPIKeyByHash_Success(t *testing.T) {
	testDB := testutil.NewTestDB(t)
	defer testDB.Close(t)

	db := &DB{pool: testDB.Pool}
	ctx := context.Background()

	account, err := db.CreateAccount(ctx, nil, nil)
	require.NoError(t, err)

	_, rawKey, err := db.CreateAPIKey(ctx, account.ID, "lookup test")
	require.NoError(t, err)

	// Look up by hash
	keyHash := HashToken(rawKey)
	found, err := db.GetAPIKeyByHash(ctx, keyHash)
	require.NoError(t, err)
	require.NotNil(t, found)

	assert.Equal(t, account.ID, found.AccountID)
	assert.Equal(t, "lookup test", found.Label)

	// last_used_at should have been updated
	assert.NotNil(t, found.LastUsedAt, "GetAPIKeyByHash should update last_used_at")
}

func TestGetAPIKeyByHash_RevokedKeyReturnsError(t *testing.T) {
	testDB := testutil.NewTestDB(t)
	defer testDB.Close(t)

	db := &DB{pool: testDB.Pool}
	ctx := context.Background()

	account, err := db.CreateAccount(ctx, nil, nil)
	require.NoError(t, err)

	apiKey, rawKey, err := db.CreateAPIKey(ctx, account.ID, "revoke test")
	require.NoError(t, err)

	// Revoke the key
	err = db.RevokeAPIKey(ctx, account.ID, apiKey.ID)
	require.NoError(t, err)

	// Lookup should fail
	keyHash := HashToken(rawKey)
	_, err = db.GetAPIKeyByHash(ctx, keyHash)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrAPIKeyNotFound)
}

func TestGetAPIKeyByHash_NonExistentHash(t *testing.T) {
	testDB := testutil.NewTestDB(t)
	defer testDB.Close(t)

	db := &DB{pool: testDB.Pool}
	ctx := context.Background()

	_, err := db.GetAPIKeyByHash(ctx, "nonexistenthash0000000000000000000000000000000000000000000000000000")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrAPIKeyNotFound)
}

func TestListAPIKeys_ReturnsAllIncludingRevoked(t *testing.T) {
	testDB := testutil.NewTestDB(t)
	defer testDB.Close(t)

	db := &DB{pool: testDB.Pool}
	ctx := context.Background()

	account, err := db.CreateAccount(ctx, nil, nil)
	require.NoError(t, err)

	// Create two keys
	key1, _, err := db.CreateAPIKey(ctx, account.ID, "active key")
	require.NoError(t, err)

	key2, _, err := db.CreateAPIKey(ctx, account.ID, "revoked key")
	require.NoError(t, err)

	// Revoke the second key
	err = db.RevokeAPIKey(ctx, account.ID, key2.ID)
	require.NoError(t, err)

	// List should return both
	keys, err := db.ListAPIKeys(ctx, account.ID)
	require.NoError(t, err)
	assert.Len(t, keys, 2)

	// Check that we can find both keys and their states
	foundActive := false
	foundRevoked := false
	for _, k := range keys {
		if k.ID == key1.ID {
			foundActive = true
			assert.Nil(t, k.RevokedAt, "active key should not be revoked")
		}
		if k.ID == key2.ID {
			foundRevoked = true
			assert.NotNil(t, k.RevokedAt, "revoked key should have revoked_at set")
		}
	}
	assert.True(t, foundActive, "active key should be in the list")
	assert.True(t, foundRevoked, "revoked key should be in the list")
}

func TestListAPIKeys_EmptyForNewAccount(t *testing.T) {
	testDB := testutil.NewTestDB(t)
	defer testDB.Close(t)

	db := &DB{pool: testDB.Pool}
	ctx := context.Background()

	account, err := db.CreateAccount(ctx, nil, nil)
	require.NoError(t, err)

	keys, err := db.ListAPIKeys(ctx, account.ID)
	require.NoError(t, err)
	assert.Empty(t, keys)
}

func TestRevokeAPIKey_SetsRevokedAt(t *testing.T) {
	testDB := testutil.NewTestDB(t)
	defer testDB.Close(t)

	db := &DB{pool: testDB.Pool}
	ctx := context.Background()

	account, err := db.CreateAccount(ctx, nil, nil)
	require.NoError(t, err)

	apiKey, _, err := db.CreateAPIKey(ctx, account.ID, "to revoke")
	require.NoError(t, err)

	err = db.RevokeAPIKey(ctx, account.ID, apiKey.ID)
	require.NoError(t, err)

	// Verify via list
	keys, err := db.ListAPIKeys(ctx, account.ID)
	require.NoError(t, err)
	require.Len(t, keys, 1)
	assert.NotNil(t, keys[0].RevokedAt)
}

func TestRevokeAPIKey_WrongAccountReturnsError(t *testing.T) {
	testDB := testutil.NewTestDB(t)
	defer testDB.Close(t)

	db := &DB{pool: testDB.Pool}
	ctx := context.Background()

	account1, err := db.CreateAccount(ctx, nil, nil)
	require.NoError(t, err)

	account2, err := db.CreateAccount(ctx, nil, nil)
	require.NoError(t, err)

	// Create key for account1
	apiKey, _, err := db.CreateAPIKey(ctx, account1.ID, "account1 key")
	require.NoError(t, err)

	// Try to revoke from account2
	err = db.RevokeAPIKey(ctx, account2.ID, apiKey.ID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found or already revoked")
}

func TestRevokeAPIKey_AlreadyRevokedReturnsError(t *testing.T) {
	testDB := testutil.NewTestDB(t)
	defer testDB.Close(t)

	db := &DB{pool: testDB.Pool}
	ctx := context.Background()

	account, err := db.CreateAccount(ctx, nil, nil)
	require.NoError(t, err)

	apiKey, _, err := db.CreateAPIKey(ctx, account.ID, "double revoke")
	require.NoError(t, err)

	// First revoke
	err = db.RevokeAPIKey(ctx, account.ID, apiKey.ID)
	require.NoError(t, err)

	// Second revoke should fail
	err = db.RevokeAPIKey(ctx, account.ID, apiKey.ID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found or already revoked")
}

func TestHasActiveAPIKeys_TrueWhenActiveKeysExist(t *testing.T) {
	testDB := testutil.NewTestDB(t)
	defer testDB.Close(t)

	db := &DB{pool: testDB.Pool}
	ctx := context.Background()

	account, err := db.CreateAccount(ctx, nil, nil)
	require.NoError(t, err)

	// No keys yet
	hasKeys, err := db.HasActiveAPIKeys(ctx, account.ID)
	require.NoError(t, err)
	assert.False(t, hasKeys)

	// Create a key
	_, _, err = db.CreateAPIKey(ctx, account.ID, "active")
	require.NoError(t, err)

	hasKeys, err = db.HasActiveAPIKeys(ctx, account.ID)
	require.NoError(t, err)
	assert.True(t, hasKeys)
}

func TestHasActiveAPIKeys_FalseAfterAllRevoked(t *testing.T) {
	testDB := testutil.NewTestDB(t)
	defer testDB.Close(t)

	db := &DB{pool: testDB.Pool}
	ctx := context.Background()

	account, err := db.CreateAccount(ctx, nil, nil)
	require.NoError(t, err)

	key1, _, err := db.CreateAPIKey(ctx, account.ID, "key1")
	require.NoError(t, err)

	key2, _, err := db.CreateAPIKey(ctx, account.ID, "key2")
	require.NoError(t, err)

	// Revoke both
	err = db.RevokeAPIKey(ctx, account.ID, key1.ID)
	require.NoError(t, err)
	err = db.RevokeAPIKey(ctx, account.ID, key2.ID)
	require.NoError(t, err)

	hasKeys, err := db.HasActiveAPIKeys(ctx, account.ID)
	require.NoError(t, err)
	assert.False(t, hasKeys, "should be false after all keys are revoked")
}
