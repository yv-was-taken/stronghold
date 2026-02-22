// Package db provides PostgreSQL database operations for Stronghold
package db

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// DefaultQueryTimeout is the maximum time allowed for database queries.
// This prevents hanging queries from causing outages.
const DefaultQueryTimeout = 30 * time.Second

// DB wraps a PostgreSQL connection pool
type DB struct {
	pool *pgxpool.Pool
}

// Config holds database configuration
type Config struct {
	Host     string
	Port     string
	User     string
	Password string
	Name     string
	SSLMode  string
	MaxConns int32
}

// LoadConfig loads database configuration from environment variables
func LoadConfig() *Config {
	var maxConns int32
	if v := os.Getenv("DB_MAX_CONNS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			maxConns = int32(n)
		}
	}

	return &Config{
		Host:     getEnv("DB_HOST", "localhost"),
		Port:     getEnv("DB_PORT", "5432"),
		User:     getEnv("DB_USER", "stronghold"),
		Password: getEnv("DB_PASSWORD", ""),
		Name:     getEnv("DB_NAME", "stronghold"),
		SSLMode:  getEnv("DB_SSLMODE", "require"),
		MaxConns: maxConns,
	}
}

// NewFromPool creates a DB instance from an existing connection pool.
// This is primarily useful for testing.
func NewFromPool(pool *pgxpool.Pool) *DB {
	return &DB{pool: pool}
}

// New creates a new database connection pool
func New(cfg *Config) (*DB, error) {
	connString := fmt.Sprintf(
		"postgres://%s:%s@%s:%s/%s?sslmode=%s",
		cfg.User, cfg.Password, cfg.Host, cfg.Port, cfg.Name, cfg.SSLMode,
	)

	poolConfig, err := pgxpool.ParseConfig(connString)
	if err != nil {
		return nil, fmt.Errorf("failed to parse connection string: %w", err)
	}

	// Configure pool settings
	maxConns := cfg.MaxConns
	if maxConns <= 0 {
		maxConns = 25
	}
	poolConfig.MaxConns = maxConns
	poolConfig.MinConns = 5
	poolConfig.MaxConnLifetime = time.Hour
	poolConfig.MaxConnIdleTime = 30 * time.Minute
	poolConfig.HealthCheckPeriod = 5 * time.Minute

	pool, err := pgxpool.NewWithConfig(context.Background(), poolConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create connection pool: %w", err)
	}

	// Verify connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := pool.Ping(ctx); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	return &DB{pool: pool}, nil
}

// Close closes the database connection pool
func (db *DB) Close() {
	if db.pool != nil {
		db.pool.Close()
	}
}

// Pool returns the underlying connection pool
func (db *DB) Pool() *pgxpool.Pool {
	return db.pool
}

// Ping checks database connectivity
func (db *DB) Ping(ctx context.Context) error {
	return db.pool.Ping(ctx)
}

// BeginTx starts a new transaction.
// Note: Callers are responsible for managing transaction timeouts via the provided context.
func (db *DB) BeginTx(ctx context.Context) (pgx.Tx, error) {
	return db.pool.Begin(ctx)
}

// Exec executes a query without returning rows
func (db *DB) Exec(ctx context.Context, sql string, args ...interface{}) error {
	ctx, cancel := context.WithTimeout(ctx, DefaultQueryTimeout)
	defer cancel()
	_, err := db.pool.Exec(ctx, sql, args...)
	return err
}

// ExecResult executes a query and returns the command tag (for RowsAffected checks)
func (db *DB) ExecResult(ctx context.Context, sql string, args ...interface{}) (pgconn.CommandTag, error) {
	ctx, cancel := context.WithTimeout(ctx, DefaultQueryTimeout)
	defer cancel()
	return db.pool.Exec(ctx, sql, args...)
}

// cancelRow wraps pgx.Row to cancel the timeout context when Scan is called.
// This is necessary because pgx defers reading the response to Scan time;
// cancelling the context before Scan (via defer) would cause spurious failures.
//
// IMPORTANT: Callers MUST call Scan on the returned Row. If the Row is
// discarded without Scan, the timeout context will leak.
type cancelRow struct {
	row    pgx.Row
	cancel context.CancelFunc
}

// Scan reads the row result and then cancels the timeout context.
func (r *cancelRow) Scan(dest ...any) error {
	err := r.row.Scan(dest...)
	r.cancel()
	return err
}

// QueryRow executes a query that returns a single row.
// The returned Row holds the timeout context alive until Scan is called.
func (db *DB) QueryRow(ctx context.Context, sql string, args ...interface{}) pgx.Row {
	ctx, cancel := context.WithTimeout(ctx, DefaultQueryTimeout)
	return &cancelRow{row: db.pool.QueryRow(ctx, sql, args...), cancel: cancel}
}

// cancelRows wraps pgx.Rows to call a context cancel function when Close is called.
// This is necessary because Query creates a timeout context that must remain alive
// while the caller iterates over rows, but must be canceled when done.
type cancelRows struct {
	pgx.Rows
	cancel context.CancelFunc
}

// Close closes the rows and cancels the associated context.
func (r *cancelRows) Close() {
	r.Rows.Close()
	r.cancel()
}

// Query executes a query that returns multiple rows.
// The returned Rows must be closed by the caller, which will also cancel the
// timeout context. Do not defer cancel() here since rows need the context alive
// during iteration.
func (db *DB) Query(ctx context.Context, sql string, args ...interface{}) (pgx.Rows, error) {
	ctx, cancel := context.WithTimeout(ctx, DefaultQueryTimeout)
	rows, err := db.pool.Query(ctx, sql, args...)
	if err != nil {
		cancel()
		return nil, err
	}
	return &cancelRows{Rows: rows, cancel: cancel}, nil
}

// HashToken creates a SHA-256 hash of a token for storage
func HashToken(token string) string {
	hash := sha256.Sum256([]byte(token))
	return hex.EncodeToString(hash[:])
}

// getEnv gets an environment variable with a default value
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
