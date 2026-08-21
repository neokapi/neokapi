package host

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/reconcile"
)

// para builds a block the way a prose reader does: named by structure, so the
// name shifts when a sibling above it is deleted.
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

// carryForward turns one resolution into the priors the next one matches
// against — the same thing a venue's tree carries, expressed locally so a test
// can say "and then the project already knew this".
func carryForward(docs []ResolvedDocument) Priors {
	var p Priors
	for _, d := range docs {
		doc := reconcile.DocUnit{Key: d.Scope, Path: d.Path}
		for _, b := range d.Blocks {
			id := reconcile.Identify(d.Scope, b.Block)
			doc.Content = append(doc.Content, id.ContentHash)
			p.Units = append(p.Units, reconcile.Unit{
				Key:         b.Unit,
				Scope:       d.Scope,
				ContentHash: id.ContentHash,
				ContextHash: id.ContextHash,
			})
		}
		p.Documents = append(p.Documents, doc)
	}
	return p
}

// The whole point: a paragraph is deleted, the reader re-addresses every block
// below it, and the units that decisions are recorded against do not move.
func TestResolveIdentity_SurvivesAnUnrelatedDeletion(t *testing.T) {
	v1 := ResolveIdentity(map[string][]*model.Block{
		"docs/intro.md": {para("p", "Alpha"), para("p#2", "Bravo"), para("p#3", "Charlie")},
	}, Priors{})
	before := unitsByText(v1)

	// Bravo is deleted; the reader re-addresses Charlie from p#3 to p#2.
	v2 := ResolveIdentity(map[string][]*model.Block{
		"docs/intro.md": {para("p", "Alpha"), para("p#2", "Charlie")},
	}, carryForward(v1))
	after := unitsByText(v2)

	assert.Equal(t, before["Charlie"].Unit, after["Charlie"].Unit,
		"Charlie's text never changed, so its decisions must follow it")
	assert.Equal(t, reconcile.Moved, after["Charlie"].Kind)
	assert.Equal(t, reconcile.Unchanged, after["Alpha"].Kind)

	assert.NotEqual(t, before["Bravo"].Unit, after["Charlie"].Unit,
		"and Charlie must not inherit the unit of the paragraph that was deleted")
}

// The resolved unit is written onto the block, because that is what every
// downstream key — the declared tree, the chunk, the venue's source id — reads.
func TestResolveIdentity_WritesTheUnitOntoTheBlock(t *testing.T) {
	blocks := []*model.Block{para("p", "Alpha")}
	docs := ResolveIdentity(map[string][]*model.Block{"docs/intro.md": blocks}, Priors{})

	require.Len(t, docs, 1)
	require.Len(t, docs[0].Blocks, 1)
	assert.NotEmpty(t, blocks[0].Unit)
	assert.Equal(t, docs[0].Blocks[0].Unit, blocks[0].Unit)
}

// Renaming a file must not disturb anything inside it.
func TestResolveIdentity_RenameCostsNothing(t *testing.T) {
	blocks := []*model.Block{para("p", "Alpha"), para("p#2", "Bravo")}
	v1 := ResolveIdentity(map[string][]*model.Block{"docs/intro.md": blocks}, Priors{})

	moved := []*model.Block{para("p", "Alpha"), para("p#2", "Bravo")}
	v2 := ResolveIdentity(map[string][]*model.Block{"docs/getting-started.md": moved}, carryForward(v1))

	assert.Equal(t, v1[0].Scope, v2[0].Scope, "the document keeps its identity across a rename")
	before, after := unitsByText(v1), unitsByText(v2)
	for _, text := range []string{"Alpha", "Bravo"} {
		assert.Equal(t, before[text].Unit, after[text].Unit)
		assert.Equal(t, reconcile.Unchanged, after[text].Kind,
			"a rename must not report the blocks inside as changed")
	}
}

// Every prose document has a `p`. Units must not leak between documents.
func TestResolveIdentity_DoesNotConfuseDocuments(t *testing.T) {
	docs := ResolveIdentity(map[string][]*model.Block{
		"docs/intro.md": {para("p", "Welcome")},
		"docs/guide.md": {para("p", "Getting started")},
	}, Priors{})

	units := unitsByText(docs)
	assert.NotEqual(t, units["Welcome"].Unit, units["Getting started"].Unit)
	assert.Len(t, docs, 2)
}

// An edit keeps the unit and reports itself as an edit, which is what lets a
// caller retire an approval made for different words.
func TestResolveIdentity_ReportsAnEditAgainstTheSameUnit(t *testing.T) {
	v1 := ResolveIdentity(map[string][]*model.Block{
		"docs/intro.md": {para("p", "Alpha")},
	}, Priors{})
	unitKey := v1[0].Blocks[0].Unit

	v2 := ResolveIdentity(map[string][]*model.Block{
		"docs/intro.md": {para("p", "Alpha, revised")},
	}, carryForward(v1))

	assert.Equal(t, unitKey, v2[0].Blocks[0].Unit, "the unit survives the edit")
	assert.Equal(t, reconcile.Edited, v2[0].Blocks[0].Kind,
		"and the edit is reported, so an approval of the old words can be retired")
}

// Resolution against a venue's existing keys returns THOSE keys. This is what
// makes the change need no migration: nothing is re-keyed, and only genuinely
// new content mints.
func TestResolveIdentity_KeepsTheKeysThePriorsCarry(t *testing.T) {
	b := para("install/p", "Run the installer.")
	id := reconcile.Identify("d-existing", b)

	docs := ResolveIdentity(map[string][]*model.Block{
		"docs/guide.md": {para("install/p", "Run the installer.")},
	}, Priors{
		Documents: []reconcile.DocUnit{{
			Key: "d-existing", Path: "docs/guide.md", Content: []string{id.ContentHash},
		}},
		Units: []reconcile.Unit{{
			Key: "the-venues-own-key", Scope: "d-existing",
			ContentHash: id.ContentHash, ContextHash: id.ContextHash,
		}},
	})

	require.Len(t, docs, 1)
	assert.Equal(t, "d-existing", docs[0].Scope, "the document keeps the venue's id")
	assert.Equal(t, "the-venues-own-key", docs[0].Blocks[0].Unit,
		"a matched block keeps the key the venue already files it under")
}

// With no priors at all, resolution mints — and two runs over the same tree
// must agree, or identity would depend on map iteration order.
func TestResolveIdentity_IsDeterministic(t *testing.T) {
	in := func() map[string][]*model.Block {
		return map[string][]*model.Block{
			"docs/a.md": {para("p", "Alpha")},
			"docs/b.md": {para("p", "Bravo")},
		}
	}
	first := ResolveIdentity(in(), Priors{})
	second := ResolveIdentity(in(), Priors{})

	assert.Equal(t, unitsByText(first)["Alpha"].Unit, unitsByText(second)["Alpha"].Unit)
	assert.Equal(t, first[0].Scope, second[0].Scope)
}
