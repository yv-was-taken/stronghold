package db

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Session represents a user session with refresh token
type Session struct {
	ID               uuid.UUID  `json:"id"`
	AccountID        uuid.UUID  `json:"account_id"`
	RefreshTokenHash string     `json:"-"` // Never expose in JSON
	ExpiresAt        time.Time  `json:"expires_at"`
	CreatedAt        time.Time  `json:"created_at"`
	LastUsedAt       *time.Time `json:"last_used_at,omitempty"`
	IPAddress        *net.IP    `json:"ip_address,omitempty"`
	UserAgent        *string    `json:"user_agent,omitempty"`
}

// GenerateRefreshToken creates a cryptographically secure refresh token
func GenerateRefreshToken() (string, error) {
	// Generate 32 random bytes (256 bits)
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("failed to generate refresh token: %w", err)
	}
	// Encode as URL-safe base64
	return base64.URLEncoding.EncodeToString(bytes), nil
}

// CreateSession creates a new session for an account
func (db *DB) CreateSession(ctx context.Context, accountID uuid.UUID, ipAddress net.IP, userAgent string, duration time.Duration) (*Session, string, error) {
	// Generate refresh token
	refreshToken, err := GenerateRefreshToken()
	if err != nil {
		return nil, "", err
	}

	tokenHash := HashToken(refreshToken)
	now := time.Now().UTC()

	session := &Session{
		ID:               uuid.New(),
		AccountID:        accountID,
		RefreshTokenHash: tokenHash,
		ExpiresAt:        now.Add(duration),
		CreatedAt:        now,
		LastUsedAt:       &now,
	}

	if ipAddress != nil {
		session.IPAddress = &ipAddress
	}
	if userAgent != "" {
		session.UserAgent = &userAgent
	}

	_, err = db.pool.Exec(ctx, `
		INSERT INTO sessions (id, account_id, refresh_token_hash, expires_at, created_at, last_used_at, ip_address, user_agent)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`, session.ID, session.AccountID, session.RefreshTokenHash,
		session.ExpiresAt, session.CreatedAt, session.LastUsedAt,
		session.IPAddress, session.UserAgent)

	if err != nil {
		return nil, "", fmt.Errorf("failed to create session: %w", err)
	}

	return session, refreshToken, nil
}

// GetSessionByRefreshToken retrieves a session by its refresh token
func (db *DB) GetSessionByRefreshToken(ctx context.Context, refreshToken string) (*Session, error) {
	tokenHash := HashToken(refreshToken)

	session := &Session{}
	err := db.QueryRow(ctx, `
		SELECT id, account_id, refresh_token_hash, expires_at, created_at, last_used_at, ip_address, user_agent
		FROM sessions
		WHERE refresh_token_hash = $1
	`, tokenHash).Scan(
		&session.ID, &session.AccountID, &session.RefreshTokenHash,
		&session.ExpiresAt, &session.CreatedAt, &session.LastUsedAt,
		&session.IPAddress, &session.UserAgent,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, errors.New("session not found")
		}
		return nil, fmt.Errorf("failed to get session: %w", err)
	}

	// Check if session is expired
	if time.Now().UTC().After(session.ExpiresAt) {
		return nil, errors.New("session expired")
	}

	return session, nil
}

// UpdateSessionLastUsed updates the last used timestamp for a session
func (db *DB) UpdateSessionLastUsed(ctx context.Context, sessionID uuid.UUID) error {
	now := time.Now().UTC()
	_, err := db.pool.Exec(ctx, `
		UPDATE sessions
		SET last_used_at = $1
		WHERE id = $2
	`, now, sessionID)

	if err != nil {
		return fmt.Errorf("failed to update session last used: %w", err)
	}

	return nil
}

// DeleteSession deletes a session by its ID
func (db *DB) DeleteSession(ctx context.Context, sessionID uuid.UUID) error {
	_, err := db.pool.Exec(ctx, `
		DELETE FROM sessions
		WHERE id = $1
	`, sessionID)

	if err != nil {
		return fmt.Errorf("failed to delete session: %w", err)
	}

	return nil
}

// DeleteSessionByRefreshToken deletes a session by its refresh token
func (db *DB) DeleteSessionByRefreshToken(ctx context.Context, refreshToken string) error {
	tokenHash := HashToken(refreshToken)
	_, err := db.pool.Exec(ctx, `
		DELETE FROM sessions
		WHERE refresh_token_hash = $1
	`, tokenHash)

	if err != nil {
		return fmt.Errorf("failed to delete session: %w", err)
	}

	return nil
}

// DeleteAllAccountSessions deletes all sessions for an account
func (db *DB) DeleteAllAccountSessions(ctx context.Context, accountID uuid.UUID) error {
	_, err := db.pool.Exec(ctx, `
		DELETE FROM sessions
		WHERE account_id = $1
	`, accountID)

	if err != nil {
		return fmt.Errorf("failed to delete account sessions: %w", err)
	}

	return nil
}

// GetAccountSessions retrieves all active sessions for an account
func (db *DB) GetAccountSessions(ctx context.Context, accountID uuid.UUID) ([]*Session, error) {
	rows, err := db.pool.Query(ctx, `
		SELECT id, account_id, refresh_token_hash, expires_at, created_at, last_used_at, ip_address, user_agent
		FROM sessions
		WHERE account_id = $1 AND expires_at > $2
		ORDER BY last_used_at DESC
	`, accountID, time.Now().UTC())

	if err != nil {
		return nil, fmt.Errorf("failed to get account sessions: %w", err)
	}
	defer rows.Close()

	var sessions []*Session
	for rows.Next() {
		session := &Session{}
		err := rows.Scan(
			&session.ID, &session.AccountID, &session.RefreshTokenHash,
			&session.ExpiresAt, &session.CreatedAt, &session.LastUsedAt,
			&session.IPAddress, &session.UserAgent,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan session: %w", err)
		}
		sessions = append(sessions, session)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating sessions: %w", err)
	}

	return sessions, nil
}

// CleanupExpiredSessions deletes all expired sessions
func (db *DB) CleanupExpiredSessions(ctx context.Context) (int64, error) {
	result, err := db.pool.Exec(ctx, `
		DELETE FROM sessions
		WHERE expires_at < $1
	`, time.Now().UTC())

	if err != nil {
		return 0, fmt.Errorf("failed to cleanup expired sessions: %w", err)
	}

	return result.RowsAffected(), nil
}

// RotateRefreshToken creates a new refresh token for a session
func (db *DB) RotateRefreshToken(ctx context.Context, oldRefreshToken string, duration time.Duration) (*Session, string, error) {
	// Get the session first
	session, err := db.GetSessionByRefreshToken(ctx, oldRefreshToken)
	if err != nil {
		return nil, "", err
	}

	// Generate new refresh token
	newRefreshToken, err := GenerateRefreshToken()
	if err != nil {
		return nil, "", err
	}

	newTokenHash := HashToken(newRefreshToken)
	now := time.Now().UTC()
	newExpiresAt := now.Add(duration)

	// Update the session with new token
	_, err = db.pool.Exec(ctx, `
		UPDATE sessions
		SET refresh_token_hash = $1, expires_at = $2, last_used_at = $3
		WHERE id = $4
	`, newTokenHash, newExpiresAt, now, session.ID)

	if err != nil {
		return nil, "", fmt.Errorf("failed to rotate refresh token: %w", err)
	}

	// Update session object
	session.RefreshTokenHash = newTokenHash
	session.ExpiresAt = newExpiresAt
	session.LastUsedAt = &now

	return session, newRefreshToken, nil
}
