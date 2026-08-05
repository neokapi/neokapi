package markdown_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/formats/markdown"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/reconcile"
)

// A markdown block's name is its structural address — the heading trail it sits
// under, the structures it sits inside, and its kind — not the count of blocks
// that preceded it. See core/model/structural.go for why: reconcile folds the
// name into a block's context hash and grades content and context as
// independent signals, so a name that moves whenever an unrelated paragraph is
// deleted carries no information.
//
// These tests pin what the scheme does and, just as importantly, what it does
// NOT do. The honest boundary is TestMarkdownNames_ShiftWithinTheSameSection.

// namesByText reads content and maps each block's source text to its name.
func namesByText(t *testing.T, content string) map[string]string {
	t.Helper()

	ctx := context.Background()
	r := markdown.NewReader()
	require.NoError(t, r.Open(ctx, testutil.RawDocFromString(content, model.LocaleEnglish)))

	out := map[string]string{}
	for pr := range r.Read(ctx) {
		require.NoError(t, pr.Error)
		if pr.Part == nil || pr.Part.Type != model.PartBlock {
			continue
		}
		b, ok := pr.Part.Resource.(*model.Block)
		if !ok || b == nil {
			continue
		}
		if text := b.SourceText(); text != "" {
			out[text] = b.Name
		}
	}
	return out
}

// namesInOrder reads content and returns each block's name in document order.
func namesInOrder(t *testing.T, content string) []string {
	t.Helper()

	ctx := context.Background()
	r := markdown.NewReader()
	require.NoError(t, r.Open(ctx, testutil.RawDocFromString(content, model.LocaleEnglish)))

	var out []string
	for pr := range r.Read(ctx) {
		require.NoError(t, pr.Error)
		if pr.Part == nil || pr.Part.Type != model.PartBlock {
			continue
		}
		if b, ok := pr.Part.Resource.(*model.Block); ok && b != nil {
			out = append(out, b.Name)
		}
	}
	return out
}

// The scheme itself, on a document that exercises every structure that carries
// its own scope. Read this first: everything below is a property of these names.
func TestMarkdownNames_AreStructuralAddresses(t *testing.T) {
	const doc = `# Getting Started

Welcome.

## Install

Run the installer.

Then restart.

- first
- second

` + "```go\nfmt.Println()\n```" + `

| Name | Value |
| --- | --- |
| alpha | 1 |

## Configure

Edit the file.
`

	got := namesByText(t, doc)

	assert.Equal(t, "h", got["Getting Started"],
		"the top heading is addressed by its position at the root, not by its own words")
	assert.Equal(t, "getting-started/p", got["Welcome."])
	assert.Equal(t, "getting-started/h", got["Install"],
		"a heading hangs off its PARENT trail — its own text addresses what is below it")
	assert.Equal(t, "getting-started/install/p", got["Run the installer."])
	assert.Equal(t, "getting-started/install/p#2", got["Then restart."])
	assert.Equal(t, "getting-started/install/list/item", got["first"])
	assert.Equal(t, "getting-started/install/list/item#2", got["second"])
	assert.Equal(t, "getting-started/install/code", got["fmt.Println()\n"])
	assert.Equal(t, "getting-started/install/table/row/cell", got["Name"])
	assert.Equal(t, "getting-started/install/table/row/cell#2", got["Value"])
	assert.Equal(t, "getting-started/install/table/row#2/cell", got["alpha"])
	assert.Equal(t, "getting-started/h#2", got["Configure"],
		"and is told from its siblings by an ordinal, not by its title")

	// The paragraph under the SECOND h2 is "p", not "p#3": the ordinal is scoped
	// to the section, so it restarts when a new section opens.
	assert.Equal(t, "getting-started/configure/p", got["Edit the file."])
}

// Property 1 — a deletion in one section does not touch names in another.
//
// This is the property the whole scheme exists for. A paragraph removed from
// "Install" must not re-address the paragraphs under "Configure", or a decision
// recorded about one of them silently names a different string.
func TestMarkdownNames_SurviveADeletionInAnotherSection(t *testing.T) {
	const v1 = `## Install

Alpha

Bravo

## Configure

Charlie

Delta
`
	const v2 = `## Install

Alpha

## Configure

Charlie

Delta
`

	before, after := namesByText(t, v1), namesByText(t, v2)

	require.Contains(t, before, "Bravo", "v1 must contain the paragraph the edit deletes")
	require.NotContains(t, after, "Bravo")

	assert.Equal(t, before["Alpha"], after["Alpha"],
		"a block before the deletion keeps its name")
	assert.Equal(t, before["Charlie"], after["Charlie"],
		"a block in ANOTHER section keeps its name across the deletion")
	assert.Equal(t, before["Delta"], after["Delta"],
		"and so does the one after it — the ordinal is scoped to the section")
}

// The same property for the structures nested inside a section: a list, a table
// row and a fenced code block each open their own scope, so an edit inside one
// cannot re-address the others.
func TestMarkdownNames_SurviveADeletionInAnotherStructure(t *testing.T) {
	const v1 = `## Install

- alpha
- bravo

Charlie

| one | two |
| --- | --- |
| three | four |
`
	const v2 = `## Install

- alpha

Charlie

| one | two |
| --- | --- |
| three | four |
`

	before, after := namesByText(t, v1), namesByText(t, v2)

	require.Contains(t, before, "bravo")
	require.NotContains(t, after, "bravo")

	assert.Equal(t, before["Charlie"], after["Charlie"],
		"a paragraph is not addressed relative to the list above it")
	for _, cell := range []string{"one", "two", "three", "four"} {
		assert.Equal(t, before[cell], after[cell],
			"table cell %q is addressed by its row and column, not by a document counter", cell)
	}
}

// Property 2 — the honest gap.
//
// Two paragraphs in the SAME section are, by construction, indistinguishable by
// structure: the section is the tightest thing the author gives them. Something
// has to tell them apart, and the only candidates are position (an ordinal) or
// content. Content is ruled out — model.ComputeIdentity builds the context hash
// FROM the name, so a content-derived name collapses content and context into
// one signal and a reworded paragraph becomes an unrelated block.
//
// So the ordinal stays, and deleting a paragraph still re-addresses its later
// siblings WITHIN that section. The blast radius is the section rather than the
// document, which is the whole of the improvement. reconcile.Blocks is what
// closes the remaining gap: it grades content and context together, and an
// unchanged text with a shifted ordinal still matches on content.
func TestMarkdownNames_ShiftWithinTheSameSection(t *testing.T) {
	const v1 = `## Install

Alpha

Bravo

Charlie
`
	const v2 = `## Install

Alpha

Charlie
`

	before, after := namesByText(t, v1), namesByText(t, v2)

	assert.Equal(t, "install/p", before["Alpha"])
	assert.Equal(t, "install/p#2", before["Bravo"])
	assert.Equal(t, "install/p#3", before["Charlie"])

	assert.Equal(t, "install/p", after["Alpha"], "a block before the deletion is unaffected")
	assert.Equal(t, "install/p#2", after["Charlie"],
		"a later sibling in the SAME section still shifts — the ordinal is all "+
			"the structure there is, and deriving it from content would destroy "+
			"the independent context signal reconcile depends on")
}

// A document with no headings at all is one flat section, so the same limit
// applies to it end to end. Pinned so the boundary is not mistaken for a bug.
func TestMarkdownNames_FlatDocumentIsOneSection(t *testing.T) {
	before := namesByText(t, "Alpha\n\nBravo\n\nCharlie\n")
	after := namesByText(t, "Alpha\n\nCharlie\n")

	assert.Equal(t, "p", before["Alpha"])
	assert.Equal(t, "p#3", before["Charlie"])
	assert.Equal(t, "p#2", after["Charlie"],
		"with no heading trail there is no structure to scope the ordinal to")
}

// Property 3 — editing a heading DOES re-address everything beneath it.
//
// That is not a defect: the heading is the structure, so changing it changes the
// address of its contents, exactly as renaming a JSON key changes the address of
// its value. Pinned so the consequence is a known one.
func TestMarkdownNames_ChangeWhenTheHeadingChanges(t *testing.T) {
	before := namesByText(t, "## Install\n\nAlpha\n")
	after := namesByText(t, "## Installation\n\nAlpha\n")

	assert.Equal(t, "install/p", before["Alpha"])
	assert.Equal(t, "installation/p", after["Alpha"])
	assert.NotEqual(t, before["Alpha"], after["Alpha"],
		"a heading edit is a structural edit — the blocks beneath it move")

	// The heading's OWN name does not move, which is what lets it grade as an
	// edit rather than as a new block. See
	// TestMarkdownNames_HeadingSurvivesBeingReworded.
	assert.Equal(t, "h", before["Install"])
	assert.Equal(t, "h", after["Installation"])

	// Reflowing the heading's whitespace is not a structural change.
	assert.Equal(t, "getting-started/p",
		namesByText(t, "## Getting   Started\n\nAlpha\n")["Alpha"])
}

// Property 4 — two blocks that are genuinely indistinguishable still get
// distinct names. Identical text under one heading is the case that would
// otherwise collide, and a collision would make two blocks share a context hash.
func TestMarkdownNames_AreUniqueForRepeatedText(t *testing.T) {
	names := namesInOrder(t, "## Install\n\nSee below.\n\nSee below.\n\nSee below.\n")

	require.Len(t, names, 4) // the heading plus three paragraphs
	assert.Equal(t, []string{"h", "install/p", "install/p#2", "install/p#3"}, names)
}

// Two headings with the same text are the same case one level up: the section
// itself is disambiguated, so the blocks beneath them do not interleave.
func TestMarkdownNames_DisambiguateRepeatedHeadings(t *testing.T) {
	got := namesByText(t, "## Notes\n\nAlpha\n\n## Notes\n\nBravo\n")

	assert.Equal(t, "notes/p", got["Alpha"])
	assert.Equal(t, "notes#2/p", got["Bravo"])
}

// Property 5 — reading the same bytes twice gives the same names, both from one
// reader instance and from two. Names are derived, so nothing may leak between
// reads.
func TestMarkdownNames_AreStableAcrossReads(t *testing.T) {
	const doc = `# Title

Alpha

## Section

- one
- two

| a | b |
| --- | --- |
`

	first := namesInOrder(t, doc)
	second := namesInOrder(t, doc)
	require.NotEmpty(t, first)
	assert.Equal(t, first, second, "two readers over identical input agree")

	// The same reader, read twice: the naming state resets with the counters.
	ctx := context.Background()
	r := markdown.NewReader()
	read := func() []string {
		require.NoError(t, r.Open(ctx, testutil.RawDocFromString(doc, model.LocaleEnglish)))
		var out []string
		for pr := range r.Read(ctx) {
			require.NoError(t, pr.Error)
			if pr.Part == nil || pr.Part.Type != model.PartBlock {
				continue
			}
			if b, ok := pr.Part.Resource.(*model.Block); ok && b != nil {
				out = append(out, b.Name)
			}
		}
		return out
	}
	assert.Equal(t, first, read())
	assert.Equal(t, first, read(), "a second read through one reader does not carry ordinals over")
}

// A heading must NOT be named after its own text.
//
// The name becomes the CONTEXT hash and the text becomes the CONTENT hash, and
// reconcile grades the pair. Name a heading by its own title and rewording it
// changes both hashes at once — the one combination that carries nothing — so
// the heading grades New and loses its history and its translation. That is the
// exact two-signals-collapse this whole scheme exists to avoid, and naming a
// block after its own content is how it gets reintroduced.
//
// So a heading is named by its PARENT trail plus its ordinal among the headings
// there. Its own text still addresses everything BENEATH it, which is what makes
// a descendant's name readable and what makes a reworded heading move its
// contents rather than replace them.
func TestMarkdownNames_HeadingSurvivesBeingReworded(t *testing.T) {
	const doc = "docs/guide.md"

	v1 := readBlocksForReconcile(t, "# Guide\n\n## Install\n\nAlpha\n")
	prior := make([]reconcile.Unit, 0, len(v1))
	keys := map[string]string{}
	for i, r := range reconcile.Blocks(doc, v1, nil) {
		u := reconcile.Identify(doc, v1[i])
		u.Key = r.Key
		prior = append(prior, u)
		keys[v1[i].SourceText()] = r.Key
	}

	v2 := readBlocksForReconcile(t, "# Guide\n\n## Installation\n\nAlpha\n")
	got := map[string]reconcile.Result{}
	for _, r := range reconcile.Blocks(doc, v2, prior) {
		got[r.Block.SourceText()] = r
	}

	// The heading itself: its words changed, its place did not.
	require.Contains(t, got, "Installation")
	assert.Equal(t, reconcile.Edited, got["Installation"].Kind,
		"a reworded heading is an EDIT — if this says New, the heading is being "+
			"named after its own text and has just lost its history")
	assert.Equal(t, keys["Install"], got["Installation"].Key,
		"and it keeps the identity every recorded decision hangs off")

	// The paragraph beneath it: its words did not change, its trail did.
	assert.Equal(t, reconcile.Moved, got["Alpha"].Kind,
		"a descendant of a reworded heading moves — content carries it across")
	assert.Equal(t, keys["Alpha"], got["Alpha"].Key)
}

// readBlocksForReconcile reads content and returns the blocks in order.
func readBlocksForReconcile(t *testing.T, content string) []*model.Block {
	t.Helper()

	ctx := context.Background()
	r := markdown.NewReader()
	require.NoError(t, r.Open(ctx, testutil.RawDocFromString(content, model.LocaleEnglish)))

	var out []*model.Block
	for pr := range r.Read(ctx) {
		require.NoError(t, pr.Error)
		if pr.Part == nil || pr.Part.Type != model.PartBlock {
			continue
		}
		if b, ok := pr.Part.Resource.(*model.Block); ok && b != nil {
			out = append(out, b)
		}
	}
	return out
}

// Names must survive input that has no words in it. A heading of pure
// punctuation, or of a script with no case, still has to yield a usable segment.
func TestMarkdownNames_HandleHeadingsWithoutWords(t *testing.T) {
	assert.Equal(t, "section/p", namesByText(t, "## ***\n\nAlpha\n")["Alpha"],
		"a heading with no word characters falls back to a fixed segment")
	assert.Equal(t, "安装/p", namesByText(t, "## 安装\n\nAlpha\n")["Alpha"],
		"a script without case keeps its characters")
	assert.Equal(t, "a-b/p", namesByText(t, "## a/b\n\nAlpha\n")["Alpha"],
		"the path separator cannot appear inside a segment")
}

// Nested structure keeps its own scope: a sublist is addressed under the item it
// hangs off, so editing one item's sublist cannot re-address another's.
func TestMarkdownNames_ScopeNestedLists(t *testing.T) {
	const v1 = `## Install

- alpha
  - one
  - two
- bravo
  - three
`
	const v2 = `## Install

- alpha
  - one
- bravo
  - three
`

	before, after := namesByText(t, v1), namesByText(t, v2)

	assert.Equal(t, "install/list/item", before["alpha"])
	assert.Equal(t, "install/list/item/list/item", before["one"])
	assert.Equal(t, "install/list/item#2/list/item", before["three"])

	require.NotContains(t, after, "two")
	assert.Equal(t, before["three"], after["three"],
		"deleting an item from one sublist does not re-address another sublist")
	assert.Equal(t, before["bravo"], after["bravo"])
}

// Every name in a document must be unique — a duplicate would give two blocks
// the same context hash. Exercised over a document that mixes every construct.
func TestMarkdownNames_AreUniqueWithinADocument(t *testing.T) {
	const doc = `---
title: Front matter
---

# Title

Alpha

## Section

Alpha

- Alpha
- Alpha
  - Alpha

> Alpha

| Alpha | Alpha |
| --- | --- |
| Alpha | Alpha |

<p>Alpha</p>

<img src="x.png" alt="Alpha" />

## Section

Alpha

[ref]: http://example.com "Alpha"

See [ref].
`

	names := namesInOrder(t, doc)
	require.NotEmpty(t, names)

	seen := map[string]bool{}
	for _, n := range names {
		assert.False(t, seen[n], "duplicate block name %q", n)
		assert.NotEmpty(t, n, "every block carries a name")
		seen[n] = true
	}
}
