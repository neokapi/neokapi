package storage

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
)

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
// wiring an AfterConnect hook (e.g., graph.AfterConnect for AGE). The pool
// is exposed via PgDB.Pool() for subsystems that need native pgx access.
func OpenPostgresWithPool(connStr string, afterConnect AfterConnectFunc) (*PgDB, error) {
	poolConfig, err := pgxpool.ParseConfig(connStr)
	if err != nil {
		return nil, fmt.Errorf("parse postgres config: %w", err)
	}

	if afterConnect != nil {
		poolConfig.AfterConnect = afterConnect
	}

	pool, err := pgxpool.NewWithConfig(context.Background(), poolConfig)
	if err != nil {
		return nil, fmt.Errorf("create postgres pool: %w", err)
	}

	db := stdlib.OpenDBFromPool(pool)
	pgDB, err := configureAndPing(db, connStr)
	if err != nil {
		pool.Close()
		return nil, err
	}
	pgDB.pool = pool
	return pgDB, nil
}

func configureAndPing(db *sql.DB, connStr string) (*PgDB, error) {
	// Connection pooling.
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(30 * time.Minute)

	if err := db.PingContext(context.Background()); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping postgres: %w", err)
	}

	return &PgDB{DB: db, connStr: connStr}, nil
}

// ConnStr returns the connection string used to open the database.
func (db *PgDB) ConnStr() string {
	return db.connStr
}
