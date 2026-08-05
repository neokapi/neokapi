package xml_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	xmlfmt "github.com/neokapi/neokapi/core/formats/xml"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/reconcile"
)

// A block's Name is folded into its context hash (core/reconcile), so it has to
// be a structural address — where the element sits in the tree — and not the
// order the reader happened to reach it in. These tests pin the four properties
// that makes it: an edit elsewhere leaves a name alone, repeats are still
// distinguishable, identical text does not collapse two blocks into one, and a
// second read of the same bytes produces the same names.

// namesByText reads content and maps each block's source text to its Name.
func namesByText(t *testing.T, content string) map[string]string {
	t.Helper()
	ctx := t.Context()
	reader := xmlfmt.NewReader()
	require.NoError(t, reader.Open(ctx, testutil.RawDocFromString(content, model.LocaleEnglish)))
	defer reader.Close()

	out := map[string]string{}
	for _, b := range testutil.CollectBlocks(t, reader.Read(ctx)) {
		out[b.SourceText()] = b.Name
	}
	return out
}

func blockNames(t *testing.T, content string) []string {
	t.Helper()
	ctx := t.Context()
	reader := xmlfmt.NewReader()
	require.NoError(t, reader.Open(ctx, testutil.RawDocFromString(content, model.LocaleEnglish)))
	defer reader.Close()

	var out []string
	for _, b := range testutil.CollectBlocks(t, reader.Read(ctx)) {
		out = append(out, b.Name)
	}
	return out
}

func TestStructuralName_DeletionInAnotherSubtreeDoesNotRename(t *testing.T) {
	const v1 = `<?xml version="1.0"?><root>` +
		`<intro><p>Alpha</p><p>Bravo</p></intro>` +
		`<body><p>Charlie</p><p>Delta</p></body>` +
		`</root>`
	// Bravo is deleted from <intro>. Nothing in <body> moved.
	const v2 = `<?xml version="1.0"?><root>` +
		`<intro><p>Alpha</p></intro>` +
		`<body><p>Charlie</p><p>Delta</p></body>` +
		`</root>`

	before, after := namesByText(t, v1), namesByText(t, v2)

	require.Equal(t, "root.intro.p", before["Alpha"])
	require.Equal(t, "root.intro.p#2", before["Bravo"])
	require.Equal(t, "root.body.p", before["Charlie"])
	require.Equal(t, "root.body.p#2", before["Delta"])

	assert.Equal(t, before["Charlie"], after["Charlie"],
		"a block in an untouched subtree must keep its name — the ordinal is scoped to its own parent")
	assert.Equal(t, before["Delta"], after["Delta"],
		"the second <p> of <body> is still the second <p> of <body>")
	assert.Equal(t, before["Alpha"], after["Alpha"],
		"appending or removing a LATER sibling must not rename an earlier one")
}

func TestStructuralName_IdenticalTextGetsDistinctNames(t *testing.T) {
	const doc = `<?xml version="1.0"?><root>` +
		`<a><p>Same</p><p>Same</p></a>` +
		`<b><p>Same</p></b>` +
		`</root>`

	names := blockNames(t, doc)
	require.Len(t, names, 3)
	assert.Equal(t, []string{"root.a.p", "root.a.p#2", "root.b.p"}, names)

	seen := map[string]bool{}
	for _, n := range names {
		assert.False(t, seen[n], "duplicate block name %q — identity would collapse two blocks into one", n)
		seen[n] = true
	}
}

func TestStructuralName_StableAcrossTwoReads(t *testing.T) {
	const doc = `<?xml version="1.0"?><root>` +
		`<sec><title>One</title><p>Alpha</p><p>Bravo</p></sec>` +
		`<sec><title>Two</title><p>Charlie</p></sec>` +
		`</root>`

	first, second := blockNames(t, doc), blockNames(t, doc)
	assert.Equal(t, first, second,
		"two reads of identical bytes must produce identical names")
	assert.Equal(t,
		[]string{
			"root.sec.title", "root.sec.p", "root.sec.p#2",
			"root.sec#2.title", "root.sec#2.p",
		},
		blockNames(t, doc))
}

// An id attribute is a natural key the author controls, and it beats any path
// we could compute. It stays the name.
func TestStructuralName_IDAttributeStillWins(t *testing.T) {
	const doc = `<?xml version="1.0"?><root><p id="greeting">Hello</p><p id="farewell">Bye</p></root>`
	names := blockNames(t, doc)
	assert.Equal(t, []string{"greeting", "farewell"}, names)
}

// Attribute blocks are addressed by the element path plus the attribute name,
// and pick up the same parent-scoped ordinal.
func TestStructuralName_AttributeBlocksCarryTheOrdinal(t *testing.T) {
	cfg := &xmlfmt.Config{}
	require.NoError(t, cfg.ApplyMap(map[string]any{
		"translatableAttributes": []any{"title"},
	}))
	ctx := t.Context()
	reader := xmlfmt.NewReader()
	require.NoError(t, reader.SetConfig(cfg))
	const doc = `<?xml version="1.0"?><root><img title="First"/><img title="Second"/></root>`
	require.NoError(t, reader.Open(ctx, testutil.RawDocFromString(doc, model.LocaleEnglish)))
	defer reader.Close()

	var names []string
	for _, b := range testutil.CollectBlocks(t, reader.Read(ctx)) {
		names = append(names, b.Name)
	}
	assert.Equal(t, []string{"root.img@title", "root.img#2@title"}, names)
}

// A block must never be named by its own text. reconcile.Identify folds the name
// into the CONTEXT hash while the same text is the CONTENT hash, so a
// self-derived name moves both at once: rewording the block grades it New and it
// loses the translation and approval it carried. See core/model/structural.go.
//
// XML is the format where this is most tempting to get wrong, because an id
// attribute genuinely IS the name — the check is that the ELEMENT TEXT never is.
func TestStructuralName_RewordingATitleKeepsItsIdentity(t *testing.T) {
	const v1 = `<?xml version="1.0"?><doc><title>Getting started</title><p>Body text</p></doc>`
	const v2 = `<?xml version="1.0"?><doc><title>Getting started, revised</title><p>Body text</p></doc>`

	assert.Equal(t, "doc.title", namesByText(t, v1)["Getting started"],
		"a title named after its own words collapses the two hashes reconcile grades apart")

	v1Blocks := xmlBlocks(t, v1)
	v1Res := xmlKeys(v1Blocks, nil)
	prior := xmlUnits(v1Blocks, v1Res)

	v2Res := xmlKeys(xmlBlocks(t, v2), prior)

	assert.Equal(t, v1Res["Getting started"].Key, v2Res["Getting started, revised"].Key,
		"the title was rewritten, not replaced — its history must follow it")
	assert.Equal(t, reconcile.Edited, v2Res["Getting started, revised"].Kind)
	assert.Equal(t, reconcile.Unchanged, v2Res["Body text"].Kind)
}

// An id attribute is markup the author controls, not the block's own words, so
// naming by it is safe: rewording the element still grades as an edit.
func TestStructuralName_RewordingAnIDedElementKeepsItsIdentity(t *testing.T) {
	const v1 = `<?xml version="1.0"?><doc><p id="intro">Getting started</p></doc>`
	const v2 = `<?xml version="1.0"?><doc><p id="intro">Getting started, revised</p></doc>`

	v1Blocks := xmlBlocks(t, v1)
	v1Res := xmlKeys(v1Blocks, nil)
	v2Res := xmlKeys(xmlBlocks(t, v2), xmlUnits(v1Blocks, v1Res))

	assert.Equal(t, v1Res["Getting started"].Key, v2Res["Getting started, revised"].Key)
	assert.Equal(t, reconcile.Edited, v2Res["Getting started, revised"].Kind)
}

const xmlIdentityScope = "doc.xml"

func xmlBlocks(t *testing.T, content string) []*model.Block {
	t.Helper()
	ctx := t.Context()
	reader := xmlfmt.NewReader()
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

func xmlKeys(blocks []*model.Block, prior []reconcile.Unit) map[string]reconcile.Result {
	out := map[string]reconcile.Result{}
	for _, r := range reconcile.Blocks(xmlIdentityScope, blocks, prior) {
		out[r.Block.SourceText()] = r
	}
	return out
}

func xmlUnits(blocks []*model.Block, res map[string]reconcile.Result) []reconcile.Unit {
	var out []reconcile.Unit
	for _, b := range blocks {
		u := reconcile.Identify(xmlIdentityScope, b)
		u.Key = res[b.SourceText()].Key
		out = append(out, u)
	}
	return out
}
