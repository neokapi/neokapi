package storage

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
)

// DefaultMaxConns is the connection ceiling applied to a PostgreSQL handle when
// the connection string does not name one via pool_max_conns.
//
// It is set on the pgxpool, not only on the database/sql handle in front of it,
// because pgxpool's own default is max(4, NumCPU) — on a two-vCPU container,
// four. See ConfigureSQLOverPool for why the two ceilings must be the same
// number rather than merely compatible ones.
const DefaultMaxConns int32 = 25

// PgDB wraps a sql.DB connected to PostgreSQL with shared configuration applied.
// When opened via OpenPostgresWithPool, a pgxpool.Pool is available for
// subsystems (like AGE graph) that need native pgx features.
type PgDB struct {
	*sql.DB
	connStr string
	pool    *pgxpool.Pool // nil when opened via sql.Open (no pgx pool)
}

// Pool returns the underlying pgxpool.Pool, or nil if not available.
// The AGE graph store requires a pool for AfterConnect hooks.
func (db *PgDB) Pool() *pgxpool.Pool {
	return db.pool
}

// WrapPgDB creates a PgDB from an existing *sql.DB and optional pool.
// Used by test infrastructure that needs to configure connections before wrapping.
func WrapPgDB(db *sql.DB, connStr string, pool *pgxpool.Pool) *PgDB {
	return &PgDB{DB: db, connStr: connStr, pool: pool}
}

// AfterConnectFunc is the type for pgx AfterConnect hooks.
type AfterConnectFunc func(ctx context.Context, conn *pgx.Conn) error

// OpenPostgres opens a PostgreSQL database with the given connection string.
// The connection string should be a PostgreSQL DSN or URL, e.g.:
//
//	"postgres://user:pass@host:5432/dbname?sslmode=disable"
func OpenPostgres(connStr string) (*PgDB, error) {
	return OpenPostgresWithPool(connStr, nil)
}

// OpenPostgresWithPool opens a PostgreSQL database via pgxpool, optionally
// wiring an AfterConnect hook for per-connection session setup. The pool
// is exposed via PgDB.Pool() for subsystems that need native pgx access.
func OpenPostgresWithPool(connStr string, afterConnect AfterConnectFunc) (*PgDB, error) {
	poolConfig, err := pgxpool.ParseConfig(connStr)
	if err != nil {
		return nil, fmt.Errorf("parse postgres config: %w", err)
	}

	// pgxpool.ParseConfig fills MaxConns with max(4, NumCPU) when the
	// connection string is silent, which is a container-sized number, not a
	// server-sized one. Name the ceiling here so the deployment's concurrency
	// does not depend on how many cores the scheduler handed the task.
	if !dsnSetsPoolMaxConns(connStr) {
		poolConfig.MaxConns = DefaultMaxConns
	}

	if afterConnect != nil {
		poolConfig.AfterConnect = afterConnect
	}

	pool, err := pgxpool.NewWithConfig(context.Background(), poolConfig)
	if err != nil {
		return nil, fmt.Errorf("create postgres pool: %w", err)
	}

	db := stdlib.OpenDBFromPool(pool)
	ConfigureSQLOverPool(db, poolConfig.MaxConns)

	if err := db.PingContext(context.Background()); err != nil {
		db.Close()
		pool.Close()
		return nil, fmt.Errorf("ping postgres: %w", err)
	}

	return &PgDB{DB: db, connStr: connStr, pool: pool}, nil
}

// ConfigureSQLOverPool settles a database/sql handle that is backed by a
// pgxpool, where there are two connection ceilings and only one set of
// connections.
//
// A pool-backed handle acquires a pgxpool connection when it opens a driver
// connection and releases it when it closes one. So database/sql must never
// believe it may open more connections than the pool can supply: when it does,
// the surplus openers block inside pgxpool.Acquire, and that is a wait
// database/sql cannot see. Its own waiter queue — the queue a returned
// connection wakes — is the one taken by callers that found no free connection
// AND no room to open one. A caller that took the "open a new one" branch has
// left that queue, so every connection returned afterwards goes to an idle list
// it will never look at again. It waits until its context is canceled, however
// long that is, while requests arriving later are served from the idle list in
// milliseconds. Nothing looks unhealthy from the outside: the handle reports
// idle connections and zero waiters while the stranded callers hang.
//
// So the two ceilings are set to the same number, which makes the "open a new
// one" branch reachable only when the pool has a connection to give, and puts
// all real contention in database/sql's own queue, where a returned connection
// wakes a waiter.
//
// Idle connections are left to pgxpool (the stdlib pool connector's own
// default, which OpenDBFromPool sets and this must not undo): a connection
// database/sql holds idle is one it has acquired and not released, so an idle
// list here is connections withheld from every other caller of the same pool.
func ConfigureSQLOverPool(db *sql.DB, poolMaxConns int32) {
	db.SetMaxOpenConns(int(poolMaxConns))
	db.SetMaxIdleConns(0)
}

// dsnSetsPoolMaxConns reports whether the connection string names a pool size
// itself, in either form pgxpool accepts: a URL query parameter, or a
// keyword/value pair. When it does, that number is the operator's — a
// deployment budgets connections across its replicas against the server's
// max_connections, and a default must not raise it.
func dsnSetsPoolMaxConns(connStr string) bool {
	const key = "pool_max_conns"
	if u, err := url.Parse(connStr); err == nil && (u.Scheme == "postgres" || u.Scheme == "postgresql") {
		return u.Query().Has(key)
	}
	for _, field := range strings.Fields(connStr) {
		if k, _, ok := strings.Cut(field, "="); ok && k == key {
			return true
		}
	}
	return false
}

// ConnStr returns the connection string used to open the database.
func (db *PgDB) ConnStr() string {
	return db.connStr
}
