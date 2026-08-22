package storage

import (
	"context"
	"database/sql"
	"fmt"
)

// One transition, spanning every store on the pool.
//
// The stores are separate types with separate schemas, but they are not
// separate databases: bowrain-worker opens ONE PgDB and builds the content
// store, the voice store, the content memory and the terms store from it. A
// transaction begun here therefore covers all of them, and a step that writes
// through two stores can still land whole.
//
// This is the seam that makes that expressible. Each store binds itself to a
// Runner — the interface a *sql.DB and a *sql.Tx both satisfy — so the same
// verb serves a caller invoking it alone and a caller running it as one step of
// a larger transition.

// Execer is the write half of what a *sql.DB and a *sql.Tx share.
type Execer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

// Querier is the read half. Deliberately the narrowest thing a helper needs, so
// a caller holding nothing more than a query method can still use one.
type Querier interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

// Runner is everything a *sql.DB and a *sql.Tx both do.
//
// It is what lets one body serve two callers: a verb invoked on its own, which
// opens a transaction of its own, and the same verb invoked as one step of a
// larger transition that has to land whole. Without it every write owns its
// transaction, which is exactly why a push used to apply in pieces — one per
// chunk, one per item, and more again after the loop.
type Runner interface {
	Execer
	Querier
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
	PrepareContext(ctx context.Context, query string) (*sql.Stmt, error)
}

// Transition runs fn against one transaction and commits it. An error from fn
// rolls the whole thing back: nothing fn wrote survives.
//
// The Runner it hands over is the transaction. Passing it to each store's Bind
// is what puts their writes in the same transition — the point being that a
// step which reconciles collections, writes a voice profile and then stores
// content either does all three or none.
func (db *PgDB) Transition(ctx context.Context, fn func(Runner) error) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transition: %w", err)
	}
	// Rollback after a successful Commit is a no-op that returns
	// sql.ErrTxDone, so this is safe on both paths and needs no flag.
	defer func() { _ = tx.Rollback() }()

	if err := fn(tx); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit transition: %w", err)
	}
	return nil
}
