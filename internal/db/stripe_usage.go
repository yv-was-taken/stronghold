package db

import (
	"context"
	"fmt"
	"time"

	"stronghold/internal/usdc"

	"github.com/google/uuid"
)

// StripeUsageRecord represents a metered usage record sent to Stripe
type StripeUsageRecord struct {
	ID                  uuid.UUID      `json:"id"`
	AccountID           uuid.UUID      `json:"account_id"`
	UsageLogID          *uuid.UUID     `json:"usage_log_id,omitempty"`
	StripeMeterEventID  *string        `json:"stripe_meter_event_id,omitempty"`
	Endpoint            string         `json:"endpoint"`
	AmountUSDC          usdc.MicroUSDC `json:"amount_usdc"`
	CreatedAt           time.Time      `json:"created_at"`
}

// CreateStripeUsageRecord creates a new Stripe usage record
func (db *DB) CreateStripeUsageRecord(ctx context.Context, record *StripeUsageRecord) (*StripeUsageRecord, error) {
	record.ID = uuid.New()
	record.CreatedAt = time.Now().UTC()

	_, err := db.pool.Exec(ctx, `
		INSERT INTO stripe_usage_records (id, account_id, usage_log_id, stripe_meter_event_id, endpoint, amount_usdc, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, record.ID, record.AccountID, record.UsageLogID, record.StripeMeterEventID,
		record.Endpoint, record.AmountUSDC, record.CreatedAt)

	if err != nil {
		return nil, fmt.Errorf("failed to create stripe usage record: %w", err)
	}

	return record, nil
}

// GetStripeUsageRecords retrieves Stripe usage records for an account with pagination
func (db *DB) GetStripeUsageRecords(ctx context.Context, accountID uuid.UUID, limit, offset int) ([]StripeUsageRecord, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 1000 {
		limit = 1000
	}

	rows, err := db.pool.Query(ctx, `
		SELECT id, account_id, usage_log_id, stripe_meter_event_id, endpoint, amount_usdc, created_at
		FROM stripe_usage_records
		WHERE account_id = $1
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3
	`, accountID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to get stripe usage records: %w", err)
	}
	defer rows.Close()

	var records []StripeUsageRecord
	for rows.Next() {
		var r StripeUsageRecord
		if err := rows.Scan(
			&r.ID, &r.AccountID, &r.UsageLogID, &r.StripeMeterEventID,
			&r.Endpoint, &r.AmountUSDC, &r.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan stripe usage record: %w", err)
		}
		records = append(records, r)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating stripe usage records: %w", err)
	}

	return records, nil
}
