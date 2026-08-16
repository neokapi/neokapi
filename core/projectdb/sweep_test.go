package projectdb_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/projectdb"
	"github.com/neokapi/neokapi/core/state"
)

// The two predecessor layouts, spelled here as the test's own fixtures. The
// names are constants nowhere — core/projectdb keeps private copies so nothing
// live can reach for one — and a test that borrowed those would assert against
// the implementation rather than against the disk.
func flatCacheDir(layout project.Layout) string {
	return filepath.Join(layout.StateDir, "cache")
}

func flatStorePath(layout project.Layout) string {
	return filepath.Join(layout.StateDir, "store.db")
}

func flatDecisionsDir(layout project.Layout) string {
	return filepath.Join(layout.StateDir, "units")
}

func flatVaultDir(layout project.Layout) string {
	return filepath.Join(layout.StateDir, "vault")
}

func oldBlockStorePath(layout project.Layout) string {
	return filepath.Join(flatCacheDir(layout), "blocks.db")
}

// oldWorkStorePath is the four-file layout's working store. Its directory is
// the one the current layout keeps ALL machine state in, so the file goes and
// the directory stays — the one asymmetry these tests exist to pin.
func oldWorkStorePath(layout project.Layout) string {
	return filepath.Join(layout.WorkDir(), "state.db")
}

// predecessorFiles lists everything the four-file layout left in a state
// directory, plus the two spellings the vocabulary sweep retired before it.
func predecessorFiles(layout project.Layout) []string {
	blocks := oldBlockStorePath(layout)
	files := []string{
		blocks,
		blocks + "-wal",
		blocks + ".kapiversion",
		blocks + ".sources.json",
		oldWorkStorePath(layout),
		oldWorkStorePath(layout) + "-wal",
	}
	for _, name := range []string{"memory.db", "terms.db", "tm.db", "termbase.db"} {
		files = append(files,
			filepath.Join(layout.StateDir, name),
			filepath.Join(layout.StateDir, name+"-wal"))
	}
	return files
}

// writePredecessorLayout materialises the four-file layout. The contents do not
// matter: nothing in them is migrated, so the sweep never opens them except for
// the working store — which a caller that cares about staged decisions has
// already written for real, and is left alone here.
func writePredecessorLayout(t *testing.T, layout project.Layout) {
	t.Helper()
	require.NoError(t, os.MkdirAll(flatCacheDir(layout), 0o755))
	require.NoError(t, os.MkdirAll(layout.WorkDir(), 0o755))
	for _, path := range predecessorFiles(layout) {
		if _, err := os.Stat(path); err == nil {
			continue
		}
		require.NoError(t, os.WriteFile(path, []byte("predecessor"), 0o644))
	}
}

func assertPredecessorsGone(t *testing.T, layout project.Layout) {
	t.Helper()
	for _, path := range predecessorFiles(layout) {
		assert.NoFileExists(t, path)
	}
	assert.NoDirExists(t, flatCacheDir(layout), "the flat cache root goes whole")
	assert.DirExists(t, layout.WorkDir(), "the work directory is the live machine-state root, not a leftover")
}

// A staged decision is the one thing a predecessor working store holds that no
// committed source can reproduce, so it is the one thing the sweep carries.
func TestSweep_CarriesStagedDecisionsForward(t *testing.T) {
	layout := newLayout(t)
	require.NoError(t, os.MkdirAll(layout.StateDir, 0o755))

	// A committed record the new working store will re-seed from, so the test
	// can tell a carried decision from a re-derived one.
	require.NoError(t, state.WriteCommitted(layout.UnitStateDir(), []state.UnitState{
		unit("u-committed", "d-intro", "Already committed"),
	}))

	old, err := state.OpenWork(t.Context(), oldWorkStorePath(layout), layout.UnitStateDir())
	require.NoError(t, err)
	require.NoError(t, old.Put(t.Context(), unit("u-staged", "d-intro", "Staged, never committed")))
	pending, err := old.Pending(t.Context())
	require.NoError(t, err)
	require.Equal(t, 1, pending, "the seeded unit is not staged; the new decision is")
	require.NoError(t, old.Close())

	writePredecessorLayout(t, layout)

	db := openStore(t, layout)

	staged, err := db.Work().Staged(t.Context())
	require.NoError(t, err)
	require.Len(t, staged, 1, "exactly the staged decision came across")
	assert.Equal(t, "u-staged", staged[0].Unit)
	assert.Equal(t, "approved", staged[0].Decision.ReviewState)

	// The committed unit is present too, but as a re-seed from the committed
	// shards rather than a carried decision.
	all, err := db.Work().All(t.Context())
	require.NoError(t, err)
	require.Len(t, all, 2)
	_, ok := db.Work().Get(t.Context(), state.Key{Scope: "d-intro", Unit: "u-committed", Variant: model.Variant("nb")})
	assert.True(t, ok)

	assertPredecessorsGone(t, layout)
}

// The browser build's predecessor is a JSON sidecar beside the database it
// stood in for. Same contract: staged decisions come across, the file goes.
func TestSweep_CarriesStagedDecisionsFromSidecar(t *testing.T) {
	layout := newLayout(t)
	require.NoError(t, os.MkdirAll(layout.WorkDir(), 0o755))

	sidecar := filepath.Join(layout.WorkDir(), "state.json")
	old, err := state.OpenWorkSidecar(t.Context(), sidecar, layout.UnitStateDir())
	require.NoError(t, err)
	require.NoError(t, old.Put(t.Context(), unit("u-sidecar", "d-intro", "Staged in the browser")))
	require.NoError(t, old.Close())
	require.FileExists(t, sidecar)

	db := openStore(t, layout)

	staged, err := db.Work().Staged(t.Context())
	require.NoError(t, err)
	require.Len(t, staged, 1)
	assert.Equal(t, "u-sidecar", staged[0].Unit)

	assert.NoFileExists(t, sidecar)
}

// No predecessor is the ordinary case, and the common one after the first
// open: the sweep must be a no-op that costs a few stats.
func TestSweep_NoPredecessorIsNoOp(t *testing.T) {
	layout := newLayout(t)
	db := openStore(t, layout)

	pending, err := db.Work().Pending(t.Context())
	require.NoError(t, err)
	assert.Zero(t, pending)
	assert.NoFileExists(t, oldWorkStorePath(layout))

	// Re-opening finds nothing to sweep and changes nothing.
	require.NoError(t, db.Close())
	again := openStore(t, layout)
	pending, err = again.Work().Pending(t.Context())
	require.NoError(t, err)
	assert.Zero(t, pending)
}

// The sweep runs on every open, so a predecessor that reappears (a stale file
// restored from a backup, a branch switch) is swept the next time too.
func TestSweep_RunsOnEveryOpen(t *testing.T) {
	layout := newLayout(t)
	db := openStore(t, layout)
	require.NoError(t, db.Close())

	writePredecessorLayout(t, layout)
	openStore(t, layout)
	assertPredecessorsGone(t, layout)
}

// A work directory holding something the sweep does not own stays, along with
// what is in it: deleting an unrecognised file is not the sweep's business.
func TestSweep_LeavesUnrecognisedWorkDirContents(t *testing.T) {
	layout := newLayout(t)
	writePredecessorLayout(t, layout)
	stray := filepath.Join(layout.WorkDir(), "notes.txt")
	require.NoError(t, os.WriteFile(stray, []byte("mine"), 0o644))

	openStore(t, layout)

	assert.FileExists(t, stray)
	assert.NoFileExists(t, oldWorkStorePath(layout), "the store this package owns still goes")
}

// A predecessor working store that will not open must not fail the project
// open: the alternative is a project that cannot be used because of a file
// about to be deleted.
func TestSweep_UnreadablePredecessorDoesNotFailOpen(t *testing.T) {
	layout := newLayout(t)
	require.NoError(t, os.MkdirAll(layout.WorkDir(), 0o755))
	require.NoError(t, os.WriteFile(oldWorkStorePath(layout), []byte("this is not a database"), 0o644))

	db, err := projectdb.Open(t.Context(), layout)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	require.NotNil(t, db.Work())
	assert.NoFileExists(t, oldWorkStorePath(layout))
}

// ─── the flat layout: units, vault and the store hung straight off `.kapi/` ──

// The committed decision record is authored data. It MOVES, byte for byte:
// a shard that arrived rewritten, or arrived not at all, is review history
// silently rewritten by a directory rename.
func TestFold_MovesCommittedRecordIntoUnitState(t *testing.T) {
	layout := newLayout(t)
	require.NoError(t, state.WriteCommitted(flatDecisionsDir(layout), []state.UnitState{
		unit("u-flat", "d-intro", "Committed under the flat layout"),
	}))

	shards, err := os.ReadDir(flatDecisionsDir(layout))
	require.NoError(t, err)
	require.Len(t, shards, 1)
	before, err := os.ReadFile(filepath.Join(flatDecisionsDir(layout), shards[0].Name()))
	require.NoError(t, err)

	db := openStore(t, layout)

	after, err := os.ReadFile(filepath.Join(layout.UnitStateDir(), shards[0].Name()))
	require.NoError(t, err, "the shard landed under .kapi/state/")
	assert.Equal(t, before, after, "the committed record moves byte-identical")
	assert.NoDirExists(t, flatDecisionsDir(layout), "an emptied source directory goes")

	// And it is a record, not just a file: the working store seeded from it,
	// which is why the fold has to precede the open.
	_, ok := db.Work().Get(t.Context(), state.Key{Scope: "d-intro", Unit: "u-flat", Variant: model.Variant("nb")})
	assert.True(t, ok, "the moved record seeded the working set on this same open")
}

// The vault holds the only copy of a withheld original. It moves for the same
// reason the decision record does, and is the one thing under work/ that kapi
// never deletes on its own initiative.
func TestFold_MovesVaultIntoWork(t *testing.T) {
	layout := newLayout(t)
	require.NoError(t, os.MkdirAll(filepath.Join(flatVaultDir(layout), "batches"), 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(flatVaultDir(layout), "redaction.json"),
		[]byte(`{"secret":"the original"}`), 0o600))
	require.NoError(t, os.WriteFile(
		filepath.Join(flatVaultDir(layout), "batches", "b-1.json"),
		[]byte(`{"secret":"nested"}`), 0o600))

	openStore(t, layout)

	moved, err := os.ReadFile(layout.RedactionVaultPath())
	require.NoError(t, err, "the project vault landed under .kapi/work/vault/")
	assert.JSONEq(t, `{"secret":"the original"}`, string(moved))

	nested, err := os.ReadFile(filepath.Join(layout.VaultDir(), "batches", "b-1.json"))
	require.NoError(t, err, "a subdirectory of the vault moves whole")
	assert.JSONEq(t, `{"secret":"nested"}`, string(nested))

	assert.NoDirExists(t, flatVaultDir(layout))
}

// The per-batch redaction sidecars are the one thing under the cache root that
// no rerun reproduces: they hold the originals withheld from an extract → merge
// batch, so a batch still in flight cannot be restored without them. They move
// out before the cache root is deleted.
func TestFold_MovesRedactionSidecarsOutOfTheCache(t *testing.T) {
	layout := newLayout(t)
	flatRedaction := filepath.Join(flatCacheDir(layout), "redaction")
	require.NoError(t, os.MkdirAll(flatRedaction, 0o755))

	sidecar := []byte(`{"placeholder":"[[EMAIL_1]]","original":"ada@example.com"}`)
	require.NoError(t, os.WriteFile(filepath.Join(flatRedaction, "b-inflight.json"), sidecar, 0o600))
	// Something regenerable in the same cache root, to show the move is
	// targeted rather than a reprieve for the whole directory.
	require.NoError(t, os.MkdirAll(filepath.Join(flatCacheDir(layout), "docs"), 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(flatCacheDir(layout), "docs", "parsed.json"), []byte("cached"), 0o644))

	openStore(t, layout)

	moved, err := os.ReadFile(layout.RedactionSidecarPath("b-inflight"))
	require.NoError(t, err, "the batch sidecar landed under .kapi/work/cache/redaction/")
	assert.Equal(t, sidecar, moved, "the withheld originals move byte-identical")

	assert.NoDirExists(t, flatCacheDir(layout), "the rest of the flat cache still goes")
}

// A sidecar already at the destination is not overwritten, and the source is
// not deleted unread — losing an original is the failure this fold exists to
// avoid, so a collision is left for a person to resolve.
func TestFold_RedactionSidecarCollisionKeepsBothCopies(t *testing.T) {
	layout := newLayout(t)
	flatRedaction := filepath.Join(flatCacheDir(layout), "redaction")
	require.NoError(t, os.MkdirAll(flatRedaction, 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(flatRedaction, "b-1.json"), []byte(`{"original":"the flat copy"}`), 0o600))

	require.NoError(t, os.MkdirAll(filepath.Dir(layout.RedactionSidecarPath("b-1")), 0o755))
	current := []byte(`{"original":"the current copy"}`)
	require.NoError(t, os.WriteFile(layout.RedactionSidecarPath("b-1"), current, 0o600))

	openStore(t, layout)

	kept, err := os.ReadFile(layout.RedactionSidecarPath("b-1"))
	require.NoError(t, err)
	assert.Equal(t, current, kept, "the destination sidecar is never overwritten")
	assert.FileExists(t, filepath.Join(flatRedaction, "b-1.json"),
		"and the source stays, visibly, rather than being deleted unread")
}

// The flat store and cache are projections under a path nothing reads any
// more. They go; nothing is carried out of them.
func TestFold_RetiresFlatStoreAndCache(t *testing.T) {
	layout := newLayout(t)
	require.NoError(t, os.MkdirAll(filepath.Join(flatCacheDir(layout), "extractions"), 0o755))
	for _, path := range []string{
		flatStorePath(layout),
		flatStorePath(layout) + "-wal",
		flatStorePath(layout) + "-shm",
		filepath.Join(layout.StateDir, "store.json"),
		filepath.Join(flatCacheDir(layout), "extractions", "b-1.json"),
	} {
		require.NoError(t, os.WriteFile(path, []byte("flat"), 0o644))
	}

	openStore(t, layout)

	for _, path := range []string{
		flatStorePath(layout),
		flatStorePath(layout) + "-wal",
		flatStorePath(layout) + "-shm",
		filepath.Join(layout.StateDir, "store.json"),
	} {
		assert.NoFileExists(t, path)
	}
	assert.NoDirExists(t, flatCacheDir(layout))
	assert.FileExists(t, layout.StorePath(), "the live store is the one under work/")
}

// The second open must find nothing to do. A fold that ran twice on a project
// that had already been folded would be a fold that could move live data.
func TestFold_SecondOpenIsANoOp(t *testing.T) {
	layout := newLayout(t)
	require.NoError(t, state.WriteCommitted(flatDecisionsDir(layout), []state.UnitState{
		unit("u-flat", "d-intro", "Committed under the flat layout"),
	}))
	require.NoError(t, os.MkdirAll(flatVaultDir(layout), 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(flatVaultDir(layout), "redaction.json"), []byte(`{"a":1}`), 0o600))

	db := openStore(t, layout)
	require.NoError(t, db.Close())

	shards, err := os.ReadDir(layout.UnitStateDir())
	require.NoError(t, err)
	require.Len(t, shards, 1)
	firstShard, err := os.ReadFile(filepath.Join(layout.UnitStateDir(), shards[0].Name()))
	require.NoError(t, err)
	firstVault, err := os.ReadFile(layout.RedactionVaultPath())
	require.NoError(t, err)

	openStore(t, layout)

	secondShard, err := os.ReadFile(filepath.Join(layout.UnitStateDir(), shards[0].Name()))
	require.NoError(t, err)
	secondVault, err := os.ReadFile(layout.RedactionVaultPath())
	require.NoError(t, err)
	assert.Equal(t, firstShard, secondShard)
	assert.Equal(t, firstVault, secondVault)
	assert.NoDirExists(t, flatDecisionsDir(layout))
	assert.NoDirExists(t, flatVaultDir(layout))
}

// A collision is the case where losing something is possible, so it is the case
// the fold refuses to resolve: the destination wins, the source stays put, and
// the leftover directory is what tells a person there was a conflict at all.
func TestFold_CollisionKeepsBothCopies(t *testing.T) {
	layout := newLayout(t)
	require.NoError(t, state.WriteCommitted(flatDecisionsDir(layout), []state.UnitState{
		unit("u-old", "d-intro", "The flat copy"),
	}))
	shards, err := os.ReadDir(flatDecisionsDir(layout))
	require.NoError(t, err)
	require.Len(t, shards, 1)

	// A shard already at the destination, holding a different decision for the
	// same document — the shape a half-migrated project would arrive in.
	require.NoError(t, state.WriteCommitted(layout.UnitStateDir(), []state.UnitState{
		unit("u-current", "d-intro", "The current copy"),
	}))
	occupied := filepath.Join(layout.UnitStateDir(), shards[0].Name())
	before, err := os.ReadFile(occupied)
	require.NoError(t, err)

	openStore(t, layout)

	kept, err := os.ReadFile(occupied)
	require.NoError(t, err)
	assert.Equal(t, before, kept, "the destination is never overwritten")
	assert.FileExists(t, filepath.Join(flatDecisionsDir(layout), shards[0].Name()),
		"and the source stays, visibly, rather than being deleted unread")
}

// ─── the `context/` umbrella: committed sources one level too deep ───────────

// umbrellaDir is the generation that grouped the committed sources under
// `.kapi/context/`, spelled here as the test's own fixture for the same reason
// the flat names are.
func umbrellaDir(layout project.Layout) string {
	return filepath.Join(layout.StateDir, "context")
}

func writeUnder(t *testing.T, path, content string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
}

func readString(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	require.NoError(t, err)
	return string(b)
}

// Every committed source the umbrella held comes up one level, under the name
// the flattened layout gives it.
func TestFold_LiftsContextUmbrella(t *testing.T) {
	layout := newLayout(t)
	umbrella := umbrellaDir(layout)

	writeUnder(t, filepath.Join(umbrella, "terms.json"), `{"concepts":[]}`)
	writeUnder(t, filepath.Join(umbrella, "memory.json"), `{"entries":["primary"]}`)
	writeUnder(t, filepath.Join(umbrella, "memory", "docs-nb.memory.json"), `{"entries":["docs"]}`)
	writeUnder(t, filepath.Join(umbrella, "README.md"), "notes a project kept\n")
	require.NoError(t, state.WriteCommitted(filepath.Join(umbrella, "decisions"), []state.UnitState{
		unit("u-umbrella", "d-intro", "Committed under the umbrella"),
	}))

	db := openStore(t, layout)

	assert.Equal(t, `{"concepts":[]}`, readString(t, filepath.Join(layout.StateDir, "terms.json")))
	assert.Equal(t, `{"entries":["primary"]}`,
		readString(t, filepath.Join(layout.MemoryDir(), "memory.json")),
		"the single conventional bundle becomes the primary inside memory/")
	assert.Equal(t, `{"entries":["docs"]}`,
		readString(t, filepath.Join(layout.MemoryDir(), "docs-nb.memory.json")))
	assert.Equal(t, "notes a project kept\n", readString(t, filepath.Join(layout.StateDir, "README.md")),
		"a file the fold has no rule for is lifted verbatim, not left behind")

	shards, err := os.ReadDir(layout.UnitStateDir())
	require.NoError(t, err)
	require.Len(t, shards, 1)
	_, ok := db.Work().Get(t.Context(), state.Key{Scope: "d-intro", Unit: "u-umbrella", Variant: model.Variant("nb")})
	assert.True(t, ok, "the lifted record seeded the working set on this same open")

	assert.NoDirExists(t, umbrella, "an emptied umbrella goes")
}

// A voice profile's scope is carried by its directory, not by a prefix on its
// filename, so the fold renames as it moves.
func TestFold_MovesGovernanceIntoProfiles(t *testing.T) {
	layout := newLayout(t)
	umbrella := umbrellaDir(layout)

	writeUnder(t, filepath.Join(umbrella, "brand-voice.yaml"), "name: Default\n")
	writeUnder(t, filepath.Join(umbrella, "bowrain-voice.yaml"), "name: Bowrain\n")
	writeUnder(t, filepath.Join(umbrella, "bowrain-terms.json"), `{"concepts":["bowrain"]}`)
	writeUnder(t, filepath.Join(umbrella, "terms.json"), `{"concepts":["default"]}`)

	openStore(t, layout)

	assert.Equal(t, "name: Default\n", readString(t, filepath.Join(layout.StateDir, "voice.yaml")),
		"the project default drops the prefix the directory now states")
	assert.Equal(t, "name: Bowrain\n",
		readString(t, filepath.Join(layout.ProfileDir("bowrain"), "voice.yaml")))
	assert.Equal(t, `{"concepts":["bowrain"]}`,
		readString(t, filepath.Join(layout.ProfileDir("bowrain"), "terms.json")))
	assert.Equal(t, `{"concepts":["default"]}`,
		readString(t, filepath.Join(layout.StateDir, "terms.json")),
		"an unprefixed terms bundle is the project's own and stays flat")
}

// The same governance rename applies to a project that already flattened its
// umbrella but still names its profiles in its filenames.
func TestFold_MovesGovernanceFromFlatStateDir(t *testing.T) {
	layout := newLayout(t)
	writeUnder(t, filepath.Join(layout.StateDir, "brand-voice.yaml"), "name: Default\n")
	writeUnder(t, filepath.Join(layout.StateDir, "bowrain-voice.yaml"), "name: Bowrain\n")

	openStore(t, layout)

	assert.Equal(t, "name: Default\n", readString(t, filepath.Join(layout.StateDir, "voice.yaml")))
	assert.Equal(t, "name: Bowrain\n",
		readString(t, filepath.Join(layout.ProfileDir("bowrain"), "voice.yaml")))
	assert.NoFileExists(t, filepath.Join(layout.StateDir, "brand-voice.yaml"))
}

// The umbrella fold loses nothing on a collision either: the destination wins
// and the source stays where a person can see it.
func TestFold_ContextUmbrellaCollisionKeepsBothCopies(t *testing.T) {
	layout := newLayout(t)
	umbrella := umbrellaDir(layout)
	writeUnder(t, filepath.Join(umbrella, "terms.json"), `{"concepts":["umbrella"]}`)
	writeUnder(t, filepath.Join(umbrella, "memory.json"), `{"entries":["umbrella"]}`)
	writeUnder(t, filepath.Join(layout.StateDir, "terms.json"), `{"concepts":["flat"]}`)
	writeUnder(t, filepath.Join(layout.MemoryDir(), "memory.json"), `{"entries":["flat"]}`)

	openStore(t, layout)

	assert.Equal(t, `{"concepts":["flat"]}`, readString(t, filepath.Join(layout.StateDir, "terms.json")))
	assert.Equal(t, `{"entries":["flat"]}`, readString(t, filepath.Join(layout.MemoryDir(), "memory.json")))
	assert.Equal(t, `{"concepts":["umbrella"]}`, readString(t, filepath.Join(umbrella, "terms.json")),
		"the source stays, visibly, rather than being deleted unread")
	assert.Equal(t, `{"entries":["umbrella"]}`, readString(t, filepath.Join(umbrella, "memory.json")))
	assert.DirExists(t, umbrella, "and its directory outlives the fold, which is how a person notices")
}

// Idempotent: a second open finds nothing to do and changes nothing.
func TestFold_ContextUmbrellaSecondOpenIsANoOp(t *testing.T) {
	layout := newLayout(t)
	umbrella := umbrellaDir(layout)
	writeUnder(t, filepath.Join(umbrella, "terms.json"), `{"concepts":[]}`)
	writeUnder(t, filepath.Join(umbrella, "bowrain-voice.yaml"), "name: Bowrain\n")

	db := openStore(t, layout)
	require.NoError(t, db.Close())
	first := readString(t, filepath.Join(layout.ProfileDir("bowrain"), "voice.yaml"))

	openStore(t, layout)

	assert.Equal(t, first, readString(t, filepath.Join(layout.ProfileDir("bowrain"), "voice.yaml")))
	assert.Equal(t, `{"concepts":[]}`, readString(t, filepath.Join(layout.StateDir, "terms.json")))
	assert.NoDirExists(t, umbrella)
}

// A project that never had an umbrella is untouched by the fold that lifts one.
func TestFold_NoContextUmbrellaIsANoOp(t *testing.T) {
	layout := newLayout(t)
	writeUnder(t, filepath.Join(layout.StateDir, "terms.json"), `{"concepts":[]}`)

	openStore(t, layout)

	assert.Equal(t, `{"concepts":[]}`, readString(t, filepath.Join(layout.StateDir, "terms.json")))
	assert.NoDirExists(t, umbrellaDir(layout))
}
