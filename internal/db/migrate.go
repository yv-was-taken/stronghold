package db

import (
	"context"
	"fmt"
	"io/fs"
	"log/slog"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"
	"stronghold/internal/db/migrations"
)

// advisoryLockID is a fixed int64 used for pg_advisory_lock to prevent
// concurrent migration runs across multiple server instances.
const advisoryLockID int64 = 0x5374726F6E67686F // "Strongho" as int64

// Migrate runs all pending database migrations.
// It acquires a dedicated connection from the pool and holds a PostgreSQL
// advisory lock on that connection for the entire run. This guarantees
// the lock and unlock happen on the same session, preventing stuck locks.
func (db *DB) Migrate(ctx context.Context) error {
	// Acquire a dedicated connection so the advisory lock stays on one session.
	conn, err := db.pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("failed to acquire connection for migrations: %w", err)
	}
	defer conn.Release()

	return runMigrations(ctx, conn.Conn())
}

// runMigrations performs the full migration sequence on a single connection.
func runMigrations(ctx context.Context, conn *pgx.Conn) error {
	// Acquire advisory lock to prevent concurrent migrations
	if _, err := conn.Exec(ctx, "SELECT pg_advisory_lock($1)", advisoryLockID); err != nil {
		return fmt.Errorf("failed to acquire migration lock: %w", err)
	}
	defer conn.Exec(ctx, "SELECT pg_advisory_unlock($1)", advisoryLockID) //nolint:errcheck

	// Create schema_migrations table if it doesn't exist
	if _, err := conn.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version TEXT PRIMARY KEY,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)
	`); err != nil {
		return fmt.Errorf("failed to create schema_migrations table: %w", err)
	}

	// Bootstrap: if this is a pre-migration-infrastructure database
	// (accounts table exists but 001_initial_schema isn't tracked), record it.
	if err := bootstrapExisting(ctx, conn); err != nil {
		return fmt.Errorf("failed to bootstrap existing database: %w", err)
	}

	// Read and sort migration files
	migs, err := readMigrations()
	if err != nil {
		return fmt.Errorf("failed to read migrations: %w", err)
	}

	// Get already-applied migrations
	applied, err := appliedMigrations(ctx, conn)
	if err != nil {
		return fmt.Errorf("failed to query applied migrations: %w", err)
	}

	// Apply each pending migration
	for _, m := range migs {
		if applied[m.version] {
			continue
		}

		slog.Info("applying migration", "version", m.version)

		tx, err := conn.Begin(ctx)
		if err != nil {
			return fmt.Errorf("failed to begin transaction for %s: %w", m.version, err)
		}

		if _, err := tx.Exec(ctx, m.sql); err != nil {
			tx.Rollback(ctx) //nolint:errcheck
			return fmt.Errorf("failed to apply migration %s: %w", m.version, err)
		}

		if _, err := tx.Exec(ctx, "INSERT INTO schema_migrations (version) VALUES ($1)", m.version); err != nil {
			tx.Rollback(ctx) //nolint:errcheck
			return fmt.Errorf("failed to record migration %s: %w", m.version, err)
		}

		if err := tx.Commit(ctx); err != nil {
			return fmt.Errorf("failed to commit migration %s: %w", m.version, err)
		}

		slog.Info("applied migration", "version", m.version)
	}

	return nil
}

// migration holds a parsed migration file.
type migration struct {
	version string // e.g. "001_initial_schema"
	sql     string
}

// readMigrations reads all .sql files from the embedded FS, sorted lexicographically.
func readMigrations() ([]migration, error) {
	migrationsFS := migrations.FS()

	entries, err := fs.ReadDir(migrationsFS, ".")
	if err != nil {
		return nil, fmt.Errorf("failed to read migrations directory: %w", err)
	}

	var migs []migration
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		content, err := fs.ReadFile(migrationsFS, entry.Name())
		if err != nil {
			return nil, fmt.Errorf("failed to read migration %s: %w", entry.Name(), err)
		}
		version := strings.TrimSuffix(entry.Name(), ".sql")
		migs = append(migs, migration{version: version, sql: string(content)})
	}

	sort.Slice(migs, func(i, j int) bool {
		return migs[i].version < migs[j].version
	})

	return migs, nil
}

// appliedMigrations returns a set of already-applied migration versions.
func appliedMigrations(ctx context.Context, conn *pgx.Conn) (map[string]bool, error) {
	rows, err := conn.Query(ctx, "SELECT version FROM schema_migrations")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	applied := make(map[string]bool)
	for rows.Next() {
		var version string
		if err := rows.Scan(&version); err != nil {
			return nil, err
		}
		applied[version] = true
	}
	return applied, rows.Err()
}

// bootstrapExisting detects a database that was created before the migration
// infrastructure existed. If the accounts table is present but 001_initial_schema
// isn't tracked, it inserts the record so the migration runner won't try to
// re-apply it.
func bootstrapExisting(ctx context.Context, conn *pgx.Conn) error {
	// Check if accounts table exists (proxy for "initial schema was applied")
	var exists bool
	err := conn.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT FROM information_schema.tables
			WHERE table_schema = 'public' AND table_name = 'accounts'
		)
	`).Scan(&exists)
	if err != nil {
		return err
	}
	if !exists {
		// Fresh database — nothing to bootstrap
		return nil
	}

	// Check if 001_initial_schema is already tracked
	var tracked bool
	err = conn.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM schema_migrations WHERE version = '001_initial_schema'
		)
	`).Scan(&tracked)
	if err != nil {
		return err
	}
	if tracked {
		return nil
	}

	// Bootstrap: record the initial migration as already applied
	slog.Info("bootstrapping existing database: recording 001_initial_schema as applied")
	_, err = conn.Exec(ctx, "INSERT INTO schema_migrations (version) VALUES ('001_initial_schema')")
	return err
}
