package db

import (
	"context"
	"regexp"
	"testing"

	"stronghold/internal/db/testutil"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateAccount_GeneratesUniqueNumber(t *testing.T) {
	testDB := testutil.NewTestDB(t)
	defer testDB.Close(t)

	db := &DB{pool: testDB.Pool}
	ctx := context.Background()

	// Create multiple accounts and verify unique numbers
	accounts := make([]*Account, 5)
	numberSet := make(map[string]bool)

	for i := 0; i < 5; i++ {
		account, err := db.CreateAccount(ctx, nil)
		require.NoError(t, err)
		require.NotNil(t, account)

		accounts[i] = account

		// Verify format: XXXX-XXXX-XXXX-XXXX (16 digits with dashes)
		pattern := regexp.MustCompile(`^\d{4}-\d{4}-\d{4}-\d{4}$`)
		assert.True(t, pattern.MatchString(account.AccountNumber),
			"Account number should match XXXX-XXXX-XXXX-XXXX format, got: %s", account.AccountNumber)

		// Verify uniqueness
		assert.False(t, numberSet[account.AccountNumber],
			"Account number should be unique: %s", account.AccountNumber)
		numberSet[account.AccountNumber] = true

		// Verify cryptographic randomness (not sequential)
		if i > 0 {
			assert.NotEqual(t, accounts[i-1].AccountNumber[:4], account.AccountNumber[:4],
				"First group should not be sequential (likely random)")
		}
	}
}

func TestCreateAccount_WithWallet(t *testing.T) {
	testDB := testutil.NewTestDB(t)
	defer testDB.Close(t)

	db := &DB{pool: testDB.Pool}
	ctx := context.Background()

	t.Run("valid Ethereum address", func(t *testing.T) {
		wallet := "0x1234567890abcdef1234567890abcdef12345678"
		account, err := db.CreateAccount(ctx, &wallet)
		require.NoError(t, err)
		require.NotNil(t, account)
		require.NotNil(t, account.WalletAddress)
		assert.Equal(t, wallet, *account.WalletAddress)
	})

	t.Run("uppercase hex digits valid", func(t *testing.T) {
		wallet := "0xABCDEF1234567890ABCDEF1234567890ABCDEF12"
		account, err := db.CreateAccount(ctx, &wallet)
		require.NoError(t, err)
		require.NotNil(t, account.WalletAddress)
		assert.Equal(t, wallet, *account.WalletAddress)
	})

	t.Run("mixed case valid", func(t *testing.T) {
		wallet := "0xAbCdEf1234567890AbCdEf1234567890AbCdEf12"
		account, err := db.CreateAccount(ctx, &wallet)
		require.NoError(t, err)
		require.NotNil(t, account.WalletAddress)
	})

	t.Run("nil wallet creates account without wallet", func(t *testing.T) {
		account, err := db.CreateAccount(ctx, nil)
		require.NoError(t, err)
		assert.Nil(t, account.WalletAddress)
	})
}

func TestGetAccountByNumber_Normalized(t *testing.T) {
	testDB := testutil.NewTestDB(t)
	defer testDB.Close(t)

	db := &DB{pool: testDB.Pool}
	ctx := context.Background()

	// Create an account
	account, err := db.CreateAccount(ctx, nil)
	require.NoError(t, err)

	// Get the raw 16 digits without dashes
	digitsOnly := ""
	for _, c := range account.AccountNumber {
		if c >= '0' && c <= '9' {
			digitsOnly += string(c)
		}
	}
	require.Len(t, digitsOnly, 16)

	testCases := []struct {
		name  string
		input string
	}{
		{"with dashes", account.AccountNumber},
		{"without dashes", digitsOnly},
		{"with spaces", digitsOnly[:4] + " " + digitsOnly[4:8] + " " + digitsOnly[8:12] + " " + digitsOnly[12:]},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			found, err := db.GetAccountByNumber(ctx, tc.input)
			require.NoError(t, err)
			require.NotNil(t, found)
			assert.Equal(t, account.ID, found.ID)
			assert.Equal(t, account.AccountNumber, found.AccountNumber)
		})
	}
}

func TestLinkWallet_RejectsAlreadyLinked(t *testing.T) {
	testDB := testutil.NewTestDB(t)
	defer testDB.Close(t)

	db := &DB{pool: testDB.Pool}
	ctx := context.Background()

	// Create first account with wallet
	wallet := "0x1234567890abcdef1234567890abcdef12345678"
	account1, err := db.CreateAccount(ctx, &wallet)
	require.NoError(t, err)

	// Create second account without wallet
	account2, err := db.CreateAccount(ctx, nil)
	require.NoError(t, err)

	// Try to link the same wallet to second account - should fail
	err = db.LinkWallet(ctx, account2.ID, wallet)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already linked to another account")

	// Verify first account still has the wallet
	found, err := db.GetAccountByID(ctx, account1.ID)
	require.NoError(t, err)
	require.NotNil(t, found.WalletAddress)
	assert.Equal(t, wallet, *found.WalletAddress)
}

func TestLinkWallet_AllowsRelinking(t *testing.T) {
	testDB := testutil.NewTestDB(t)
	defer testDB.Close(t)

	db := &DB{pool: testDB.Pool}
	ctx := context.Background()

	// Create account without wallet
	account, err := db.CreateAccount(ctx, nil)
	require.NoError(t, err)

	// Link first wallet
	wallet1 := "0x1111111111111111111111111111111111111111"
	err = db.LinkWallet(ctx, account.ID, wallet1)
	require.NoError(t, err)

	// Verify link
	found, err := db.GetAccountByID(ctx, account.ID)
	require.NoError(t, err)
	require.NotNil(t, found.WalletAddress)
	assert.Equal(t, wallet1, *found.WalletAddress)

	// Link same wallet again (should succeed - same account)
	err = db.LinkWallet(ctx, account.ID, wallet1)
	require.NoError(t, err)
}

func TestLinkWallet_InvalidFormat(t *testing.T) {
	testDB := testutil.NewTestDB(t)
	defer testDB.Close(t)

	db := &DB{pool: testDB.Pool}
	ctx := context.Background()

	account, err := db.CreateAccount(ctx, nil)
	require.NoError(t, err)

	invalidAddresses := []struct {
		name    string
		address string
	}{
		{"too short", "0x1234"},
		{"too long", "0x1234567890abcdef1234567890abcdef123456789"},
		{"missing 0x prefix", "1234567890abcdef1234567890abcdef12345678"},
		{"invalid chars", "0xGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGG"},
	}

	for _, tc := range invalidAddresses {
		t.Run(tc.name, func(t *testing.T) {
			err := db.LinkWallet(ctx, account.ID, tc.address)
			// Database constraint should reject invalid format
			require.Error(t, err)
		})
	}
}

func TestGetAccountByWalletAddress(t *testing.T) {
	testDB := testutil.NewTestDB(t)
	defer testDB.Close(t)

	db := &DB{pool: testDB.Pool}
	ctx := context.Background()

	wallet := "0xabcdef1234567890abcdef1234567890abcdef12"
	account, err := db.CreateAccount(ctx, &wallet)
	require.NoError(t, err)

	// Find by wallet
	found, err := db.GetAccountByWalletAddress(ctx, wallet)
	require.NoError(t, err)
	assert.Equal(t, account.ID, found.ID)
}

func TestAccountStatus(t *testing.T) {
	testDB := testutil.NewTestDB(t)
	defer testDB.Close(t)

	db := &DB{pool: testDB.Pool}
	ctx := context.Background()

	account, err := db.CreateAccount(ctx, nil)
	require.NoError(t, err)
	assert.Equal(t, AccountStatusActive, account.Status)

	t.Run("suspend account", func(t *testing.T) {
		err := db.SuspendAccount(ctx, account.ID)
		require.NoError(t, err)

		found, err := db.GetAccountByID(ctx, account.ID)
		require.NoError(t, err)
		assert.Equal(t, AccountStatusSuspended, found.Status)
	})

	t.Run("close account", func(t *testing.T) {
		err := db.CloseAccount(ctx, account.ID)
		require.NoError(t, err)

		found, err := db.GetAccountByID(ctx, account.ID)
		require.NoError(t, err)
		assert.Equal(t, AccountStatusClosed, found.Status)
	})
}

func TestUpdateBalance(t *testing.T) {
	testDB := testutil.NewTestDB(t)
	defer testDB.Close(t)

	db := &DB{pool: testDB.Pool}
	ctx := context.Background()

	account, err := db.CreateAccount(ctx, nil)
	require.NoError(t, err)
	assert.True(t, decimal.Zero.Equal(account.BalanceUSDC))

	// Update balance
	newBalance := decimal.NewFromFloat(100.50)
	err = db.UpdateBalance(ctx, account.ID, newBalance)
	require.NoError(t, err)

	found, err := db.GetAccountByID(ctx, account.ID)
	require.NoError(t, err)
	assert.True(t, newBalance.Equal(found.BalanceUSDC))
}

func TestAccountExists(t *testing.T) {
	testDB := testutil.NewTestDB(t)
	defer testDB.Close(t)

	db := &DB{pool: testDB.Pool}
	ctx := context.Background()

	account, err := db.CreateAccount(ctx, nil)
	require.NoError(t, err)

	// Should exist
	exists, err := db.AccountExists(ctx, account.AccountNumber)
	require.NoError(t, err)
	assert.True(t, exists)

	// Non-existent account
	exists, err = db.AccountExists(ctx, "0000-0000-0000-0000")
	require.NoError(t, err)
	assert.False(t, exists)
}

func TestUpdateLastLogin(t *testing.T) {
	testDB := testutil.NewTestDB(t)
	defer testDB.Close(t)

	db := &DB{pool: testDB.Pool}
	ctx := context.Background()

	account, err := db.CreateAccount(ctx, nil)
	require.NoError(t, err)
	assert.Nil(t, account.LastLoginAt)

	// Update last login
	err = db.UpdateLastLogin(ctx, account.ID)
	require.NoError(t, err)

	found, err := db.GetAccountByID(ctx, account.ID)
	require.NoError(t, err)
	assert.NotNil(t, found.LastLoginAt)
}

func TestGetAccountByID_NotFound(t *testing.T) {
	testDB := testutil.NewTestDB(t)
	defer testDB.Close(t)

	db := &DB{pool: testDB.Pool}
	ctx := context.Background()

	_, err := db.GetAccountByID(ctx, uuid.New())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestGetAccountByNumber_NotFound(t *testing.T) {
	testDB := testutil.NewTestDB(t)
	defer testDB.Close(t)

	db := &DB{pool: testDB.Pool}
	ctx := context.Background()

	_, err := db.GetAccountByNumber(ctx, "9999-9999-9999-9999")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}
