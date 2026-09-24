package memory

import (
	"context"
	"errors"
	"fmt"
	"sort"
)

// Version history groups content-memory entries by Entry.Unit. Rewriting a
// block's source creates a new entry while retaining the approved earlier entry.
// Versions are ordered by write time and may be restricted to one context point.
//
// History uses an exact block identity. Unlike fuzzy lookup, it returns prior
// versions without ranking them as candidates for reuse.

// VersionQuery selects a block's prior answers.
type VersionQuery struct {
	// Unit identifies the block whose history to return. Required: entries with
	// an empty unit cannot be associated with a specific block.
	Unit string
	// Point restricts history to versions approved at one context point. Empty
	// includes all points, including those the block occupied before a move.
	Point string
	// Limit caps how many are returned, newest first. Zero means DefaultVersionLimit.
	Limit int
}

// DefaultVersionLimit caps the history returned when a query has no limit.
const DefaultVersionLimit = 10

// Version is one prior answer, with what governed it when it was approved.
type Version struct {
	Entry Entry
	// ContextFingerprint identifies the governing context recorded in the entry's
	// most recent origin with a fingerprint. Empty means no origin recorded one.
	// Callers can compare it with the current context before using the version as
	// reference material.
	ContextFingerprint string
}

// GovernedBy reports whether this answer was approved under the given context
// fingerprint. An empty fingerprint on either side is not a match: an ungoverned
// answer cannot be asserted to satisfy governance, and a caller that has no
// fingerprint of its own has nothing to compare against.
func (v Version) GovernedBy(fingerprint string) bool {
	return fingerprint != "" && v.ContextFingerprint == fingerprint
}

// VersionReader is an optional capability for retrieving a block's history.
// Callers use a type assertion because some content-memory backends have no
// persistent history.
type VersionReader interface {
	// Versions returns a block's prior answers, newest first, excluding any
	// entry whose ID matches excludeID (the answer currently in force, which
	// the caller already has).
	Versions(ctx context.Context, q VersionQuery, excludeID string) ([]Version, error)
}

// VersionsFrom filters loaded entries and returns the most recent versions.
// Backends can select entries in their own storage layer, then use this function
// for consistent filtering, ordering and limits.
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

// LatestFingerprint returns the fingerprint of the most recent origin that
// recorded one, or empty if none did. Version history and decision re-seeding
// use this value to identify the entry's governing context.
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

// ErrVersionQueryNeedsUnit reports a history query without a block identity.
// Entries with no unit cannot establish that they belong to the same block.
var ErrVersionQueryNeedsUnit = errors.New("memory: version query needs a unit")

// Versions returns a block's history from the SQLite corpus. It selects entries
// using the (unit, point) index and loads their variants, entities and origins
// through the shared entry loader.
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
