package storage

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
)

// PgDB wraps a sql.DB connected to PostgreSQL with shared configuration applied.
// When opened via OpenPostgresWithPool or OpenPostgresAzure, a pgxpool.Pool is
// available for subsystems (like AGE graph) that need native pgx features.
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

// OpenPostgresAzure opens a PostgreSQL database using Azure Managed Identity
// for authentication. An Entra ID access token is fetched before each new
// connection, so credentials are never stored and rotate automatically.
//
// clientID is the user-assigned managed identity client ID. If empty,
// the system-assigned managed identity is used.
func OpenPostgresAzure(connStr string, clientID string) (*PgDB, error) {
	poolConfig, err := pgxpool.ParseConfig(connStr)
	if err != nil {
		return nil, fmt.Errorf("parse postgres pool config: %w", err)
	}

	var cred *azidentity.ManagedIdentityCredential
	if clientID != "" {
		cred, err = azidentity.NewManagedIdentityCredential(&azidentity.ManagedIdentityCredentialOptions{
			ID: azidentity.ClientID(clientID),
		})
	} else {
		cred, err = azidentity.NewManagedIdentityCredential(nil)
	}
	if err != nil {
		return nil, fmt.Errorf("create managed identity credential: %w", err)
	}

	// Fetch a fresh Entra ID token before each new connection.
	poolConfig.BeforeConnect = func(ctx context.Context, cfg *pgx.ConnConfig) error {
		token, err := cred.GetToken(ctx, policy.TokenRequestOptions{
			Scopes: []string{"https://ossrdbms-aad.database.windows.net/.default"},
		})
		if err != nil {
			return fmt.Errorf("get azure token: %w", err)
		}
		cfg.Password = token.Token
		return nil
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

// OpenPostgresAzureWithHook is like OpenPostgresAzure but also wires an
// AfterConnect hook (e.g., for AGE graph support).
func OpenPostgresAzureWithHook(connStr, clientID string, afterConnect AfterConnectFunc) (*PgDB, error) {
	poolConfig, err := pgxpool.ParseConfig(connStr)
	if err != nil {
		return nil, fmt.Errorf("parse postgres pool config: %w", err)
	}

	var cred *azidentity.ManagedIdentityCredential
	if clientID != "" {
		cred, err = azidentity.NewManagedIdentityCredential(&azidentity.ManagedIdentityCredentialOptions{
			ID: azidentity.ClientID(clientID),
		})
	} else {
		cred, err = azidentity.NewManagedIdentityCredential(nil)
	}
	if err != nil {
		return nil, fmt.Errorf("create managed identity credential: %w", err)
	}

	poolConfig.BeforeConnect = func(ctx context.Context, cfg *pgx.ConnConfig) error {
		token, err := cred.GetToken(ctx, policy.TokenRequestOptions{
			Scopes: []string{"https://ossrdbms-aad.database.windows.net/.default"},
		})
		if err != nil {
			return fmt.Errorf("get azure token: %w", err)
		}
		cfg.Password = token.Token
		return nil
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

	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping postgres: %w", err)
	}

	return &PgDB{DB: db, connStr: connStr}, nil
}

// ConnStr returns the connection string used to open the database.
func (db *PgDB) ConnStr() string {
	return db.connStr
}
