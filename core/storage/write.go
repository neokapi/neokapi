package storage

import (
	"context"
	"database/sql"
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
	return db.DB.ExecContext(ctx, query, args...)
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
		return nil, err
	}
	return &Tx{Tx: tx, gate: db.gate}, nil
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

	gate *writeGate
	done atomic.Bool
}

// Commit commits the transaction and releases the write permit.
func (tx *Tx) Commit() error {
	defer tx.finish()
	return tx.Tx.Commit()
}

// Rollback rolls the transaction back and releases the write permit. It is safe
// to call after Commit — the usual `defer tx.Rollback()` — and releases the
// permit exactly once either way.
func (tx *Tx) Rollback() error {
	defer tx.finish()
	return tx.Tx.Rollback()
}

func (tx *Tx) finish() {
	if tx.done.CompareAndSwap(false, true) {
		tx.gate.release()
	}
}
