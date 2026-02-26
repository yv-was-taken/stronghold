package db

import (
	"context"
	"fmt"
)

// ClaimWebhookEvent atomically claims a webhook event for processing.
// Returns true if the event was claimed (first arrival), false if already
// claimed by a concurrent delivery. Uses INSERT ON CONFLICT DO NOTHING so
// exactly one concurrent caller wins the claim.
func (db *DB) ClaimWebhookEvent(ctx context.Context, eventID, eventType string) (bool, error) {
	result, err := db.pool.Exec(ctx, `
		INSERT INTO processed_webhook_events (event_id, event_type)
		VALUES ($1, $2)
		ON CONFLICT (event_id) DO NOTHING
	`, eventID, eventType)
	if err != nil {
		return false, fmt.Errorf("failed to claim webhook event: %w", err)
	}
	return result.RowsAffected() > 0, nil
}

// UnclaimWebhookEvent removes a previously claimed webhook event, allowing
// Stripe retries to reprocess it. Called when the handler fails so that
// the event is not permanently marked as processed.
func (db *DB) UnclaimWebhookEvent(ctx context.Context, eventID string) error {
	_, err := db.pool.Exec(ctx, `
		DELETE FROM processed_webhook_events WHERE event_id = $1
	`, eventID)
	if err != nil {
		return fmt.Errorf("failed to unclaim webhook event: %w", err)
	}
	return nil
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
