package host

import (
	"path/filepath"
	"strings"

	coreproj "github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/reconcile"
	"github.com/neokapi/neokapi/memory/kmb"
	"github.com/neokapi/neokapi/terms/ktb"
)

// IsContextFile reports whether a project-relative path names a context file:
// one `kapi store import` reads into the store, or one a context export or
// sync writes. These files govern the content and are never content
// themselves, so a listing of untracked or unclaimed files leaves them out.
//
// A file is recognised by its name, wherever it sits, because a project keeps
// its context bundles where it likes (`.kapi/`, `context/`, a `seeds/`
// directory):
//
//   - a voice profile (`voice.yaml`);
//   - a terms bundle (`terms.json` or `*.terms.json`);
//   - a content-memory bundle (`memory.json` or `*.memory.json`);
//   - a decision-record shard: `state/<document key>.jsonl`;
//   - a context package (`*.kpz`).
func IsContextFile(rel string) bool {
	rel = filepath.ToSlash(rel)
	base := filepath.Base(rel)
	switch {
	case base == VoiceConventionalName, ktb.IsBundlePath(rel), kmb.IsBundlePath(rel):
		return true
	case strings.EqualFold(filepath.Ext(base), workspaceExt):
		return true
	case strings.HasSuffix(base, ".jsonl"):
		dir := filepath.Base(filepath.Dir(rel))
		return dir == coreproj.UnitStateDirName && reconcile.IsDocumentKey(strings.TrimSuffix(base, ".jsonl"))
	}
	return false
}

// ContextFileFilter returns the test a walk over the project at recipePath
// uses to leave context out: IsContextFile, plus everything under the
// directory a `file` context backend keeps the shared context in, when the
// recipe places that directory inside the project.
func ContextFileFilter(recipePath string) func(rel string) bool {
	root := filepath.Dir(recipePath)
	var backendDir string
	if spec, err := recipeContextBackend(recipePath); err == nil && spec != nil &&
		spec.Kind() == coreproj.ContextBackendFile && spec.Path != "" {
		dir := spec.Path
		if !filepath.IsAbs(dir) {
			dir = filepath.Join(root, dir)
		}
		if rel, err := filepath.Rel(root, dir); err == nil && rel != "." && !strings.HasPrefix(rel, "..") {
			backendDir = filepath.ToSlash(rel)
		}
	}
	return func(rel string) bool {
		rel = filepath.ToSlash(rel)
		if backendDir != "" && (rel == backendDir || strings.HasPrefix(rel, backendDir+"/")) {
			return true
		}
		return IsContextFile(rel)
	}
}
