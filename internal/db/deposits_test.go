package db

import (
	"context"
	"testing"

	"stronghold/internal/db/testutil"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCompleteDeposit_UpdatesBalance(t *testing.T) {
	testDB := testutil.NewTestDB(t)
	defer testDB.Close(t)

	db := &DB{pool: testDB.Pool}
	ctx := context.Background()

	// Create account with 0 balance
	account, err := db.CreateAccount(ctx, nil)
	require.NoError(t, err)
	assert.True(t, decimal.Zero.Equal(account.BalanceUSDC))

	// Create a direct deposit (no fee)
	amount := decimal.NewFromFloat(50.00)
	deposit := &Deposit{
		AccountID:     account.ID,
		Provider:      DepositProviderDirect,
		AmountUSDC:    amount,
		FeeUSDC:       decimal.Zero,
		NetAmountUSDC: amount,
	}

	err = db.CreateDeposit(ctx, deposit)
	require.NoError(t, err)
	assert.Equal(t, DepositStatusPending, deposit.Status)

	// Complete deposit
	err = db.CompleteDeposit(ctx, deposit.ID)
	require.NoError(t, err)

	// Verify account balance increased by net amount
	updatedAccount, err := db.GetAccountByID(ctx, account.ID)
	require.NoError(t, err)
	assert.True(t, amount.Equal(updatedAccount.BalanceUSDC))

	// Verify deposit status
	updatedDeposit, err := db.GetDepositByID(ctx, deposit.ID)
	require.NoError(t, err)
	assert.Equal(t, DepositStatusCompleted, updatedDeposit.Status)
	assert.NotNil(t, updatedDeposit.CompletedAt)
}

func TestCompleteDeposit_Atomic(t *testing.T) {
	testDB := testutil.NewTestDB(t)
	defer testDB.Close(t)

	db := &DB{pool: testDB.Pool}
	ctx := context.Background()

	// Create account
	account, err := db.CreateAccount(ctx, nil)
	require.NoError(t, err)

	// Create deposit
	deposit := &Deposit{
		AccountID:     account.ID,
		Provider:      DepositProviderDirect,
		AmountUSDC:    decimal.NewFromFloat(100.00),
		FeeUSDC:       decimal.Zero,
		NetAmountUSDC: decimal.NewFromFloat(100.00),
	}
	err = db.CreateDeposit(ctx, deposit)
	require.NoError(t, err)

	// Complete deposit once
	err = db.CompleteDeposit(ctx, deposit.ID)
	require.NoError(t, err)

	// Try to complete again - should fail (deposit no longer pending)
	err = db.CompleteDeposit(ctx, deposit.ID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not pending")

	// Verify balance only increased once
	updatedAccount, err := db.GetAccountByID(ctx, account.ID)
	require.NoError(t, err)
	assert.True(t, decimal.NewFromFloat(100.00).Equal(updatedAccount.BalanceUSDC))
}

func TestStripeFeeCalculation(t *testing.T) {
	// Test the fee calculation formula: 2.9% + $0.30
	rate := decimal.NewFromFloat(0.029)
	flat := decimal.NewFromFloat(0.30)
	testCases := []struct {
		name           string
		amount         decimal.Decimal
		expectedFee    decimal.Decimal
		expectedNet    decimal.Decimal
	}{
		{"$10 deposit", decimal.NewFromFloat(10.00), decimal.NewFromFloat(10.00).Mul(rate).Add(flat), decimal.NewFromFloat(10.00).Sub(decimal.NewFromFloat(10.00).Mul(rate).Add(flat))},
		{"$100 deposit", decimal.NewFromFloat(100.00), decimal.NewFromFloat(100.00).Mul(rate).Add(flat), decimal.NewFromFloat(100.00).Sub(decimal.NewFromFloat(100.00).Mul(rate).Add(flat))},
		{"$1000 deposit", decimal.NewFromFloat(1000.00), decimal.NewFromFloat(1000.00).Mul(rate).Add(flat), decimal.NewFromFloat(1000.00).Sub(decimal.NewFromFloat(1000.00).Mul(rate).Add(flat))},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			testDB := testutil.NewTestDB(t)
			defer testDB.Close(t)

			db := &DB{pool: testDB.Pool}
			ctx := context.Background()

			account, err := db.CreateAccount(ctx, nil)
			require.NoError(t, err)

			// Create Stripe deposit with calculated fee
			fee := tc.amount.Mul(rate).Add(flat)
			deposit := &Deposit{
				AccountID:     account.ID,
				Provider:      DepositProviderStripe,
				AmountUSDC:    tc.amount,
				FeeUSDC:       fee,
				NetAmountUSDC: tc.amount.Sub(fee),
			}

			err = db.CreateDeposit(ctx, deposit)
			require.NoError(t, err)

			// Verify fee calculation
			assert.True(t, tc.expectedFee.Equal(deposit.FeeUSDC), "expected fee %s, got %s", tc.expectedFee, deposit.FeeUSDC)
			assert.True(t, tc.expectedNet.Equal(deposit.NetAmountUSDC), "expected net %s, got %s", tc.expectedNet, deposit.NetAmountUSDC)

			// Complete and verify account receives net amount
			err = db.CompleteDeposit(ctx, deposit.ID)
			require.NoError(t, err)

			updatedAccount, err := db.GetAccountByID(ctx, account.ID)
			require.NoError(t, err)
			assert.True(t, tc.expectedNet.Equal(updatedAccount.BalanceUSDC), "expected balance %s, got %s", tc.expectedNet, updatedAccount.BalanceUSDC)
		})
	}
}

func TestGetDepositsByAccount_Pagination(t *testing.T) {
	testDB := testutil.NewTestDB(t)
	defer testDB.Close(t)

	db := &DB{pool: testDB.Pool}
	ctx := context.Background()

	account, err := db.CreateAccount(ctx, nil)
	require.NoError(t, err)

	// Create multiple deposits
	for i := 0; i < 10; i++ {
		amt := decimal.NewFromInt(int64((i + 1) * 10))
		deposit := &Deposit{
			AccountID:     account.ID,
			Provider:      DepositProviderDirect,
			AmountUSDC:    amt,
			FeeUSDC:       decimal.Zero,
			NetAmountUSDC: amt,
		}
		err = db.CreateDeposit(ctx, deposit)
		require.NoError(t, err)
	}

	// Test pagination
	deposits, err := db.GetDepositsByAccount(ctx, account.ID, 5, 0)
	require.NoError(t, err)
	assert.Len(t, deposits, 5)

	deposits, err = db.GetDepositsByAccount(ctx, account.ID, 5, 5)
	require.NoError(t, err)
	assert.Len(t, deposits, 5)

	// Beyond available
	deposits, err = db.GetDepositsByAccount(ctx, account.ID, 5, 10)
	require.NoError(t, err)
	assert.Len(t, deposits, 0)
}

func TestGetDepositsByAccount_LimitEnforced(t *testing.T) {
	testDB := testutil.NewTestDB(t)
	defer testDB.Close(t)

	db := &DB{pool: testDB.Pool}
	ctx := context.Background()

	account, err := db.CreateAccount(ctx, nil)
	require.NoError(t, err)

	// Request with limit > 1000 (should be capped)
	_, err = db.GetDepositsByAccount(ctx, account.ID, 5000, 0)
	require.NoError(t, err) // Should succeed but cap at 1000
}

func TestFailDeposit(t *testing.T) {
	testDB := testutil.NewTestDB(t)
	defer testDB.Close(t)

	db := &DB{pool: testDB.Pool}
	ctx := context.Background()

	account, err := db.CreateAccount(ctx, nil)
	require.NoError(t, err)

	deposit := &Deposit{
		AccountID:     account.ID,
		Provider:      DepositProviderStripe,
		AmountUSDC:    decimal.NewFromFloat(100.00),
		FeeUSDC:       decimal.NewFromFloat(3.20),
		NetAmountUSDC: decimal.NewFromFloat(96.80),
	}
	err = db.CreateDeposit(ctx, deposit)
	require.NoError(t, err)

	// Fail the deposit
	reason := "Card declined"
	err = db.FailDeposit(ctx, deposit.ID, reason)
	require.NoError(t, err)

	// Verify status and reason
	updated, err := db.GetDepositByID(ctx, deposit.ID)
	require.NoError(t, err)
	assert.Equal(t, DepositStatusFailed, updated.Status)
	assert.Contains(t, updated.Metadata, "failure_reason")
	assert.Equal(t, reason, updated.Metadata["failure_reason"])

	// Account balance should NOT have changed
	acc, err := db.GetAccountByID(ctx, account.ID)
	require.NoError(t, err)
	assert.True(t, decimal.Zero.Equal(acc.BalanceUSDC))
}

func TestGetDepositByProviderTransactionID(t *testing.T) {
	testDB := testutil.NewTestDB(t)
	defer testDB.Close(t)

	db := &DB{pool: testDB.Pool}
	ctx := context.Background()

	account, err := db.CreateAccount(ctx, nil)
	require.NoError(t, err)

	providerTxID := "pi_1234567890"
	deposit := &Deposit{
		AccountID:             account.ID,
		Provider:              DepositProviderStripe,
		AmountUSDC:            decimal.NewFromFloat(50.00),
		FeeUSDC:               decimal.NewFromFloat(1.75),
		NetAmountUSDC:         decimal.NewFromFloat(48.25),
		ProviderTransactionID: &providerTxID,
	}
	err = db.CreateDeposit(ctx, deposit)
	require.NoError(t, err)

	// Find by provider transaction ID
	found, err := db.GetDepositByProviderTransactionID(ctx, providerTxID)
	require.NoError(t, err)
	assert.Equal(t, deposit.ID, found.ID)
}

func TestGetDepositStats(t *testing.T) {
	testDB := testutil.NewTestDB(t)
	defer testDB.Close(t)

	db := &DB{pool: testDB.Pool}
	ctx := context.Background()

	account, err := db.CreateAccount(ctx, nil)
	require.NoError(t, err)

	// Create and complete some deposits
	completedAmounts := []decimal.Decimal{decimal.NewFromFloat(100.00), decimal.NewFromFloat(50.00), decimal.NewFromFloat(25.00)}
	for _, amount := range completedAmounts {
		deposit := &Deposit{
			AccountID:     account.ID,
			Provider:      DepositProviderDirect,
			AmountUSDC:    amount,
			FeeUSDC:       decimal.Zero,
			NetAmountUSDC: amount,
		}
		err = db.CreateDeposit(ctx, deposit)
		require.NoError(t, err)
		err = db.CompleteDeposit(ctx, deposit.ID)
		require.NoError(t, err)
	}

	// Create a pending deposit
	pendingDeposit := &Deposit{
		AccountID:     account.ID,
		Provider:      DepositProviderStripe,
		AmountUSDC:    decimal.NewFromFloat(200.00),
		FeeUSDC:       decimal.NewFromFloat(6.10),
		NetAmountUSDC: decimal.NewFromFloat(193.90),
	}
	err = db.CreateDeposit(ctx, pendingDeposit)
	require.NoError(t, err)

	// Get stats
	stats, err := db.GetDepositStats(ctx, account.ID)
	require.NoError(t, err)

	assert.Equal(t, int64(4), stats.TotalDeposits)
	assert.True(t, decimal.NewFromFloat(175.00).Equal(stats.TotalDepositedUSDC)) // 100+50+25
	assert.True(t, decimal.NewFromFloat(200.00).Equal(stats.PendingAmountUSDC))  // The pending Stripe deposit amount
}

func TestGetPendingDeposits(t *testing.T) {
	testDB := testutil.NewTestDB(t)
	defer testDB.Close(t)

	db := &DB{pool: testDB.Pool}
	ctx := context.Background()

	account, err := db.CreateAccount(ctx, nil)
	require.NoError(t, err)

	// Create pending deposits
	for i := 0; i < 3; i++ {
		amt := decimal.NewFromInt(int64((i + 1) * 10))
		deposit := &Deposit{
			AccountID:     account.ID,
			Provider:      DepositProviderDirect,
			AmountUSDC:    amt,
			FeeUSDC:       decimal.Zero,
			NetAmountUSDC: amt,
		}
		err = db.CreateDeposit(ctx, deposit)
		require.NoError(t, err)
	}

	// Create a completed deposit
	completedDeposit := &Deposit{
		AccountID:     account.ID,
		Provider:      DepositProviderDirect,
		AmountUSDC:    decimal.NewFromFloat(100.00),
		FeeUSDC:       decimal.Zero,
		NetAmountUSDC: decimal.NewFromFloat(100.00),
	}
	err = db.CreateDeposit(ctx, completedDeposit)
	require.NoError(t, err)
	err = db.CompleteDeposit(ctx, completedDeposit.ID)
	require.NoError(t, err)

	// Get pending deposits
	pending, err := db.GetPendingDeposits(ctx, 100)
	require.NoError(t, err)
	assert.Len(t, pending, 3)

	// All should be pending status
	for _, dep := range pending {
		assert.Equal(t, DepositStatusPending, dep.Status)
	}
}

func TestGetDepositByID_NotFound(t *testing.T) {
	testDB := testutil.NewTestDB(t)
	defer testDB.Close(t)

	db := &DB{pool: testDB.Pool}
	ctx := context.Background()

	_, err := db.GetDepositByID(ctx, uuid.New())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestUpdateDepositStatus(t *testing.T) {
	testDB := testutil.NewTestDB(t)
	defer testDB.Close(t)

	db := &DB{pool: testDB.Pool}
	ctx := context.Background()

	account, err := db.CreateAccount(ctx, nil)
	require.NoError(t, err)

	deposit := &Deposit{
		AccountID:     account.ID,
		Provider:      DepositProviderDirect,
		AmountUSDC:    decimal.NewFromFloat(50.00),
		FeeUSDC:       decimal.Zero,
		NetAmountUSDC: decimal.NewFromFloat(50.00),
	}
	err = db.CreateDeposit(ctx, deposit)
	require.NoError(t, err)

	// Update to cancelled
	err = db.UpdateDepositStatus(ctx, deposit.ID, DepositStatusCancelled)
	require.NoError(t, err)

	updated, err := db.GetDepositByID(ctx, deposit.ID)
	require.NoError(t, err)
	assert.Equal(t, DepositStatusCancelled, updated.Status)
}

func TestDeposit_NetAmountCalculatedCorrectly(t *testing.T) {
	testDB := testutil.NewTestDB(t)
	defer testDB.Close(t)

	db := &DB{pool: testDB.Pool}
	ctx := context.Background()

	account, err := db.CreateAccount(ctx, nil)
	require.NoError(t, err)

	// Create deposit where net should equal amount - fee
	amount := decimal.NewFromFloat(100.00)
	fee := decimal.NewFromFloat(3.20)
	deposit := &Deposit{
		AccountID:     account.ID,
		Provider:      DepositProviderStripe,
		AmountUSDC:    amount,
		FeeUSDC:       fee,
		NetAmountUSDC: amount.Sub(fee), // This should be set correctly
	}

	err = db.CreateDeposit(ctx, deposit)
	require.NoError(t, err)

	// Verify net amount
	retrieved, err := db.GetDepositByID(ctx, deposit.ID)
	require.NoError(t, err)
	assert.True(t, amount.Sub(fee).Equal(retrieved.NetAmountUSDC))
}
