// Package pgtest provides PostgreSQL test infrastructure using testcontainers-go.
// It starts a throwaway PostgreSQL container per test binary and gives each test
// an isolated database to itself.
//
// How that isolation is built depends on whether the binary declared a template
// (see UseTemplate):
//
//   - Declared: the migrations run once into a template database, and each test
//     gets a copy of it. A test finds its schema already built.
//   - Not declared: each test gets an empty schema of its own and whatever store
//     it constructs migrates into that schema from nothing.
//
// The second is the original behaviour and remains the default, so a package
// that has not opted in is unaffected.
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
	// Unique per process. The nanosecond alone is not: `go test` starts a
	// binary per package, several at once, and two that landed in the same
	// bucket would name the same objects. That was survivable when the objects
	// were schemas inside one database and is not now that a binary can create
	// and drop whole databases.
	runID = strconv.Itoa(os.Getpid()) + "_" + strconv.FormatInt(time.Now().UnixNano()%100000, 10)

	// The schema every test database starts from, and the once that builds it.
	templateMu      sync.Mutex
	templateBuild   func(*storage.PgDB) error
	templateName    string
	templateBuiltAt bool
)

// testMaxConns is the connection ceiling for a per-test schema pool. Small
// enough to keep a test binary's total connection use modest, and it is the
// ceiling on both pools at once — see storage.ConfigureSQLOverPool.
const testMaxConns int32 = 5

// UseTemplate declares the schema every test database in this binary starts
// from, and is the difference between a test paying for its schema and
// inheriting one.
//
// Without it each test gets an empty schema and whatever store it constructs
// migrates into it from nothing: 279ms of DDL, against 7.5ms to make the schema
// and 1.4ms to run the same migrations again once they are current. Nearly all
// of a database-backed test's fixed cost is rebuilding a schema identical to
// the one the previous test just dropped.
//
// With it, the migrations run ONCE per test binary into a template database,
// and each test gets `CREATE DATABASE … TEMPLATE` — a file copy, 37ms, plus
// 31ms to drop it after. The stores a test constructs still call their own
// Migrate; they simply find the schema already current and return in 1.4ms.
//
// The build function is passed in rather than imported because it cannot be
// imported. Half the subsystems in `bowrain/migrations` are packages whose own
// internal test files use this one, so a pgtest that reached for
// migrations.Apply would close a cycle through `memory`, `auth` and `brand`.
// Passing it keeps the dependency pointing the way it already points: a package
// hands over the migration it can legally name.
//
//	func TestMain(m *testing.M) {
//	    pgtest.UseTemplate(func(db *storage.PgDB) error { return migrations.Apply(db, nil) })
//	    os.Exit(m.Run())
//	}
//
// A package that does not call it keeps the schema-per-test behaviour exactly,
// which is why this is opt-in: adopting it is a per-package decision about what
// schema that package's tests should find waiting for them.
func UseTemplate(build func(*storage.PgDB) error) {
	templateMu.Lock()
	defer templateMu.Unlock()
	templateBuild = build
}

// NewTestDB returns a *storage.PgDB connected to an isolated PostgreSQL schema.
// The first call in a test binary starts a container (shared across tests);
// subsequent calls reuse it but create a fresh schema per test for isolation.
//
// If Docker is not available, the test is skipped.
func NewTestDB(t *testing.T) *storage.PgDB {
	t.Helper()
	return NewTestDBWithMaxConns(t, testMaxConns)
}

// NewTestDBWithMaxConns is NewTestDB with an explicit connection ceiling, for
// tests that need concurrency to exceed the pool on purpose.
func NewTestDBWithMaxConns(t *testing.T, maxConns int32) *storage.PgDB {
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
		if err := anchorExtensionsInPublic(sharedDB); err != nil {
			sharedMu.Unlock()
			t.Fatalf("anchor extensions in public: %v", err)
		}
	}
	schemaCounter++
	seq := schemaCounter
	schemaName := "t" + runID + "_" + sanitize(t.Name()) + "_" + strconv.Itoa(seq)
	sharedMu.Unlock()

	if templateDeclared() {
		return newFromTemplate(t, seq, maxConns)
	}

	// Create an isolated schema for this test.
	if _, err := sharedDB.Exec("CREATE SCHEMA " + schemaName); err != nil {
		t.Fatalf("create test schema: %v", err)
	}

	// Open a connection pool where every connection uses this schema.
	db, err := openWithSchema(sharedConnStr, schemaName, maxConns)
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

// anchorExtensionsInPublic puts the extensions the schemas depend on into
// public, before any test schema exists.
//
// A PostgreSQL extension belongs to a DATABASE but is installed INTO a SCHEMA,
// and every test here shares one database while getting its own schema at the
// front of search_path. An unqualified CREATE EXTENSION in a migration
// therefore lands in whichever test schema got there first, is invisible to
// every other test's search_path (IF NOT EXISTS makes their own attempt a
// no-op), and is dropped along with that schema when its test ends. Sequential
// tests never see it; a parallel suite fails whichever test loses the draw,
// with "operator class gin_trgm_ops does not exist".
//
// The migrations name public themselves now, so this is the harness's half:
// public is created before any test schema, is on every test's search_path, and
// is never dropped. The ALTER is the self-heal — a database left behind by an
// interrupted run can still hold the extension in a dead schema, where CREATE
// EXTENSION IF NOT EXISTS is a no-op that fixes nothing. pg_trgm is
// relocatable, and the ALTER is a no-op when it is already in public.
func anchorExtensionsInPublic(db *storage.PgDB) error {
	for _, ext := range []string{"pg_trgm"} {
		if _, err := db.Exec("CREATE EXTENSION IF NOT EXISTS " + ext + " WITH SCHEMA public"); err != nil {
			return fmt.Errorf("create %s: %w", ext, err)
		}
		if _, err := db.Exec("ALTER EXTENSION " + ext + " SET SCHEMA public"); err != nil {
			return fmt.Errorf("relocate %s to public: %w", ext, err)
		}
	}
	return nil
}

// openWithSchema opens a PgDB where every connection in the pool
// sets search_path to the given schema via AfterConnect.
func openWithSchema(connStr, schema string, maxConns int32) (*storage.PgDB, error) {
	poolConfig, err := pgxpool.ParseConfig(connStr)
	if err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	poolConfig.MaxConns = maxConns
	poolConfig.AfterConnect = func(ctx context.Context, conn *pgx.Conn) error {
		_, err := conn.Exec(ctx, "SET search_path TO "+schema+", public")
		return err
	}

	pool, err := pgxpool.NewWithConfig(context.Background(), poolConfig)
	if err != nil {
		return nil, fmt.Errorf("create pool: %w", err)
	}

	db := stdlib.OpenDBFromPool(pool)
	storage.ConfigureSQLOverPool(db, maxConns)

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

func templateDeclared() bool {
	templateMu.Lock()
	defer templateMu.Unlock()
	return templateBuild != nil
}

// newFromTemplate hands back a database copied from this binary's template.
//
// A whole database rather than a schema because `CREATE DATABASE … TEMPLATE` is
// the only copy PostgreSQL performs itself: it clones the directory, so what
// the test gets is exactly what the migrations built, including the extensions.
// There is no CREATE SCHEMA … LIKE to do the same one level down.
func newFromTemplate(t *testing.T, seq int, maxConns int32) *storage.PgDB {
	t.Helper()

	tmpl, err := ensureTemplate()
	if err != nil {
		t.Fatalf("build the template database: %v", err)
	}

	// Bounded so the whole thing stays inside PostgreSQL's 63-byte identifier
	// limit, which it silently truncates to — and a truncated name is how two
	// tests come to share a database.
	name := "t" + runID + "_" + strconv.Itoa(seq) + "_" + sanitize(t.Name())
	if len(name) > 60 {
		name = name[:60]
	}

	// A copy cannot be taken while anything is connected to the template. Only
	// ensureTemplate ever connects to it, and it closes its pool before
	// returning, so this is uncontended — but say so if that ever changes,
	// because "source database is being accessed by other users" reads as a
	// PostgreSQL quirk rather than as a leaked connection.
	if _, err := sharedDB.Exec(`CREATE DATABASE "` + name + `" TEMPLATE "` + tmpl + `"`); err != nil {
		t.Fatalf("copy the template database (a connection to %q would prevent this): %v", tmpl, err)
	}

	db, err := openDatabase(sharedConnStr, name, maxConns)
	if err != nil {
		_, _ = sharedDB.Exec(`DROP DATABASE IF EXISTS "` + name + `" WITH (FORCE)`)
		t.Fatalf("open the test database: %v", err)
	}

	t.Cleanup(func() {
		db.Close()
		if p := db.Pool(); p != nil {
			p.Close()
		}
		// FORCE detaches anything still attached (PostgreSQL 13+). Without it a
		// single connection this test forgot would leave the database behind
		// for the rest of the run.
		if _, err := sharedDB.Exec(`DROP DATABASE IF EXISTS "` + name + `" WITH (FORCE)`); err != nil {
			_, _ = sharedDB.Exec(`DROP DATABASE IF EXISTS "` + name + `"`)
		}
	})

	return db
}

// ensureTemplate builds this binary's template database once and returns its
// name. Every later test copies it.
func ensureTemplate() (string, error) {
	templateMu.Lock()
	defer templateMu.Unlock()

	if templateBuiltAt {
		return templateName, nil
	}

	name := "tmpl" + runID
	if _, err := sharedDB.Exec(`DROP DATABASE IF EXISTS "` + name + `" WITH (FORCE)`); err != nil {
		// A server too old for FORCE, or nothing to drop. Either is fine; the
		// CREATE below is the statement that has to succeed.
		_, _ = sharedDB.Exec(`DROP DATABASE IF EXISTS "` + name + `"`)
	}
	if _, err := sharedDB.Exec(`CREATE DATABASE "` + name + `"`); err != nil {
		return "", fmt.Errorf("create template database: %w", err)
	}

	db, err := openDatabase(sharedConnStr, name, testMaxConns)
	if err != nil {
		return "", fmt.Errorf("open template database: %w", err)
	}

	buildErr := func() error {
		// Extensions belong to a database and are copied with it, so putting
		// pg_trgm here is what makes every test database have it — the reason
		// the schema path has to anchor it in public instead.
		if _, err := db.Exec("CREATE EXTENSION IF NOT EXISTS pg_trgm"); err != nil {
			return fmt.Errorf("create pg_trgm in the template: %w", err)
		}
		return templateBuild(db)
	}()

	// Closed unconditionally and before returning: a template with a live
	// connection cannot be copied, so this pool must not outlive the build even
	// when the build failed.
	db.Close()
	if p := db.Pool(); p != nil {
		p.Close()
	}
	if buildErr != nil {
		_, _ = sharedDB.Exec(`DROP DATABASE IF EXISTS "` + name + `" WITH (FORCE)`)
		return "", fmt.Errorf("migrate the template: %w", buildErr)
	}

	templateName = name
	templateBuiltAt = true
	return name, nil
}

// openDatabase opens a pool against another database on the same server.
//
// The database is set on the parsed config rather than spliced into the
// connection string, so it works whichever form the caller supplied — a
// postgres:// URL from the container, a key=value DSN from an operator's
// environment.
func openDatabase(connStr, database string, maxConns int32) (*storage.PgDB, error) {
	poolConfig, err := pgxpool.ParseConfig(connStr)
	if err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	poolConfig.ConnConfig.Database = database
	poolConfig.MaxConns = maxConns

	pool, err := pgxpool.NewWithConfig(context.Background(), poolConfig)
	if err != nil {
		return nil, fmt.Errorf("create pool: %w", err)
	}

	db := stdlib.OpenDBFromPool(pool)
	storage.ConfigureSQLOverPool(db, maxConns)

	if err := db.PingContext(context.Background()); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping: %w", err)
	}

	return storage.WrapPgDB(db, poolConfig.ConnString(), pool), nil
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
