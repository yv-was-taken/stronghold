package db

import (
	"context"
	"errors"
	"regexp"
	"testing"

	"stronghold/internal/db/testutil"
	"stronghold/internal/usdc"

	"github.com/google/uuid"
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
		account, err := db.CreateAccount(ctx, nil, nil)
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

func TestCreateAccount_WithEVMWallet(t *testing.T) {
	testDB := testutil.NewTestDB(t)
	defer testDB.Close(t)

	db := &DB{pool: testDB.Pool}
	ctx := context.Background()

	t.Run("valid Ethereum address", func(t *testing.T) {
		wallet := "0x1234567890abcdef1234567890abcdef12345678"
		account, err := db.CreateAccount(ctx, &wallet, nil)
		require.NoError(t, err)
		require.NotNil(t, account)
		require.NotNil(t, account.EVMWalletAddress)
		assert.Equal(t, wallet, *account.EVMWalletAddress)
		assert.Nil(t, account.SolanaWalletAddress)
	})

	t.Run("uppercase hex digits valid", func(t *testing.T) {
		wallet := "0xABCDEF1234567890ABCDEF1234567890ABCDEF12"
		account, err := db.CreateAccount(ctx, &wallet, nil)
		require.NoError(t, err)
		require.NotNil(t, account.EVMWalletAddress)
		assert.Equal(t, wallet, *account.EVMWalletAddress)
	})

	t.Run("mixed case valid", func(t *testing.T) {
		wallet := "0xAbCdEf1234567890AbCdEf1234567890AbCdEf12"
		account, err := db.CreateAccount(ctx, &wallet, nil)
		require.NoError(t, err)
		require.NotNil(t, account.EVMWalletAddress)
	})

	t.Run("nil wallet creates account without wallet", func(t *testing.T) {
		account, err := db.CreateAccount(ctx, nil, nil)
		require.NoError(t, err)
		assert.Nil(t, account.EVMWalletAddress)
		assert.Nil(t, account.SolanaWalletAddress)
	})
}

func TestCreateAccount_WithSolanaWallet(t *testing.T) {
	testDB := testutil.NewTestDB(t)
	defer testDB.Close(t)

	db := &DB{pool: testDB.Pool}
	ctx := context.Background()

	t.Run("valid Solana address", func(t *testing.T) {
		wallet := "9WzDXwBbmkg8ZTbNMqUxvQRAyrZzDsGYdLVL9zYtAWWM"
		account, err := db.CreateAccount(ctx, nil, &wallet)
		require.NoError(t, err)
		require.NotNil(t, account)
		require.NotNil(t, account.SolanaWalletAddress)
		assert.Equal(t, wallet, *account.SolanaWalletAddress)
		assert.Nil(t, account.EVMWalletAddress)
	})

	t.Run("both wallets", func(t *testing.T) {
		evmWallet := "0xdeadbeefdeadbeefdeadbeefdeadbeefdeadbeef"
		solWallet := "4fYNw3dojWmQ4dXtSGE9epjRGy9pFSx62YypT7avPYvA"
		account, err := db.CreateAccount(ctx, &evmWallet, &solWallet)
		require.NoError(t, err)
		require.NotNil(t, account.EVMWalletAddress)
		require.NotNil(t, account.SolanaWalletAddress)
		assert.Equal(t, evmWallet, *account.EVMWalletAddress)
		assert.Equal(t, solWallet, *account.SolanaWalletAddress)
	})
}

func TestGetAccountByNumber_Normalized(t *testing.T) {
	testDB := testutil.NewTestDB(t)
	defer testDB.Close(t)

	db := &DB{pool: testDB.Pool}
	ctx := context.Background()

	// Create an account
	account, err := db.CreateAccount(ctx, nil, nil)
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

func TestLinkWallet_EVM_RejectsAlreadyLinked(t *testing.T) {
	testDB := testutil.NewTestDB(t)
	defer testDB.Close(t)

	db := &DB{pool: testDB.Pool}
	ctx := context.Background()

	// Create first account with EVM wallet
	wallet := "0x1234567890abcdef1234567890abcdef12345678"
	account1, err := db.CreateAccount(ctx, &wallet, nil)
	require.NoError(t, err)

	// Create second account without wallet
	account2, err := db.CreateAccount(ctx, nil, nil)
	require.NoError(t, err)

	// Try to link the same wallet to second account - should fail
	err = db.LinkEVMWallet(ctx, account2.ID, wallet)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already linked to another account")

	// Verify first account still has the wallet
	found, err := db.GetAccountByID(ctx, account1.ID)
	require.NoError(t, err)
	require.NotNil(t, found.EVMWalletAddress)
	assert.Equal(t, wallet, *found.EVMWalletAddress)
}

func TestLinkWallet_Solana_RejectsAlreadyLinked(t *testing.T) {
	testDB := testutil.NewTestDB(t)
	defer testDB.Close(t)

	db := &DB{pool: testDB.Pool}
	ctx := context.Background()

	// Create first account with Solana wallet
	wallet := "9WzDXwBbmkg8ZTbNMqUxvQRAyrZzDsGYdLVL9zYtAWWM"
	account1, err := db.CreateAccount(ctx, nil, &wallet)
	require.NoError(t, err)

	// Create second account without wallet
	account2, err := db.CreateAccount(ctx, nil, nil)
	require.NoError(t, err)

	// Try to link the same wallet to second account - should fail
	err = db.LinkSolanaWallet(ctx, account2.ID, wallet)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already linked to another account")

	// Verify first account still has the wallet
	found, err := db.GetAccountByID(ctx, account1.ID)
	require.NoError(t, err)
	require.NotNil(t, found.SolanaWalletAddress)
	assert.Equal(t, wallet, *found.SolanaWalletAddress)
}

func TestLinkWallet_AllowsRelinking(t *testing.T) {
	testDB := testutil.NewTestDB(t)
	defer testDB.Close(t)

	db := &DB{pool: testDB.Pool}
	ctx := context.Background()

	// Create account without wallet
	account, err := db.CreateAccount(ctx, nil, nil)
	require.NoError(t, err)

	// Link EVM wallet
	wallet1 := "0x1111111111111111111111111111111111111111"
	err = db.LinkEVMWallet(ctx, account.ID, wallet1)
	require.NoError(t, err)

	// Verify link
	found, err := db.GetAccountByID(ctx, account.ID)
	require.NoError(t, err)
	require.NotNil(t, found.EVMWalletAddress)
	assert.Equal(t, wallet1, *found.EVMWalletAddress)

	// Link same wallet again (should succeed - same account)
	err = db.LinkEVMWallet(ctx, account.ID, wallet1)
	require.NoError(t, err)
}

func TestLinkWallet_AutoDetectsChain(t *testing.T) {
	testDB := testutil.NewTestDB(t)
	defer testDB.Close(t)

	db := &DB{pool: testDB.Pool}
	ctx := context.Background()

	account, err := db.CreateAccount(ctx, nil, nil)
	require.NoError(t, err)

	// LinkWallet with 0x prefix should go to EVM
	evmAddr := "0x2222222222222222222222222222222222222222"
	err = db.LinkWallet(ctx, account.ID, evmAddr)
	require.NoError(t, err)

	found, err := db.GetAccountByID(ctx, account.ID)
	require.NoError(t, err)
	require.NotNil(t, found.EVMWalletAddress)
	assert.Equal(t, evmAddr, *found.EVMWalletAddress)

	// LinkWallet without 0x prefix should go to Solana
	solAddr := "9WzDXwBbmkg8ZTbNMqUxvQRAyrZzDsGYdLVL9zYtAWWM"
	err = db.LinkWallet(ctx, account.ID, solAddr)
	require.NoError(t, err)

	found, err = db.GetAccountByID(ctx, account.ID)
	require.NoError(t, err)
	require.NotNil(t, found.SolanaWalletAddress)
	assert.Equal(t, solAddr, *found.SolanaWalletAddress)
}

func TestLinkWallet_InvalidFormat(t *testing.T) {
	testDB := testutil.NewTestDB(t)
	defer testDB.Close(t)

	db := &DB{pool: testDB.Pool}
	ctx := context.Background()

	account, err := db.CreateAccount(ctx, nil, nil)
	require.NoError(t, err)

	invalidAddresses := []struct {
		name    string
		address string
	}{
		{"too short EVM", "0x1234"},
		{"too long EVM", "0x1234567890abcdef1234567890abcdef123456789"},
		{"invalid chars EVM", "0xGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGG"},
	}

	for _, tc := range invalidAddresses {
		t.Run(tc.name, func(t *testing.T) {
			err := db.LinkEVMWallet(ctx, account.ID, tc.address)
			// Database constraint should reject invalid format
			require.Error(t, err)
		})
	}
}

func TestGetAccountByEVMWallet(t *testing.T) {
	testDB := testutil.NewTestDB(t)
	defer testDB.Close(t)

	db := &DB{pool: testDB.Pool}
	ctx := context.Background()

	wallet := "0xabcdef1234567890abcdef1234567890abcdef12"
	account, err := db.CreateAccount(ctx, &wallet, nil)
	require.NoError(t, err)

	// Find by EVM wallet
	found, err := db.GetAccountByEVMWallet(ctx, wallet)
	require.NoError(t, err)
	assert.Equal(t, account.ID, found.ID)

	// Also test via auto-detect
	found, err = db.GetAccountByWalletAddress(ctx, wallet)
	require.NoError(t, err)
	assert.Equal(t, account.ID, found.ID)
}

func TestGetAccountBySolanaWallet(t *testing.T) {
	testDB := testutil.NewTestDB(t)
	defer testDB.Close(t)

	db := &DB{pool: testDB.Pool}
	ctx := context.Background()

	wallet := "4fYNw3dojWmQ4dXtSGE9epjRGy9pFSx62YypT7avPYvA"
	account, err := db.CreateAccount(ctx, nil, &wallet)
	require.NoError(t, err)

	// Find by Solana wallet
	found, err := db.GetAccountBySolanaWallet(ctx, wallet)
	require.NoError(t, err)
	assert.Equal(t, account.ID, found.ID)

	// Also test via auto-detect
	found, err = db.GetAccountByWalletAddress(ctx, wallet)
	require.NoError(t, err)
	assert.Equal(t, account.ID, found.ID)
}

func TestUpdateWalletAddresses(t *testing.T) {
	testDB := testutil.NewTestDB(t)
	defer testDB.Close(t)

	db := &DB{pool: testDB.Pool}
	ctx := context.Background()

	account, err := db.CreateAccount(ctx, nil, nil)
	require.NoError(t, err)

	t.Run("update EVM only", func(t *testing.T) {
		evmAddr := "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
		err := db.UpdateWalletAddresses(ctx, account.ID, &evmAddr, nil)
		require.NoError(t, err)

		found, err := db.GetAccountByID(ctx, account.ID)
		require.NoError(t, err)
		require.NotNil(t, found.EVMWalletAddress)
		assert.Equal(t, evmAddr, *found.EVMWalletAddress)
		assert.Nil(t, found.SolanaWalletAddress)
	})

	t.Run("update Solana only", func(t *testing.T) {
		solAddr := "9WzDXwBbmkg8ZTbNMqUxvQRAyrZzDsGYdLVL9zYtAWWM"
		err := db.UpdateWalletAddresses(ctx, account.ID, nil, &solAddr)
		require.NoError(t, err)

		found, err := db.GetAccountByID(ctx, account.ID)
		require.NoError(t, err)
		require.NotNil(t, found.EVMWalletAddress) // Should still be set from previous subtest
		require.NotNil(t, found.SolanaWalletAddress)
		assert.Equal(t, solAddr, *found.SolanaWalletAddress)
	})

	t.Run("update both", func(t *testing.T) {
		evmAddr := "0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
		solAddr := "4fYNw3dojWmQ4dXtSGE9epjRGy9pFSx62YypT7avPYvA"
		err := db.UpdateWalletAddresses(ctx, account.ID, &evmAddr, &solAddr)
		require.NoError(t, err)

		found, err := db.GetAccountByID(ctx, account.ID)
		require.NoError(t, err)
		require.NotNil(t, found.EVMWalletAddress)
		require.NotNil(t, found.SolanaWalletAddress)
		assert.Equal(t, evmAddr, *found.EVMWalletAddress)
		assert.Equal(t, solAddr, *found.SolanaWalletAddress)
	})

	t.Run("nil both is no-op", func(t *testing.T) {
		err := db.UpdateWalletAddresses(ctx, account.ID, nil, nil)
		require.NoError(t, err)
	})

	t.Run("returns conflict error when EVM is already linked", func(t *testing.T) {
		occupiedEVM := "0x1234567890abcdef1234567890abcdef12345678"
		_, err := db.CreateAccount(ctx, &occupiedEVM, nil)
		require.NoError(t, err)

		err = db.UpdateWalletAddresses(ctx, account.ID, &occupiedEVM, nil)
		require.Error(t, err)
		assert.True(t, errors.Is(err, ErrEVMWalletAddressConflict))
	})

	t.Run("returns validation error for empty EVM input", func(t *testing.T) {
		empty := "   "
		err := db.UpdateWalletAddresses(ctx, account.ID, &empty, nil)
		require.Error(t, err)
		assert.True(t, errors.Is(err, ErrInvalidEVMWalletAddress))
	})
}

func TestAccountStatus(t *testing.T) {
	testDB := testutil.NewTestDB(t)
	defer testDB.Close(t)

	db := &DB{pool: testDB.Pool}
	ctx := context.Background()

	account, err := db.CreateAccount(ctx, nil, nil)
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

	account, err := db.CreateAccount(ctx, nil, nil)
	require.NoError(t, err)
	assert.Equal(t, usdc.MicroUSDC(0), account.BalanceUSDC)

	// Update balance
	err = db.UpdateBalance(ctx, account.ID, usdc.FromFloat(100.50))
	require.NoError(t, err)

	found, err := db.GetAccountByID(ctx, account.ID)
	require.NoError(t, err)
	assert.Equal(t, usdc.FromFloat(100.50), found.BalanceUSDC)
}

func TestAccountExists(t *testing.T) {
	testDB := testutil.NewTestDB(t)
	defer testDB.Close(t)

	db := &DB{pool: testDB.Pool}
	ctx := context.Background()

	account, err := db.CreateAccount(ctx, nil, nil)
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

	account, err := db.CreateAccount(ctx, nil, nil)
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

func TestCreateB2BAccount(t *testing.T) {
	testDB := testutil.NewTestDB(t)
	defer testDB.Close(t)

	db := &DB{pool: testDB.Pool}
	ctx := context.Background()

	workosUserID := "user_01EXAMPLE"
	email := "test@example.com"
	companyName := "Test Company"

	account, err := db.CreateB2BAccount(ctx, workosUserID, email, companyName)
	require.NoError(t, err)
	require.NotNil(t, account)

	// Verify account type
	assert.Equal(t, AccountTypeB2B, account.AccountType)

	// Verify email is set
	require.NotNil(t, account.Email)
	assert.Equal(t, email, *account.Email)

	// Verify company name is set
	require.NotNil(t, account.CompanyName)
	assert.Equal(t, companyName, *account.CompanyName)

	// Verify WorkOS user ID is set
	require.NotNil(t, account.WorkOSUserID)
	assert.Equal(t, workosUserID, *account.WorkOSUserID)

	// B2B accounts should not have an account_number
	assert.Empty(t, account.AccountNumber)

	// Verify default fields
	assert.Equal(t, usdc.MicroUSDC(0), account.BalanceUSDC)
	assert.Equal(t, AccountStatusActive, account.Status)
	assert.NotZero(t, account.CreatedAt)
	assert.NotZero(t, account.UpdatedAt)
	assert.NotEqual(t, uuid.Nil, account.ID)

	// Verify the account can be retrieved from DB
	found, err := db.GetAccountByID(ctx, account.ID)
	require.NoError(t, err)
	assert.Equal(t, AccountTypeB2B, found.AccountType)
	require.NotNil(t, found.Email)
	assert.Equal(t, email, *found.Email)
	require.NotNil(t, found.CompanyName)
	assert.Equal(t, companyName, *found.CompanyName)
}

func TestCreateB2BAccount_EmptyCompanyName(t *testing.T) {
	testDB := testutil.NewTestDB(t)
	defer testDB.Close(t)

	db := &DB{pool: testDB.Pool}
	ctx := context.Background()

	account, err := db.CreateB2BAccount(ctx, "user_01NOCO", "noco@example.com", "")
	require.NoError(t, err)
	require.NotNil(t, account)
	assert.Nil(t, account.CompanyName, "empty company name should store as NULL")
}

func TestGetAccountByWorkOSUserID(t *testing.T) {
	testDB := testutil.NewTestDB(t)
	defer testDB.Close(t)

	db := &DB{pool: testDB.Pool}
	ctx := context.Background()

	workosUserID := "user_01LOOKUP"
	email := "workos@example.com"

	account, err := db.CreateB2BAccount(ctx, workosUserID, email, "WorkOS Co")
	require.NoError(t, err)

	found, err := db.GetAccountByWorkOSUserID(ctx, workosUserID)
	require.NoError(t, err)
	require.NotNil(t, found)
	assert.Equal(t, account.ID, found.ID)
	assert.Equal(t, AccountTypeB2B, found.AccountType)
	require.NotNil(t, found.WorkOSUserID)
	assert.Equal(t, workosUserID, *found.WorkOSUserID)
}

func TestGetAccountByWorkOSUserID_NotFound(t *testing.T) {
	testDB := testutil.NewTestDB(t)
	defer testDB.Close(t)

	db := &DB{pool: testDB.Pool}
	ctx := context.Background()

	_, err := db.GetAccountByWorkOSUserID(ctx, "user_01NONEXISTENT")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrAccountNotFound)
}

func TestUpdateCompanyName(t *testing.T) {
	testDB := testutil.NewTestDB(t)
	defer testDB.Close(t)

	db := &DB{pool: testDB.Pool}
	ctx := context.Background()

	account, err := db.CreateB2BAccount(ctx, "user_01ONBOARD", "onboard@example.com", "")
	require.NoError(t, err)
	assert.Nil(t, account.CompanyName)

	err = db.UpdateCompanyName(ctx, account.ID, "Acme Corp")
	require.NoError(t, err)

	found, err := db.GetAccountByID(ctx, account.ID)
	require.NoError(t, err)
	require.NotNil(t, found.CompanyName)
	assert.Equal(t, "Acme Corp", *found.CompanyName)
}

func TestCreateB2BAccount_DuplicateEmail(t *testing.T) {
	testDB := testutil.NewTestDB(t)
	defer testDB.Close(t)

	db := &DB{pool: testDB.Pool}
	ctx := context.Background()

	email := "duplicate@example.com"

	// Create first account
	_, err := db.CreateB2BAccount(ctx, "user_01DUP1", email, "Test Company")
	require.NoError(t, err)

	// Attempt to create second account with same email
	_, err = db.CreateB2BAccount(ctx, "user_01DUP2", email, "Another Company")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrEmailAlreadyExists)
}

func TestGetAccountByEmail(t *testing.T) {
	testDB := testutil.NewTestDB(t)
	defer testDB.Close(t)

	db := &DB{pool: testDB.Pool}
	ctx := context.Background()

	email := "lookup@example.com"

	account, err := db.CreateB2BAccount(ctx, "user_01EMAIL", email, "Test Company")
	require.NoError(t, err)

	// Lookup by email
	found, err := db.GetAccountByEmail(ctx, email)
	require.NoError(t, err)
	require.NotNil(t, found)
	assert.Equal(t, account.ID, found.ID)
	assert.Equal(t, AccountTypeB2B, found.AccountType)
	require.NotNil(t, found.Email)
	assert.Equal(t, email, *found.Email)
}

func TestGetAccountByEmail_NotFound(t *testing.T) {
	testDB := testutil.NewTestDB(t)
	defer testDB.Close(t)

	db := &DB{pool: testDB.Pool}
	ctx := context.Background()

	_, err := db.GetAccountByEmail(ctx, "nonexistent@example.com")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestUpdateStripeCustomerID(t *testing.T) {
	testDB := testutil.NewTestDB(t)
	defer testDB.Close(t)

	db := &DB{pool: testDB.Pool}
	ctx := context.Background()

	// Create a B2B account
	account, err := db.CreateB2BAccount(ctx, "user_01STRIPE", "stripe@example.com", "Stripe Co")
	require.NoError(t, err)
	assert.Nil(t, account.StripeCustomerID)

	// Update Stripe customer ID
	customerID := "cus_test123456"
	err = db.UpdateStripeCustomerID(ctx, account.ID, customerID)
	require.NoError(t, err)

	// Verify it was updated
	found, err := db.GetAccountByID(ctx, account.ID)
	require.NoError(t, err)
	require.NotNil(t, found.StripeCustomerID)
	assert.Equal(t, customerID, *found.StripeCustomerID)
}

func TestGetAccountByStripeCustomerID(t *testing.T) {
	testDB := testutil.NewTestDB(t)
	defer testDB.Close(t)

	db := &DB{pool: testDB.Pool}
	ctx := context.Background()

	// Create a B2B account and set Stripe customer ID
	account, err := db.CreateB2BAccount(ctx, "user_01SLOOKUP", "stripelookup@example.com", "Stripe Lookup Co")
	require.NoError(t, err)

	customerID := "cus_lookup789"
	err = db.UpdateStripeCustomerID(ctx, account.ID, customerID)
	require.NoError(t, err)

	// Lookup by Stripe customer ID
	found, err := db.GetAccountByStripeCustomerID(ctx, customerID)
	require.NoError(t, err)
	require.NotNil(t, found)
	assert.Equal(t, account.ID, found.ID)
	require.NotNil(t, found.StripeCustomerID)
	assert.Equal(t, customerID, *found.StripeCustomerID)

	// Lookup nonexistent customer ID
	_, err = db.GetAccountByStripeCustomerID(ctx, "cus_nonexistent")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestDeductBalance(t *testing.T) {
	testDB := testutil.NewTestDB(t)
	defer testDB.Close(t)

	db := &DB{pool: testDB.Pool}
	ctx := context.Background()

	// Create a B2B account and fund it
	account, err := db.CreateB2BAccount(ctx, "user_01DEDUCT", "deduct@example.com", "Deduct Co")
	require.NoError(t, err)

	err = db.UpdateBalance(ctx, account.ID, usdc.FromFloat(100.0))
	require.NoError(t, err)

	// Deduct a portion
	deducted, err := db.DeductBalance(ctx, account.ID, usdc.FromFloat(30.0))
	require.NoError(t, err)
	assert.True(t, deducted)

	// Verify remaining balance
	found, err := db.GetAccountByID(ctx, account.ID)
	require.NoError(t, err)
	assert.Equal(t, usdc.FromFloat(70.0), found.BalanceUSDC)

	// Deduct exactly the remaining balance
	deducted, err = db.DeductBalance(ctx, account.ID, usdc.FromFloat(70.0))
	require.NoError(t, err)
	assert.True(t, deducted)

	// Verify balance is now zero
	found, err = db.GetAccountByID(ctx, account.ID)
	require.NoError(t, err)
	assert.Equal(t, usdc.MicroUSDC(0), found.BalanceUSDC)
}

func TestDeductBalance_InsufficientFunds(t *testing.T) {
	testDB := testutil.NewTestDB(t)
	defer testDB.Close(t)

	db := &DB{pool: testDB.Pool}
	ctx := context.Background()

	// Create a B2B account with limited funds
	account, err := db.CreateB2BAccount(ctx, "user_01BROKE", "insufficient@example.com", "Broke Co")
	require.NoError(t, err)

	err = db.UpdateBalance(ctx, account.ID, usdc.FromFloat(10.0))
	require.NoError(t, err)

	// Attempt to deduct more than balance
	deducted, err := db.DeductBalance(ctx, account.ID, usdc.FromFloat(50.0))
	require.NoError(t, err)
	assert.False(t, deducted)

	// Verify balance was not changed
	found, err := db.GetAccountByID(ctx, account.ID)
	require.NoError(t, err)
	assert.Equal(t, usdc.FromFloat(10.0), found.BalanceUSDC)
}
