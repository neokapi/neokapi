package state

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/neokapi/neokapi/core/storage"
)

// Journal is the operation log a ledger records its entries in before they are
// written (core/projector). The ledger is then a projection of the log: every
// entry the ledger holds is one the log carries, so replaying the log rebuilds
// the ledger, and a log merged from another machine brings its decisions.
//
// RecordEntries returns once the log holds the entries and they are applied to
// the ledger, which the journal does through ApplyEntries.
type Journal interface {
	RecordEntries(ctx context.Context, entries []JournalEntry) error
}

// JournalEntry is one ledger write, as the log carries it.
type JournalEntry struct {
	// ID is the entry's content address (Address). An entry recorded for the
	// first time is one operation in every log that holds it, however many
	// times and on however many machines it is recorded.
	ID string `json:"id"`
	// State is the record the entry asserts.
	State UnitState `json:"state"`
	// Actor and Origin are who recorded it and how it arrived.
	Actor  string      `json:"actor,omitempty"`
	Origin EntryOrigin `json:"origin,omitempty"`
	// Revoked marks a withdrawal.
	Revoked bool `json:"revoked,omitempty"`
	// Recorded is the moment the entry was written, which orders the entries
	// of one pairing.
	Recorded time.Time `json:"recorded"`
	// Reassert moves the moment of an entry the ledger already holds forward,
	// so it answers for its pairing again. An entry written without it is
	// left as the ledger holds it, which is what reading a record in does.
	Reassert bool `json:"reassert,omitempty"`
	// Held reports that the ledger held the entry when it was written, so the
	// write re-asserts an old decision rather than recording a new one.
	Held bool `json:"held,omitempty"`
}

// SetJournal routes every ledger write through a journal. A store opened in a
// workspace has one; the embedded layout, with no log, writes directly.
func (w *WorkStore) SetJournal(j Journal) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.journal = j
}

// ApplyEntries writes journal entries into the ledger of a context database, in
// one transaction. It is how a journal applies what it recorded, and how a
// rebuild replays it.
func ApplyEntries(ctx context.Context, db *storage.DB, entries []JournalEntry) error {
	if len(entries) == 0 {
		return nil
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("state: apply entries: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	for _, e := range entries {
		if err := insertEntry(ctx, tx, e.State, e.Actor, e.Origin, e.Revoked, e.Recorded, e.Reassert); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("state: apply entries: %w", err)
	}
	return nil
}

// answering returns the id of the entry in force at a pairing, withdrawal or
// not, and "" when the ledger holds none.
func (w *WorkStore) answering(ctx context.Context, p Pairing) (string, error) {
	variant, _ := p.Key.Variant.MarshalText()
	var id string
	err := w.db.QueryRowContext(ctx, `
SELECT id FROM unit_decision
 WHERE scope = ? AND unit = ? AND variant = ? AND content_hash = ? AND target_hash = ?
 ORDER BY recorded_at DESC, rowid DESC LIMIT 1`,
		p.Key.Scope, p.Key.Unit, string(variant), p.ContentHash, p.TargetHash).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("state: read the entry in force: %w", err)
	}
	return id, nil
}

// holds reports whether the ledger holds an entry.
func (w *WorkStore) holds(ctx context.Context, id string) (bool, error) {
	var n int
	if err := w.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM unit_decision WHERE id = ?`, id).Scan(&n); err != nil {
		return false, fmt.Errorf("state: look up entry: %w", err)
	}
	return n > 0, nil
}

// entryFor renders a write as the journal entry that records it, and reports
// whether the write changes the ledger at all.
func (w *WorkStore) entryFor(ctx context.Context, u UnitState, actor string, origin EntryOrigin, revoked bool, stamp time.Time, reassert bool) (JournalEntry, bool, error) {
	id, err := Address(u, actor, revoked)
	if err != nil {
		return JournalEntry{}, false, err
	}
	held, err := w.holds(ctx, id)
	if err != nil {
		return JournalEntry{}, false, err
	}
	if held && !reassert {
		// Reading an entry in that the ledger holds changes nothing.
		return JournalEntry{}, false, nil
	}
	if held {
		answering, err := w.answering(ctx, u.Pairing())
		if err != nil {
			return JournalEntry{}, false, err
		}
		if answering == id {
			// Re-asserting what already answers changes nothing either.
			return JournalEntry{}, false, nil
		}
	}
	return JournalEntry{
		ID: id, State: u, Actor: actor, Origin: origin, Revoked: revoked,
		Recorded: stamp.UTC(), Reassert: reassert, Held: held,
	}, true, nil
}

// appendJournaled records one entry through the journal and points this
// checkout's view at its pairing.
func (w *WorkStore) appendJournaled(ctx context.Context, u UnitState, actor string, origin EntryOrigin, revoked bool, stamp time.Time) error {
	entry, changes, err := w.entryFor(ctx, u, actor, origin, revoked, stamp, true)
	if err != nil {
		return err
	}
	if changes {
		if err := w.journal.RecordEntries(ctx, []JournalEntry{entry}); err != nil {
			return fmt.Errorf("state: record decision: %w", err)
		}
	}
	if revoked || w.checkout == "" {
		return nil
	}
	tx, err := w.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("state: record decision: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := putView(ctx, tx, w.checkout, u.Pairing(), false); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("state: record decision: %w", err)
	}
	return nil
}
