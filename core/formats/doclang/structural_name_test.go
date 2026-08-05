package doclang_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	doclangfmt "github.com/neokapi/neokapi/core/formats/doclang"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/reconcile"
)

// A DocLang block used to carry no Name at all, which left convergence.BlockKey
// falling back to the reader's `bN` counter — pure reading order. These tests
// pin the structural address that replaced it: a block's Name is folded into its
// context hash (core/reconcile), so an edit in one container must not move
// anything in another.

func doclangNames(t *testing.T, doc string) []string {
	t.Helper()
	ctx := t.Context()
	r := doclangfmt.NewReader()
	require.NoError(t, r.Open(ctx, testutil.RawDocFromString(doc, model.LocaleEnglish)))
	defer r.Close()

	var out []string
	for _, b := range testutil.CollectBlocks(t, r.Read(ctx)) {
		out = append(out, b.Name)
	}
	return out
}

func doclangNamesByText(t *testing.T, doc string) map[string]string {
	t.Helper()
	ctx := t.Context()
	r := doclangfmt.NewReader()
	require.NoError(t, r.Open(ctx, testutil.RawDocFromString(doc, model.LocaleEnglish)))
	defer r.Close()

	out := map[string]string{}
	for _, b := range testutil.CollectBlocks(t, r.Read(ctx)) {
		out[b.SourceText()] = b.Name
	}
	return out
}

// doclangDoc wraps body markup in a minimal DocLang document.
func doclangDoc(body string) string {
	return `<?xml version="1.0" encoding="UTF-8"?><doclang>` + body + `</doclang>`
}

func TestStructuralName_DeletionInAnotherGroupDoesNotRename(t *testing.T) {
	v1 := doclangDoc(
		`<group><text>Alpha</text><text>Bravo</text></group>` +
			`<group><text>Charlie</text><text>Delta</text></group>`)
	// Bravo goes from the first group. Nothing in the second moved.
	v2 := doclangDoc(
		`<group><text>Alpha</text></group>` +
			`<group><text>Charlie</text><text>Delta</text></group>`)

	before, after := doclangNamesByText(t, v1), doclangNamesByText(t, v2)

	require.Equal(t, "group/text", before["Alpha"])
	require.Equal(t, "group/text#2", before["Bravo"])
	require.Equal(t, "group#2/text", before["Charlie"])
	require.Equal(t, "group#2/text#2", before["Delta"])

	assert.Equal(t, before["Charlie"], after["Charlie"],
		"a block in an untouched group must keep its name — the ordinal is scoped to its own container")
	assert.Equal(t, before["Delta"], after["Delta"])
	assert.Equal(t, before["Alpha"], after["Alpha"],
		"removing a LATER sibling must not rename an earlier one")
}

func TestStructuralName_IdenticalTextGetsDistinctNames(t *testing.T) {
	names := doclangNames(t, doclangDoc(
		`<group><text>Same</text><text>Same</text></group><text>Same</text>`))

	assert.Equal(t, []string{"group/text", "group/text#2", "text"}, names)

	seen := map[string]bool{}
	for _, n := range names {
		assert.False(t, seen[n], "duplicate block name %q — three identical paragraphs would share one identity", n)
		seen[n] = true
	}
}

func TestStructuralName_StableAcrossTwoReads(t *testing.T) {
	doc := doclangDoc(
		`<heading level="1">Title</heading>` +
			`<group><text>Alpha</text><text>Bravo</text></group>` +
			`<text>Tail</text>`)

	first, second := doclangNames(t, doc), doclangNames(t, doc)
	assert.Equal(t, first, second,
		"two reads of identical bytes must produce identical names")
	assert.Equal(t,
		[]string{"heading", "group/text", "group/text#2", "text"},
		doclangNames(t, doc))
}

// OTSL already gives a cell a grid address, so that is its name. Adding a row
// below leaves every existing cell where it was.
func TestStructuralName_TableCellsUseTheGridAddress(t *testing.T) {
	doc := doclangDoc(`<table>` +
		`<ched/>Region<ched/>Total<nl/>` +
		`<fcel/>North<fcel/>120<nl/>` +
		`</table>`)

	names := doclangNames(t, doc)
	assert.Equal(t, []string{
		"table/row/cell", "table/row/cell#2",
		"table/row#2/cell", "table/row#2/cell#2",
	}, names)

	extended := doclangDoc(`<table>` +
		`<ched/>Region<ched/>Total<nl/>` +
		`<fcel/>North<fcel/>120<nl/>` +
		`<fcel/>South<fcel/>80<nl/>` +
		`</table>`)
	before, after := doclangNamesByText(t, doc), doclangNamesByText(t, extended)
	assert.Equal(t, before["North"], after["North"],
		"appending a row must not move the cells above it")
	assert.Equal(t, before["Total"], after["Total"])
}

// A block must never be named by its own text. reconcile.Identify folds the name
// into the CONTEXT hash while the same text is the CONTENT hash, so a
// self-derived name moves both at once: rewording the block grades it New and it
// loses the translation and approval it carried. A DocLang <heading> is where
// this is tempting, and exactly where it is wrong. See core/model/structural.go.
func TestStructuralName_RewordingAHeadingKeepsItsIdentity(t *testing.T) {
	v1 := doclangDoc(`<heading level="1">Getting started</heading>` + `<text>Body text</text>`)
	v2 := doclangDoc(`<heading level="1">Getting started, revised</heading>` + `<text>Body text</text>`)

	assert.Equal(t, "heading", doclangNamesByText(t, v1)["Getting started"],
		"a heading named after its own title collapses the two hashes reconcile grades apart")

	v1Blocks := doclangBlocks(t, v1)
	v1Res := doclangKeys(v1Blocks, nil)
	v2Res := doclangKeys(doclangBlocks(t, v2), doclangUnits(v1Blocks, v1Res))

	assert.Equal(t, v1Res["Getting started"].Key, v2Res["Getting started, revised"].Key,
		"the heading was rewritten, not replaced — its history must follow it")
	assert.Equal(t, reconcile.Edited, v2Res["Getting started, revised"].Kind)
	assert.Equal(t, reconcile.Unchanged, v2Res["Body text"].Kind,
		"and the paragraph beneath it did not move")
}

const doclangIdentityScope = "doc.dclg.xml"

func doclangBlocks(t *testing.T, doc string) []*model.Block {
	t.Helper()
	ctx := t.Context()
	r := doclangfmt.NewReader()
	require.NoError(t, r.Open(ctx, testutil.RawDocFromString(doc, model.LocaleEnglish)))
	defer r.Close()

	var out []*model.Block
	for _, b := range testutil.CollectBlocks(t, r.Read(ctx)) {
		if b.SourceText() != "" {
			out = append(out, b)
		}
	}
	return out
}

func doclangKeys(blocks []*model.Block, prior []reconcile.Unit) map[string]reconcile.Result {
	out := map[string]reconcile.Result{}
	for _, r := range reconcile.Blocks(doclangIdentityScope, blocks, prior) {
		out[r.Block.SourceText()] = r
	}
	return out
}

func doclangUnits(blocks []*model.Block, res map[string]reconcile.Result) []reconcile.Unit {
	var out []reconcile.Unit
	for _, b := range blocks {
		u := reconcile.Identify(doclangIdentityScope, b)
		u.Key = res[b.SourceText()].Key
		out = append(out, u)
	}
	return out
}
