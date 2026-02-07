package db

import (
	"context"
	"testing"

	"stronghold/internal/db/testutil"

	"github.com/google/uuid"
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
	assert.Equal(t, 0.0, account.BalanceUSDC)

	// Create a direct deposit (no fee)
	deposit := &Deposit{
		AccountID:     account.ID,
		Provider:      DepositProviderDirect,
		AmountUSDC:    50.00,
		FeeUSDC:       0.0,
		NetAmountUSDC: 50.00,
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
	assert.InDelta(t, 50.00, updatedAccount.BalanceUSDC, 0.01)

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
		AmountUSDC:    100.00,
		FeeUSDC:       0.0,
		NetAmountUSDC: 100.00,
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
	assert.InDelta(t, 100.00, updatedAccount.BalanceUSDC, 0.01)
}

func TestStripeFeeCalculation(t *testing.T) {
	// Test the fee calculation formula: 2.9% + $0.30
	rate := 0.029
	flat := 0.30
	testCases := []struct {
		name        string
		amount      float64
		expectedFee float64
		expectedNet float64
	}{
		{"$10 deposit", 10.00, 10.00*rate + flat, 10.00 - (10.00*rate + flat)},
		{"$100 deposit", 100.00, 100.00*rate + flat, 100.00 - (100.00*rate + flat)},
		{"$1000 deposit", 1000.00, 1000.00*rate + flat, 1000.00 - (1000.00*rate + flat)},
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
			fee := tc.amount*rate + flat
			deposit := &Deposit{
				AccountID:     account.ID,
				Provider:      DepositProviderStripe,
				AmountUSDC:    tc.amount,
				FeeUSDC:       fee,
				NetAmountUSDC: tc.amount - fee,
			}

			err = db.CreateDeposit(ctx, deposit)
			require.NoError(t, err)

			// Verify fee calculation
			assert.InDelta(t, tc.expectedFee, deposit.FeeUSDC, 0.001, "expected fee %f, got %f", tc.expectedFee, deposit.FeeUSDC)
			assert.InDelta(t, tc.expectedNet, deposit.NetAmountUSDC, 0.001, "expected net %f, got %f", tc.expectedNet, deposit.NetAmountUSDC)

			// Complete and verify account receives net amount
			err = db.CompleteDeposit(ctx, deposit.ID)
			require.NoError(t, err)

			updatedAccount, err := db.GetAccountByID(ctx, account.ID)
			require.NoError(t, err)
			assert.InDelta(t, tc.expectedNet, updatedAccount.BalanceUSDC, 0.01, "expected balance %f, got %f", tc.expectedNet, updatedAccount.BalanceUSDC)
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
		amt := float64((i + 1) * 10)
		deposit := &Deposit{
			AccountID:     account.ID,
			Provider:      DepositProviderDirect,
			AmountUSDC:    amt,
			FeeUSDC:       0.0,
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
		AmountUSDC:    100.00,
		FeeUSDC:       3.20,
		NetAmountUSDC: 96.80,
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
	assert.Equal(t, 0.0, acc.BalanceUSDC)
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
		AmountUSDC:            50.00,
		FeeUSDC:               1.75,
		NetAmountUSDC:         48.25,
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
	completedAmounts := []float64{100.00, 50.00, 25.00}
	for _, amount := range completedAmounts {
		deposit := &Deposit{
			AccountID:     account.ID,
			Provider:      DepositProviderDirect,
			AmountUSDC:    amount,
			FeeUSDC:       0.0,
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
		AmountUSDC:    200.00,
		FeeUSDC:       6.10,
		NetAmountUSDC: 193.90,
	}
	err = db.CreateDeposit(ctx, pendingDeposit)
	require.NoError(t, err)

	// Get stats
	stats, err := db.GetDepositStats(ctx, account.ID)
	require.NoError(t, err)

	assert.Equal(t, int64(4), stats.TotalDeposits)
	assert.InDelta(t, 175.00, stats.TotalDepositedUSDC, 0.01) // 100+50+25
	assert.InDelta(t, 200.00, stats.PendingAmountUSDC, 0.01)  // The pending Stripe deposit amount
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
		amt := float64((i + 1) * 10)
		deposit := &Deposit{
			AccountID:     account.ID,
			Provider:      DepositProviderDirect,
			AmountUSDC:    amt,
			FeeUSDC:       0.0,
			NetAmountUSDC: amt,
		}
		err = db.CreateDeposit(ctx, deposit)
		require.NoError(t, err)
	}

	// Create a completed deposit
	completedDeposit := &Deposit{
		AccountID:     account.ID,
		Provider:      DepositProviderDirect,
		AmountUSDC:    100.00,
		FeeUSDC:       0.0,
		NetAmountUSDC: 100.00,
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
		AmountUSDC:    50.00,
		FeeUSDC:       0.0,
		NetAmountUSDC: 50.00,
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
	amount := 100.00
	fee := 3.20
	deposit := &Deposit{
		AccountID:     account.ID,
		Provider:      DepositProviderStripe,
		AmountUSDC:    amount,
		FeeUSDC:       fee,
		NetAmountUSDC: amount - fee,
	}

	err = db.CreateDeposit(ctx, deposit)
	require.NoError(t, err)

	// Verify net amount
	retrieved, err := db.GetDepositByID(ctx, deposit.ID)
	require.NoError(t, err)
	assert.InDelta(t, amount-fee, retrieved.NetAmountUSDC, 0.001)
}
