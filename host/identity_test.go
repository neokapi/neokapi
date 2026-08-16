package host

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/reconcile"
	"github.com/neokapi/neokapi/core/state"
)

func openStore(t *testing.T) *state.WorkStore {
	t.Helper()
	dir := t.TempDir()
	st, err := state.OpenWork(t.Context(), filepath.Join(dir, "work", "state.db"), filepath.Join(dir, "units"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = st.Close() })
	return st
}

// para builds a block the way a prose reader does: named by position.
func para(n string, text string) *model.Block {
	b := model.NewBlock("tu-"+n, text)
	b.Name = n
	return b
}

func unitsByText(docs []ResolvedDocument) map[string]ResolvedBlock {
	out := map[string]ResolvedBlock{}
	for _, d := range docs {
		for _, b := range d.Blocks {
			out[b.Block.SourceText()] = b
		}
	}
	return out
}

// The whole point, through the layer the ingest paths call: a paragraph is
// deleted, every block below it is renamed by the reader, and the units that
// decisions are recorded against do not move.
func TestResolveIdentity_SurvivesAnUnrelatedDeletion(t *testing.T) {
	st := openStore(t)

	v1, err := ResolveIdentity(t.Context(), st, map[string][]*model.Block{
		"docs/intro.md": {para("para1", "Alpha"), para("para2", "Bravo"), para("para3", "Charlie")},
	})
	require.NoError(t, err)
	before := unitsByText(v1)

	// Bravo is deleted; the reader renames Charlie from para3 to para2.
	v2, err := ResolveIdentity(t.Context(), st, map[string][]*model.Block{
		"docs/intro.md": {para("para1", "Alpha"), para("para2", "Charlie")},
	})
	require.NoError(t, err)
	after := unitsByText(v2)

	assert.Equal(t, before["Charlie"].Unit, after["Charlie"].Unit,
		"Charlie's text never changed, so its decisions must follow it")
	assert.Equal(t, reconcile.Moved, after["Charlie"].Kind)
	assert.Equal(t, reconcile.Unchanged, after["Alpha"].Kind)
}

// Renaming a file must not disturb anything inside it.
func TestResolveIdentity_RenameCostsNothing(t *testing.T) {
	st := openStore(t)
	blocks := []*model.Block{para("para1", "Alpha"), para("para2", "Bravo")}

	v1, err := ResolveIdentity(t.Context(), st, map[string][]*model.Block{"docs/intro.md": blocks})
	require.NoError(t, err)

	v2, err := ResolveIdentity(t.Context(), st, map[string][]*model.Block{"docs/getting-started.md": blocks})
	require.NoError(t, err)

	assert.Equal(t, v1[0].Scope, v2[0].Scope, "the document keeps its identity across a rename")
	before, after := unitsByText(v1), unitsByText(v2)
	for _, text := range []string{"Alpha", "Bravo"} {
		assert.Equal(t, before[text].Unit, after[text].Unit)
		assert.Equal(t, reconcile.Unchanged, after[text].Kind,
			"a rename must not report the blocks inside as changed")
	}
}

// Every markdown file has a para1. Units must not leak between documents.
func TestResolveIdentity_DoesNotConfuseDocuments(t *testing.T) {
	st := openStore(t)

	docs, err := ResolveIdentity(t.Context(), st, map[string][]*model.Block{
		"docs/intro.md": {para("para1", "Welcome")},
		"docs/guide.md": {para("para1", "Getting started")},
	})
	require.NoError(t, err)

	units := unitsByText(docs)
	assert.NotEqual(t, units["Welcome"].Unit, units["Getting started"].Unit)
	assert.Len(t, docs, 2)
}

// Editing a block keeps the unit and drops the approval: the translation that
// was blessed was blessed for different words.
func TestResolveIdentity_EditClearsTheDecision(t *testing.T) {
	st := openStore(t)

	v1, err := ResolveIdentity(t.Context(), st, map[string][]*model.Block{
		"docs/intro.md": {para("para1", "Alpha")},
	})
	require.NoError(t, err)
	unitKey := v1[0].Blocks[0].Unit

	// Approve it. The document is half of the record's identity, so the key
	// names it alongside the unit.
	key := state.Key{Scope: v1[0].Scope, Unit: unitKey, Variant: model.VariantKey{}}
	u, _ := st.Get(t.Context(), key)
	u.Decision = state.Decision{ReviewState: "approved", By: "someone"}
	u.TargetHash = "hash-of-the-approved-translation"
	require.NoError(t, st.Put(t.Context(), u))

	// Reword the source.
	v2, err := ResolveIdentity(t.Context(), st, map[string][]*model.Block{
		"docs/intro.md": {para("para1", "Alpha, revised")},
	})
	require.NoError(t, err)

	assert.Equal(t, unitKey, v2[0].Blocks[0].Unit, "the unit survives the edit")
	assert.Equal(t, reconcile.Edited, v2[0].Blocks[0].Kind)

	got, ok := st.Get(t.Context(), key)
	require.True(t, ok)
	assert.Empty(t, got.Decision.ReviewState, "an approval of the old words must not survive")
	assert.Empty(t, got.TargetHash)
}

// Identity is staged like any other change, so a run writes the record once.
func TestResolveIdentity_StagesRatherThanCommits(t *testing.T) {
	st := openStore(t)

	_, err := ResolveIdentity(t.Context(), st, map[string][]*model.Block{
		"docs/intro.md": {para("para1", "Alpha")},
	})
	require.NoError(t, err)

	n, err := st.Pending(t.Context())
	require.NoError(t, err)
	assert.Positive(t, n, "resolution stages; the caller commits once at the end of the run")
}

// Two runs over an unchanged tree must agree, or identity would depend on map
// iteration order.
func TestResolveIdentity_IsDeterministic(t *testing.T) {
	in := map[string][]*model.Block{
		"docs/a.md": {para("para1", "Alpha")},
		"docs/b.md": {para("para1", "Bravo")},
	}

	first, err := ResolveIdentity(t.Context(), openStore(t), in)
	require.NoError(t, err)
	second, err := ResolveIdentity(t.Context(), openStore(t), in)
	require.NoError(t, err)

	assert.Equal(t, unitsByText(first)["Alpha"].Unit, unitsByText(second)["Alpha"].Unit)
	assert.Equal(t, first[0].Scope, second[0].Scope)
}
