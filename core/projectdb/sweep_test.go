package projectdb_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/state"
)

// The flat layout's decision record, spelled here as the test's own fixture
// rather than borrowed from core/projectdb, so the test asserts against the
// disk.
func flatDecisionsDir(layout project.Layout) string {
	return filepath.Join(layout.StateDir, "units")
}

// inTreeState lists what earlier layouts kept inside a checkout as state,
// relative to its `.kapi/`.
var inTreeState = []string{
	"store.db",
	"cache/blocks.db",
	"cache/redaction/b-1.json",
	"memory.db",
	"work/state.db",
	"work/store.db",
	"work/vault/redaction.json",
	"work/cache/sync-cache.json",
	"vault/redaction.json",
	"sync/sync-cache.json",
	"filters.local.json",
}

// A store open reads, moves and deletes none of the state an earlier kapi left
// inside the checkout: the checkout's state lives under the data root, and a
// person imports what cannot be rebuilt with `kapi state import`.
func TestOpenLeavesStateInsideTheCheckoutAlone(t *testing.T) {
	layout := newLayout(t)
	for _, rel := range inTreeState {
		p := filepath.Join(layout.StateDir, filepath.FromSlash(rel))
		require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
		require.NoError(t, os.WriteFile(p, []byte("left by an earlier kapi"), 0o600))
	}
	db := openStore(t, layout)

	for _, rel := range inTreeState {
		assert.FileExists(t, filepath.Join(layout.StateDir, filepath.FromSlash(rel)), "%s is left as it was", rel)
	}
	assert.NoFileExists(t, layout.RedactionVaultPath(), "no vault is taken from the checkout")
	assert.NoFileExists(t, filepath.Join(layout.SyncDir(), "sync-cache.json"), "no sync state is taken from it")
	assert.NoFileExists(t, layout.LocalFiltersPath())
	assert.NotEqual(t, filepath.Join(layout.StateDir, "work", "store.db"), db.Path(), "the store is the one under the data root")
}

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

	after, err := os.ReadFile(filepath.Join(layout.Export().UnitStateDir(), shards[0].Name()))
	require.NoError(t, err, "the shard landed under .kapi/state/")
	assert.Equal(t, before, after, "the committed record moves byte-identical")
	assert.NoDirExists(t, flatDecisionsDir(layout), "an emptied source directory goes")

	// And it is a record, not just a file: an import reads it from where the
	// fold put it, which is why the fold has to precede the open.
	require.NoError(t, db.Work().Import(t.Context()))
	_, ok := db.Work().Get(t.Context(), state.Key{Scope: "d-intro", Unit: "u-flat", Variant: model.Variant("nb")})
	assert.True(t, ok, "an import reads the moved record")
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
	require.NoError(t, state.WriteCommitted(layout.Export().UnitStateDir(), []state.UnitState{
		unit("u-current", "d-intro", "The current copy"),
	}))
	occupied := filepath.Join(layout.Export().UnitStateDir(), shards[0].Name())
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
		readString(t, filepath.Join(layout.Export().MemoryDir(), "memory.json")),
		"the single conventional bundle becomes the primary inside memory/")
	assert.Equal(t, `{"entries":["docs"]}`,
		readString(t, filepath.Join(layout.Export().MemoryDir(), "docs-nb.memory.json")))
	assert.Equal(t, "notes a project kept\n", readString(t, filepath.Join(layout.StateDir, "README.md")),
		"a file the fold has no rule for is lifted verbatim, not left behind")

	shards, err := os.ReadDir(layout.Export().UnitStateDir())
	require.NoError(t, err)
	require.Len(t, shards, 1)
	require.NoError(t, db.Work().Import(t.Context()))
	_, ok := db.Work().Get(t.Context(), state.Key{Scope: "d-intro", Unit: "u-umbrella", Variant: model.Variant("nb")})
	assert.True(t, ok, "an import reads the lifted record")

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
		readString(t, filepath.Join(layout.Export().ProfileDir("bowrain"), "voice.yaml")))
	assert.Equal(t, `{"concepts":["bowrain"]}`,
		readString(t, filepath.Join(layout.Export().ProfileDir("bowrain"), "terms.json")))
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
		readString(t, filepath.Join(layout.Export().ProfileDir("bowrain"), "voice.yaml")))
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
	writeUnder(t, filepath.Join(layout.Export().MemoryDir(), "memory.json"), `{"entries":["flat"]}`)

	openStore(t, layout)

	assert.Equal(t, `{"concepts":["flat"]}`, readString(t, filepath.Join(layout.StateDir, "terms.json")))
	assert.Equal(t, `{"entries":["flat"]}`, readString(t, filepath.Join(layout.Export().MemoryDir(), "memory.json")))
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
	first := readString(t, filepath.Join(layout.Export().ProfileDir("bowrain"), "voice.yaml"))

	openStore(t, layout)

	assert.Equal(t, first, readString(t, filepath.Join(layout.Export().ProfileDir("bowrain"), "voice.yaml")))
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
