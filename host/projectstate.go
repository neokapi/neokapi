package host

import (
	"context"
	"time"

	"github.com/neokapi/neokapi/core/state"
)

// nowRFC3339 is the current UTC time as an RFC 3339 string, for stamping state
// decisions.
func nowRFC3339() string { return time.Now().UTC().Format(time.RFC3339) }

// OpenProjectState returns the project's working store — the staging area
// between a decision being made and the project's committed record of it.
//
// It is the working-set schema of the project store (ProjectDB), so the store
// opens once per project per App and the value returned here is the same one
// every other caller under that App holds. Close() on it is a no-op: the handle
// owns the pool. Seeding from the committed record happens at open, once.
//
// Decisions accumulate here and reach the committed record only on Commit.
// Callers own that lifecycle: the point of staging is that a run writes once
// rather than once per decision, so a caller recording many decisions must
// commit after the batch, not inside the loop.
//
// It is a method rather than the free function it was, because a working store
// can no longer be opened on its own: it lives in the project's one database,
// and a second opener would be a second connection pool on that file — two sets
// of writers this process could no longer serialize.
func (a *App) OpenProjectState(ctx context.Context, root string) (*state.WorkStore, error) {
	db, err := a.ProjectDB(ctx, root)
	if err != nil {
		return nil, err
	}
	return db.Work(), nil
}

// targetHash is the content hash of a translation, used to bind a review decision
// to the specific text it blessed — so an edit invalidates a stale approval. It
// trims surrounding whitespace so insignificant reformatting doesn't invalidate.
func targetHash(text string) string {
	return state.TargetHash(text)
}

// CommitProjectState writes staged decisions into the project's committed
// record and reports how many were written.
//
// Committing is explicit. A decision is durable the moment it is recorded — the
// working store is a database, not a buffer — but becoming part of the project's
// reviewable record is a separate act, the same shape as staging and committing
// in git. That is what keeps a run of automated decisions from landing in the
// tracked record before anyone has looked at them.
func (a *App) CommitProjectState(ctx context.Context, root string) (int, error) {
	st, err := a.OpenProjectState(ctx, root)
	if err != nil {
		return 0, err
	}
	n, err := st.Pending(ctx)
	if err != nil {
		return 0, err
	}
	if n == 0 {
		return 0, nil
	}
	if err := st.Commit(ctx); err != nil {
		return 0, err
	}
	return n, nil
}

// PendingDecisions reports how many decisions are staged and not yet committed.
// An unreadable store is not an error: status stays informational.
func (a *App) PendingDecisions(ctx context.Context, root string) int {
	st, err := a.OpenProjectState(ctx, root)
	if err != nil {
		return 0
	}
	n, err := st.Pending(ctx)
	if err != nil {
		return 0
	}
	return n
}
