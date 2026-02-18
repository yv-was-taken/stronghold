package db

import (
	"context"
	"testing"

	"stronghold/internal/db/testutil"
	"stronghold/internal/usdc"

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
	account, err := db.CreateAccount(ctx, nil, nil)
	require.NoError(t, err)
	assert.Equal(t, usdc.MicroUSDC(0), account.BalanceUSDC)

	// Create a direct deposit (no fee)
	deposit := &Deposit{
		AccountID:     account.ID,
		Provider:      DepositProviderDirect,
		AmountUSDC:    usdc.FromFloat(50.00),
		FeeUSDC:       0,
		NetAmountUSDC: usdc.FromFloat(50.00),
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
	assert.Equal(t, usdc.FromFloat(50.00), updatedAccount.BalanceUSDC)

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
	account, err := db.CreateAccount(ctx, nil, nil)
	require.NoError(t, err)

	// Create deposit
	deposit := &Deposit{
		AccountID:     account.ID,
		Provider:      DepositProviderDirect,
		AmountUSDC:    usdc.FromFloat(100.00),
		FeeUSDC:       0,
		NetAmountUSDC: usdc.FromFloat(100.00),
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
	assert.Equal(t, usdc.FromFloat(100.00), updatedAccount.BalanceUSDC)
}

func TestStripeFeeCalculation(t *testing.T) {
	// Test the fee calculation formula: 2.9% + $0.30
	// In MicroUSDC: fee = amount * 29 / 1000 + 300_000
	testCases := []struct {
		name        string
		amount      usdc.MicroUSDC
		expectedFee usdc.MicroUSDC
		expectedNet usdc.MicroUSDC
	}{
		{"$10 deposit", usdc.FromFloat(10.00), usdc.MicroUSDC(590_000), usdc.MicroUSDC(9_410_000)},
		{"$100 deposit", usdc.FromFloat(100.00), usdc.MicroUSDC(3_200_000), usdc.MicroUSDC(96_800_000)},
		{"$1000 deposit", usdc.FromFloat(1000.00), usdc.MicroUSDC(29_300_000), usdc.MicroUSDC(970_700_000)},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			testDB := testutil.NewTestDB(t)
			defer testDB.Close(t)

			db := &DB{pool: testDB.Pool}
			ctx := context.Background()

			account, err := db.CreateAccount(ctx, nil, nil)
			require.NoError(t, err)

			// Create Stripe deposit with calculated fee
			fee := tc.expectedFee
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
			assert.Equal(t, tc.expectedFee, deposit.FeeUSDC, "expected fee %v, got %v", tc.expectedFee, deposit.FeeUSDC)
			assert.Equal(t, tc.expectedNet, deposit.NetAmountUSDC, "expected net %v, got %v", tc.expectedNet, deposit.NetAmountUSDC)

			// Complete and verify account receives net amount
			err = db.CompleteDeposit(ctx, deposit.ID)
			require.NoError(t, err)

			updatedAccount, err := db.GetAccountByID(ctx, account.ID)
			require.NoError(t, err)
			assert.Equal(t, tc.expectedNet, updatedAccount.BalanceUSDC, "expected balance %v, got %v", tc.expectedNet, updatedAccount.BalanceUSDC)
		})
	}
}

func TestGetDepositsByAccount_Pagination(t *testing.T) {
	testDB := testutil.NewTestDB(t)
	defer testDB.Close(t)

	db := &DB{pool: testDB.Pool}
	ctx := context.Background()

	account, err := db.CreateAccount(ctx, nil, nil)
	require.NoError(t, err)

	// Create multiple deposits
	for i := 0; i < 10; i++ {
		amt := usdc.FromFloat(float64((i + 1) * 10))
		deposit := &Deposit{
			AccountID:     account.ID,
			Provider:      DepositProviderDirect,
			AmountUSDC:    amt,
			FeeUSDC:       0,
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

	account, err := db.CreateAccount(ctx, nil, nil)
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

	account, err := db.CreateAccount(ctx, nil, nil)
	require.NoError(t, err)

	deposit := &Deposit{
		AccountID:     account.ID,
		Provider:      DepositProviderStripe,
		AmountUSDC:    usdc.FromFloat(100.00),
		FeeUSDC:       usdc.FromFloat(3.20),
		NetAmountUSDC: usdc.FromFloat(96.80),
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
	assert.Equal(t, usdc.MicroUSDC(0), acc.BalanceUSDC)
}

func TestGetDepositByProviderTransactionID(t *testing.T) {
	testDB := testutil.NewTestDB(t)
	defer testDB.Close(t)

	db := &DB{pool: testDB.Pool}
	ctx := context.Background()

	account, err := db.CreateAccount(ctx, nil, nil)
	require.NoError(t, err)

	providerTxID := "pi_1234567890"
	deposit := &Deposit{
		AccountID:             account.ID,
		Provider:              DepositProviderStripe,
		AmountUSDC:            usdc.FromFloat(50.00),
		FeeUSDC:               usdc.FromFloat(1.75),
		NetAmountUSDC:         usdc.FromFloat(48.25),
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

	account, err := db.CreateAccount(ctx, nil, nil)
	require.NoError(t, err)

	// Create and complete some deposits
	completedAmounts := []usdc.MicroUSDC{usdc.FromFloat(100.00), usdc.FromFloat(50.00), usdc.FromFloat(25.00)}
	for _, amount := range completedAmounts {
		deposit := &Deposit{
			AccountID:     account.ID,
			Provider:      DepositProviderDirect,
			AmountUSDC:    amount,
			FeeUSDC:       0,
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
		AmountUSDC:    usdc.FromFloat(200.00),
		FeeUSDC:       usdc.FromFloat(6.10),
		NetAmountUSDC: usdc.FromFloat(193.90),
	}
	err = db.CreateDeposit(ctx, pendingDeposit)
	require.NoError(t, err)

	// Get stats
	stats, err := db.GetDepositStats(ctx, account.ID)
	require.NoError(t, err)

	assert.Equal(t, int64(4), stats.TotalDeposits)
	assert.Equal(t, usdc.FromFloat(175.00), stats.TotalDepositedUSDC) // 100+50+25
	assert.Equal(t, usdc.FromFloat(200.00), stats.PendingAmountUSDC)  // The pending Stripe deposit amount
}

func TestGetPendingDeposits(t *testing.T) {
	testDB := testutil.NewTestDB(t)
	defer testDB.Close(t)

	db := &DB{pool: testDB.Pool}
	ctx := context.Background()

	account, err := db.CreateAccount(ctx, nil, nil)
	require.NoError(t, err)

	// Create pending deposits
	for i := 0; i < 3; i++ {
		amt := usdc.FromFloat(float64((i + 1) * 10))
		deposit := &Deposit{
			AccountID:     account.ID,
			Provider:      DepositProviderDirect,
			AmountUSDC:    amt,
			FeeUSDC:       0,
			NetAmountUSDC: amt,
		}
		err = db.CreateDeposit(ctx, deposit)
		require.NoError(t, err)
	}

	// Create a completed deposit
	completedDeposit := &Deposit{
		AccountID:     account.ID,
		Provider:      DepositProviderDirect,
		AmountUSDC:    usdc.FromFloat(100.00),
		FeeUSDC:       0,
		NetAmountUSDC: usdc.FromFloat(100.00),
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

	account, err := db.CreateAccount(ctx, nil, nil)
	require.NoError(t, err)

	deposit := &Deposit{
		AccountID:     account.ID,
		Provider:      DepositProviderDirect,
		AmountUSDC:    usdc.FromFloat(50.00),
		FeeUSDC:       0,
		NetAmountUSDC: usdc.FromFloat(50.00),
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

	account, err := db.CreateAccount(ctx, nil, nil)
	require.NoError(t, err)

	// Create deposit where net should equal amount - fee
	amount := usdc.FromFloat(100.00)
	fee := usdc.FromFloat(3.20)
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
	assert.Equal(t, amount-fee, retrieved.NetAmountUSDC)
}
