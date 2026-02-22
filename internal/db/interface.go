package db

import (
	"context"
	"net"
	"time"

	"stronghold/internal/usdc"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Database defines the interface for all database operations
// This interface enables mocking in handler unit tests
type Database interface {
	// Connection management
	Ping(ctx context.Context) error
	Close()

	// Account operations
	CreateAccount(ctx context.Context, evmWalletAddress *string, solanaWalletAddress *string) (*Account, error)
	GetAccountByID(ctx context.Context, id uuid.UUID) (*Account, error)
	GetAccountByNumber(ctx context.Context, accountNumber string) (*Account, error)
	GetAccountByWalletAddress(ctx context.Context, walletAddress string) (*Account, error)
	GetAccountByEVMWallet(ctx context.Context, evmAddress string) (*Account, error)
	GetAccountBySolanaWallet(ctx context.Context, solanaAddress string) (*Account, error)
	UpdateAccount(ctx context.Context, account *Account) error
	UpdateLastLogin(ctx context.Context, accountID uuid.UUID) error
	LinkWallet(ctx context.Context, accountID uuid.UUID, walletAddress string) error
	LinkEVMWallet(ctx context.Context, accountID uuid.UUID, evmAddress string) error
	LinkSolanaWallet(ctx context.Context, accountID uuid.UUID, solanaAddress string) error
	UpdateBalance(ctx context.Context, accountID uuid.UUID, newBalance usdc.MicroUSDC) error
	SuspendAccount(ctx context.Context, accountID uuid.UUID) error
	CloseAccount(ctx context.Context, accountID uuid.UUID) error
	AccountExists(ctx context.Context, accountNumber string) (bool, error)
	StoreEncryptedKey(ctx context.Context, accountID uuid.UUID, encryptedKey, kmsKeyID string) error
	GetEncryptedKey(ctx context.Context, accountID uuid.UUID) (string, error)
	HasEncryptedKey(ctx context.Context, accountID uuid.UUID) (bool, error)
	UpdateWalletAddress(ctx context.Context, accountID uuid.UUID, walletAddress string) error
	UpdateWalletAddresses(ctx context.Context, accountID uuid.UUID, evmAddr *string, solanaAddr *string) error

	// Session operations
	CreateSession(ctx context.Context, accountID uuid.UUID, ipAddress net.IP, userAgent string, duration time.Duration) (*Session, string, error)
	GetSessionByRefreshToken(ctx context.Context, refreshToken string) (*Session, error)
	UpdateSessionLastUsed(ctx context.Context, sessionID uuid.UUID) error
	DeleteSession(ctx context.Context, sessionID uuid.UUID) error
	DeleteSessionByRefreshToken(ctx context.Context, refreshToken string) error
	DeleteAllAccountSessions(ctx context.Context, accountID uuid.UUID) error
	GetAccountSessions(ctx context.Context, accountID uuid.UUID) ([]*Session, error)
	CleanupExpiredSessions(ctx context.Context) (int64, error)
	RotateRefreshToken(ctx context.Context, oldRefreshToken string, duration time.Duration) (*Session, string, error)

	// Deposit operations
	CreateDeposit(ctx context.Context, deposit *Deposit) error
	GetDepositByID(ctx context.Context, id uuid.UUID) (*Deposit, error)
	GetDepositByProviderTransactionID(ctx context.Context, providerTxID string) (*Deposit, error)
	GetDepositsByAccount(ctx context.Context, accountID uuid.UUID, limit, offset int) ([]*Deposit, error)
	UpdateDepositStatus(ctx context.Context, depositID uuid.UUID, status DepositStatus) error
	CompleteDeposit(ctx context.Context, depositID uuid.UUID) error
	FailDeposit(ctx context.Context, depositID uuid.UUID, reason string) error
	GetPendingDeposits(ctx context.Context, limit int) ([]*Deposit, error)
	GetDepositStats(ctx context.Context, accountID uuid.UUID) (*DepositStats, error)
	UpdateDepositProviderTransaction(ctx context.Context, depositID uuid.UUID, providerTxID string) error

	// Usage operations
	CreateUsageLog(ctx context.Context, log *UsageLog) error
	GetUsageLogs(ctx context.Context, accountID uuid.UUID, limit, offset int) ([]*UsageLog, error)
	GetUsageLogsByDateRange(ctx context.Context, accountID uuid.UUID, start, end time.Time, limit, offset int) ([]*UsageLog, error)
	GetUsageStats(ctx context.Context, accountID uuid.UUID, start, end time.Time) (*UsageStats, error)
	GetDailyUsageStats(ctx context.Context, accountID uuid.UUID, days int) ([]*DailyUsageStats, error)
	GetEndpointUsageStats(ctx context.Context, accountID uuid.UUID, start, end time.Time) ([]*EndpointUsageStats, error)

	// Transaction support
	BeginTx(ctx context.Context) (pgx.Tx, error)

	// Payment transaction operations
	CreatePaymentTransaction(ctx context.Context, tx *PaymentTransaction) error
	CreateOrGetPaymentTransaction(ctx context.Context, tx *PaymentTransaction) (*PaymentTransaction, bool, error)
	GetPaymentByNonce(ctx context.Context, nonce string) (*PaymentTransaction, error)
	GetPaymentByID(ctx context.Context, id uuid.UUID) (*PaymentTransaction, error)
	TransitionStatus(ctx context.Context, id uuid.UUID, from, to PaymentStatus) error
	RecordExecution(ctx context.Context, id uuid.UUID, result map[string]interface{}) error
	CompleteSettlement(ctx context.Context, id uuid.UUID, facilitatorPaymentID string) error
	FailSettlement(ctx context.Context, id uuid.UUID, errorMsg string) error
	GetPendingSettlements(ctx context.Context, maxAttempts int, limit int) ([]*PaymentTransaction, pgx.Tx, error)
	GetSettlementCandidates(ctx context.Context, maxAttempts int, limit int) ([]*PaymentTransaction, error)
	ClaimForSettlement(ctx context.Context, id uuid.UUID) (bool, error)
	ExpireStaleReservations(ctx context.Context) (int64, error)
	MarkSettling(ctx context.Context, id uuid.UUID) error
	LinkUsageLog(ctx context.Context, usageLogID, paymentTxID uuid.UUID) error

	// Webhook event idempotency
	CheckAndRecordWebhookEvent(ctx context.Context, eventID, eventType string) (bool, error)
}

// Ensure DB implements Database interface
var _ Database = (*DB)(nil)
