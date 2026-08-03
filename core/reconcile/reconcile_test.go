package reconcile_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/reconcile"
)

// blk builds a block whose two hashes are independent: text drives the content
// hash, name drives the context hash.
func blk(name, text string) *model.Block {
	b := model.NewBlock("tu-"+name, text)
	b.Name = name
	return b
}

func priorOf(key string, b *model.Block) reconcile.Unit {
	id := model.ComputeIdentity(b)
	return reconcile.Unit{Key: key, ContentHash: id.ContentHash, ContextHash: id.ContextHash}
}

// byText indexes results so a test can ask about one block by its words.
func byText(rs []reconcile.Result) map[string]reconcile.Result {
	out := map[string]reconcile.Result{}
	for _, r := range rs {
		out[r.Block.SourceText()] = r
	}
	return out
}

// The four gradings, each in isolation.
func TestBlocks_Grades(t *testing.T) {
	original := blk("para3", "Charlie")
	prior := []reconcile.Unit{priorOf("u-charlie", original)}

	t.Run("nothing changed", func(t *testing.T) {
		got := byText(reconcile.Blocks([]*model.Block{blk("para3", "Charlie")}, prior))
		assert.Equal(t, reconcile.Unchanged, got["Charlie"].Kind)
		assert.Equal(t, "u-charlie", got["Charlie"].Key)
	})

	// The case a content-only key cannot see. Someone fixed a typo: same block,
	// new words. Its history has to follow it, and its translation is now stale.
	t.Run("edited in place", func(t *testing.T) {
		got := byText(reconcile.Blocks([]*model.Block{blk("para3", "Charlie, revised")}, prior))
		assert.Equal(t, reconcile.Edited, got["Charlie, revised"].Kind)
		assert.Equal(t, "u-charlie", got["Charlie, revised"].Key,
			"an edit must keep the unit's identity, not read as a delete plus an add")
	})

	// The case a position-only key cannot see. An earlier paragraph was deleted,
	// so this block's positional name shifted, but its words are untouched.
	t.Run("moved", func(t *testing.T) {
		got := byText(reconcile.Blocks([]*model.Block{blk("para2", "Charlie")}, prior))
		assert.Equal(t, reconcile.Moved, got["Charlie"].Kind)
		assert.Equal(t, "u-charlie", got["Charlie"].Key,
			"an unrelated deletion must not mint a new unit")
	})

	t.Run("new", func(t *testing.T) {
		got := byText(reconcile.Blocks([]*model.Block{blk("para9", "Echo")}, prior))
		assert.Equal(t, reconcile.New, got["Echo"].Kind)
		assert.NotEmpty(t, got["Echo"].Key)
	})
}

// Two blocks with identical source text are distinct units. Keying on content
// alone merges them, and then approving one silently approves the other — the
// reason a content-derived unit key was tried here and abandoned.
func TestBlocks_IdenticalTextStaysTwoUnits(t *testing.T) {
	first, second := blk("cell1", "Yes"), blk("cell2", "Yes")
	prior := []reconcile.Unit{priorOf("u-first", first), priorOf("u-second", second)}

	got := reconcile.Blocks([]*model.Block{blk("cell1", "Yes"), blk("cell2", "Yes")}, prior)
	require.Len(t, got, 2)

	assert.Equal(t, "u-first", got[0].Key)
	assert.Equal(t, "u-second", got[1].Key, "same words, different place, different unit")
	assert.Equal(t, reconcile.Unchanged, got[0].Kind)
	assert.Equal(t, reconcile.Unchanged, got[1].Kind)
}

// No prior may be claimed twice, or two blocks would share one decision record.
func TestBlocks_APriorIsClaimedOnce(t *testing.T) {
	prior := []reconcile.Unit{priorOf("u-only", blk("para1", "Yes"))}

	got := reconcile.Blocks([]*model.Block{blk("para1", "Yes"), blk("para2", "Yes")}, prior)
	require.Len(t, got, 2)

	assert.Equal(t, "u-only", got[0].Key, "the exact match wins the prior")
	assert.NotEqual(t, "u-only", got[1].Key, "the second block cannot reuse it")
	assert.Equal(t, reconcile.New, got[1].Kind)
}

// A block removed in one revision and restored in a later one comes back to its
// own history, so already-translated text is not re-translated.
func TestBlocks_SurvivesRemovalAndReturn(t *testing.T) {
	v1 := []*model.Block{blk("para1", "Alpha"), blk("para2", "Bravo")}
	prior := []reconcile.Unit{priorOf("u-alpha", v1[0]), priorOf("u-bravo", v1[1])}

	// v2 drops Bravo. Prior units are retained — that is what makes the return
	// recoverable, and why removal must not purge them.
	v2 := reconcile.Blocks([]*model.Block{blk("para1", "Alpha")}, prior)
	require.Len(t, v2, 1)

	// v3 restores Bravo further down the file, after new text.
	v3 := byText(reconcile.Blocks(
		[]*model.Block{blk("para1", "Alpha"), blk("para2", "Delta"), blk("para3", "Bravo")}, prior))

	assert.Equal(t, "u-bravo", v3["Bravo"].Key, "it returns to its own history")
	assert.Equal(t, reconcile.Moved, v3["Bravo"].Kind)
	assert.Equal(t, reconcile.New, v3["Delta"].Kind)
}

// Minting is deterministic so a fresh project reconciles to the same keys twice.
func TestBlocks_MintsDeterministically(t *testing.T) {
	in := []*model.Block{blk("para1", "Alpha"), blk("para2", "Alpha")}

	first := reconcile.Blocks(in, nil)
	second := reconcile.Blocks(in, nil)

	require.Len(t, first, 2)
	assert.Equal(t, first[0].Key, second[0].Key)
	assert.Equal(t, first[1].Key, second[1].Key)
	assert.NotEqual(t, first[0].Key, first[1].Key, "same words, different place, different key")
}

// Both hashes changing at once is the irreducible case: nothing links the new
// block to the old one, so it is honestly new rather than a bad guess.
func TestBlocks_EditedAndMovedIsANewUnit(t *testing.T) {
	prior := []reconcile.Unit{priorOf("u-charlie", blk("para3", "Charlie"))}

	got := byText(reconcile.Blocks([]*model.Block{blk("para2", "Charlie, revised")}, prior))
	assert.Equal(t, reconcile.New, got["Charlie, revised"].Kind)
}
