package doclang_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	doclangfmt "github.com/neokapi/neokapi/core/formats/doclang"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
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
	require.Equal(t, "group/text[2]", before["Bravo"])
	require.Equal(t, "group[2]/text", before["Charlie"])
	require.Equal(t, "group[2]/text[2]", before["Delta"])

	assert.Equal(t, before["Charlie"], after["Charlie"],
		"a block in an untouched group must keep its name — the ordinal is scoped to its own container")
	assert.Equal(t, before["Delta"], after["Delta"])
	assert.Equal(t, before["Alpha"], after["Alpha"],
		"removing a LATER sibling must not rename an earlier one")
}

func TestStructuralName_IdenticalTextGetsDistinctNames(t *testing.T) {
	names := doclangNames(t, doclangDoc(
		`<group><text>Same</text><text>Same</text></group><text>Same</text>`))

	assert.Equal(t, []string{"group/text", "group/text[2]", "text"}, names)

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

	assert.Equal(t, doclangNames(t, doc), doclangNames(t, doc),
		"two reads of identical bytes must produce identical names")
	assert.Equal(t,
		[]string{"heading", "group/text", "group/text[2]", "text"},
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
		"table/row/cell", "table/row/cell[2]",
		"table/row[2]/cell", "table/row[2]/cell[2]",
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
