package db

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// APIKey represents an API key for B2B authentication
type APIKey struct {
	ID         uuid.UUID  `json:"id"`
	AccountID  uuid.UUID  `json:"account_id"`
	KeyPrefix  string     `json:"key_prefix"`
	KeyHash    string     `json:"-"`
	Name       string     `json:"name"`
	CreatedAt  time.Time  `json:"created_at"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
	RevokedAt  *time.Time `json:"revoked_at,omitempty"`
}

// ErrAPIKeyLimitReached is returned when the account has reached the maximum number of active API keys.
var ErrAPIKeyLimitReached = errors.New("API key limit reached")

// ErrAPIKeyNotFound is returned when the specified API key does not exist or is already revoked.
var ErrAPIKeyNotFound = errors.New("API key not found or already revoked")

// CreateAPIKey creates a new API key record, enforcing a maximum number of active
// keys per account. The cap is checked atomically under a row lock on the account
// to prevent concurrent requests from exceeding the limit.
func (db *DB) CreateAPIKey(ctx context.Context, accountID uuid.UUID, keyPrefix, keyHash, name string, maxKeys int) (*APIKey, error) {
	key := &APIKey{
		ID:        uuid.New(),
		AccountID: accountID,
		KeyPrefix: keyPrefix,
		KeyHash:   keyHash,
		Name:      name,
		CreatedAt: time.Now().UTC(),
	}

	tx, err := db.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	// Lock the account row to serialize concurrent key creation
	var lockedID uuid.UUID
	if err := tx.QueryRow(ctx, `SELECT id FROM accounts WHERE id = $1 FOR UPDATE`, accountID).Scan(&lockedID); err != nil {
		return nil, fmt.Errorf("failed to lock account: %w", err)
	}

	// Count active keys under the lock
	var count int
	if err := tx.QueryRow(ctx, `SELECT COUNT(*) FROM api_keys WHERE account_id = $1 AND revoked_at IS NULL`, accountID).Scan(&count); err != nil {
		return nil, fmt.Errorf("failed to count API keys: %w", err)
	}
	if count >= maxKeys {
		return nil, ErrAPIKeyLimitReached
	}

	// Insert the key
	if _, err := tx.Exec(ctx, `
		INSERT INTO api_keys (id, account_id, key_prefix, key_hash, name, created_at)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, key.ID, key.AccountID, key.KeyPrefix, key.KeyHash, key.Name, key.CreatedAt); err != nil {
		return nil, fmt.Errorf("failed to create API key: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("failed to commit: %w", err)
	}

	return key, nil
}

// GetAPIKeyByHash retrieves an API key by its SHA-256 hash (must not be revoked)
func (db *DB) GetAPIKeyByHash(ctx context.Context, keyHash string) (*APIKey, error) {
	key := &APIKey{}
	err := db.QueryRow(ctx, `
		SELECT id, account_id, key_prefix, key_hash, name, created_at, last_used_at, revoked_at
		FROM api_keys
		WHERE key_hash = $1 AND revoked_at IS NULL
	`, keyHash).Scan(
		&key.ID, &key.AccountID, &key.KeyPrefix, &key.KeyHash, &key.Name,
		&key.CreatedAt, &key.LastUsedAt, &key.RevokedAt,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrAPIKeyNotFound
		}
		return nil, fmt.Errorf("failed to get API key: %w", err)
	}

	return key, nil
}

// ListAPIKeys lists all non-revoked API keys for an account
func (db *DB) ListAPIKeys(ctx context.Context, accountID uuid.UUID) ([]APIKey, error) {
	rows, err := db.pool.Query(ctx, `
		SELECT id, account_id, key_prefix, key_hash, name, created_at, last_used_at, revoked_at
		FROM api_keys
		WHERE account_id = $1 AND revoked_at IS NULL
		ORDER BY created_at DESC
	`, accountID)
	if err != nil {
		return nil, fmt.Errorf("failed to list API keys: %w", err)
	}
	defer rows.Close()

	var keys []APIKey
	for rows.Next() {
		var key APIKey
		if err := rows.Scan(
			&key.ID, &key.AccountID, &key.KeyPrefix, &key.KeyHash, &key.Name,
			&key.CreatedAt, &key.LastUsedAt, &key.RevokedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan API key: %w", err)
		}
		keys = append(keys, key)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating API keys: %w", err)
	}

	return keys, nil
}

// RevokeAPIKey revokes an API key, verifying ownership
func (db *DB) RevokeAPIKey(ctx context.Context, keyID, accountID uuid.UUID) error {
	result, err := db.pool.Exec(ctx, `
		UPDATE api_keys SET revoked_at = $1
		WHERE id = $2 AND account_id = $3 AND revoked_at IS NULL
	`, time.Now().UTC(), keyID, accountID)

	if err != nil {
		return fmt.Errorf("failed to revoke API key: %w", err)
	}

	if result.RowsAffected() == 0 {
		return ErrAPIKeyNotFound
	}

	return nil
}

// UpdateAPIKeyLastUsed updates the last_used_at timestamp for an API key
func (db *DB) UpdateAPIKeyLastUsed(ctx context.Context, keyID uuid.UUID) error {
	_, err := db.pool.Exec(ctx, `
		UPDATE api_keys SET last_used_at = $1 WHERE id = $2
	`, time.Now().UTC(), keyID)

	if err != nil {
		return fmt.Errorf("failed to update API key last used: %w", err)
	}

	return nil
}

// HasActiveAPIKeys returns true if the account has at least one non-revoked API key.
func (db *DB) HasActiveAPIKeys(ctx context.Context, accountID uuid.UUID) (bool, error) {
	count, err := db.CountActiveAPIKeys(ctx, accountID)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// CountActiveAPIKeys counts non-revoked API keys for an account
func (db *DB) CountActiveAPIKeys(ctx context.Context, accountID uuid.UUID) (int, error) {
	var count int
	err := db.QueryRow(ctx, `
		SELECT COUNT(*) FROM api_keys WHERE account_id = $1 AND revoked_at IS NULL
	`, accountID).Scan(&count)

	if err != nil {
		return 0, fmt.Errorf("failed to count API keys: %w", err)
	}

	return count, nil
}
