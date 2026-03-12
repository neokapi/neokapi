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
type PgDB struct {
	*sql.DB
	connStr string
}

// OpenPostgres opens a PostgreSQL database with the given connection string.
// The connection string should be a PostgreSQL DSN or URL, e.g.:
//
//	"postgres://user:pass@host:5432/dbname?sslmode=disable"
func OpenPostgres(connStr string) (*PgDB, error) {
	db, err := sql.Open("pgx", connStr)
	if err != nil {
		return nil, fmt.Errorf("open postgres: %w", err)
	}
	return configureAndPing(db, connStr)
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
	return configureAndPing(db, connStr)
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
