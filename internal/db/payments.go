// Package db provides PostgreSQL database operations for Stronghold
package db

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"stronghold/internal/usdc"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// PaymentStatus represents the state of a payment transaction
type PaymentStatus string

const (
	PaymentStatusReserved  PaymentStatus = "reserved"
	PaymentStatusExecuting PaymentStatus = "executing"
	PaymentStatusSettling  PaymentStatus = "settling"
	PaymentStatusCompleted PaymentStatus = "completed"
	PaymentStatusFailed    PaymentStatus = "failed"
	PaymentStatusExpired   PaymentStatus = "expired"
)

// PaymentTransaction represents an atomic payment in the reserve-commit pattern
type PaymentTransaction struct {
	ID                     uuid.UUID              `json:"id"`
	PaymentNonce           string                 `json:"payment_nonce"`
	PaymentHeader          string                 `json:"payment_header"`
	PayerAddress           string                 `json:"payer_address"`
	ReceiverAddress        string                 `json:"receiver_address"`
	Endpoint               string                 `json:"endpoint"`
	AmountUSDC             usdc.MicroUSDC         `json:"amount_usdc"`
	Network                string                 `json:"network"`
	Status                 PaymentStatus          `json:"status"`
	FacilitatorPaymentID   *string                `json:"facilitator_payment_id,omitempty"`
	SettlementAttempts     int                    `json:"settlement_attempts"`
	LastError              *string                `json:"last_error,omitempty"`
	ServiceResult          map[string]interface{} `json:"service_result,omitempty"`
	CreatedAt              time.Time              `json:"created_at"`
	ExecutedAt             *time.Time             `json:"executed_at,omitempty"`
	SettledAt              *time.Time             `json:"settled_at,omitempty"`
	ExpiresAt              time.Time              `json:"expires_at"`
}

// CreatePaymentTransaction creates a new payment transaction in reserved state
func (db *DB) CreatePaymentTransaction(ctx context.Context, tx *PaymentTransaction) error {
	query := `
		INSERT INTO payment_transactions (
			payment_nonce, payment_header, payer_address, receiver_address,
			endpoint, amount_usdc, network, status, expires_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING id, created_at
	`

	err := db.QueryRow(ctx, query,
		tx.PaymentNonce,
		tx.PaymentHeader,
		tx.PayerAddress,
		tx.ReceiverAddress,
		tx.Endpoint,
		tx.AmountUSDC,
		tx.Network,
		PaymentStatusReserved,
		tx.ExpiresAt,
	).Scan(&tx.ID, &tx.CreatedAt)

	if err != nil {
		return fmt.Errorf("failed to create payment transaction: %w", err)
	}

	tx.Status = PaymentStatusReserved
	return nil
}

// CreateOrGetPaymentTransaction atomically creates a payment transaction or returns an existing one.
// This prevents TOCTOU race conditions where concurrent requests with the same nonce
// could both pass the existence check and then one would fail on insert.
// Returns (transaction, wasCreated, error) where wasCreated is true if a new transaction was inserted.
func (db *DB) CreateOrGetPaymentTransaction(ctx context.Context, tx *PaymentTransaction) (*PaymentTransaction, bool, error) {
	// Use INSERT ... ON CONFLICT DO NOTHING and then check if we got an ID back
	// If no ID returned, the row already existed and we need to fetch it
	query := `
		INSERT INTO payment_transactions (
			payment_nonce, payment_header, payer_address, receiver_address,
			endpoint, amount_usdc, network, status, expires_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT (payment_nonce) DO NOTHING
		RETURNING id, created_at
	`

	err := db.QueryRow(ctx, query,
		tx.PaymentNonce,
		tx.PaymentHeader,
		tx.PayerAddress,
		tx.ReceiverAddress,
		tx.Endpoint,
		tx.AmountUSDC,
		tx.Network,
		PaymentStatusReserved,
		tx.ExpiresAt,
	).Scan(&tx.ID, &tx.CreatedAt)

	if err != nil {
		// No rows returned means the nonce already exists
		if errors.Is(err, pgx.ErrNoRows) {
			// Fetch the existing transaction
			existing, fetchErr := db.GetPaymentByNonce(ctx, tx.PaymentNonce)
			if fetchErr != nil {
				return nil, false, fmt.Errorf("failed to fetch existing payment: %w", fetchErr)
			}
			return existing, false, nil
		}
		return nil, false, fmt.Errorf("failed to create payment transaction: %w", err)
	}

	tx.Status = PaymentStatusReserved
	return tx, true, nil
}

// GetPaymentByNonce retrieves a payment transaction by its nonce (idempotency key)
func (db *DB) GetPaymentByNonce(ctx context.Context, nonce string) (*PaymentTransaction, error) {
	query := `
		SELECT id, payment_nonce, payment_header, payer_address, receiver_address,
			   endpoint, amount_usdc, network, status, facilitator_payment_id,
			   settlement_attempts, last_error, service_result,
			   created_at, executed_at, settled_at, expires_at
		FROM payment_transactions
		WHERE payment_nonce = $1
	`

	var tx PaymentTransaction
	var serviceResultJSON []byte
	err := db.QueryRow(ctx, query, nonce).Scan(
		&tx.ID,
		&tx.PaymentNonce,
		&tx.PaymentHeader,
		&tx.PayerAddress,
		&tx.ReceiverAddress,
		&tx.Endpoint,
		&tx.AmountUSDC,
		&tx.Network,
		&tx.Status,
		&tx.FacilitatorPaymentID,
		&tx.SettlementAttempts,
		&tx.LastError,
		&serviceResultJSON,
		&tx.CreatedAt,
		&tx.ExecutedAt,
		&tx.SettledAt,
		&tx.ExpiresAt,
	)

	if err != nil {
		return nil, err
	}

	if serviceResultJSON != nil {
		if err := json.Unmarshal(serviceResultJSON, &tx.ServiceResult); err != nil {
			return nil, fmt.Errorf("failed to unmarshal service result: %w", err)
		}
	}

	return &tx, nil
}

// GetPaymentByID retrieves a payment transaction by its ID
func (db *DB) GetPaymentByID(ctx context.Context, id uuid.UUID) (*PaymentTransaction, error) {
	query := `
		SELECT id, payment_nonce, payment_header, payer_address, receiver_address,
			   endpoint, amount_usdc, network, status, facilitator_payment_id,
			   settlement_attempts, last_error, service_result,
			   created_at, executed_at, settled_at, expires_at
		FROM payment_transactions
		WHERE id = $1
	`

	var tx PaymentTransaction
	var serviceResultJSON []byte
	err := db.QueryRow(ctx, query, id).Scan(
		&tx.ID,
		&tx.PaymentNonce,
		&tx.PaymentHeader,
		&tx.PayerAddress,
		&tx.ReceiverAddress,
		&tx.Endpoint,
		&tx.AmountUSDC,
		&tx.Network,
		&tx.Status,
		&tx.FacilitatorPaymentID,
		&tx.SettlementAttempts,
		&tx.LastError,
		&serviceResultJSON,
		&tx.CreatedAt,
		&tx.ExecutedAt,
		&tx.SettledAt,
		&tx.ExpiresAt,
	)

	if err != nil {
		return nil, err
	}

	if serviceResultJSON != nil {
		if err := json.Unmarshal(serviceResultJSON, &tx.ServiceResult); err != nil {
			return nil, fmt.Errorf("failed to unmarshal service result: %w", err)
		}
	}

	return &tx, nil
}

// TransitionStatus atomically transitions a payment from one status to another
// Uses FOR UPDATE to prevent concurrent modifications
func (db *DB) TransitionStatus(ctx context.Context, id uuid.UUID, from, to PaymentStatus) error {
	query := `
		UPDATE payment_transactions
		SET status = $3
		WHERE id = $1 AND status = $2
	`

	result, err := db.ExecResult(ctx, query, id, from, to)
	if err != nil {
		return fmt.Errorf("failed to transition status: %w", err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("status transition failed: expected status %s", from)
	}

	return nil
}

// RecordExecution stores the service result on a payment transaction.
// It does NOT transition status — the middleware owns state transitions
// (executing -> settling happens in AtomicPayment after the handler returns).
func (db *DB) RecordExecution(ctx context.Context, id uuid.UUID, result map[string]interface{}) error {
	resultJSON, err := json.Marshal(result)
	if err != nil {
		return fmt.Errorf("failed to marshal service result: %w", err)
	}

	query := `
		UPDATE payment_transactions
		SET service_result = $2, executed_at = NOW()
		WHERE id = $1 AND status = $3
	`

	res, err := db.ExecResult(ctx, query, id, resultJSON, PaymentStatusExecuting)
	if err != nil {
		return fmt.Errorf("failed to record execution: %w", err)
	}

	if res.RowsAffected() == 0 {
		return fmt.Errorf("record execution failed: payment not in executing state")
	}

	return nil
}

// CompleteSettlement marks a payment as successfully settled
func (db *DB) CompleteSettlement(ctx context.Context, id uuid.UUID, facilitatorPaymentID string) error {
	query := `
		UPDATE payment_transactions
		SET status = $2, facilitator_payment_id = $3, settled_at = NOW()
		WHERE id = $1 AND status IN ($4, $5)
	`

	result, err := db.ExecResult(ctx, query, id, PaymentStatusCompleted, facilitatorPaymentID, PaymentStatusSettling, PaymentStatusFailed)
	if err != nil {
		return fmt.Errorf("failed to complete settlement: %w", err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("complete settlement failed: payment not in settling or failed state")
	}

	return nil
}

// FailSettlement records a settlement failure and increments the retry counter
func (db *DB) FailSettlement(ctx context.Context, id uuid.UUID, errorMsg string) error {
	query := `
		UPDATE payment_transactions
		SET status = $2, last_error = $3, settlement_attempts = settlement_attempts + 1
		WHERE id = $1 AND status = $4
	`

	result, err := db.ExecResult(ctx, query, id, PaymentStatusFailed, errorMsg, PaymentStatusSettling)
	if err != nil {
		return fmt.Errorf("failed to record settlement failure: %w", err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("fail settlement failed: payment not in settling state")
	}

	return nil
}

// GetPendingSettlements returns payments that need settlement retry within a transaction.
// The returned pgx.Tx holds FOR UPDATE SKIP LOCKED locks on the selected rows.
// The caller MUST commit or rollback the transaction when done processing.
func (db *DB) GetPendingSettlements(ctx context.Context, maxAttempts int, limit int) ([]*PaymentTransaction, pgx.Tx, error) {
	tx, err := db.pool.Begin(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to begin transaction: %w", err)
	}

	query := `
		SELECT id, payment_nonce, payment_header, payer_address, receiver_address,
			   endpoint, amount_usdc, network, status, facilitator_payment_id,
			   settlement_attempts, last_error, service_result,
			   created_at, executed_at, settled_at, expires_at
		FROM payment_transactions
		WHERE (status = $1 AND settlement_attempts < $2)
		   OR (status = $3 AND executed_at < NOW() - INTERVAL '5 minutes')
		ORDER BY created_at ASC
		LIMIT $4
		FOR UPDATE SKIP LOCKED
	`

	rows, err := tx.Query(ctx, query, PaymentStatusFailed, maxAttempts, PaymentStatusSettling, limit)
	if err != nil {
		_ = tx.Rollback(ctx)
		return nil, nil, fmt.Errorf("failed to query pending settlements: %w", err)
	}
	defer rows.Close()

	var transactions []*PaymentTransaction
	for rows.Next() {
		var ptx PaymentTransaction
		var serviceResultJSON []byte
		err := rows.Scan(
			&ptx.ID,
			&ptx.PaymentNonce,
			&ptx.PaymentHeader,
			&ptx.PayerAddress,
			&ptx.ReceiverAddress,
			&ptx.Endpoint,
			&ptx.AmountUSDC,
			&ptx.Network,
			&ptx.Status,
			&ptx.FacilitatorPaymentID,
			&ptx.SettlementAttempts,
			&ptx.LastError,
			&serviceResultJSON,
			&ptx.CreatedAt,
			&ptx.ExecutedAt,
			&ptx.SettledAt,
			&ptx.ExpiresAt,
		)
		if err != nil {
			_ = tx.Rollback(ctx)
			return nil, nil, fmt.Errorf("failed to scan payment transaction: %w", err)
		}

		if serviceResultJSON != nil {
			if err := json.Unmarshal(serviceResultJSON, &ptx.ServiceResult); err != nil {
				_ = tx.Rollback(ctx)
				return nil, nil, fmt.Errorf("failed to unmarshal service result: %w", err)
			}
		}

		transactions = append(transactions, &ptx)
	}

	return transactions, tx, nil
}

// ExpireStaleReservations marks old reserved payments as expired.
// Also expires payments stuck in 'executing' state for more than 5 minutes,
// which indicates the handler crashed or timed out without completing.
func (db *DB) ExpireStaleReservations(ctx context.Context) (int64, error) {
	query := `
		UPDATE payment_transactions
		SET status = $1
		WHERE (status = $2 AND expires_at < NOW())
		   OR (status = $3 AND created_at < NOW() - INTERVAL '5 minutes')
	`

	result, err := db.ExecResult(ctx, query, PaymentStatusExpired, PaymentStatusReserved, PaymentStatusExecuting)
	if err != nil {
		return 0, fmt.Errorf("failed to expire stale reservations: %w", err)
	}

	return result.RowsAffected(), nil
}

// MarkSettling transitions a payment from failed to settling for retry
func (db *DB) MarkSettling(ctx context.Context, id uuid.UUID) error {
	return db.TransitionStatus(ctx, id, PaymentStatusFailed, PaymentStatusSettling)
}

// LinkUsageLog links a usage log entry to a payment transaction
func (db *DB) LinkUsageLog(ctx context.Context, usageLogID, paymentTxID uuid.UUID) error {
	query := `
		UPDATE usage_logs
		SET payment_transaction_id = $2
		WHERE id = $1
	`

	err := db.Exec(ctx, query, usageLogID, paymentTxID)
	if err != nil {
		return fmt.Errorf("failed to link usage log: %w", err)
	}

	return nil
}
