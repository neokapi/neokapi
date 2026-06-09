// Package pgtest provides PostgreSQL test infrastructure using testcontainers-go.
// It starts a throwaway PostgreSQL container per test suite and provides isolated
// schemas for each test.
package pgtest

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/neokapi/neokapi/bowrain/storage"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

var (
	sharedMu      sync.Mutex
	sharedDB      *storage.PgDB
	sharedConnStr string
	schemaCounter int
	runID         = strconv.FormatInt(time.Now().UnixNano()%100000, 10)
)

// NewTestDB returns a *storage.PgDB connected to an isolated PostgreSQL schema.
// The first call in a test binary starts a container (shared across tests);
// subsequent calls reuse it but create a fresh schema per test for isolation.
//
// If Docker is not available, the test is skipped.
func NewTestDB(t *testing.T) *storage.PgDB {
	t.Helper()

	// In -short mode (PR fast-feedback CI), skip the container-backed suites
	// unless a ready PostgreSQL instance is supplied. Starting a throwaway
	// container per test binary dominates bowrain's test wall-clock; push to
	// main and the nightly run exercise these without -short. An explicit
	// BOWRAIN_TEST_POSTGRES_URL still opts in (e.g. a docker-compose database).
	if testing.Short() && os.Getenv("BOWRAIN_TEST_POSTGRES_URL") == "" {
		t.Skip("skipping PostgreSQL container test in -short mode")
	}

	sharedMu.Lock()
	if sharedDB == nil {
		// Allow using an existing PostgreSQL instance via env var (e.g., from docker compose).
		if envURL := os.Getenv("BOWRAIN_TEST_POSTGRES_URL"); envURL != "" {
			db, err := storage.OpenPostgres(envURL)
			if err != nil {
				sharedMu.Unlock()
				t.Fatalf("open postgres from BOWRAIN_TEST_POSTGRES_URL: %v", err)
			}
			sharedDB = db
			sharedConnStr = envURL
		} else {
			connStr, cleanup, err := startContainer(t)
			if err != nil {
				sharedMu.Unlock()
				t.Skipf("PostgreSQL container not available: %v", err)
				return nil
			}
			db, err := storage.OpenPostgres(connStr)
			if err != nil {
				cleanup()
				sharedMu.Unlock()
				t.Fatalf("open postgres: %v", err)
			}
			sharedDB = db
			sharedConnStr = connStr
			// cleanup is intentionally not called — the container lives for the entire test binary.
			// Docker will clean it up via Ryuk (testcontainers' resource reaper).
		}
	}
	schemaCounter++
	schemaName := "t" + runID + "_" + sanitize(t.Name()) + "_" + strconv.Itoa(schemaCounter)
	sharedMu.Unlock()

	// Create an isolated schema for this test.
	if _, err := sharedDB.Exec("CREATE SCHEMA " + schemaName); err != nil {
		t.Fatalf("create test schema: %v", err)
	}

	// Open a connection pool where every connection uses this schema.
	db, err := openWithSchema(sharedConnStr, schemaName)
	if err != nil {
		t.Fatalf("open postgres with schema: %v", err)
	}

	t.Cleanup(func() {
		db.Close()
		// Close the pool after sql.DB to stop the background health check goroutine.
		if p := db.Pool(); p != nil {
			p.Close()
		}
		_, _ = sharedDB.Exec("DROP SCHEMA " + schemaName + " CASCADE")
	})

	return db
}

// openWithSchema opens a PgDB where every connection in the pool
// sets search_path to the given schema via AfterConnect.
func openWithSchema(connStr, schema string) (*storage.PgDB, error) {
	poolConfig, err := pgxpool.ParseConfig(connStr)
	if err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	poolConfig.AfterConnect = func(ctx context.Context, conn *pgx.Conn) error {
		_, err := conn.Exec(ctx, "SET search_path TO "+schema+", public")
		return err
	}

	pool, err := pgxpool.NewWithConfig(context.Background(), poolConfig)
	if err != nil {
		return nil, fmt.Errorf("create pool: %w", err)
	}

	db := stdlib.OpenDBFromPool(pool)
	db.SetMaxOpenConns(5)
	db.SetMaxIdleConns(2)
	db.SetConnMaxLifetime(5 * time.Minute)

	if err := db.PingContext(context.Background()); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping: %w", err)
	}

	return storage.WrapPgDB(db, connStr, pool), nil
}

// startContainer launches a PostgreSQL testcontainer and returns the connection string.
// It recovers from panics (testcontainers panics when Docker is unreachable) and
// returns them as errors so callers can skip gracefully.
func startContainer(t *testing.T) (connStr string, cleanup func(), err error) {
	t.Helper()

	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("docker not available: %v", r)
		}
	}()

	ctx := context.Background()
	container, err := postgres.Run(ctx,
		"postgres:16-alpine",
		postgres.WithDatabase("bowrain_test"),
		postgres.WithUsername("bowrain"),
		postgres.WithPassword("bowrain"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(30*time.Second),
		),
	)
	if err != nil {
		return "", nil, err
	}

	connStr, err = container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		_ = container.Terminate(ctx)
		return "", nil, err
	}

	cleanup = func() {
		_ = container.Terminate(context.Background())
	}
	return connStr, cleanup, nil
}

// sanitize replaces non-alphanumeric characters with underscores for schema names.
func sanitize(s string) string {
	s = strings.ToLower(s)
	var b strings.Builder
	for _, c := range s {
		if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') {
			b.WriteRune(c)
		} else {
			b.WriteByte('_')
		}
	}
	result := b.String()
	if len(result) > 40 {
		result = result[:40]
	}
	return result
}
