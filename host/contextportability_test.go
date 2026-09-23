package host

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/state"
	"github.com/neokapi/neokapi/kpz"
	"github.com/neokapi/neokapi/memory/kmb"
	"github.com/neokapi/neokapi/terms"
	"github.com/neokapi/neokapi/terms/ktb"
)

// Making a project's context portable: the four verbs and the properties that
// make them usable as a recovery story.
//
// A store holding authored work has to be writable back out, readable back in,
// and carryable to another machine, and each of those has a property a test can
// hold: an import changes nothing the second time, a snapshot's bytes do not
// move on their own, a snapshot read into an empty store snapshots identically,
// a clean clone of a snapshot governs its content by the same fingerprint, and
// a bundle survives a round trip byte for byte.

const portableVoiceYAML = `name: Portable Voice
version: 1
tone:
  formality: neutral
vocabulary:
  forbidden_terms:
    - term: utilize
      replacement: use
      advisory: true
`

const portableProfileVoiceYAML = `name: Portable Landing Voice
version: 1
tone:
  formality: casual
`

const portableTermsJSON = `{
  "schemaVersion": "1.0",
  "kind": "kapi-terms",
  "concepts": [
    {
      "id": "c-widget",
      "domain": "product",
      "definition": "The thing the product is about.",
      "terms": [
        { "text": "widget", "locale": "en", "status": "approved" },
        { "text": "dings", "locale": "nb", "status": "approved" }
      ]
    }
  ]
}
`

// writePortableProject builds a project whose whole context sits in `.kapi/`:
// a bound voice profile, a per-profile voice profile, a bound terms bundle and
// a source document with a translated twin the record absorb learns from.
func writePortableProject(t *testing.T) (a *App, root, recipe string) {
	t.Helper()
	root = t.TempDir()
	return newPortableApp(t, root), root, writePortableTree(t, root)
}

// writePortableTree lays the project out under root and returns its recipe.
func writePortableTree(t *testing.T, root string) string {
	t.Helper()
	layout := project.LayoutAt(root)
	require.NoError(t, os.MkdirAll(layout.StateDir, 0o755))
	require.NoError(t, os.MkdirAll(layout.Export().ProfileDir("landing"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "locales", "en"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "locales", "nb"), 0o755))

	recipe := `version: v1
name: portable
defaults:
  source_language: en
  target_languages: [nb]
profiles:
  landing:
    channels: [web]
collections:
  - name: app
    path: "locales/en/*.json"
    target: "locales/{lang}/*.json"
`
	write := func(rel, content string) {
		path := filepath.Join(root, filepath.FromSlash(rel))
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	}
	write(project.RecipeFileName, recipe)
	write(".kapi/voice.yaml", portableVoiceYAML)
	write(".kapi/profiles/landing/voice.yaml", portableProfileVoiceYAML)
	write(".kapi/terms.json", portableTermsJSON)
	write("locales/en/app.json", "{\n  \"greeting\": \"Hello there\",\n  \"farewell\": \"Goodbye\"\n}\n")
	write("locales/nb/app.json", "{\n  \"greeting\": \"Hei der\",\n  \"farewell\": \"Ha det\"\n}\n")
	return filepath.Join(root, project.RecipeFileName)
}

// newPortableApp returns an App whose project stores are closed when the test
// ends, so a second App over the same tree opens the file rather than a handle
// the first one still holds.
func newPortableApp(t *testing.T, root string) *App {
	t.Helper()
	a := &App{SourceLang: "en"}
	a.InitRegistries()
	t.Cleanup(a.Shutdown)
	_ = root
	return a
}

// seedPortable compiles the project's committed context and records one
// decision, so the store holds something of every kind a snapshot writes.
//
// The decision is recorded before the seeding pass, the order a real project
// meets them in: a review approves wording, and the next run compiles the
// committed context and absorbs the translations that approval blessed. The
// pass carries the decision's governing context onto the pair it learns, so the
// store the snapshot is taken from is the one a run leaves behind.
func seedPortable(t *testing.T, a *App, root, recipe string) {
	t.Helper()
	ctx := context.Background()

	st, err := a.OpenProjectState(ctx, root)
	require.NoError(t, err)
	scope := a.DocumentScope(ctx, root, filepath.Join(root, "locales", "en", "app.json"))
	require.NoError(t, st.Put(ctx, state.UnitState{
		Unit: "greeting", Variant: model.Variant("nb"), Scope: scope,
		Status:               model.TargetStatusReviewed,
		Decision:             state.Decision{ReviewState: "approved", By: "reviewer"},
		TargetHash:           state.TargetHash("Hei der"),
		ContentHash:          state.SourceHash("Hello there"),
		GoverningFingerprint: "fp-portable",
	}))

	_, err = a.seedContext(ctx, recipe)
	require.NoError(t, err)
}

// snapshotInto snapshots the project into a fresh directory and returns it.
func snapshotInto(t *testing.T, a *App, recipe string) (string, ContextSnapshot) {
	t.Helper()
	out := t.TempDir()
	res, err := a.SnapshotProjectContext(context.Background(), recipe, ContextSnapshotRequest{Out: out})
	require.NoError(t, err)
	return out, res
}

// treeBytes reads every file under dir, keyed by its slash path relative to it.
func treeBytes(t *testing.T, dir string) map[string][]byte {
	t.Helper()
	out := map[string][]byte{}
	require.NoError(t, filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		rel, rerr := filepath.Rel(dir, path)
		if rerr != nil {
			return rerr
		}
		data, rerr := os.ReadFile(path)
		if rerr != nil {
			return rerr
		}
		out[filepath.ToSlash(rel)] = data
		return nil
	}))
	return out
}

// assertSameTree compares two written layouts file by file, so a failure names
// the file that moved rather than printing two archives at each other.
//
// Nothing is normalized. An entry in the content memory records three instants
// about its own history: when this store first held it, when it last changed,
// and when each origin was added. They are compared like every other byte,
// because a clean clone that re-learns the committed pairs recognises the
// entries the import already supplied and leaves their stamps alone. A
// comparison that passed only when both snapshots fell inside one second would
// say nothing about the clock.
func assertSameTree(t *testing.T, want, got string, msg string) {
	t.Helper()
	wantFiles, gotFiles := treeBytes(t, want), treeBytes(t, got)
	assert.Equal(t, treePaths(t, want), treePaths(t, got), "%s: the same files", msg)
	for path, data := range wantFiles {
		other, ok := gotFiles[path]
		if !ok {
			continue
		}
		assert.Equal(t, string(data), string(other), "%s: %s", msg, path)
	}
}

// treePaths lists a directory's files in path order.
func treePaths(t *testing.T, dir string) []string {
	t.Helper()
	files := treeBytes(t, dir)
	out := make([]string, 0, len(files))
	for p := range files {
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}

// TestSnapshotProjectContext_WritesTheLayoutTheSeedingPassReads: a snapshot
// produces the `.kapi/` shape, holding one file of every kind the store has.
func TestSnapshotProjectContext_WritesTheLayoutTheSeedingPassReads(t *testing.T) {
	a, root, recipe := writePortableProject(t)
	seedPortable(t, a, root, recipe)

	out, res := snapshotInto(t, a, recipe)

	assert.Equal(t, 1, res.Concepts)
	assert.Positive(t, res.Entries, "the absorbed translations reach the content memory")
	assert.Equal(t, 2, res.VoiceProfiles, "the project default and the profile override")
	assert.Positive(t, res.Decisions)

	paths := treePaths(t, out)
	assert.Contains(t, paths, "terms.json")
	assert.Contains(t, paths, "memory/memory.json")
	assert.Contains(t, paths, "voice.yaml")
	assert.Contains(t, paths, "profiles/landing/voice.yaml")
	var shards int
	for _, p := range paths {
		if strings.HasPrefix(p, project.UnitStateDirName+"/") {
			shards++
		}
	}
	assert.Positive(t, shards, "the decision record is written as shards")

	head, err := os.ReadFile(filepath.Join(out, "voice.yaml"))
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(string(head), snapshotVoiceHeader),
		"a generated profile says so at the top of itself")
}

// TestSnapshotProjectContext_IsByteStable: snapshotting twice with nothing
// changed in between moves no bytes.
func TestSnapshotProjectContext_IsByteStable(t *testing.T) {
	a, root, recipe := writePortableProject(t)
	seedPortable(t, a, root, recipe)

	out := t.TempDir()
	ctx := context.Background()
	first, err := a.SnapshotProjectContext(ctx, recipe, ContextSnapshotRequest{Out: out})
	require.NoError(t, err)
	require.NotEmpty(t, first.Written, "the first snapshot writes the files")
	before := treeBytes(t, out)

	second, err := a.SnapshotProjectContext(ctx, recipe, ContextSnapshotRequest{Out: out})
	require.NoError(t, err)
	assert.Empty(t, second.Written, "a snapshot with nothing to say writes nothing")
	assert.Equal(t, before, treeBytes(t, out))
}

// TestImportProjectContext_IsIdempotent: reading the same layout twice leaves
// the store holding exactly what it held after the first read.
func TestImportProjectContext_IsIdempotent(t *testing.T) {
	a, root, recipe := writePortableProject(t)
	seedPortable(t, a, root, recipe)
	ctx := context.Background()

	first, err := a.ImportProjectContext(ctx, recipe, ContextImportRequest{Force: true})
	require.NoError(t, err)
	require.True(t, first.Read())
	afterFirst, _ := snapshotInto(t, a, recipe)

	second, err := a.ImportProjectContext(ctx, recipe, ContextImportRequest{Force: true})
	require.NoError(t, err)
	assert.Equal(t, first.Concepts, second.Concepts)
	assert.Equal(t, first.VoiceProfiles, second.VoiceProfiles)
	afterSecond, _ := snapshotInto(t, a, recipe)

	assertSameTree(t, afterFirst, afterSecond, "a second read leaves the store saying the same thing")

	// A third read without --force reads nothing at all: this checkout has
	// already read these sources at these bytes, and the store says so.
	third, err := a.ImportProjectContext(ctx, recipe, ContextImportRequest{})
	require.NoError(t, err)
	assert.False(t, third.Read(), "a source at bytes this checkout has read is skipped")
	assert.Positive(t, third.Unchanged)
	afterThird, _ := snapshotInto(t, a, recipe)
	assertSameTree(t, afterFirst, afterThird, "and the store still says the same thing")
}

// TestContextSnapshot_RoundTripsThroughAnEmptyStore: importing a snapshot into
// a project with no store and snapshotting again produces the same bytes.
func TestContextSnapshot_RoundTripsThroughAnEmptyStore(t *testing.T) {
	a, root, recipe := writePortableProject(t)
	seedPortable(t, a, root, recipe)
	first, _ := snapshotInto(t, a, recipe)

	clone, cloneRecipe := clonePortableProject(t, root, first)
	b := newPortableApp(t, clone)
	res, err := b.ImportProjectContext(context.Background(), cloneRecipe, ContextImportRequest{})
	require.NoError(t, err)
	require.True(t, res.Read())

	second, _ := snapshotInto(t, b, cloneRecipe)
	assertSameTree(t, first, second, "a snapshot read into an empty store snapshots back identically")
}

// learnStamp matches the three instants a content-memory entry records about
// its own history: when this store first held it, when it last changed, and
// when each origin was added.
var learnStamp = regexp.MustCompile(`("(?:addedAt|created|updated)": ")20\d\d-\d\d-\d\dT\d\d:\d\d:\d\dZ"`)

// TestContextImport_KeepsTheInstantsTheSnapshotCarried: a clean clone reads a
// snapshot whose content-memory entries were learned long ago, and the pass
// that re-learns the committed pairs from its own target documents leaves those
// instants where they are.
//
// Stamping them with this machine's clock is what made two snapshots of an
// unchanged project differ, and it is invisible: every entry, id, variant and
// origin agrees, and only the second each store says it learned them in moves.
// A doctored instant says so outright rather than by a comparison that passes
// whenever both snapshots fall in one second.
func TestContextImport_KeepsTheInstantsTheSnapshotCarried(t *testing.T) {
	a, root, recipe := writePortableProject(t)
	seedPortable(t, a, root, recipe)
	snapshot, _ := snapshotInto(t, a, recipe)

	bundle := filepath.Join(snapshot, project.MemoryDirName, kmb.ConventionalName)
	data, err := os.ReadFile(bundle)
	require.NoError(t, err)
	const learned = "2020-01-01T00:00:00Z"
	doctored := learnStamp.ReplaceAll(data, []byte(`${1}`+learned+`"`))
	require.Contains(t, string(doctored), learned, "the fixture has instants to doctor")
	require.NoError(t, os.WriteFile(bundle, doctored, 0o644))

	clone, cloneRecipe := clonePortableProject(t, root, snapshot)
	b := newPortableApp(t, clone)
	res, err := b.ImportProjectContext(context.Background(), cloneRecipe, ContextImportRequest{})
	require.NoError(t, err)
	require.True(t, res.Read())

	out, _ := snapshotInto(t, b, cloneRecipe)
	written, err := os.ReadFile(filepath.Join(out, project.MemoryDirName, kmb.ConventionalName))
	require.NoError(t, err)
	assert.Equal(t, string(doctored), string(written),
		"the clone snapshots the entries it read, instants and all")
}

// clonePortableProject writes a fresh checkout of the project holding the
// snapshot at `.kapi/` and no store, the shape of a clean clone.
func clonePortableProject(t *testing.T, root, snapshot string) (string, string) {
	t.Helper()
	clone := t.TempDir()
	recipe := writePortableTree(t, clone)
	layout := project.LayoutAt(clone)
	require.NoError(t, os.RemoveAll(layout.StateDir))
	require.NoError(t, copyTree(snapshot, layout.StateDir))
	require.NoFileExists(t, layout.StorePath(), "a clean clone holds no store")
	return clone, recipe
}

// copyTree copies every file under src into dst.
func copyTree(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, rerr := filepath.Rel(src, path)
		if rerr != nil {
			return rerr
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, rerr := os.ReadFile(path)
		if rerr != nil {
			return rerr
		}
		if rerr := os.MkdirAll(filepath.Dir(target), 0o755); rerr != nil {
			return rerr
		}
		return os.WriteFile(target, data, 0o644)
	})
}

// TestContextSnapshot_CleanCloneGovernsByTheSameFingerprint: a checkout holding
// only a snapshot resolves the governing context its source project resolves,
// which is the property that makes a snapshot a recovery story rather than a
// listing.
func TestContextSnapshot_CleanCloneGovernsByTheSameFingerprint(t *testing.T) {
	a, root, recipe := writePortableProject(t)
	seedPortable(t, a, root, recipe)
	want := governingNow(t, a, recipe, root)

	snapshot, _ := snapshotInto(t, a, recipe)
	clone, cloneRecipe := clonePortableProject(t, root, snapshot)
	b := newPortableApp(t, clone)
	_, err := b.ImportProjectContext(context.Background(), cloneRecipe, ContextImportRequest{})
	require.NoError(t, err)

	assert.Equal(t, want, governingNow(t, b, cloneRecipe, clone),
		"the clone governs its content exactly as the project that wrote the snapshot does")
}

// TestContextSnapshotAndExport_LeaveTheVaultBehind: the withheld originals stay
// on the machine, out of both the files a snapshot writes and the bundle an
// export carries.
func TestContextSnapshotAndExport_LeaveTheVaultBehind(t *testing.T) {
	a, root, recipe := writePortableProject(t)
	seedPortable(t, a, root, recipe)

	const secret = "the-withheld-original"
	layout := project.LayoutAt(root)
	require.NoError(t, os.MkdirAll(layout.VaultDir(), 0o755))
	require.NoError(t, os.WriteFile(layout.RedactionVaultPath(),
		[]byte(`{"blocks":{"b1":{"TOKEN":"`+secret+`"}}}`), 0o600))

	snapshot, _ := snapshotInto(t, a, recipe)
	for path, data := range treeBytes(t, snapshot) {
		assert.NotContains(t, path, project.VaultDirName, "the vault is not a snapshot path")
		assert.NotContains(t, string(data), secret, "no snapshot file carries a withheld original")
	}

	bundle := filepath.Join(t.TempDir(), "context.kpz")
	_, err := a.ExportProjectContext(context.Background(), recipe, bundle)
	require.NoError(t, err)
	data, err := os.ReadFile(bundle)
	require.NoError(t, err)
	assert.NotContains(t, string(data), secret, "the bundle carries no withheld original")

	pkg, err := kpz.Unmarshal(data)
	require.NoError(t, err)
	assert.Empty(t, pkg.Source, "a context bundle carries no source documents")
	assert.Empty(t, pkg.Blocks, "a context bundle carries no content")

	// The vault itself is untouched: excluding it must not mean deleting it.
	held, err := os.ReadFile(layout.RedactionVaultPath())
	require.NoError(t, err)
	assert.Contains(t, string(held), secret)
}

// TestContextBundle_RoundTripsByteForByte: exporting, restoring into a fresh
// store and exporting again yields the same archive.
func TestContextBundle_RoundTripsByteForByte(t *testing.T) {
	a, root, recipe := writePortableProject(t)
	seedPortable(t, a, root, recipe)
	ctx := context.Background()

	first := filepath.Join(t.TempDir(), "context.kpz")
	res, err := a.ExportProjectContext(ctx, recipe, first)
	require.NoError(t, err)
	assert.Equal(t, 1, res.Concepts)
	assert.Equal(t, 2, res.VoiceProfiles)
	assert.Positive(t, res.Decisions)
	assert.NotEmpty(t, res.RootHash)

	clone := t.TempDir()
	cloneRecipe := writePortableTree(t, clone)
	require.NoError(t, os.RemoveAll(project.LayoutAt(clone).StateDir))
	require.NoError(t, os.MkdirAll(project.LayoutAt(clone).StateDir, 0o755))
	b := newPortableApp(t, clone)

	restored, err := b.RestoreProjectContext(ctx, cloneRecipe, first, RestoreRefuse)
	require.NoError(t, err)
	assert.Equal(t, res.Concepts, restored.Concepts)
	assert.Equal(t, res.Entries, restored.Entries)
	assert.Equal(t, res.VoiceProfiles, restored.VoiceProfiles)
	assert.Equal(t, res.Decisions, restored.Decisions)

	second := filepath.Join(t.TempDir(), "context.kpz")
	again, err := b.ExportProjectContext(ctx, cloneRecipe, second)
	require.NoError(t, err)
	assert.Equal(t, res.RootHash, again.RootHash, "the same context has the same content identity")

	firstBytes, err := os.ReadFile(first)
	require.NoError(t, err)
	secondBytes, err := os.ReadFile(second)
	require.NoError(t, err)
	assert.Equal(t, firstBytes, secondBytes, "a bundle survives a restore byte for byte")
}

// TestContextBundle_CarriesTheLedgerNotOneCheckoutsView: two checkouts of one
// project sit on different branches and answer one unit differently, sharing
// the one context store the workspace keeps for the project. A bundle exported
// from either carries both pairings, and both answer again after a restore into
// a fresh store.
//
// The view a checkout reads is by construction one pairing per unit, so an
// export built from it is lossless for that checkout and lossy for the project:
// the other branch's approval is in the ledger and in no bundle.
func TestContextBundle_CarriesTheLedgerNotOneCheckoutsView(t *testing.T) {
	ctx := context.Background()
	base := t.TempDir()

	branchA := filepath.Join(base, "branch-a")
	branchB := filepath.Join(base, "branch-b")
	recipeA := writePortableTree(t, branchA)
	recipeB := writePortableTree(t, branchB)

	// One App over both checkouts, pointed at one workspace, which is what puts
	// them on one context store: the recipes name one project.
	a := newPortableApp(t, branchA)
	a.SetWorkspaceRoot(filepath.Join(base, "workspaces", "shared"))
	seedPortable(t, a, branchA, recipeA)

	stA, err := a.OpenProjectState(ctx, branchA)
	require.NoError(t, err)
	key := state.Key{
		Scope:   a.DocumentScope(ctx, branchA, filepath.Join(branchA, "locales", "en", "app.json")),
		Unit:    "greeting",
		Variant: model.Variant("nb"),
	}
	onBranchA, ok := stA.Get(ctx, key)
	require.True(t, ok, "the seeded project holds the decision it recorded")

	// The other branch translates the same source differently and approves that.
	stB, err := a.OpenProjectState(ctx, branchB)
	require.NoError(t, err)
	onBranchB := onBranchA
	onBranchB.TargetHash = state.TargetHash("Hallo der")
	onBranchB.Decision = state.Decision{ReviewState: "approved", By: "other-reviewer"}
	require.NoError(t, stB.Put(ctx, onBranchB))
	_ = recipeB

	view, err := stA.All(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, countForKey(view, key), "a checkout holds one pairing per unit")
	ledger, err := stA.Ledger(ctx)
	require.NoError(t, err)
	assert.Equal(t, 2, countForKey(ledger, key), "the project has decided both")

	bundle := filepath.Join(t.TempDir(), "context.kpz")
	exported, err := a.ExportProjectContext(ctx, recipeA, bundle)
	require.NoError(t, err)
	assert.Equal(t, len(ledger), exported.Decisions, "the bundle carries every entry the ledger holds")

	restoredRoot := filepath.Join(base, "restored")
	restoredRecipe := writePortableTree(t, restoredRoot)
	require.NoError(t, os.RemoveAll(project.LayoutAt(restoredRoot).StateDir))
	require.NoError(t, os.MkdirAll(project.LayoutAt(restoredRoot).StateDir, 0o755))
	c := newPortableApp(t, restoredRoot)
	c.SetWorkspaceRoot(filepath.Join(base, "workspaces", "fresh"))

	read, err := c.RestoreProjectContext(ctx, restoredRecipe, bundle, RestoreRefuse)
	require.NoError(t, err)
	assert.Equal(t, exported.Decisions, read.Decisions)

	stC, err := c.OpenProjectState(ctx, restoredRoot)
	require.NoError(t, err)
	fromA, ok := stC.Lookup(ctx, key, onBranchA.ContentHash, onBranchA.TargetHash)
	assert.True(t, ok, "the pairing the exporting checkout held answers")
	assert.Equal(t, onBranchA.Decision.By, fromA.Decision.By)
	fromB, ok := stC.Lookup(ctx, key, onBranchB.ContentHash, onBranchB.TargetHash)
	assert.True(t, ok, "and so does the one the other branch held")
	assert.Equal(t, onBranchB.Decision.By, fromB.Decision.By)

	// Bundle identity survives the wider record: exporting the restored store
	// writes the archive it was restored from.
	again := filepath.Join(t.TempDir(), "context.kpz")
	reexported, err := c.ExportProjectContext(ctx, restoredRecipe, again)
	require.NoError(t, err)
	assert.Equal(t, exported.RootHash, reexported.RootHash)
	firstBytes, err := os.ReadFile(bundle)
	require.NoError(t, err)
	secondBytes, err := os.ReadFile(again)
	require.NoError(t, err)
	assert.Equal(t, firstBytes, secondBytes, "a bundle holding two branches survives a restore byte for byte")
}

// TestRestoreProjectContext_LeavesTheCheckoutHoldingItsOwnPairing: restoring a
// bundle that carries another branch's answer for a unit this checkout has
// decided leaves this checkout answering with its own.
func TestRestoreProjectContext_LeavesTheCheckoutHoldingItsOwnPairing(t *testing.T) {
	ctx := context.Background()
	base := t.TempDir()

	branchA := filepath.Join(base, "branch-a")
	branchB := filepath.Join(base, "branch-b")
	recipeA := writePortableTree(t, branchA)
	recipeB := writePortableTree(t, branchB)

	a := newPortableApp(t, branchA)
	a.SetWorkspaceRoot(filepath.Join(base, "workspaces", "shared"))
	seedPortable(t, a, branchA, recipeA)

	stA, err := a.OpenProjectState(ctx, branchA)
	require.NoError(t, err)
	key := state.Key{
		Scope:   a.DocumentScope(ctx, branchA, filepath.Join(branchA, "locales", "en", "app.json")),
		Unit:    "greeting",
		Variant: model.Variant("nb"),
	}
	onBranchA, ok := stA.Get(ctx, key)
	require.True(t, ok)

	stB, err := a.OpenProjectState(ctx, branchB)
	require.NoError(t, err)
	onBranchB := onBranchA
	onBranchB.TargetHash = state.TargetHash("Hallo der")
	onBranchB.Decision = state.Decision{ReviewState: "approved", By: "other-reviewer"}
	require.NoError(t, stB.Put(ctx, onBranchB))

	bundle := filepath.Join(t.TempDir(), "context.kpz")
	_, err = a.ExportProjectContext(ctx, recipeA, bundle)
	require.NoError(t, err)

	_, err = a.RestoreProjectContext(ctx, recipeB, bundle, RestoreMerge)
	require.NoError(t, err)

	held, ok := stB.Get(ctx, key)
	require.True(t, ok)
	assert.Equal(t, onBranchB.TargetHash, held.TargetHash,
		"the branch keeps the translation it approved")
	assert.Equal(t, onBranchB.Decision.By, held.Decision.By)
}

// countForKey counts the records a slice holds for one unit identity.
func countForKey(units []state.UnitState, key state.Key) int {
	n := 0
	for _, u := range units {
		if u.Key() == key {
			n++
		}
	}
	return n
}

// TestRestoreProjectContext_RefusesAStoreThatHoldsContext covers the three
// modes: refuse by default, merge idempotently, replace on request.
func TestRestoreProjectContext_RefusesAStoreThatHoldsContext(t *testing.T) {
	a, root, recipe := writePortableProject(t)
	seedPortable(t, a, root, recipe)
	ctx := context.Background()

	bundle := filepath.Join(t.TempDir(), "context.kpz")
	_, err := a.ExportProjectContext(ctx, recipe, bundle)
	require.NoError(t, err)

	_, err = a.RestoreProjectContext(ctx, recipe, bundle, RestoreRefuse)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--merge")
	assert.Contains(t, err.Error(), "--replace")

	before, _ := snapshotInto(t, a, recipe)
	merged, err := a.RestoreProjectContext(ctx, recipe, bundle, RestoreMerge)
	require.NoError(t, err)
	assert.False(t, merged.Cleared)
	after, _ := snapshotInto(t, a, recipe)
	assertSameTree(t, before, after, "a merge of the same bundle changes nothing")

	replaced, err := a.RestoreProjectContext(ctx, recipe, bundle, RestoreReplace)
	require.NoError(t, err)
	assert.True(t, replaced.Cleared)
	afterReplace, _ := snapshotInto(t, a, recipe)
	assertSameTree(t, before, afterReplace, "replacing a store with the bundle it came from restores the same context")
}

// TestRestoreProjectContext_ReplaceDropsWhatTheBundleDoesNotCarry: a replace
// leaves the store holding the bundle and nothing else.
func TestRestoreProjectContext_ReplaceDropsWhatTheBundleDoesNotCarry(t *testing.T) {
	a, root, recipe := writePortableProject(t)
	seedPortable(t, a, root, recipe)
	ctx := context.Background()

	bundle := filepath.Join(t.TempDir(), "context.kpz")
	_, err := a.ExportProjectContext(ctx, recipe, bundle)
	require.NoError(t, err)

	db, err := a.ProjectDB(ctx, root)
	require.NoError(t, err)
	require.NoError(t, db.Terms().AddConcept(ctx, portableExtraConcept()))
	count, err := db.Terms().Count(ctx)
	require.NoError(t, err)
	require.Equal(t, 2, count)

	_, err = a.RestoreProjectContext(ctx, recipe, bundle, RestoreReplace)
	require.NoError(t, err)
	count, err = db.Terms().Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, count, "the concept the bundle does not carry is gone")
}

// TestRestoreProjectContext_RefusesAPackageOfAnotherKind: the container carries
// several profiles, and only the context one is a thing to restore.
func TestRestoreProjectContext_RefusesAPackageOfAnotherKind(t *testing.T) {
	a, root, recipe := writePortableProject(t)
	seedPortable(t, a, root, recipe)
	_ = root

	pkg := &kpz.Package{Kind: kpz.KindProject, Terms: portableTermsFile(t)}
	data, err := pkg.Marshal()
	require.NoError(t, err)
	path := filepath.Join(t.TempDir(), "project.kpz")
	require.NoError(t, os.WriteFile(path, data, 0o644))

	_, err = a.RestoreProjectContext(context.Background(), recipe, path, RestoreMerge)
	require.Error(t, err)
	assert.Contains(t, err.Error(), kpz.KindProject)
	assert.Contains(t, err.Error(), "kapi context export")
}

// TestRestoreProjectContext_RefusesABundleFromALaterBuild: a package format
// major this build does not speak is refused by name rather than half-read.
func TestRestoreProjectContext_RefusesABundleFromALaterBuild(t *testing.T) {
	a, root, recipe := writePortableProject(t)
	seedPortable(t, a, root, recipe)
	ctx := context.Background()

	bundle := filepath.Join(t.TempDir(), "context.kpz")
	_, err := a.ExportProjectContext(ctx, recipe, bundle)
	require.NoError(t, err)
	rewriteBundleSchemaVersion(t, bundle, "9.0")

	_, err = a.RestoreProjectContext(ctx, recipe, bundle, RestoreMerge)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "later kapi")
	assert.Contains(t, err.Error(), "context.kpz")
}

// rewriteBundleSchemaVersion rebuilds a bundle with another schema version in
// its manifest, so the archive stays intact and only the version a reader
// checks has moved. Editing the bytes in place would break the archive's own
// checksums and the reader would refuse it for the wrong reason.
func rewriteBundleSchemaVersion(t *testing.T, path, version string) {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	require.NoError(t, err)

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, f := range zr.File {
		rc, oerr := f.Open()
		require.NoError(t, oerr)
		body, rerr := io.ReadAll(rc)
		require.NoError(t, rc.Close())
		require.NoError(t, rerr)
		if f.Name == kpz.ManifestPath {
			var manifest map[string]any
			require.NoError(t, json.Unmarshal(body, &manifest))
			manifest["schemaVersion"] = version
			body, err = json.MarshalIndent(manifest, "", "  ")
			require.NoError(t, err)
		}
		w, cerr := zw.CreateHeader(&zip.FileHeader{Name: f.Name, Method: zip.Store})
		require.NoError(t, cerr)
		_, werr := w.Write(body)
		require.NoError(t, werr)
	}
	require.NoError(t, zw.Close())
	require.NoError(t, os.WriteFile(path, buf.Bytes(), 0o644))
}

// TestImportProjectContext_ReadsAnotherCheckoutsLayout: the migration path, for
// a project bringing an existing context across.
func TestImportProjectContext_ReadsAnotherCheckoutsLayout(t *testing.T) {
	a, root, recipe := writePortableProject(t)
	seedPortable(t, a, root, recipe)
	donor, _ := snapshotInto(t, a, recipe)

	clone := t.TempDir()
	cloneRecipe := writePortableTree(t, clone)
	require.NoError(t, os.RemoveAll(project.LayoutAt(clone).StateDir))
	require.NoError(t, os.MkdirAll(project.LayoutAt(clone).StateDir, 0o755))
	b := newPortableApp(t, clone)

	res, err := b.ImportProjectContext(context.Background(), cloneRecipe, ContextImportRequest{Dir: donor})
	require.NoError(t, err)
	assert.Equal(t, 1, res.Concepts)
	assert.Equal(t, 2, res.VoiceProfiles)
	assert.Positive(t, res.Entries)
	assert.Positive(t, res.Decisions, "the donor's decision record comes across")
}

// portableExtraConcept is a concept no bundle in these tests carries.
func portableExtraConcept() terms.Concept {
	return terms.Concept{
		ID:         "c-extra",
		Domain:     "product",
		Definition: "Written after the bundle was made.",
		Terms: []terms.Term{
			{Text: "gadget", Locale: "en", Status: model.TermApproved},
		},
	}
}

// portableTermsFile builds the terms member for a package of another kind.
func portableTermsFile(t *testing.T) *ktb.File {
	t.Helper()
	f, err := ktb.Unmarshal([]byte(portableTermsJSON))
	require.NoError(t, err)
	return f
}
