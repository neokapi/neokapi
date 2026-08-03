package yaml_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	yamlfmt "github.com/neokapi/neokapi/core/formats/yaml"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/reconcile"
)

// YAML names blocks by key path, and a sequence item by its index WITHIN its own
// sequence. That was structural from the start — these tests pin it, because a
// block's Name is folded into its context hash (core/reconcile) and the rest of
// the identity work now depends on this format staying where it is.

func yamlNamesByText(t *testing.T, content string) map[string]string {
	t.Helper()
	ctx := t.Context()
	reader := yamlfmt.NewReader()
	require.NoError(t, reader.Open(ctx, testutil.RawDocFromString(content, model.LocaleEnglish)))
	defer reader.Close()

	out := map[string]string{}
	for _, b := range testutil.CollectBlocks(t, reader.Read(ctx)) {
		out[b.SourceText()] = b.Name
	}
	return out
}

func yamlBlockNames(t *testing.T, content string) []string {
	t.Helper()
	ctx := t.Context()
	reader := yamlfmt.NewReader()
	require.NoError(t, reader.Open(ctx, testutil.RawDocFromString(content, model.LocaleEnglish)))
	defer reader.Close()

	var out []string
	for _, b := range testutil.CollectBlocks(t, reader.Read(ctx)) {
		out = append(out, b.Name)
	}
	return out
}

// The stability claim pinned in core/formats/identity_conformance_test.go, kept
// here too so a change to the YAML reader fails in the YAML package first.
func TestStructuralName_UnrelatedKeyDeletionDoesNotRename(t *testing.T) {
	before := yamlNamesByText(t, "a: Alpha\nb: Bravo\nc: Charlie\n")
	after := yamlNamesByText(t, "a: Alpha\nc: Charlie\n")

	require.Equal(t, "a", before["Alpha"])
	require.Equal(t, "c", before["Charlie"])
	assert.Equal(t, before["Charlie"], after["Charlie"],
		"deleting an unrelated key must not rename the block after it")
	assert.Equal(t, before["Alpha"], after["Alpha"])
}

// A sequence item is indexed within its own sequence, so deleting an item from
// one list leaves every item of every other list alone.
func TestStructuralName_DeletionInAnotherSequenceDoesNotRename(t *testing.T) {
	const v1 = "intro:\n  - Alpha\n  - Bravo\nbody:\n  - Charlie\n  - Delta\n"
	const v2 = "intro:\n  - Alpha\nbody:\n  - Charlie\n  - Delta\n"

	before, after := yamlNamesByText(t, v1), yamlNamesByText(t, v2)

	require.Equal(t, "intro.[0]", before["Alpha"])
	require.Equal(t, "intro.[1]", before["Bravo"])
	require.Equal(t, "body.[0]", before["Charlie"])
	require.Equal(t, "body.[1]", before["Delta"])

	assert.Equal(t, before["Charlie"], after["Charlie"],
		"an item of an untouched sequence must keep its name")
	assert.Equal(t, before["Delta"], after["Delta"])
	assert.Equal(t, before["Alpha"], after["Alpha"])
}

func TestStructuralName_IdenticalTextGetsDistinctNames(t *testing.T) {
	names := yamlBlockNames(t, "a: Same\nb: Same\nlist:\n  - Same\n  - Same\n")
	require.Len(t, names, 4)
	assert.Equal(t, []string{"a", "b", "list.[0]", "list.[1]"}, names)

	seen := map[string]bool{}
	for _, n := range names {
		assert.False(t, seen[n], "duplicate block name %q", n)
		seen[n] = true
	}
}

func TestStructuralName_StableAcrossTwoReads(t *testing.T) {
	const doc = "app:\n  title: Hello\n  items:\n    - One\n    - Two\nfooter: Bye\n"
	first, second := yamlBlockNames(t, doc), yamlBlockNames(t, doc)
	assert.Equal(t, first, second,
		"two reads of identical bytes must produce identical names")
	assert.Equal(t,
		[]string{"app.title", "app.items.[0]", "app.items.[1]", "footer"},
		yamlBlockNames(t, doc))
}

// A block must never be named by its own text. reconcile.Identify folds the name
// into the CONTEXT hash while the same text is the CONTENT hash, so a
// self-derived name moves both at once: rewording the block grades it New and it
// loses the translation and approval it carried. YAML is safe by construction —
// the name is the KEY and the content is the VALUE — and this pins it, because
// "use the string as its own name" is a tempting shortcut for a keyed format.
func TestStructuralName_RewordingAValueKeepsItsIdentity(t *testing.T) {
	const v1 = "title: Getting started\nbody: Body text\n"
	const v2 = "title: Getting started, revised\nbody: Body text\n"

	assert.Equal(t, "title", yamlNamesByText(t, v1)["Getting started"],
		"the key is the name; the value must never be")

	v1Blocks := yamlBlocks(t, v1)
	v1Res := yamlKeys(v1Blocks, nil)
	v2Res := yamlKeys(yamlBlocks(t, v2), yamlUnits(v1Blocks, v1Res))

	assert.Equal(t, v1Res["Getting started"].Key, v2Res["Getting started, revised"].Key,
		"the value was rewritten, not replaced — its history must follow it")
	assert.Equal(t, reconcile.Edited, v2Res["Getting started, revised"].Kind)
	assert.Equal(t, reconcile.Unchanged, v2Res["Body text"].Kind)
}

const yamlIdentityScope = "messages.yaml"

func yamlBlocks(t *testing.T, content string) []*model.Block {
	t.Helper()
	ctx := t.Context()
	reader := yamlfmt.NewReader()
	require.NoError(t, reader.Open(ctx, testutil.RawDocFromString(content, model.LocaleEnglish)))
	defer reader.Close()

	var out []*model.Block
	for _, b := range testutil.CollectBlocks(t, reader.Read(ctx)) {
		if b.SourceText() != "" {
			out = append(out, b)
		}
	}
	return out
}

func yamlKeys(blocks []*model.Block, prior []reconcile.Unit) map[string]reconcile.Result {
	out := map[string]reconcile.Result{}
	for _, r := range reconcile.Blocks(yamlIdentityScope, blocks, prior) {
		out[r.Block.SourceText()] = r
	}
	return out
}

func yamlUnits(blocks []*model.Block, res map[string]reconcile.Result) []reconcile.Unit {
	var out []reconcile.Unit
	for _, b := range blocks {
		u := reconcile.Identify(yamlIdentityScope, b)
		u.Key = res[b.SourceText()].Key
		out = append(out, u)
	}
	return out
}
