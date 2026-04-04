// Package pgtest provides PostgreSQL test infrastructure using testcontainers-go.
// It starts a throwaway PostgreSQL container per test suite and provides isolated
// schemas for each test.
package pgtest

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/neokapi/neokapi/bowrain/storage"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

var (
	sharedMu      sync.Mutex
	sharedDB      *storage.PgDB
	sharedConnStr string
	sharedCleanup func()
	schemaCounter int
)

// NewTestDB returns a *storage.PgDB connected to an isolated PostgreSQL schema.
// The first call in a test binary starts a container (shared across tests);
// subsequent calls reuse it but create a fresh schema per test for isolation.
//
// If Docker is not available, the test is skipped.
func NewTestDB(t *testing.T) *storage.PgDB {
	t.Helper()

	sharedMu.Lock()
	if sharedDB == nil {
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
		sharedCleanup = cleanup
	}
	schemaCounter++
	schemaName := fmt.Sprintf("test_%s_%d", sanitize(t.Name()), schemaCounter)
	sharedMu.Unlock()

	// Create an isolated schema for this test.
	_, err := sharedDB.Exec(fmt.Sprintf("CREATE SCHEMA %s", schemaName))
	if err != nil {
		t.Fatalf("create test schema: %v", err)
	}

	// Open a new connection that uses this schema as search_path.
	connStr := sharedConnStr + "&search_path=" + schemaName
	db, err := storage.OpenPostgres(connStr)
	if err != nil {
		t.Fatalf("open postgres with schema: %v", err)
	}

	t.Cleanup(func() {
		db.Close()
		// Drop the schema to clean up.
		sharedDB.Exec(fmt.Sprintf("DROP SCHEMA %s CASCADE", schemaName))
	})

	return db
}

// startContainer launches a PostgreSQL testcontainer and returns the connection string.
func startContainer(t *testing.T) (string, func(), error) {
	t.Helper()

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
		return "", nil, fmt.Errorf("start postgres container: %w", err)
	}

	connStr, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		container.Terminate(ctx)
		return "", nil, fmt.Errorf("get connection string: %w", err)
	}

	cleanup := func() {
		container.Terminate(context.Background())
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
