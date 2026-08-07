package project_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/neokapi/neokapi/core/blockstore"
	"github.com/neokapi/neokapi/core/formats"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// recordingStamper is the test's stand-in for the project store. Extraction
// names the capability it needs (project.BlockStoreStamper) rather than the
// store that provides it, which is exactly what lets this package test the
// extraction path without a database — and what lets the store test the stamps
// without an extraction (core/projectdb).
type recordingStamper struct {
	versions int
	stamps   map[string]project.SourceStamp
	failWith error
}

func newStamper() *recordingStamper { return &recordingStamper{} }

func (s *recordingStamper) StampBlockStoreVersion(context.Context) error {
	if s.failWith != nil {
		return s.failWith
	}
	s.versions++
	return nil
}

func (s *recordingStamper) SaveSourceStamps(_ context.Context, stamps map[string]project.SourceStamp) error {
	if s.failWith != nil {
		return s.failWith
	}
	s.stamps = stamps
	return nil
}

// CompareSourceStamps is the half of drift detection that does not care where
// the stamps came from. Both backends share it, so it is pinned here once.
func TestCompareSourceStamps(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "a.json")
	require.NoError(t, os.WriteFile(src, []byte(`{"k":"v"}`), 0o644))
	files := []project.ResolvedFile{{Path: src, Relative: "a.json", Format: "json"}}

	// Never stamped → changed.
	changed, removed := project.CompareSourceStamps(nil, files)
	assert.Equal(t, []string{"a.json"}, changed, "unstamped source reads as drifted")
	assert.Empty(t, removed)

	info, err := os.Stat(src)
	require.NoError(t, err)
	hash, err := project.HashFile(src)
	require.NoError(t, err)
	stamps := map[string]project.SourceStamp{
		"a.json": {Hash: hash, Size: info.Size(), MTimeNS: info.ModTime().UnixNano()},
	}

	// Stamped and unchanged → clean.
	changed, removed = project.CompareSourceStamps(stamps, files)
	assert.Empty(t, changed, "stamped, unchanged source ⇒ no drift")
	assert.Empty(t, removed)

	// Touch without changing bytes → the hash confirm keeps it clean.
	future := time.Now().Add(2 * time.Second)
	require.NoError(t, os.Chtimes(src, future, future))
	changed, _ = project.CompareSourceStamps(stamps, files)
	assert.Empty(t, changed, "byte-identical touch is not drift (hash confirm)")

	// Edit the bytes → changed.
	require.NoError(t, os.WriteFile(src, []byte(`{"k":"edited"}`), 0o644))
	changed, _ = project.CompareSourceStamps(stamps, files)
	assert.Equal(t, []string{"a.json"}, changed)

	// A stamped file that no longer resolves → removed.
	changed, removed = project.CompareSourceStamps(stamps, nil)
	assert.Empty(t, changed)
	assert.Equal(t, []string{"a.json"}, removed)
}

func TestStoreDrift_Any(t *testing.T) {
	assert.False(t, project.StoreDrift{}.Any())
	assert.True(t, project.StoreDrift{StoreMissing: true}.Any())
	assert.True(t, project.StoreDrift{VersionStale: true}.Any())
	assert.True(t, project.StoreDrift{Changed: []string{"a"}}.Any())
	assert.True(t, project.StoreDrift{Removed: []string{"a"}}.Any())
}

// extractFixture scaffolds a one-collection project with the given source files
// and returns everything ExtractToBlockStore needs.
func extractFixture(t *testing.T, sources map[string]string) (*registry.FormatRegistry, *project.ProjectContext, []project.ResolvedFile) {
	t.Helper()
	real, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)

	recipe := filepath.Join(real, "app.kapi")
	proj := &project.KapiProject{
		Version:  "v1",
		Name:     "StoreSync",
		Defaults: project.Defaults{SourceLanguage: "en-US"},
		Collections: []project.Collection{{
			Name:   "ui",
			Path:   "src/*.json",
			Format: &project.FormatSpec{Name: "json"},
		}},
	}
	require.NoError(t, project.Save(recipe, proj))
	srcDir := filepath.Join(real, "src")
	require.NoError(t, os.MkdirAll(srcDir, 0o755))
	for name, body := range sources {
		require.NoError(t, os.WriteFile(filepath.Join(srcDir, name), []byte(body), 0o644))
	}

	reg := registry.NewFormatRegistry()
	formats.RegisterAll(reg)

	pctx := project.NewProjectContext(proj, recipe)
	files, err := pctx.ResolveContent(reg)
	require.NoError(t, err)
	return reg, pctx, files
}

func TestExtractToBlockStore_RoundTripAndStamps(t *testing.T) {
	reg, pctx, files := extractFixture(t, map[string]string{
		"a.json": `{"greeting":"Hello","farewell":"Bye"}`,
	})
	require.Len(t, files, 1)

	store := blockstore.NewMemoryStore()
	defer store.Close()
	stamper := newStamper()

	stats, err := project.ExtractToBlockStore(context.Background(), reg, pctx, store, stamper, files)
	require.NoError(t, err)
	assert.Equal(t, 1, stats.Files)
	assert.Equal(t, 2, stats.Blocks)
	assert.Empty(t, stats.Skipped)

	// Blocks are keyed by the shared store hash under the collection label.
	sess, err := store.Begin(context.Background())
	require.NoError(t, err)
	defer sess.Close()
	hashes := map[string]bool{}
	for b, berr := range sess.Blocks(blockstore.BlockFilter{Collection: "ui"}) {
		require.NoError(t, berr)
		require.NotEmpty(t, b.ID)
		assert.Equal(t, project.BlockStoreHash("src/a.json", b.ID, ""), b.Hash,
			"blocks are keyed by the shared source-file-namespaced store hash")
		hashes[b.Hash] = true
	}
	assert.Len(t, hashes, 2)

	// The store was stamped with the running semantics and with a stamp per
	// extracted source, so a re-detect over the same tree reads no drift.
	assert.Equal(t, 1, stamper.versions)
	require.Contains(t, stamper.stamps, "src/a.json")
	changed, removed := project.CompareSourceStamps(stamper.stamps, files)
	assert.Empty(t, changed, "freshly extracted ⇒ no drift")
	assert.Empty(t, removed)
}

// TestExtractToBlockStore_ReportsUnrecordableStamps covers the failure that used
// to be completely invisible.
//
// The version and drift stamps are written best-effort on purpose: the blocks
// are already committed, so a failed stamp write must not fail the run. But
// "best effort" was implemented as `_ = StampBlockStoreVersion(...)` with no
// report of any kind, and the comment justifying it assumes the NEXT write
// succeeds. When the cause is permanent — a read-only state directory, a full
// disk or a changed owner in the field — drift detection reads the store as
// stale forever, so every single run re-extracts and re-translates the whole
// project. The only symptom is that kapi is slow and redoes finished work, with
// nothing anywhere naming the cause.
//
// The contract: the extraction still succeeds, and it says what went wrong.
func TestExtractToBlockStore_ReportsUnrecordableStamps(t *testing.T) {
	reg, pctx, files := extractFixture(t, map[string]string{"a.json": `{"greeting":"Hello"}`})

	store := blockstore.NewMemoryStore()
	defer store.Close()
	stamper := &recordingStamper{failWith: errors.New("attempt to write a readonly database")}

	stats, err := project.ExtractToBlockStore(context.Background(), reg, pctx, store, stamper, files)

	// The content is safe: unrecordable stamps must not fail the extraction.
	require.NoError(t, err, "a failed cache hint must not throw away committed blocks")
	assert.Equal(t, 1, stats.Files)
	assert.Positive(t, stats.Blocks)

	// ...and the operator is told, once per failed stamp.
	require.Len(t, stats.Warnings, 2, "both the version stamp and the drift stamps failed")
	joined := strings.Join(stats.Warnings, "\n")
	assert.Contains(t, joined, "attempt to write a readonly database",
		"the warning must carry the cause, not just that a write failed")
	assert.Contains(t, joined, "re-extract on every run",
		"the warning must say what the consequence is, not just that a write failed")
}

// TestExtractToBlockStore_NoWarningsOnHealthyRun is the companion guard: the
// warning channel must stay empty when nothing is wrong, or it becomes noise
// that operators learn to ignore.
func TestExtractToBlockStore_NoWarningsOnHealthyRun(t *testing.T) {
	reg, pctx, files := extractFixture(t, map[string]string{"a.json": `{"greeting":"Hello"}`})

	store := blockstore.NewMemoryStore()
	defer store.Close()

	stats, err := project.ExtractToBlockStore(context.Background(), reg, pctx, store, newStamper(), files)
	require.NoError(t, err)
	assert.Empty(t, stats.Warnings, "a healthy extraction must report nothing")
}

// An extraction with nowhere to record its stamps is refused outright rather
// than run: every source it wrote would read as drifted on every later run, so
// the "successful" extraction would be work the project can never bank.
func TestExtractToBlockStore_RefusesWithoutStamper(t *testing.T) {
	reg, pctx, files := extractFixture(t, map[string]string{"a.json": `{"greeting":"Hello"}`})

	store := blockstore.NewMemoryStore()
	defer store.Close()

	_, err := project.ExtractToBlockStore(context.Background(), reg, pctx, store, nil, files)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "nil stamper")
}

func TestExtractToBlockStore_SkipsUnreadable(t *testing.T) {
	reg, pctx, files := extractFixture(t, map[string]string{"ok.json": `{"k":"v"}`})
	// Add a file with no detectable format: skipped, not fatal.
	files = append(files, project.ResolvedFile{
		Path:     filepath.Join(filepath.Dir(files[0].Path), "mystery.bin"),
		Relative: "src/mystery.bin",
	})

	store := blockstore.NewMemoryStore()
	defer store.Close()
	stats, err := project.ExtractToBlockStore(context.Background(), reg, pctx, store, newStamper(), files)
	require.NoError(t, err)
	assert.Equal(t, 1, stats.Files)
	require.Len(t, stats.Skipped, 1)
	assert.Equal(t, "src/mystery.bin", stats.Skipped[0].Path)
}

func TestCollectionLabel(t *testing.T) {
	assert.Equal(t, "(unnamed)", project.CollectionLabel(""))
	assert.Equal(t, "docs", project.CollectionLabel("docs"))
}

func TestMaterializePolicy(t *testing.T) {
	var d project.Defaults
	assert.Equal(t, project.MaterializeManual, d.ResolvedMaterialize(), "manual is the default")
	d.Materialize = project.MaterializeOnConverge
	assert.Equal(t, project.MaterializeOnConverge, d.ResolvedMaterialize())

	// Validation rejects unknown values.
	p := &project.KapiProject{Version: "v1", Defaults: project.Defaults{Materialize: "sometimes"}}
	err := p.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "defaults.materialize")

	p.Defaults.Materialize = project.MaterializeOnConverge
	assert.NoError(t, p.Validate())
}
