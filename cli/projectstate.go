package cli

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

// defaultStateFile is the conventional committed state artifact when a project
// does not bind one via defaults.state. It is git-tracked (committed), distinct
// from the gitignored .kapi/ working dir.
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

// openProjectState opens the project's state store, importing the committed
// artifact into a transient working set. Mutations stay in-transit until Export.
func openProjectState(proj *project.KapiProject, root string) (*state.FileStore, error) {
	return state.Open(stateFilePath(proj, root))
}

// targetHash is the content hash of a translation, used to bind a review decision
// to the specific text it blessed — so an edit invalidates a stale approval. It
// trims surrounding whitespace so insignificant reformatting doesn't invalidate.
func targetHash(text string) string {
	return project.HashBytes([]byte(strings.TrimSpace(text)))
}
