package blockstore

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/neokapi/neokapi/core/model"
)

// The project-wide block store keys blocks and overlays by a single string.
// Format readers assign *file-local* block IDs ("tu1", "tu2", …) that restart in
// every source file, so keying on the raw ID lets blocks and target overlays
// from different source files collide — the last writer wins, corrupting
// per-file coverage and the `kapi run` → `kapi merge` round-trip. StoreKey
// namespaces the key by the source file's project-relative path, keeping every
// (file, block) distinct while staying stable across re-reads so overlays
// written by a run are found again at merge time.
func StoreKey(sourceRel, blockID, sourceText string) string {
	seed := sourceRel + "\x00" + blockID
	if blockID == "" {
		seed = sourceRel + "\x00" + sourceText
	}
	return model.ComputeContentHash(seed)
}

// ProjectRel derives the namespace half of StoreKey for a source file: the
// file's path relative to the project root, in the exact spelling the store's
// keys use (project.ResolvedFile.Relative, which is filepath.Rel of the
// resolver's absolute path against the project directory).
//
// Both ends of the `kapi run --process-only` → `kapi merge` round-trip must spell
// this identically or the overlays the run commits cannot be found again, and the
// symptom is not an error: merge finds no translation for any block, treats that
// as pending work (blockstore.ErrNotFound is the legitimate "not translated yet"
// sentinel) and writes the source text into every target file, exit 0. Hence one
// function, and hence an error rather than a fallback — a run that cannot name
// its own file the way merge will has nowhere to put its work.
//
// path may be relative (a CLI argument as the user typed it) and is resolved
// against the process working directory, exactly as the resolver did. Symlinks
// are deliberately NOT resolved: the resolver does not resolve them either, and
// agreement between the two sides is what matters, not canonicality.
func ProjectRel(projectRoot, path string) (string, error) {
	absRoot, err := filepath.Abs(projectRoot)
	if err != nil {
		return "", fmt.Errorf("resolve project root %q: %w", projectRoot, err)
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve %q: %w", path, err)
	}
	rel, err := filepath.Rel(absRoot, absPath)
	if err != nil {
		return "", fmt.Errorf("%s is not addressable relative to the project root %s, "+
			"so its blocks cannot be keyed the way merge will look them up: %w", absPath, absRoot, err)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("%s lies outside the project root %s: a process-only run stores its "+
			"work under the file's project-relative path, and merge only ever looks up files the "+
			"recipe resolves, so this work could never be collected. Write the result directly with "+
			"-o, add the file to the recipe's content, or run outside the project", absPath, absRoot)
	}
	return rel, nil
}

// SourceNamespace names one source file inside a store many files share: its
// project-relative path when the project root can name it, its absolute path
// when it cannot.
//
// Every key in such a store must name its file distinctly — the file-local block
// id ("tu1") restarts in every source and so is not a name. A run that
// legitimately writes output for a file lying outside the project (an explicit
// `-i`/`-o` pair while a recipe happens to be in scope) therefore still gets a
// namespace of its own instead of sharing the raw id space with every other such
// file.
//
// Only project-relative namespaces are ever looked up again: merge materializes
// exactly the files the recipe resolves. An absolute-path namespace is write-only
// overlay cache scope, which is precisely what an out-of-project input is — and
// why the process-only path, whose only deliverable IS the overlays, refuses that
// case through ProjectRel rather than storing work nobody will collect.
func SourceNamespace(projectRoot, path string) string {
	if rel, err := ProjectRel(projectRoot, path); err == nil {
		return rel
	}
	if abs, err := filepath.Abs(path); err == nil {
		return abs
	}
	return path
}

type sourceRelKey struct{}

// WithSourceRel tags a context with the project-relative path of the source file
// currently being processed. Run/merge entry points set it per file so the
// overlay-writing session tools (and the merge read-back) address a block's
// overlays by StoreKey rather than the collision-prone file-local id. When
// unset (ad-hoc single-file runs with no project), OverlayKey falls back to the
// raw id — a single document can't collide with itself.
func WithSourceRel(ctx context.Context, rel string) context.Context {
	return context.WithValue(ctx, sourceRelKey{}, rel)
}

// SourceRel returns the source-file path set by WithSourceRel, or "".
func SourceRel(ctx context.Context) string {
	if v, ok := ctx.Value(sourceRelKey{}).(string); ok {
		return v
	}
	return ""
}

// OverlayKey is the key to address a block's overlays. With a source file in
// context it is the globally-unique StoreKey (matching the block's stored hash);
// without one it is the raw block id (single-document scope, no collision).
func OverlayKey(ctx context.Context, blockID, sourceText string) string {
	if rel := SourceRel(ctx); rel != "" {
		return StoreKey(rel, blockID, sourceText)
	}
	return blockID
}
