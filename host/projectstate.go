package host

import (
	"path/filepath"
	"strings"
	"time"

	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/state"
)

// nowRFC3339 is the current UTC time as an RFC 3339 string, for stamping state
// decisions.
func nowRFC3339() string { return time.Now().UTC().Format(time.RFC3339) }

// openProjectState opens the project's working store, seeding it from the
// committed record when it holds nothing yet.
//
// Decisions accumulate in the working store and reach the committed record only
// on Commit. Callers own that lifecycle: the point of staging is that a run
// writes once rather than once per decision, so a caller recording many
// decisions must commit after the batch, not inside the loop.
//
// The caller closes the returned store.
func openProjectState(root string) (*state.WorkStore, error) {
	layout := project.Layout{StateDir: filepath.Join(root, project.StateDirName)}
	return state.OpenWork(layout.WorkStorePath(), layout.UnitsDir())
}

// targetHash is the content hash of a translation, used to bind a review decision
// to the specific text it blessed — so an edit invalidates a stale approval. It
// trims surrounding whitespace so insignificant reformatting doesn't invalidate.
func targetHash(text string) string {
	return project.HashBytes([]byte(strings.TrimSpace(text)))
}

// OpenProjectState opens the project's working store for an embedder (the
// desktop) that needs access to the same decisions the review commands record.
//
// It replaces the old StateFilePath: an embedder cannot resolve a path and read
// it directly any more, because the committed record is a set of shards and the
// authoritative view is the working store over them. Handing out a path let the
// desktop read a different store than the writer used the moment the layout
// changed, which is exactly what happened.
//
// The caller closes the returned store.
func OpenProjectState(root string) (*state.WorkStore, error) {
	return openProjectState(root)
}

// CommitProjectState writes staged decisions into the project's committed
// record and reports how many were written.
//
// Committing is explicit. A decision is durable the moment it is recorded — the
// working store is a database, not a buffer — but becoming part of the project's
// reviewable record is a separate act, the same shape as staging and committing
// in git. That is what keeps a run of automated decisions from landing in the
// tracked record before anyone has looked at them.
func CommitProjectState(root string) (int, error) {
	st, err := openProjectState(root)
	if err != nil {
		return 0, err
	}
	defer st.Close()

	n, err := st.Pending()
	if err != nil {
		return 0, err
	}
	if n == 0 {
		return 0, nil
	}
	if err := st.Commit(); err != nil {
		return 0, err
	}
	return n, nil
}

// PendingDecisions reports how many decisions are staged and not yet committed.
// An unreadable store is not an error: status stays informational.
func PendingDecisions(root string) int {
	st, err := openProjectState(root)
	if err != nil {
		return 0
	}
	defer st.Close()

	n, err := st.Pending()
	if err != nil {
		return 0
	}
	return n
}
