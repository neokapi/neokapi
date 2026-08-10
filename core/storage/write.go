package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync/atomic"
)

// Every write a store issues against a *DB arrives through one of four methods:
// ExecContext, Exec, BeginTx or Begin. All four are declared here, shadowing the
// promoted methods of the embedded *sql.DB, so a handle opened with
// Options.SerializeWrites gates its writes whether or not the store issuing them
// knows the gate exists.
//
// That is the point of intercepting HERE rather than at the project store: a
// store holds a *DB and nothing else, so there is no way for a new write to
// reach the file without passing through one of these. Wrapping the subsystems
// from outside would have left the discipline as something each new call site
// has to remember, and the whole reason this exists is that busy_timeout
// already looked like it was handling the problem.
//
// Reads are untouched. Under WAL a reader never blocks a writer or waits for
// one, so QueryContext and QueryRowContext stay promoted straight through and
// cost nothing.

// ExecContext runs a statement, holding the handle's write permit for its
// duration if the handle has one.
//
// Every autocommit write in a store goes through Exec — SELECTs go through
// Query — so gating this gates them all, with no need to decide from the SQL
// text whether a statement writes.
func (db *DB) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	if err := db.gate.acquire(ctx, false); err != nil {
		return nil, err
	}
	defer db.gate.release()
	res, err := db.DB.ExecContext(ctx, query, args...)
	return res, CancelledBy(ctx, err)
}

// Exec runs a statement without a context. It exists so the promoted
// context-free method cannot slip past the gate; prefer ExecContext, whose wait
// for the permit is cancellable.
func (db *DB) Exec(query string, args ...any) (sql.Result, error) {
	return db.ExecContext(context.Background(), query, args...)
}

// BeginTx starts a transaction, taking the handle's write permit first and
// holding it until the transaction ends.
//
// The permit is held for the WHOLE transaction, not for each statement in it.
// That is what makes a long session — extraction's purge-and-refill, which is
// one transaction over tens of thousands of rows — correct rather than merely
// ordered: it is the single writer for its duration, which is what SQLite makes
// it anyway, and now the other writers queue in Go instead of retrying into a
// busy handler that will eventually give up on them.
//
// It follows that the gate is not reentrant, and that a second write from the
// goroutine holding a transaction is a deadlock. It is detected rather than
// hung; see ErrWriteGateReentrant.
func (db *DB) BeginTx(ctx context.Context, opts *sql.TxOptions) (*Tx, error) {
	if err := db.gate.acquire(ctx, true); err != nil {
		return nil, err
	}
	tx, err := db.DB.BeginTx(ctx, opts)
	if err != nil {
		db.gate.release()
		return nil, CancelledBy(ctx, err)
	}
	return &Tx{Tx: tx, ctx: ctx, gate: db.gate}, nil
}

// Begin starts a transaction without a context. Prefer BeginTx.
func (db *DB) Begin() (*Tx, error) {
	return db.BeginTx(context.Background(), nil)
}

// Tx is a transaction on a *DB. It embeds *sql.Tx, so it is used exactly like
// one; it exists so the write permit taken at BeginTx is returned when the
// transaction ends, and only then.
//
// A transaction that is neither committed nor rolled back holds the permit for
// as long as it holds its connection — the leak is the same leak database/sql
// already punishes, now with a queue behind it. Close the transaction.
type Tx struct {
	*sql.Tx

	// ctx is the context the transaction was begun with, kept so Commit can say
	// why it failed; see CancelledBy.
	ctx  context.Context
	gate *writeGate
	done atomic.Bool
}

// Commit commits the transaction and releases the write permit.
func (tx *Tx) Commit() error {
	defer tx.finish()
	return CancelledBy(tx.ctx, tx.Tx.Commit())
}

// Rollback rolls the transaction back and releases the write permit. It is safe
// to call after Commit — the usual `defer tx.Rollback()` — and releases the
// permit exactly once either way.
//
// Its error is deliberately NOT run through CancelledBy: the transaction is
// over either way, every call site discards this, and a rollback that finds the
// context's own rollback got there first has nothing to report.
func (tx *Tx) Rollback() error {
	defer tx.finish()
	return tx.Tx.Rollback()
}

func (tx *Tx) finish() {
	if tx.done.CompareAndSwap(false, true) {
		tx.gate.release()
	}
}

// CancelledBy names the caller's cancellation as the reason a write failed,
// when that is what happened.
//
// database/sql reacts to a context ending by closing the transaction and its
// prepared statements from a background goroutine (Tx.awaitDone). A statement
// or a Commit that loses the race with it returns a lifecycle error instead of
// the context's — `sql: transaction has already been committed or rolled back`,
// `sql: statement is closed` — which says nothing about the context and reads
// like a corrupted handle. Both were observed escaping the project store's
// bulk-write path when its deadline expired mid-transaction.
//
// The returned error wraps BOTH: `errors.Is(err, context.DeadlineExceeded)`
// answers "did my deadline stop this", and the underlying error is still there
// for anyone who wants the detail. A caller that must distinguish "the write did
// not happen because I cancelled" from "the store is broken" — a retry loop, an
// exit code, a test asserting the gate refused nothing — can now do so.
//
// It is applied where the error reports whether the write happened: a statement,
// a BEGIN, a Commit. Reads are untouched: they are not gated, and a cancelled
// read already returns the context error through database/sql's own checks.
func CancelledBy(ctx context.Context, err error) error {
	if err == nil {
		return nil
	}
	cerr := ctx.Err()
	if cerr == nil || errors.Is(err, cerr) {
		return err
	}
	return fmt.Errorf("%w: %w", cerr, err)
}
