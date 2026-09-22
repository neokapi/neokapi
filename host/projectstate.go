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
// pool.
//
// A decision recorded here is durable at once. Writing the committed shards is
// a separate act (PersistRecords), and a caller recording many decisions writes
// them out after the batch rather than inside the loop.
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

