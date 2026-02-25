package storage

import (
	"database/sql"
	"fmt"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
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
