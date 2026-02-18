package db

import (
	"context"
	"errors"
	"fmt"
	"time"

	"stronghold/internal/usdc"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// DepositStatus represents the status of a deposit
type DepositStatus string

const (
	DepositStatusPending   DepositStatus = "pending"
	DepositStatusCompleted DepositStatus = "completed"
	DepositStatusFailed    DepositStatus = "failed"
	DepositStatusCancelled DepositStatus = "cancelled"
)

// DepositProvider represents the payment provider
type DepositProvider string

const (
	DepositProviderStripe  DepositProvider = "stripe"
	DepositProviderDirect  DepositProvider = "direct"
)

// Deposit represents a payment deposit
type Deposit struct {
	ID                  uuid.UUID        `json:"id"`
	AccountID           uuid.UUID        `json:"account_id"`
	Provider            DepositProvider  `json:"provider"`
	AmountUSDC          usdc.MicroUSDC   `json:"amount_usdc"`
	FeeUSDC             usdc.MicroUSDC   `json:"fee_usdc"`
	NetAmountUSDC       usdc.MicroUSDC   `json:"net_amount_usdc"`
	Status              DepositStatus    `json:"status"`
	ProviderTransactionID *string        `json:"provider_transaction_id,omitempty"`
	WalletAddress       *string          `json:"wallet_address,omitempty"`
	Metadata            map[string]any   `json:"metadata,omitempty"`
	CreatedAt           time.Time        `json:"created_at"`
	CompletedAt         *time.Time       `json:"completed_at,omitempty"`
}

// CreateDeposit creates a new deposit record
func (db *DB) CreateDeposit(ctx context.Context, deposit *Deposit) error {
	deposit.ID = uuid.New()
	deposit.Status = DepositStatusPending
	deposit.CreatedAt = time.Now().UTC()

	_, err := db.pool.Exec(ctx, `
		INSERT INTO deposits (
			id, account_id, provider, amount_usdc, fee_usdc, net_amount_usdc,
			status, provider_transaction_id, wallet_address, metadata, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
	`, deposit.ID, deposit.AccountID, deposit.Provider, deposit.AmountUSDC,
		deposit.FeeUSDC, deposit.NetAmountUSDC, deposit.Status,
		deposit.ProviderTransactionID, deposit.WalletAddress,
		deposit.Metadata, deposit.CreatedAt)

	if err != nil {
		return fmt.Errorf("failed to create deposit: %w", err)
	}

	return nil
}

// GetDepositByID retrieves a deposit by its ID
func (db *DB) GetDepositByID(ctx context.Context, id uuid.UUID) (*Deposit, error) {
	deposit := &Deposit{}
	err := db.QueryRow(ctx, `
		SELECT id, account_id, provider, amount_usdc, fee_usdc, net_amount_usdc,
		       status, provider_transaction_id, wallet_address, metadata, created_at, completed_at
		FROM deposits
		WHERE id = $1
	`, id).Scan(
		&deposit.ID, &deposit.AccountID, &deposit.Provider, &deposit.AmountUSDC,
		&deposit.FeeUSDC, &deposit.NetAmountUSDC, &deposit.Status,
		&deposit.ProviderTransactionID, &deposit.WalletAddress,
		&deposit.Metadata, &deposit.CreatedAt, &deposit.CompletedAt,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, errors.New("deposit not found")
		}
		return nil, fmt.Errorf("failed to get deposit: %w", err)
	}

	return deposit, nil
}

// GetDepositByProviderTransactionID retrieves a deposit by provider transaction ID
func (db *DB) GetDepositByProviderTransactionID(ctx context.Context, providerTxID string) (*Deposit, error) {
	deposit := &Deposit{}
	err := db.QueryRow(ctx, `
		SELECT id, account_id, provider, amount_usdc, fee_usdc, net_amount_usdc,
		       status, provider_transaction_id, wallet_address, metadata, created_at, completed_at
		FROM deposits
		WHERE provider_transaction_id = $1
	`, providerTxID).Scan(
		&deposit.ID, &deposit.AccountID, &deposit.Provider, &deposit.AmountUSDC,
		&deposit.FeeUSDC, &deposit.NetAmountUSDC, &deposit.Status,
		&deposit.ProviderTransactionID, &deposit.WalletAddress,
		&deposit.Metadata, &deposit.CreatedAt, &deposit.CompletedAt,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, errors.New("deposit not found")
		}
		return nil, fmt.Errorf("failed to get deposit: %w", err)
	}

	return deposit, nil
}

// GetDepositsByAccount retrieves deposits for an account with pagination
func (db *DB) GetDepositsByAccount(ctx context.Context, accountID uuid.UUID, limit, offset int) ([]*Deposit, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 1000 {
		limit = 1000
	}

	rows, err := db.pool.Query(ctx, `
		SELECT id, account_id, provider, amount_usdc, fee_usdc, net_amount_usdc,
		       status, provider_transaction_id, wallet_address, metadata, created_at, completed_at
		FROM deposits
		WHERE account_id = $1
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3
	`, accountID, limit, offset)

	if err != nil {
		return nil, fmt.Errorf("failed to get deposits: %w", err)
	}
	defer rows.Close()

	var deposits []*Deposit
	for rows.Next() {
		deposit := &Deposit{}
		err := rows.Scan(
			&deposit.ID, &deposit.AccountID, &deposit.Provider, &deposit.AmountUSDC,
			&deposit.FeeUSDC, &deposit.NetAmountUSDC, &deposit.Status,
			&deposit.ProviderTransactionID, &deposit.WalletAddress,
			&deposit.Metadata, &deposit.CreatedAt, &deposit.CompletedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan deposit: %w", err)
		}
		deposits = append(deposits, deposit)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating deposits: %w", err)
	}

	return deposits, nil
}

// UpdateDepositStatus updates the status of a deposit
func (db *DB) UpdateDepositStatus(ctx context.Context, depositID uuid.UUID, status DepositStatus) error {
	now := time.Now().UTC()

	var completedAt *time.Time
	if status == DepositStatusCompleted {
		completedAt = &now
	}

	_, err := db.pool.Exec(ctx, `
		UPDATE deposits
		SET status = $1, completed_at = $2
		WHERE id = $3
	`, status, completedAt, depositID)

	if err != nil {
		return fmt.Errorf("failed to update deposit status: %w", err)
	}

	return nil
}

// CompleteDeposit marks a deposit as completed and updates the account balance
func (db *DB) CompleteDeposit(ctx context.Context, depositID uuid.UUID) error {
	// Start a transaction
	tx, err := db.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	// Get the deposit
	deposit := &Deposit{}
	err = tx.QueryRow(ctx, `
		SELECT id, account_id, status, net_amount_usdc
		FROM deposits
		WHERE id = $1
		FOR UPDATE
	`, depositID).Scan(&deposit.ID, &deposit.AccountID, &deposit.Status, &deposit.NetAmountUSDC)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return errors.New("deposit not found")
		}
		return fmt.Errorf("failed to get deposit: %w", err)
	}

	if deposit.Status != DepositStatusPending {
		return fmt.Errorf("deposit is not pending, current status: %s", deposit.Status)
	}

	// Update deposit status - the database trigger handles updating the account balance
	now := time.Now().UTC()
	_, err = tx.Exec(ctx, `
		UPDATE deposits
		SET status = $1, completed_at = $2
		WHERE id = $3
	`, DepositStatusCompleted, now, depositID)

	if err != nil {
		return fmt.Errorf("failed to update deposit status: %w", err)
	}

	// Commit transaction
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// FailDeposit marks a deposit as failed
func (db *DB) FailDeposit(ctx context.Context, depositID uuid.UUID, reason string) error {
	_, err := db.pool.Exec(ctx, `
		UPDATE deposits
		SET status = $1, metadata = COALESCE(metadata, '{}'::jsonb) || jsonb_build_object('failure_reason', $2::text)
		WHERE id = $3
	`, DepositStatusFailed, reason, depositID)

	if err != nil {
		return fmt.Errorf("failed to mark deposit as failed: %w", err)
	}

	return nil
}

// GetPendingDeposits retrieves all pending deposits (for background processing)
func (db *DB) GetPendingDeposits(ctx context.Context, limit int) ([]*Deposit, error) {
	if limit <= 0 {
		limit = 100
	}

	rows, err := db.pool.Query(ctx, `
		SELECT id, account_id, provider, amount_usdc, fee_usdc, net_amount_usdc,
		       status, provider_transaction_id, wallet_address, metadata, created_at, completed_at
		FROM deposits
		WHERE status = $1
		ORDER BY created_at ASC
		LIMIT $2
	`, DepositStatusPending, limit)

	if err != nil {
		return nil, fmt.Errorf("failed to get pending deposits: %w", err)
	}
	defer rows.Close()

	var deposits []*Deposit
	for rows.Next() {
		deposit := &Deposit{}
		err := rows.Scan(
			&deposit.ID, &deposit.AccountID, &deposit.Provider, &deposit.AmountUSDC,
			&deposit.FeeUSDC, &deposit.NetAmountUSDC, &deposit.Status,
			&deposit.ProviderTransactionID, &deposit.WalletAddress,
			&deposit.Metadata, &deposit.CreatedAt, &deposit.CompletedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan deposit: %w", err)
		}
		deposits = append(deposits, deposit)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating deposits: %w", err)
	}

	return deposits, nil
}

// UpdateDepositProviderTransaction updates the provider transaction ID for a deposit
func (db *DB) UpdateDepositProviderTransaction(ctx context.Context, depositID uuid.UUID, providerTxID string) error {
	_, err := db.pool.Exec(ctx, `
		UPDATE deposits
		SET provider_transaction_id = $1
		WHERE id = $2
	`, providerTxID, depositID)

	if err != nil {
		return fmt.Errorf("failed to update provider transaction ID: %w", err)
	}

	return nil
}

// GetDepositStats retrieves deposit statistics for an account
func (db *DB) GetDepositStats(ctx context.Context, accountID uuid.UUID) (*DepositStats, error) {
	stats := &DepositStats{}

	err := db.QueryRow(ctx, `
		SELECT
			COALESCE(COUNT(*), 0) as total_deposits,
			COALESCE(SUM(CASE WHEN status = 'completed' THEN net_amount_usdc ELSE 0 END), 0) as total_deposited_usdc,
			COALESCE(SUM(CASE WHEN status = 'pending' THEN amount_usdc ELSE 0 END), 0) as pending_amount_usdc
		FROM deposits
		WHERE account_id = $1
	`, accountID).Scan(
		&stats.TotalDeposits,
		&stats.TotalDepositedUSDC,
		&stats.PendingAmountUSDC,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to get deposit stats: %w", err)
	}

	return stats, nil
}

// DepositStats represents deposit statistics
type DepositStats struct {
	TotalDeposits      int64          `json:"total_deposits"`
	TotalDepositedUSDC usdc.MicroUSDC `json:"total_deposited_usdc"`
	PendingAmountUSDC  usdc.MicroUSDC `json:"pending_amount_usdc"`
}
