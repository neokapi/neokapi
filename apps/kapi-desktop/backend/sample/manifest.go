package sample

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/neokapi/neokapi/core/version"
)

// Revision is the content revision of each embedded sample. BUMP it whenever the
// embedded recipe or input files change, so the desktop can offer to refresh an
// already-scaffolded copy on disk. It is deliberately decoupled from the kapi
// binary version: a plain upgrade should not prompt when the sample itself did
// not change.
var Revision = map[string]int{
	"kapimart":  2,
	"okapimart": 2,
}

// CurrentRevision returns the embedded content revision for a sample (0 if the
// name is unknown).
func CurrentRevision(name string) int { return Revision[name] }

// manifestRel is the sample marker, written under the regenerable .kapi state
// dir rather than the user-owned recipe (which is re-marshalled on save and
// would drop comments/markers).
const manifestRel = ".kapi/sample.json"

// Manifest records how a project dir was scaffolded from a sample.
type Manifest struct {
	Sample       string `json:"sample"`
	Revision     int    `json:"revision"`
	KapiVersion  string `json:"kapi_version,omitempty"`
	ScaffoldedAt string `json:"scaffolded_at,omitempty"`
}

func manifestPath(targetDir string) string {
	return filepath.Join(targetDir, filepath.FromSlash(manifestRel))
}

// writeManifest stamps the sample manifest under targetDir/.kapi/.
func writeManifest(name, targetDir string) error {
	m := Manifest{
		Sample:       name,
		Revision:     Revision[name],
		KapiVersion:  version.Version,
		ScaffoldedAt: time.Now().UTC().Format(time.RFC3339),
	}
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(manifestPath(targetDir), data, 0o644)
}

// ReadManifest returns the sample manifest for a project dir, or (nil,false)
// when the dir was not scaffolded from a sample (or the marker is unreadable).
func ReadManifest(targetDir string) (*Manifest, bool) {
	data, err := os.ReadFile(manifestPath(targetDir))
	if err != nil {
		return nil, false
	}
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil || m.Sample == "" {
		return nil, false
	}
	return &m, true
}

// SetManifestRevision rewrites the on-disk manifest's revision — used to
// acknowledge the current version ("keep current") without re-scaffolding, so
// the desktop stops offering the upgrade.
func SetManifestRevision(targetDir string, rev int) error {
	m, ok := ReadManifest(targetDir)
	if !ok {
		return os.ErrNotExist
	}
	m.Revision = rev
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(manifestPath(targetDir), data, 0o644)
}
