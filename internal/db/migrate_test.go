package db_test

import (
	"context"
	"testing"

	"stronghold/internal/db"
	"stronghold/internal/db/testutil"
)

func TestMigrate_EmptyDatabase(t *testing.T) {
	tdb := testutil.NewBareTestDB(t)
	defer tdb.Close(t)

	database := db.NewFromPool(tdb.Pool)
	ctx := context.Background()

	if err := database.Migrate(ctx); err != nil {
		t.Fatalf("Migrate failed on empty database: %v", err)
	}

	// Verify schema_migrations table exists and has entries
	var count int
	err := tdb.Pool.QueryRow(ctx, "SELECT COUNT(*) FROM schema_migrations").Scan(&count)
	if err != nil {
		t.Fatalf("Failed to query schema_migrations: %v", err)
	}
	if count == 0 {
		t.Fatal("Expected at least one migration to be recorded")
	}

	// Verify the accounts table was created (from 001_initial_schema)
	var exists bool
	err = tdb.Pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT FROM information_schema.tables
			WHERE table_schema = 'public' AND table_name = 'accounts'
		)
	`).Scan(&exists)
	if err != nil {
		t.Fatalf("Failed to check accounts table: %v", err)
	}
	if !exists {
		t.Fatal("Expected accounts table to exist after migration")
	}
}

func TestMigrate_Idempotent(t *testing.T) {
	tdb := testutil.NewBareTestDB(t)
	defer tdb.Close(t)

	database := db.NewFromPool(tdb.Pool)
	ctx := context.Background()

	// Run migrations twice
	if err := database.Migrate(ctx); err != nil {
		t.Fatalf("First Migrate call failed: %v", err)
	}
	if err := database.Migrate(ctx); err != nil {
		t.Fatalf("Second Migrate call failed (not idempotent): %v", err)
	}

	// Verify only one record per migration
	var count int
	err := tdb.Pool.QueryRow(ctx, "SELECT COUNT(*) FROM schema_migrations WHERE version = '001_initial_schema'").Scan(&count)
	if err != nil {
		t.Fatalf("Failed to count migrations: %v", err)
	}
	if count != 1 {
		t.Fatalf("Expected exactly 1 record for 001_initial_schema, got %d", count)
	}
}

func TestMigrate_BootstrapExistingDatabase(t *testing.T) {
	// Simulate a pre-migration-infrastructure database:
	// apply the schema directly, then run Migrate and verify it bootstraps.
	tdb := testutil.NewBareTestDB(t)
	defer tdb.Close(t)

	ctx := context.Background()

	// Apply the initial schema directly (as docker-entrypoint-initdb.d would have)
	if err := tdb.ApplyMigrations(t); err != nil {
		t.Fatalf("Failed to apply migrations directly: %v", err)
	}

	// Now run the migration runner — it should detect the existing tables
	// and bootstrap the tracking record without re-applying the schema.
	database := db.NewFromPool(tdb.Pool)
	if err := database.Migrate(ctx); err != nil {
		t.Fatalf("Migrate failed on existing database: %v", err)
	}

	// Verify 001_initial_schema is tracked
	var tracked bool
	err := tdb.Pool.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM schema_migrations WHERE version = '001_initial_schema')
	`).Scan(&tracked)
	if err != nil {
		t.Fatalf("Failed to query schema_migrations: %v", err)
	}
	if !tracked {
		t.Fatal("Expected 001_initial_schema to be tracked after bootstrap")
	}
}

func TestMigrate_IncrementalOnly(t *testing.T) {
	tdb := testutil.NewBareTestDB(t)
	defer tdb.Close(t)

	database := db.NewFromPool(tdb.Pool)
	ctx := context.Background()

	// Apply all migrations
	if err := database.Migrate(ctx); err != nil {
		t.Fatalf("Migrate failed: %v", err)
	}

	// Count applied migrations
	var countBefore int
	err := tdb.Pool.QueryRow(ctx, "SELECT COUNT(*) FROM schema_migrations").Scan(&countBefore)
	if err != nil {
		t.Fatalf("Failed to count migrations: %v", err)
	}

	// Run again — count should not change
	if err := database.Migrate(ctx); err != nil {
		t.Fatalf("Second Migrate failed: %v", err)
	}

	var countAfter int
	err = tdb.Pool.QueryRow(ctx, "SELECT COUNT(*) FROM schema_migrations").Scan(&countAfter)
	if err != nil {
		t.Fatalf("Failed to count migrations after second run: %v", err)
	}

	if countBefore != countAfter {
		t.Fatalf("Expected migration count to stay at %d, got %d", countBefore, countAfter)
	}
}
