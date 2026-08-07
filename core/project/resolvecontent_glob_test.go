package project

import (
	"path"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/registry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// #1468 item 2: ResolveContent used to answer a glob expansion failure with
// `continue`, dropping the whole content item. This is the single
// content-resolution seam for ls, extract, merge --materialize, up and
// ExtractToBlockStore, so a dropped item is invisible everywhere downstream:
// coverage cannot tell "this collection has no units" from "this collection
// never resolved", and the locale's percentages are computed over a smaller
// universe and read as complete — #1449's shape at the resolution layer.
//
// The failure is a real one: doublestar reports path.ErrBadPattern for an
// unclosed `[` or `{`, which nothing in KapiProject.Validate rejects, so a
// hand-edited recipe reaches here.

// TestResolveContent_MalformedPatternFailsNamingTheCollection is the regression.
// What a user saw: `kapi extract` printing "Extracted 4 document(s)" for a
// recipe that declares six, exit 0 — and `kapi up` reporting the locale
// converged over the four it could see.
func TestResolveContent_MalformedPatternFailsNamingTheCollection(t *testing.T) {
	dir := t.TempDir()
	createFile(t, dir, "docs/guide.md", "# Guide")
	createFile(t, dir, "store/ui.json", "{}")

	reg := registry.NewFormatRegistry()
	registerBuiltIn(reg, "json", ".json")
	registerBuiltIn(reg, "markdown", ".md")

	proj := &KapiProject{
		Version: CurrentVersion,
		Collections: []Collection{
			{Name: "Docs", Content: []ContentItem{{Path: "docs/*.md"}}},
			// Unclosed character class — a plausible hand-edit typo.
			{Name: "Store", Content: []ContentItem{{Path: "store/[a-.json"}}},
		},
	}
	ctx := NewProjectContext(proj, filepath.Join(dir, "kapi.yaml"))

	files, err := ctx.ResolveContent(reg)
	require.Error(t, err, "a pattern that cannot be expanded must fail resolution, not silently drop its content")
	assert.Nil(t, files, "a partial content set must not be handed to callers as if it were complete")
	assert.Contains(t, err.Error(), `"Store"`, "the error must name the collection that failed")
	assert.Contains(t, err.Error(), "store/[a-.json", "and the pattern to fix")
	assert.ErrorIs(t, err, path.ErrBadPattern, "the underlying cause stays reachable")
}

// TestResolveContent_BareEntryMalformedPatternFails covers the bare-entry form,
// where there is no collection name to quote — the message must still name the
// pattern rather than degrade to a bare glob error.
func TestResolveContent_BareEntryMalformedPatternFails(t *testing.T) {
	dir := t.TempDir()
	createFile(t, dir, "store/ui.json", "{}")

	reg := registry.NewFormatRegistry()
	registerBuiltIn(reg, "json", ".json")

	proj := &KapiProject{
		Version:     CurrentVersion,
		Collections: []Collection{{Path: "store/{a"}},
	}
	ctx := NewProjectContext(proj, filepath.Join(dir, "kapi.yaml"))

	_, err := ctx.ResolveContent(reg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "store/{a")
}

// TestResolveContent_PatternMatchingNothingIsNotAnError is the discriminating
// control, and the reason "fail on any empty expansion" is not an acceptable
// substitute for the fix. An empty collection and a collection that failed to
// resolve are different facts: a well-formed pattern that currently matches no
// files is ordinary — content not written yet, or all of it excluded — and must
// resolve to zero files with no error, exactly as before.
func TestResolveContent_PatternMatchingNothingIsNotAnError(t *testing.T) {
	dir := t.TempDir()
	createFile(t, dir, "docs/guide.md", "# Guide")

	reg := registry.NewFormatRegistry()
	registerBuiltIn(reg, "json", ".json")
	registerBuiltIn(reg, "markdown", ".md")

	proj := &KapiProject{
		Version: CurrentVersion,
		Collections: []Collection{
			{Name: "Docs", Content: []ContentItem{{Path: "docs/*.md"}}},
			{Name: "Empty", Content: []ContentItem{{Path: "not-written-yet/**/*.json"}}},
		},
	}
	ctx := NewProjectContext(proj, filepath.Join(dir, "kapi.yaml"))

	files, err := ctx.ResolveContent(reg)
	require.NoError(t, err, "an empty collection is not a failed collection")
	require.Len(t, files, 1)
	assert.Equal(t, "Docs", files[0].Collection)
}

// TestResolveContent_ExcludedToEmptyIsNotAnError is the second half of the same
// control: a pattern whose every match is excluded also resolves to nothing
// without error.
func TestResolveContent_ExcludedToEmptyIsNotAnError(t *testing.T) {
	dir := t.TempDir()
	createFile(t, dir, "store/ui.json", "{}")

	reg := registry.NewFormatRegistry()
	registerBuiltIn(reg, "json", ".json")

	proj := &KapiProject{
		Version:     CurrentVersion,
		Defaults:    Defaults{Exclude: []string{"store/**"}},
		Collections: []Collection{{Name: "Store", Content: []ContentItem{{Path: "store/*.json"}}}},
	}
	ctx := NewProjectContext(proj, filepath.Join(dir, "kapi.yaml"))

	files, err := ctx.ResolveContent(reg)
	require.NoError(t, err)
	assert.Empty(t, files)
}
