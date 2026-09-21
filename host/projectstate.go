package host

import (
	"context"
	"time"

	"github.com/neokapi/neokapi/core/state"
)

// nowRFC3339 is the current UTC time as an RFC 3339 string, for stamping state
// decisions.
func nowRFC3339() string { return time.Now().UTC().Format(time.RFC3339) }

// OpenProjectState returns the project's decision ledger and this checkout's
// view of it.
//
// It is one schema of the project store (ProjectDB), so the store opens once
// per project per App and the value returned here is the same one every other
// caller under that App holds. Close() on it is a no-op: the handle owns the
// pool. Importing this checkout's committed shards happens at open.
//
// A decision recorded here is durable at once. Writing the committed shards is
// a separate act, and a caller recording many decisions writes them out after
// the batch rather than inside the loop.
//
// It is a method rather than a free function because the ledger lives in the
// project's one database, and a second opener would be a second connection pool
// on that file, with two sets of writers this process could no longer
// serialize.
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

// StateCommit is what a write of the committed record amounted to, or what a
// dry run says it would amount to.
type StateCommit struct {
	// Committed counts the lines the write put into the record that the record
	// did not already carry. On a dry run it counts the ones a write would put
	// there.
	Committed int
}

// CommitProjectState writes this checkout's decision record to the committed
// shards and reports how many lines changed.
func (a *App) CommitProjectState(ctx context.Context, root string) (int, error) {
	res, err := a.CommitProjectStateReport(ctx, root, false)
	return res.Committed, err
}

// CommitProjectStateReport writes the committed shards and reports what the
// write amounted to. With dryRun set it inspects and writes nothing.
//
// The record it writes is, for every unit this checkout holds, the ledger entry
// that applies to the unit's current pairing. A decision is already durable, so
// this is an export rather than a publish gate.
func (a *App) CommitProjectStateReport(ctx context.Context, root string, dryRun bool) (StateCommit, error) {
	st, err := a.OpenProjectState(ctx, root)
	if err != nil {
		return StateCommit{}, err
	}
	// The shards are read before the count is taken. A dry run needs that as
	// much as a write does, or it reports against a record this checkout no
	// longer holds.
	diff, err := st.RecordDiff(ctx)
	if err != nil {
		return StateCommit{}, err
	}
	res := StateCommit{Committed: diff.Changed()}
	if dryRun || res.Committed == 0 {
		return res, nil
	}
	if err := st.Commit(ctx); err != nil {
		return res, err
	}
	return res, nil
}
