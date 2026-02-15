// Package testutil provides testing utilities for database operations
package testutil

import (
	"context"
	"fmt"
	"io/fs"
	"os/exec"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"stronghold/internal/db/migrations"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

var (
	dockerAvailable     bool
	dockerAvailableOnce sync.Once
)

// IsDockerAvailable checks if Docker is available and running
func IsDockerAvailable() bool {
	dockerAvailableOnce.Do(func() {
		// Check if docker command exists
		_, err := exec.LookPath("docker")
		if err != nil {
			dockerAvailable = false
			return
		}

		// Check if docker daemon is running
		cmd := exec.Command("docker", "info")
		err = cmd.Run()
		dockerAvailable = err == nil
	})
	return dockerAvailable
}

// SkipIfNoDocker skips the test if Docker is not available
func SkipIfNoDocker(t *testing.T) {
	t.Helper()
	if !IsDockerAvailable() {
		t.Skip("Docker is not available, skipping test")
	}
}

// TestDB holds a test database container and connection pool
type TestDB struct {
	Container testcontainers.Container
	Pool      *pgxpool.Pool
	Host      string
	Port      string
	User      string
	Password  string
	Database  string
}

// ContainerConfig holds configuration for the test container
type ContainerConfig struct {
	PostgresVersion string
	Database        string
	User            string
	Password        string
}

// DefaultContainerConfig returns the default container configuration
func DefaultContainerConfig() ContainerConfig {
	return ContainerConfig{
		PostgresVersion: "16-alpine",
		Database:        "stronghold_test",
		User:            "stronghold_test",
		Password:        "test_password",
	}
}

// NewTestDB creates a new PostgreSQL test container with migrations applied
func NewTestDB(t *testing.T) *TestDB {
	return NewTestDBWithConfig(t, DefaultContainerConfig())
}

// NewTestDBWithConfig creates a new PostgreSQL test container with custom config
func NewTestDBWithConfig(t *testing.T, cfg ContainerConfig) *TestDB {
	t.Helper()

	testDB := newContainer(t, cfg)

	// Apply migrations
	if err := testDB.ApplyMigrations(t); err != nil {
		testDB.Close(t)
		t.Fatalf("Failed to apply migrations: %v", err)
	}

	return testDB
}

// NewBareTestDB creates a new PostgreSQL test container without applying migrations.
// This is useful for testing the migration runner itself.
func NewBareTestDB(t *testing.T) *TestDB {
	t.Helper()
	return newContainer(t, DefaultContainerConfig())
}

// newContainer creates a PostgreSQL test container and returns a connected TestDB.
func newContainer(t *testing.T, cfg ContainerConfig) *TestDB {
	t.Helper()

	// Skip test if Docker is not available
	SkipIfNoDocker(t)

	ctx := context.Background()

	// Create PostgreSQL container
	req := testcontainers.ContainerRequest{
		Image:        fmt.Sprintf("postgres:%s", cfg.PostgresVersion),
		ExposedPorts: []string{"5432/tcp"},
		Env: map[string]string{
			"POSTGRES_DB":       cfg.Database,
			"POSTGRES_USER":     cfg.User,
			"POSTGRES_PASSWORD": cfg.Password,
		},
		WaitingFor: wait.ForLog("database system is ready to accept connections").
			WithOccurrence(2).
			WithStartupTimeout(60 * time.Second),
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		t.Fatalf("Failed to start PostgreSQL container: %v", err)
	}

	// Get host and port
	host, err := container.Host(ctx)
	if err != nil {
		container.Terminate(ctx)
		t.Fatalf("Failed to get container host: %v", err)
	}

	mappedPort, err := container.MappedPort(ctx, "5432")
	if err != nil {
		container.Terminate(ctx)
		t.Fatalf("Failed to get container port: %v", err)
	}

	// Create connection pool
	connString := fmt.Sprintf(
		"postgres://%s:%s@%s:%s/%s?sslmode=disable",
		cfg.User, cfg.Password, host, mappedPort.Port(), cfg.Database,
	)

	poolConfig, err := pgxpool.ParseConfig(connString)
	if err != nil {
		container.Terminate(ctx)
		t.Fatalf("Failed to parse connection string: %v", err)
	}

	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		container.Terminate(ctx)
		t.Fatalf("Failed to create connection pool: %v", err)
	}

	// Verify connection
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		container.Terminate(ctx)
		t.Fatalf("Failed to ping database: %v", err)
	}

	return &TestDB{
		Container: container,
		Pool:      pool,
		Host:      host,
		Port:      mappedPort.Port(),
		User:      cfg.User,
		Password:  cfg.Password,
		Database:  cfg.Database,
	}
}

// ApplyMigrations applies all migrations from the embedded migrations FS
func (tdb *TestDB) ApplyMigrations(t *testing.T) error {
	t.Helper()

	ctx := context.Background()
	migrationsFS := migrations.FS()

	// Read migration files
	entries, err := fs.ReadDir(migrationsFS, ".")
	if err != nil {
		return fmt.Errorf("failed to read embedded migrations: %w", err)
	}

	// Sort files to ensure correct order
	var migrations []string
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".sql") {
			migrations = append(migrations, entry.Name())
		}
	}
	sort.Strings(migrations)

	// Apply each migration
	for _, migration := range migrations {
		content, err := fs.ReadFile(migrationsFS, migration)
		if err != nil {
			return fmt.Errorf("failed to read migration %s: %w", migration, err)
		}

		_, err = tdb.Pool.Exec(ctx, string(content))
		if err != nil {
			return fmt.Errorf("failed to apply migration %s: %w", migration, err)
		}

		t.Logf("Applied migration: %s", migration)
	}

	return nil
}

// Close terminates the container and closes the connection pool
func (tdb *TestDB) Close(t *testing.T) {
	t.Helper()

	if tdb.Pool != nil {
		tdb.Pool.Close()
	}

	if tdb.Container != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if err := tdb.Container.Terminate(ctx); err != nil {
			t.Logf("Warning: failed to terminate container: %v", err)
		}
	}
}

// Truncate removes all data from all tables (preserves schema)
func (tdb *TestDB) Truncate(t *testing.T) {
	t.Helper()

	ctx := context.Background()

	tables := []string{
		"usage_logs",
		"deposits",
		"sessions",
		"payment_transactions",
		"accounts",
	}

	for _, table := range tables {
		_, err := tdb.Pool.Exec(ctx, fmt.Sprintf("TRUNCATE TABLE %s CASCADE", table))
		if err != nil {
			t.Fatalf("Failed to truncate table %s: %v", table, err)
		}
	}
}

// ConnectionString returns the PostgreSQL connection string
func (tdb *TestDB) ConnectionString() string {
	return fmt.Sprintf(
		"postgres://%s:%s@%s:%s/%s?sslmode=disable",
		tdb.User, tdb.Password, tdb.Host, tdb.Port, tdb.Database,
	)
}

// WithTransaction runs a function within a transaction that is rolled back after the test
func (tdb *TestDB) WithTransaction(t *testing.T, fn func(ctx context.Context)) {
	t.Helper()

	ctx := context.Background()
	tx, err := tdb.Pool.Begin(ctx)
	if err != nil {
		t.Fatalf("Failed to begin transaction: %v", err)
	}
	defer tx.Rollback(ctx)

	fn(ctx)
}
