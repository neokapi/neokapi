package blockstore

import (
	"context"

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
