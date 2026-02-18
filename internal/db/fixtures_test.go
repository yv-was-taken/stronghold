package db

import (
	"context"
	"fmt"
	"net"
	"testing"
	"time"

	"stronghold/internal/usdc"

	"github.com/google/uuid"
)

// TestAccount represents an account created for testing
type TestAccount struct {
	Account       *Account
	AccountNumber string
}

// TestSession represents a session created for testing
type TestSession struct {
	Session      *Session
	RefreshToken string
}

// Fixtures provides test data factories
type Fixtures struct {
	t  *testing.T
	db *DB
}

// NewFixtures creates a new Fixtures instance
func NewFixtures(t *testing.T, database *DB) *Fixtures {
	return &Fixtures{
		t:  t,
		db: database,
	}
}

// CreateTestAccount creates a test account with optional EVM wallet address
func (f *Fixtures) CreateTestAccount(evmWalletAddress *string) *TestAccount {
	f.t.Helper()

	ctx := context.Background()
	account, err := f.db.CreateAccount(ctx, evmWalletAddress, nil)
	if err != nil {
		f.t.Fatalf("Failed to create test account: %v", err)
	}

	return &TestAccount{
		Account:       account,
		AccountNumber: account.AccountNumber,
	}
}

// CreateTestAccountWithWallet creates a test account with an EVM wallet address
func (f *Fixtures) CreateTestAccountWithWallet() *TestAccount {
	f.t.Helper()

	wallet := "0x" + fmt.Sprintf("%040x", time.Now().UnixNano())
	return f.CreateTestAccount(&wallet)
}

// CreateTestAccountWithSolanaWallet creates a test account with a Solana wallet address
func (f *Fixtures) CreateTestAccountWithSolanaWallet() *TestAccount {
	f.t.Helper()

	// Use a valid base58 Solana address for testing (43 chars, valid base58)
	solanaWallet := "9WzDXwBbmkg8ZTbNMqUxvQRAyrZzDsGYdLVL9zYtAWWM"
	ctx := context.Background()
	account, err := f.db.CreateAccount(ctx, nil, &solanaWallet)
	if err != nil {
		f.t.Fatalf("Failed to create test account with Solana wallet: %v", err)
	}

	return &TestAccount{
		Account:       account,
		AccountNumber: account.AccountNumber,
	}
}

// CreateTestSession creates a test session for an account
func (f *Fixtures) CreateTestSession(accountID uuid.UUID) *TestSession {
	f.t.Helper()

	ctx := context.Background()
	ip := net.ParseIP("127.0.0.1")
	userAgent := "test-agent/1.0"
	duration := 24 * time.Hour

	session, refreshToken, err := f.db.CreateSession(ctx, accountID, ip, userAgent, duration)
	if err != nil {
		f.t.Fatalf("Failed to create test session: %v", err)
	}

	return &TestSession{
		Session:      session,
		RefreshToken: refreshToken,
	}
}

// CreateTestSessionWithExpiry creates a test session with custom expiry duration
func (f *Fixtures) CreateTestSessionWithExpiry(accountID uuid.UUID, duration time.Duration) *TestSession {
	f.t.Helper()

	ctx := context.Background()
	ip := net.ParseIP("127.0.0.1")
	userAgent := "test-agent/1.0"

	session, refreshToken, err := f.db.CreateSession(ctx, accountID, ip, userAgent, duration)
	if err != nil {
		f.t.Fatalf("Failed to create test session: %v", err)
	}

	return &TestSession{
		Session:      session,
		RefreshToken: refreshToken,
	}
}

// CreateExpiredSession creates an already-expired session for testing
func (f *Fixtures) CreateExpiredSession(accountID uuid.UUID) *TestSession {
	f.t.Helper()

	// Create with very short duration, then wait
	ctx := context.Background()
	ip := net.ParseIP("127.0.0.1")
	userAgent := "test-agent/1.0"

	session, refreshToken, err := f.db.CreateSession(ctx, accountID, ip, userAgent, 1*time.Millisecond)
	if err != nil {
		f.t.Fatalf("Failed to create test session: %v", err)
	}

	// Wait for expiry
	time.Sleep(10 * time.Millisecond)

	return &TestSession{
		Session:      session,
		RefreshToken: refreshToken,
	}
}

// CreateTestDeposit creates a test deposit for an account
func (f *Fixtures) CreateTestDeposit(accountID uuid.UUID, amount usdc.MicroUSDC, provider DepositProvider) *Deposit {
	f.t.Helper()

	ctx := context.Background()
	deposit := &Deposit{
		AccountID:     accountID,
		Provider:      provider,
		AmountUSDC:    amount,
		FeeUSDC:       0,
		NetAmountUSDC: amount,
		Status:        DepositStatusPending,
	}

	if err := f.db.CreateDeposit(ctx, deposit); err != nil {
		f.t.Fatalf("Failed to create test deposit: %v", err)
	}

	return deposit
}

// CreateCompletedDeposit creates a deposit and completes it
func (f *Fixtures) CreateCompletedDeposit(accountID uuid.UUID, amount usdc.MicroUSDC) *Deposit {
	f.t.Helper()

	deposit := f.CreateTestDeposit(accountID, amount, DepositProviderDirect)

	ctx := context.Background()
	if err := f.db.CompleteDeposit(ctx, deposit.ID); err != nil {
		f.t.Fatalf("Failed to complete test deposit: %v", err)
	}

	// Refresh deposit from DB
	refreshed, err := f.db.GetDepositByID(ctx, deposit.ID)
	if err != nil {
		f.t.Fatalf("Failed to refresh deposit: %v", err)
	}

	return refreshed
}

// CreateStripeDeposit creates a Stripe deposit with proper fee calculation
func (f *Fixtures) CreateStripeDeposit(accountID uuid.UUID, amount usdc.MicroUSDC) *Deposit {
	f.t.Helper()

	ctx := context.Background()
	// Stripe fee: 2.9% + $0.30 in microUSDC
	fee := usdc.MicroUSDC(int64(amount)*29/1000 + 300_000)
	deposit := &Deposit{
		AccountID:     accountID,
		Provider:      DepositProviderStripe,
		AmountUSDC:    amount,
		FeeUSDC:       fee,
		NetAmountUSDC: amount - fee,
		Status:        DepositStatusPending,
	}

	if err := f.db.CreateDeposit(ctx, deposit); err != nil {
		f.t.Fatalf("Failed to create Stripe deposit: %v", err)
	}

	return deposit
}

// CreateTestUsageLog creates a test usage log entry
func (f *Fixtures) CreateTestUsageLog(accountID uuid.UUID, endpoint string, cost usdc.MicroUSDC, threatDetected bool) *UsageLog {
	f.t.Helper()

	ctx := context.Background()
	log := &UsageLog{
		AccountID:      accountID,
		RequestID:      uuid.New().String(),
		Endpoint:       endpoint,
		Method:         "POST",
		CostUSDC:       cost,
		Status:         "success",
		ThreatDetected: threatDetected,
	}

	if err := f.db.CreateUsageLog(ctx, log); err != nil {
		f.t.Fatalf("Failed to create test usage log: %v", err)
	}

	return log
}

// CreateTestPaymentTransaction creates a test payment transaction
func (f *Fixtures) CreateTestPaymentTransaction(endpoint string, amount usdc.MicroUSDC) *PaymentTransaction {
	f.t.Helper()

	ctx := context.Background()
	tx := &PaymentTransaction{
		PaymentNonce:    "test-nonce-" + uuid.New().String(),
		PaymentHeader:   "x402;test-header",
		PayerAddress:    "0x1234567890123456789012345678901234567890",
		ReceiverAddress: "0x0987654321098765432109876543210987654321",
		Endpoint:        endpoint,
		AmountUSDC:      amount,
		Network:         "base-sepolia",
		ExpiresAt:       time.Now().Add(5 * time.Minute),
	}

	if err := f.db.CreatePaymentTransaction(ctx, tx); err != nil {
		f.t.Fatalf("Failed to create test payment transaction: %v", err)
	}

	return tx
}

// CreateExpiredPaymentTransaction creates an already-expired payment transaction
func (f *Fixtures) CreateExpiredPaymentTransaction(endpoint string, amount usdc.MicroUSDC) *PaymentTransaction {
	f.t.Helper()

	ctx := context.Background()
	tx := &PaymentTransaction{
		PaymentNonce:    "expired-nonce-" + uuid.New().String(),
		PaymentHeader:   "x402;test-header",
		PayerAddress:    "0x1234567890123456789012345678901234567890",
		ReceiverAddress: "0x0987654321098765432109876543210987654321",
		Endpoint:        endpoint,
		AmountUSDC:      amount,
		Network:         "base-sepolia",
		ExpiresAt:       time.Now().Add(-1 * time.Minute), // Already expired
	}

	if err := f.db.CreatePaymentTransaction(ctx, tx); err != nil {
		f.t.Fatalf("Failed to create expired payment transaction: %v", err)
	}

	return tx
}
