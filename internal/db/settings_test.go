package db

import (
	"context"
	"testing"

	"stronghold/internal/db/testutil"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetJailbreakDetectionEnabled_DefaultWhenNotSet(t *testing.T) {
	testDB := testutil.NewTestDB(t)
	defer testDB.Close(t)

	db := &DB{pool: testDB.Pool}
	ctx := context.Background()

	account, err := db.CreateAccount(ctx, nil, nil)
	require.NoError(t, err)

	// With default true
	enabled, err := db.GetJailbreakDetectionEnabled(ctx, account.ID, true)
	require.NoError(t, err)
	assert.True(t, enabled, "should return default value when not set")

	// With default false
	enabled, err = db.GetJailbreakDetectionEnabled(ctx, account.ID, false)
	require.NoError(t, err)
	assert.False(t, enabled, "should return default value when not set")
}

func TestSetJailbreakDetectionEnabled_SetsValue(t *testing.T) {
	testDB := testutil.NewTestDB(t)
	defer testDB.Close(t)

	db := &DB{pool: testDB.Pool}
	ctx := context.Background()

	account, err := db.CreateAccount(ctx, nil, nil)
	require.NoError(t, err)

	// Set to true
	err = db.SetJailbreakDetectionEnabled(ctx, account.ID, true)
	require.NoError(t, err)

	enabled, err := db.GetJailbreakDetectionEnabled(ctx, account.ID, false)
	require.NoError(t, err)
	assert.True(t, enabled, "should be true after setting to true")

	// Toggle to false
	err = db.SetJailbreakDetectionEnabled(ctx, account.ID, false)
	require.NoError(t, err)

	enabled, err = db.GetJailbreakDetectionEnabled(ctx, account.ID, true)
	require.NoError(t, err)
	assert.False(t, enabled, "should be false after setting to false, even with default=true")
}

func TestGetJailbreakDetectionEnabled_AfterSetReturnsCorrectValue(t *testing.T) {
	testDB := testutil.NewTestDB(t)
	defer testDB.Close(t)

	db := &DB{pool: testDB.Pool}
	ctx := context.Background()

	account, err := db.CreateAccount(ctx, nil, nil)
	require.NoError(t, err)

	// Initially returns default
	enabled, err := db.GetJailbreakDetectionEnabled(ctx, account.ID, true)
	require.NoError(t, err)
	assert.True(t, enabled)

	// Set explicitly to false
	err = db.SetJailbreakDetectionEnabled(ctx, account.ID, false)
	require.NoError(t, err)

	// Now returns false regardless of default
	enabled, err = db.GetJailbreakDetectionEnabled(ctx, account.ID, true)
	require.NoError(t, err)
	assert.False(t, enabled)

	// Set back to true
	err = db.SetJailbreakDetectionEnabled(ctx, account.ID, true)
	require.NoError(t, err)

	enabled, err = db.GetJailbreakDetectionEnabled(ctx, account.ID, false)
	require.NoError(t, err)
	assert.True(t, enabled)
}

func TestSetJailbreakDetectionEnabled_PreservesOtherMetadata(t *testing.T) {
	testDB := testutil.NewTestDB(t)
	defer testDB.Close(t)

	db := &DB{pool: testDB.Pool}
	ctx := context.Background()

	account, err := db.CreateAccount(ctx, nil, nil)
	require.NoError(t, err)

	// Set jailbreak detection
	err = db.SetJailbreakDetectionEnabled(ctx, account.ID, true)
	require.NoError(t, err)

	// Verify the account metadata still works
	found, err := db.GetAccountByID(ctx, account.ID)
	require.NoError(t, err)
	require.NotNil(t, found.Metadata)

	// The jailbreak_detection_enabled key should be in metadata
	val, ok := found.Metadata["jailbreak_detection_enabled"]
	assert.True(t, ok, "metadata should contain jailbreak_detection_enabled")
	assert.Equal(t, true, val)
}

func TestGetJailbreakDetectionEnabled_IndependentPerAccount(t *testing.T) {
	testDB := testutil.NewTestDB(t)
	defer testDB.Close(t)

	db := &DB{pool: testDB.Pool}
	ctx := context.Background()

	account1, err := db.CreateAccount(ctx, nil, nil)
	require.NoError(t, err)

	account2, err := db.CreateAccount(ctx, nil, nil)
	require.NoError(t, err)

	// Set account1 to true, account2 to false
	err = db.SetJailbreakDetectionEnabled(ctx, account1.ID, true)
	require.NoError(t, err)

	err = db.SetJailbreakDetectionEnabled(ctx, account2.ID, false)
	require.NoError(t, err)

	// Verify they are independent
	enabled1, err := db.GetJailbreakDetectionEnabled(ctx, account1.ID, false)
	require.NoError(t, err)
	assert.True(t, enabled1)

	enabled2, err := db.GetJailbreakDetectionEnabled(ctx, account2.ID, true)
	require.NoError(t, err)
	assert.False(t, enabled2)
}
