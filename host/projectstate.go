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

// defaultStateFile named the committed state artifact when it was a single JSON
// document at the project root. The committed record is now a set of JSON Lines
// shards under the state directory (project.UnitsDir), so this survives only for
// projects that bind defaults.state explicitly.
const defaultStateFile = ".kapi-state.json"

// stateFilePath resolves the committed project-state artifact: defaults.state when
// bound, else the conventional default, relative to the project root.
func stateFilePath(proj *project.KapiProject, root string) string {
	rel := defaultStateFile
	if proj != nil && strings.TrimSpace(proj.Defaults.State) != "" {
		rel = proj.Defaults.State
	}
	if filepath.IsAbs(rel) {
		return rel
	}
	return filepath.Join(root, rel)
}

// openProjectState opens the project's working store, seeding it from the
// committed record when it holds nothing yet.
//
// Decisions accumulate in the working store and reach the committed record only
// on Commit. Callers own that lifecycle: the point of staging is that a run
// writes once rather than once per decision, so a caller recording many
// decisions must commit after the batch, not inside the loop.
//
// The caller closes the returned store.
func openProjectState(proj *project.KapiProject, root string) (*state.WorkStore, error) {
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
func OpenProjectState(proj *project.KapiProject, root string) (*state.WorkStore, error) {
	return openProjectState(proj, root)
}
