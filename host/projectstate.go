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

// StateCommit is what a commit wrote, or what a dry run says it would write.
type StateCommit struct {
	// Committed counts the staged decisions written into the record. On a dry
	// run it counts the ones a commit would write.
	Committed int
	// Reseeded reports that the committed record on disk had moved since the
	// working set was built from it, so the set was rebuilt from the shards
	// this checkout holds.
	Reseeded bool
	// Carried counts the staged decisions that crossed that rebuild.
	Carried int
}

// CommitProjectState writes staged decisions into the project's committed
// record and reports how many were written.
//
// Committing is explicit. A decision is durable the moment it is recorded, the
// working store being a database rather than a buffer, and becoming part of the
// project's reviewable record is a separate act, the same shape as staging and
// committing in git. That is what keeps a run of automated decisions from
// landing in the tracked record before anyone has looked at them.
func (a *App) CommitProjectState(ctx context.Context, root string) (int, error) {
	res, err := a.CommitProjectStateReport(ctx, root, false)
	return res.Committed, err
}

// CommitProjectStateReport commits and reports what the write amounted to,
// including whether the working set had to be rebuilt from a record that moved
// under it. With dryRun set it inspects and writes nothing.
func (a *App) CommitProjectStateReport(ctx context.Context, root string, dryRun bool) (StateCommit, error) {
	st, err := a.OpenProjectState(ctx, root)
	if err != nil {
		return StateCommit{}, err
	}
	// The set is agreed with the record before the count is taken. A dry run
	// needs that agreement as much as a write does, or it reports the rows of a
	// record this checkout no longer holds.
	if err := st.SyncWithCommitted(ctx); err != nil {
		return StateCommit{}, err
	}
	reseed := st.Reseed()
	res := StateCommit{Reseeded: reseed.Reseeded, Carried: reseed.Carried}

	n, err := st.Pending(ctx)
	if err != nil {
		return res, err
	}
	res.Committed = n
	if dryRun || n == 0 {
		return res, nil
	}
	if err := st.Commit(ctx); err != nil {
		return res, err
	}
	return res, nil
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

// ProjectStateReseed reports whether opening the project's working set found
// the committed record moved, and how many staged decisions crossed the
// rebuild. An unreadable store reports nothing: status stays informational.
func (a *App) ProjectStateReseed(ctx context.Context, root string) state.Reseed {
	st, err := a.OpenProjectState(ctx, root)
	if err != nil {
		return state.Reseed{}
	}
	return st.Reseed()
}
