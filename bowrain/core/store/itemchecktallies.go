package store

import (
	"context"
	"time"
)

// An ItemCheckTally is what a project-wide check counted in one item: the
// findings the checks at the source locale report on its blocks, and how
// many of them carry a fix.
type ItemCheckTally struct {
	ProjectID string
	Stream    string
	ItemID    string
	Findings  int
	Fixable   int
	CheckedAt time.Time
}

// ItemCheckTallyStore holds the tallies of a project's last project-wide
// check.
type ItemCheckTallyStore interface {
	// PutItemCheckTally records one item's tally, replacing the tally the
	// item held.
	PutItemCheckTally(ctx context.Context, t ItemCheckTally) error
	// ListItemCheckTallies returns every tally a stream of a project holds,
	// ordered by item id.
	ListItemCheckTallies(ctx context.Context, projectID, stream string) ([]ItemCheckTally, error)
	// DeleteItemCheckTally removes one item's tally; an item with none is
	// left as it is.
	DeleteItemCheckTally(ctx context.Context, projectID, stream, itemID string) error
}

// ItemCheckTallies finds the tally store behind a content store, looking
// through any decorator that names what it wraps, as ContextObjects does.
func ItemCheckTallies(cs ContentStore) (ItemCheckTallyStore, bool) {
	for cs != nil {
		if s, ok := cs.(ItemCheckTallyStore); ok {
			return s, true
		}
		inner, ok := cs.(interface{ Unwrap() ContentStore })
		if !ok {
			return nil, false
		}
		cs = inner.Unwrap()
	}
	return nil, false
}
