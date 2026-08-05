package openxml

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/reconcile"
)

// Blocks read out of an OOXML package used to carry no Name at all, which left
// convergence.BlockKey falling back to the `tuN` reader counter — reading order
// exactly. These tests pin the structural address that replaced it. A block's
// Name is folded into its context hash (core/reconcile), so an edit in one part,
// one table cell or one shape must not move anything in another.

// docxNames reads a DOCX built around documentXML and returns the block names in
// document order.
func docxNames(t *testing.T, documentXML string) []string {
	t.Helper()
	var out []string
	for _, b := range readDocxBlocks(t, documentXML) {
		out = append(out, b.Name)
	}
	return out
}

// docxNamesByText maps each block's source text to its name.
func docxNamesByText(t *testing.T, documentXML string) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, b := range readDocxBlocks(t, documentXML) {
		out[b.SourceText()] = b.Name
	}
	return out
}

func readDocxBlocks(t *testing.T, documentXML string) []*model.Block {
	t.Helper()
	input := docxWithDocumentXML(t, documentXML)

	reader := NewReader()
	doc := &model.RawDocument{
		URI:          "test.docx",
		SourceLocale: model.LocaleEnglish,
		Encoding:     "UTF-8",
		Reader:       readCloserFromBytes(input),
	}
	require.NoError(t, reader.Open(t.Context(), doc))
	defer reader.Close()
	return testutil.CollectBlocks(t, reader.Read(t.Context()))
}

// wmlDoc wraps body markup in a word/document.xml.
func wmlDoc(body string) string {
	return `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
  <w:body>` + body + `</w:body>
</w:document>`
}

func para(text string) string {
	return `<w:p><w:r><w:t>` + text + `</w:t></w:r></w:p>`
}

// A cell's paragraphs are ordinaled within that cell, so an edit in one cell
// leaves every other cell alone.
func TestStructuralName_DeletionInAnotherCellDoesNotRename(t *testing.T) {
	table := func(cell1, cell2 string) string {
		return `<w:tbl><w:tr>` +
			`<w:tc>` + cell1 + `</w:tc>` +
			`<w:tc>` + cell2 + `</w:tc>` +
			`</w:tr></w:tbl>`
	}
	v1 := wmlDoc(table(para("Alpha")+para("Bravo"), para("Charlie")+para("Delta")))
	v2 := wmlDoc(table(para("Alpha"), para("Charlie")+para("Delta")))

	before, after := docxNamesByText(t, v1), docxNamesByText(t, v2)

	require.Equal(t, "word/document.xml/table/row/cell/p", before["Alpha"])
	require.Equal(t, "word/document.xml/table/row/cell/p#2", before["Bravo"])
	require.Equal(t, "word/document.xml/table/row/cell#2/p", before["Charlie"])
	require.Equal(t, "word/document.xml/table/row/cell#2/p#2", before["Delta"])

	assert.Equal(t, before["Charlie"], after["Charlie"],
		"a paragraph in an untouched cell must keep its name — the ordinal is scoped to its own cell")
	assert.Equal(t, before["Delta"], after["Delta"])
	assert.Equal(t, before["Alpha"], after["Alpha"],
		"removing a LATER sibling must not rename an earlier one")
}

func TestStructuralName_IdenticalTextGetsDistinctNames(t *testing.T) {
	names := docxNames(t, wmlDoc(
		para("Same")+
			`<w:tbl><w:tr><w:tc>`+para("Same")+`</w:tc><w:tc>`+para("Same")+`</w:tc></w:tr></w:tbl>`+
			para("Same")))

	assert.Equal(t, []string{
		"word/document.xml/p",
		"word/document.xml/table/row/cell/p",
		"word/document.xml/table/row/cell#2/p",
		"word/document.xml/p#2",
	}, names)

	seen := map[string]bool{}
	for _, n := range names {
		assert.False(t, seen[n], "duplicate block name %q — four identical paragraphs would share one identity", n)
		seen[n] = true
	}
}

func TestStructuralName_StableAcrossTwoReads(t *testing.T) {
	doc := wmlDoc(para("Alpha") +
		`<w:tbl><w:tr><w:tc>` + para("Bravo") + `</w:tc></w:tr></w:tbl>` +
		para("Charlie"))

	first, second := docxNames(t, doc), docxNames(t, doc)
	assert.Equal(t, first, second,
		"two reads of identical bytes must produce identical names")
}

// A table is its own scope, so a paragraph that follows one is numbered among
// the body's paragraphs — not among everything the reader has seen.
func TestStructuralName_TableDoesNotAdvanceTheBodyOrdinal(t *testing.T) {
	withTable := wmlDoc(para("Alpha") +
		`<w:tbl><w:tr><w:tc>` + para("Inside") + `</w:tc></w:tr></w:tbl>` +
		para("Charlie"))
	withoutTable := wmlDoc(para("Alpha") + para("Charlie"))

	before := docxNamesByText(t, withTable)
	after := docxNamesByText(t, withoutTable)

	assert.Equal(t, "word/document.xml/p#2", before["Charlie"])
	assert.Equal(t, before["Charlie"], after["Charlie"],
		"removing a whole table must not renumber the body paragraphs around it")
}

// A block must never be named by its own text. reconcile.Identify folds the name
// into the CONTEXT hash while the same text is the CONTENT hash, so a
// self-derived name moves both at once: rewording the block grades it New and it
// loses the translation and approval it carried. A Heading-styled paragraph is
// where this is tempting, and exactly where it is wrong. See
// core/model/structural.go.
func TestStructuralName_RewordingAHeadingKeepsItsIdentity(t *testing.T) {
	heading := func(text string) string {
		return `<w:p><w:pPr><w:pStyle w:val="Heading1"/></w:pPr><w:r><w:t>` + text + `</w:t></w:r></w:p>`
	}
	v1 := wmlDoc(heading("Getting started") + para("Body text"))
	v2 := wmlDoc(heading("Getting started, revised") + para("Body text"))

	assert.Equal(t, "word/document.xml/p", docxNamesByText(t, v1)["Getting started"],
		"a heading named after its own title collapses the two hashes reconcile grades apart")

	v1Blocks := readDocxBlocks(t, v1)
	v1Res := docxKeys(v1Blocks, nil)
	v2Res := docxKeys(readDocxBlocks(t, v2), docxUnits(v1Blocks, v1Res))

	assert.Equal(t, v1Res["Getting started"].Key, v2Res["Getting started, revised"].Key,
		"the heading was rewritten, not replaced — its history must follow it")
	assert.Equal(t, reconcile.Edited, v2Res["Getting started, revised"].Kind)
	assert.Equal(t, reconcile.Unchanged, v2Res["Body text"].Kind,
		"and the paragraph beneath it did not move")
}

const docxIdentityScope = "test.docx"

func docxKeys(blocks []*model.Block, prior []reconcile.Unit) map[string]reconcile.Result {
	out := map[string]reconcile.Result{}
	for _, r := range reconcile.Blocks(docxIdentityScope, blocks, prior) {
		if r.Block.SourceText() != "" {
			out[r.Block.SourceText()] = r
		}
	}
	return out
}

func docxUnits(blocks []*model.Block, res map[string]reconcile.Result) []reconcile.Unit {
	var out []reconcile.Unit
	for _, b := range blocks {
		u := reconcile.Identify(docxIdentityScope, b)
		u.Key = res[b.SourceText()].Key
		out = append(out, u)
	}
	return out
}
