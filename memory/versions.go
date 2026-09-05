package memory

import (
	"context"
	"errors"
	"fmt"
	"sort"
)

// Prior answers for one block.
//
// The corpus has always accumulated versions. A block whose source is rewritten
// writes a NEW entry — the key is the text, and the text moved — beside the one
// that came before, which stays exactly as it was approved. What was missing was
// any way to say those two entries are successive answers for the same block
// rather than two unrelated strings that happen to share a project.
//
// Entry.Unit says it. So a version chain is a query, not a new store: the
// entries are already there, ordered by when they were written, and the only
// thing this adds is the ability to ask for them.
//
// This is deliberately NOT a matcher. A lookup asks "what has been approved for
// something like this, near here" and ranks candidates; a version chain asks
// "what did THIS block say before" and the answer is a fact with no score
// attached. Ranking it would invite the caller to treat a prior version as a
// match, which is the thing fuzzy matching did wrong.

// VersionQuery selects a block's prior answers.
type VersionQuery struct {
	// Unit is the block whose chain to return. Required: an empty unit selects
	// every entry approved before the chain existed, which is never a useful
	// answer and is why this refuses rather than returning them.
	Unit string
	// Point narrows the chain to answers approved at one place. Empty returns
	// the chain across every point the block has sat at, which is what a caller
	// reporting on a moved block wants and what a caller assembling context
	// does not.
	Point string
	// Limit caps how many are returned, newest first. Zero means DefaultVersionLimit.
	Limit int
}

// DefaultVersionLimit is how many prior answers a chain returns when the caller
// names no limit. One is the common case — the version immediately before this
// one — and the rest are for a surface showing history; a caller that wants the
// whole chain says so.
const DefaultVersionLimit = 10

// Version is one prior answer, with what governed it when it was approved.
type Version struct {
	Entry Entry
	// ContextFingerprint is the governing context this answer was produced
	// under, lifted from the entry's most recent origin. Empty when the entry
	// carries no origin that recorded one — an import, a seed, or a producer
	// that ran ungoverned.
	//
	// It is the field a caller gates on. A prior answer whose fingerprint no
	// longer matches the context in force was approved under rules that have
	// since moved, and offering it as reference would anchor the model to
	// wording those rules would now reject.
	ContextFingerprint string
}

// GovernedBy reports whether this answer was approved under the given context
// fingerprint. An empty fingerprint on either side is not a match: an ungoverned
// answer cannot be asserted to satisfy governance, and a caller that has no
// fingerprint of its own has nothing to compare against.
func (v Version) GovernedBy(fingerprint string) bool {
	return fingerprint != "" && v.ContextFingerprint == fingerprint
}

// VersionReader is the optional capability of returning a block's prior
// answers. A content memory that cannot is not broken — an in-memory corpus
// built for one run has no history worth the name — so callers type-assert for
// it rather than requiring it of every implementation.
type VersionReader interface {
	// Versions returns a block's prior answers, newest first, excluding any
	// entry whose ID matches excludeID (the answer currently in force, which
	// the caller already has).
	Versions(ctx context.Context, q VersionQuery, excludeID string) ([]Version, error)
}

// VersionsFrom builds the chain from entries already loaded, newest first. It is
// the shared half of every backend's implementation: the selection is a query
// the backend runs, and the ordering and trimming are not.
//
// Exported because a backend outside this package implements the same
// capability: the server's Postgres corpus selects by unit and point in SQL and
// then hands the entries here, so the two answer a chain identically.
func VersionsFrom(entries []Entry, q VersionQuery, excludeID string) []Version {
	limit := q.Limit
	if limit <= 0 {
		limit = DefaultVersionLimit
	}

	out := make([]Version, 0, len(entries))
	for i := range entries {
		e := entries[i]
		if e.ID == excludeID || e.Unit == "" || e.Unit != q.Unit {
			continue
		}
		if q.Point != "" && e.Point != q.Point {
			continue
		}
		out = append(out, Version{Entry: e, ContextFingerprint: LatestFingerprint(&e)})
	}

	// Newest first, and ties broken by ID so a chain written inside one second
	// does not reorder between calls. Two answers sharing a timestamp is the
	// ordinary case for a bulk absorb, not an edge one.
	sort.SliceStable(out, func(a, b int) bool {
		ta, tb := out[a].Entry.UpdatedAt, out[b].Entry.UpdatedAt
		if ta.Equal(tb) {
			return out[a].Entry.ID > out[b].Entry.ID
		}
		return ta.After(tb)
	})

	if len(out) > limit {
		out = out[:limit]
	}
	return out
}

// LatestFingerprint returns the governing fingerprint of an entry's most recent
// origin, or empty when no origin recorded one. It is the fingerprint the
// entry's answer stands under: what a version chain reports for it, and what a
// re-seed carries back onto the decision record the answer was recorded for.
func LatestFingerprint(e *Entry) string {
	best := ""
	var bestAt = e.CreatedAt
	for i := range e.Origins {
		o := &e.Origins[i]
		if o.ContextFingerprint == "" {
			continue
		}
		if best == "" || !o.AddedAt.Before(bestAt) {
			best, bestAt = o.ContextFingerprint, o.AddedAt
		}
	}
	return best
}

// Versions returns a block's prior answers from the in-memory corpus.
func (tm *InMemoryStore) Versions(ctx context.Context, q VersionQuery, excludeID string) ([]Version, error) {
	if q.Unit == "" {
		return nil, ErrVersionQueryNeedsUnit
	}
	entries, err := tm.Entries(ctx)
	if err != nil {
		return nil, err
	}
	return VersionsFrom(entries, q, excludeID), nil
}

// ErrVersionQueryNeedsUnit rejects a chain query naming no block. Returning
// every unit-less entry instead would answer with the whole pre-chain corpus,
// which is the least useful possible answer and the easiest to mistake for a
// working lookup.
var ErrVersionQueryNeedsUnit = errors.New("memory: version query needs a unit")

// Versions returns a block's prior answers from the SQLite corpus.
//
// The selection is a query — the (unit, point) index exists for it — and the
// entries are hydrated through the same path every other read uses, so a
// version carries the variants, entities and origins a caller needs to judge it
// rather than a trimmed projection that would have to grow fields later.
func (tm *SQLiteStore) Versions(ctx context.Context, q VersionQuery, excludeID string) ([]Version, error) {
	if q.Unit == "" {
		return nil, ErrVersionQueryNeedsUnit
	}

	where := "unit = ?"
	args := []any{q.Unit}
	if q.Point != "" {
		where += " AND point = ?"
		args = append(args, q.Point)
	}

	rows, err := tm.db.QueryContext(ctx,
		`SELECT id FROM tm_entries WHERE `+where+` ORDER BY updated_at DESC, id DESC`, args...)
	if err != nil {
		return nil, fmt.Errorf("select versions: %w", err)
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan version id: %w", err)
		}
		ids = append(ids, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate versions: %w", err)
	}
	if len(ids) == 0 {
		return nil, nil
	}

	entries, err := tm.loadEntriesByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	return VersionsFrom(entries, q, excludeID), nil
}
