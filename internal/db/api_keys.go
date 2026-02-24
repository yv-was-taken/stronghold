package db

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// ErrAPIKeyNotFound is returned when an API key is not found or already revoked.
var ErrAPIKeyNotFound = errors.New("api key not found or already revoked")

// APIKey represents an API key for B2B authentication
type APIKey struct {
	ID         uuid.UUID  `json:"id"`
	AccountID  uuid.UUID  `json:"account_id"`
	KeyPrefix  string     `json:"key_prefix"`
	Label      string     `json:"label,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
	RevokedAt  *time.Time `json:"revoked_at,omitempty"`
}

// CreateAPIKey generates a new API key for the given account.
// Returns the APIKey metadata and the raw key string (shown once to the user).
func (db *DB) CreateAPIKey(ctx context.Context, accountID uuid.UUID, label string) (*APIKey, string, error) {
	// Generate 32 random bytes
	randomBytes := make([]byte, 32)
	if _, err := rand.Read(randomBytes); err != nil {
		return nil, "", fmt.Errorf("failed to generate random key: %w", err)
	}

	rawKey := "sh_live_" + hex.EncodeToString(randomBytes)
	keyHash := HashToken(rawKey)
	keyPrefix := rawKey[:12] // "sh_live_XXXX"

	apiKey := &APIKey{
		ID:        uuid.New(),
		AccountID: accountID,
		KeyPrefix: keyPrefix,
		Label:     label,
		CreatedAt: time.Now().UTC(),
	}

	_, err := db.pool.Exec(ctx, `
		INSERT INTO api_keys (id, account_id, key_hash, key_prefix, label, created_at)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, apiKey.ID, apiKey.AccountID, keyHash, apiKey.KeyPrefix, apiKey.Label, apiKey.CreatedAt)

	if err != nil {
		return nil, "", fmt.Errorf("failed to create API key: %w", err)
	}

	return apiKey, rawKey, nil
}

// GetAPIKeyByHash looks up an active (non-revoked) API key by its hash and updates last_used_at.
// Only returns keys whose owning account has status 'active'.
func (db *DB) GetAPIKeyByHash(ctx context.Context, keyHash string) (*APIKey, error) {
	apiKey := &APIKey{}
	err := db.QueryRow(ctx, `
		UPDATE api_keys ak
		SET last_used_at = NOW()
		FROM accounts a
		WHERE ak.key_hash = $1
		  AND ak.revoked_at IS NULL
		  AND a.id = ak.account_id
		  AND a.status = 'active'
		RETURNING ak.id, ak.account_id, ak.key_prefix, ak.label, ak.created_at, ak.last_used_at, ak.revoked_at
	`, keyHash).Scan(
		&apiKey.ID, &apiKey.AccountID, &apiKey.KeyPrefix, &apiKey.Label,
		&apiKey.CreatedAt, &apiKey.LastUsedAt, &apiKey.RevokedAt,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrAPIKeyNotFound
		}
		return nil, fmt.Errorf("failed to get API key: %w", err)
	}

	return apiKey, nil
}

// ListAPIKeys returns all API keys for an account, including revoked ones.
func (db *DB) ListAPIKeys(ctx context.Context, accountID uuid.UUID) ([]*APIKey, error) {
	rows, err := db.Query(ctx, `
		SELECT id, account_id, key_prefix, label, created_at, last_used_at, revoked_at
		FROM api_keys
		WHERE account_id = $1
		ORDER BY created_at DESC
	`, accountID)

	if err != nil {
		return nil, fmt.Errorf("failed to list API keys: %w", err)
	}
	defer rows.Close()

	var keys []*APIKey
	for rows.Next() {
		key := &APIKey{}
		err := rows.Scan(
			&key.ID, &key.AccountID, &key.KeyPrefix, &key.Label,
			&key.CreatedAt, &key.LastUsedAt, &key.RevokedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan API key: %w", err)
		}
		keys = append(keys, key)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating API keys: %w", err)
	}

	return keys, nil
}

// RevokeAPIKey soft-deletes an API key by setting revoked_at.
// Validates that the key belongs to the specified account.
func (db *DB) RevokeAPIKey(ctx context.Context, accountID uuid.UUID, keyID uuid.UUID) error {
	result, err := db.ExecResult(ctx, `
		UPDATE api_keys
		SET revoked_at = NOW()
		WHERE id = $1 AND account_id = $2 AND revoked_at IS NULL
	`, keyID, accountID)

	if err != nil {
		return fmt.Errorf("failed to revoke API key: %w", err)
	}

	if result.RowsAffected() == 0 {
		return ErrAPIKeyNotFound
	}

	return nil
}

// HasActiveAPIKeys checks if an account has any non-revoked API keys.
func (db *DB) HasActiveAPIKeys(ctx context.Context, accountID uuid.UUID) (bool, error) {
	var hasKeys bool
	err := db.QueryRow(ctx, `
		SELECT EXISTS(SELECT 1 FROM api_keys WHERE account_id = $1 AND revoked_at IS NULL)
	`, accountID).Scan(&hasKeys)

	if err != nil {
		return false, fmt.Errorf("failed to check API keys: %w", err)
	}

	return hasKeys, nil
}
