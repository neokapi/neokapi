package projectdb

import (
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/neokapi/neokapi/core/project"
)

// Earlier layouts of a project's committed sources are folded forward here:
// the flat layout's `units/` and the `context/` umbrella move into the
// directories that hold them now, and voice profiles and term bundles named by
// prefix move into the profile tree. Everything folded is source a project
// commits, so it is moved and never deleted.
//
// A checkout's own state is not folded. kapi keeps it under the data root and
// reads none of what an earlier version left inside a checkout; a person brings
// what cannot be rebuilt across with `kapi state import`.

// Best-effort throughout. A file that will not move must not fail a project
// open; a failed move leaves the file where it was. Every fold is idempotent.

// foldLayoutForward moves committed sources an earlier layout kept into the
// places the current one reads. Filesystem only, and it runs BEFORE the store
// opens, because one move carries the committed decision record the working
// store seeds itself from.
func foldLayoutForward(layout project.Layout) {
	moveDirContents(flatDecisionsDir(layout), layout.Export().UnitStateDir())
	foldContextUmbrella(layout)
	foldGovernanceFiles(layout, layout.StateDir)
}

// umbrellaContextDir is the retired directory that grouped the committed
// sources one level below `.kapi/`.
func umbrellaContextDir(layout project.Layout) string {
	return filepath.Join(layout.StateDir, "context")
}

// foldContextUmbrella lifts the `context/` generation's committed sources up
// one level, into the directory that now holds them directly.
//
// Everything in there is authored, terms bundle, memory bundles, voice
// profiles, unit record, whatever notes a project kept beside them, so this
// moves and never deletes, entry by entry. The umbrella itself goes only once
// the last entry has left.
//
// The two subdirectories are folded first and by name, because both are
// renamed on the way up: `decisions/` becomes the unit-state record, and
// `memory/` has to land inside the state directory's own `memory/` rather than
// beside it. Everything else is lifted verbatim, minus the three names already
// handled.
func foldContextUmbrella(layout project.Layout) {
	umbrella := umbrellaContextDir(layout)
	if _, err := os.Stat(umbrella); err != nil {
		return
	}
	moveDirContents(filepath.Join(umbrella, "decisions"), layout.Export().UnitStateDir())
	moveDirContents(filepath.Join(umbrella, "memory"), layout.Export().MemoryDir())
	// The single conventional bundle, which becomes the primary inside the
	// bundle directory rather than a file of its own.
	moveFile(filepath.Join(umbrella, "memory.json"), filepath.Join(layout.Export().MemoryDir(), "memory.json"))
	moveDirContents(umbrella, layout.StateDir, "decisions", "memory", "memory.json")
	foldGovernanceFiles(layout, umbrella)
	_ = os.Remove(umbrella)
}

// foldGovernanceFiles moves the voice profiles and per-profile term bundles of
// a flat directory into the profile tree.
//
// `brand-voice.yaml` is the project default and becomes `voice.yaml`, whose
// scope the directory now states. A `<name>-voice.yaml` or `<name>-terms.json`
// is a per-profile override named by prefix, and becomes
// `profiles/<name>/voice.yaml` or `profiles/<name>/terms.json`. Unprefixed
// `terms.json` is the project's own vocabulary and is left where it is.
//
// A recipe that bound one of these files by path still names the old location
// afterwards: authored bindings are not rewritten to chase a move.
// `kapi store import` finds the new locations with no binding at all.
func foldGovernanceFiles(layout project.Layout, from string) {
	entries, err := os.ReadDir(from)
	if err != nil {
		return
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		switch {
		case name == "brand-voice.yaml":
			moveFile(filepath.Join(from, name), filepath.Join(layout.StateDir, "voice.yaml"))
		case strings.HasSuffix(name, "-voice.yaml"):
			profile := strings.TrimSuffix(name, "-voice.yaml")
			moveFile(filepath.Join(from, name), filepath.Join(layout.Export().ProfileDir(profile), "voice.yaml"))
		case strings.HasSuffix(name, "-terms.json"):
			profile := strings.TrimSuffix(name, "-terms.json")
			moveFile(filepath.Join(from, name), filepath.Join(layout.Export().ProfileDir(profile), "terms.json"))
		}
	}
}

// flatDecisionsDir is the flat layout's decision record, which hung directly
// off `.kapi/`.
func flatDecisionsDir(layout project.Layout) string {
	return filepath.Join(layout.StateDir, "units")
}

// moveDirContents moves every entry of src into dst and removes src once it is
// empty. Used for the directories whose contents no source reproduces.
//
// An entry whose destination already exists is left where it is, and so is one
// whose move fails. Both leave src behind with the data still in it, which is
// resolvable by hand; a shard silently overwritten by another is not.
//
// skip names entries a caller folding a directory in several passes has
// already dealt with elsewhere. They stay in place, and keep src from being
// removed while they are still in it.
func moveDirContents(src, dst string, skip ...string) {
	entries, err := os.ReadDir(src)
	if err != nil {
		return
	}
	if len(entries) == 0 {
		_ = os.Remove(src)
		return
	}
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return
	}
	for _, e := range entries {
		if slices.Contains(skip, e.Name()) {
			continue
		}
		to := filepath.Join(dst, e.Name())
		if _, err := os.Lstat(to); err == nil {
			continue
		}
		_ = os.Rename(filepath.Join(src, e.Name()), to)
	}
	// Succeeds only when every entry moved, which is exactly when src has
	// nothing left to say.
	_ = os.Remove(src)
}

// moveFile moves one file, creating the destination's parent. Same rules as
// moveDirContents: an occupied destination keeps both copies, and a failed move
// leaves the file where it was.
func moveFile(src, dst string) {
	if info, err := os.Lstat(src); err != nil || info.IsDir() {
		return
	}
	if _, err := os.Lstat(dst); err == nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return
	}
	_ = os.Rename(src, dst)
}
