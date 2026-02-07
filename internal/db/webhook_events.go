package db

import (
	"context"
	"fmt"
)

// CheckAndRecordWebhookEvent atomically checks if a webhook event has been processed
// and records it if not. Returns true if the event was already processed (duplicate).
func (db *DB) CheckAndRecordWebhookEvent(ctx context.Context, eventID, eventType string) (bool, error) {
	// Use INSERT ... ON CONFLICT DO NOTHING and check rows affected.
	// If 0 rows affected, the event already exists (duplicate).
	query := `
		INSERT INTO processed_webhook_events (event_id, event_type)
		VALUES ($1, $2)
		ON CONFLICT (event_id) DO NOTHING
	`

	result, err := db.pool.Exec(ctx, query, eventID, eventType)
	if err != nil {
		return false, fmt.Errorf("failed to check/record webhook event: %w", err)
	}

	// If no rows were inserted, the event was already processed
	return result.RowsAffected() == 0, nil
}

// CleanupOldWebhookEvents removes processed webhook events older than the given number of days
func (db *DB) CleanupOldWebhookEvents(ctx context.Context, retentionDays int) (int64, error) {
	query := `
		DELETE FROM processed_webhook_events
		WHERE processed_at < NOW() - make_interval(days => $1)
	`

	result, err := db.pool.Exec(ctx, query, retentionDays)
	if err != nil {
		return 0, fmt.Errorf("failed to cleanup old webhook events: %w", err)
	}

	return result.RowsAffected(), nil
}
