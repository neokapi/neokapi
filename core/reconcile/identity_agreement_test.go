package reconcile_test

import (
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/reconcile"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A block has ONE pair of hashes, wherever it is looked at.
//
// It used to have two. `reconcile.Identify` folded the document scope into the
// context hash through a namespaced property; `model.ComputeIdentity` — what
// the sync wire sends and a venue stores — did not. So the same block carried
// one context hash for reconciliation and a different one for transfer, and a
// venue's stored hashes could not be used as a prior set at all. The difference
// was invisible because nothing ever compared them.
//
// The scope still qualifies context matching. It does so in the pool's lookup
// keys, which is the only place it ever meant anything.
func TestIdentifyAgreesWithTheHashesABlockCarries(t *testing.T) {
	b := &model.Block{
		ID:           "tu3",
		Name:         "install/p#2",
		Type:         "paragraph",
		Translatable: true,
		Properties:   map[string]string{"file": "docs/guide.md"},
	}
	b.SetSourceText("Run the installer.")

	carried := model.ComputeIdentity(b)

	for _, scope := range []string{"", "d-abc123", "some/other/document"} {
		got := reconcile.Identify(scope, b)
		assert.Equal(t, carried.ContentHash, got.ContentHash,
			"the content hash is the same value everywhere, scope %q", scope)
		assert.Equal(t, carried.ContextHash, got.ContextHash,
			"the context hash is the same value everywhere, scope %q", scope)
		assert.Equal(t, scope, got.Scope, "and the scope rides beside them")
	}
}

// The scope still separates documents — it just does it in the lookup rather
// than in the hash. Two identical paragraphs in two files stay two units.
func TestScopeStillSeparatesIdenticalBlocksInDifferentDocuments(t *testing.T) {
	mk := func(name, text string) *model.Block {
		b := &model.Block{ID: name, Name: name, Type: "paragraph", Translatable: true}
		b.SetSourceText(text)
		return b
	}

	// One document's paragraph is known; an identical paragraph in another
	// document is read for the first time.
	known := mk("p", "Shared boilerplate.")
	prior := reconcile.Identify("doc-a", known)
	prior.Key = "unit-in-a"

	inB := reconcile.Blocks("doc-b", []*model.Block{mk("p", "Shared boilerplate.")},
		[]reconcile.Unit{prior})
	require.Len(t, inB, 1)

	// It matches on CONTENT — the same words really are the same words, and
	// that is the pass that keeps a translation when text moves between files.
	assert.Equal(t, "unit-in-a", inB[0].Key)
	assert.Equal(t, reconcile.Moved, inB[0].Kind,
		"identical words in another document are the same content, moved — not a coincidence to be minted twice")

	// But with the prior already claimed elsewhere, the scope is what stops the
	// context pass from handing one document's history to another's.
	claimed := reconcile.Blocks("doc-b",
		[]*model.Block{mk("p", "Different words entirely.")},
		[]reconcile.Unit{prior})
	require.Len(t, claimed, 1)
	assert.Equal(t, reconcile.New, claimed[0].Kind,
		"a block named 'p' in another document must not claim this one's history on the name alone")
	assert.NotEqual(t, "unit-in-a", claimed[0].Key)
}

// Two identical blocks in two documents, both new, mint two keys.
func TestMintSeparatesIdenticalBlocksInDifferentDocuments(t *testing.T) {
	mk := func() *model.Block {
		b := &model.Block{ID: "p", Name: "p", Type: "paragraph", Translatable: true}
		b.SetSourceText("Exactly the same.")
		return b
	}
	inA := reconcile.Blocks("doc-a", []*model.Block{mk()}, nil)
	inB := reconcile.Blocks("doc-b", []*model.Block{mk()}, nil)
	require.Len(t, inA, 1)
	require.Len(t, inB, 1)
	assert.NotEqual(t, inA[0].Key, inB[0].Key,
		"a minted key carries its document, or one file's paragraph would be the other's")
}
