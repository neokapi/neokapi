package markdown_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/model"
)

// A block's readable name carries its headings' words, so a translated document
// names the same paragraph in its own language. The translation-invariant
// address writes each heading as its own structural identity instead, so it
// reads the same on both sides — which is what pairs a source file with its
// translation unit by unit.

// addressesInOrder reads content and returns each block's invariant address in
// document order, falling back to the name where the block composes none.
func addressesInOrder(t *testing.T, content string) []string {
	t.Helper()

	var out []string
	for _, b := range readBlocksForReconcile(t, content) {
		if addr := b.StructuralAddress(); addr != "" {
			out = append(out, addr)
			continue
		}
		out = append(out, b.Name)
	}
	return out
}

func TestMarkdownAddress_SurvivesTranslation(t *testing.T) {
	const source = `# Tidewatch

An opening paragraph.

## What it reads

The first paragraph of the section.

The second paragraph of the section.

- A list item
- Another list item

## How it reports

A paragraph under the second heading.

### Detail

A paragraph one level deeper.
`

	// The same document with every word translated — headings included, which is
	// what re-addresses the blocks beneath them.
	const translated = `# Tidevakt

Et innledende avsnitt.

## Hva den leser

Det første avsnittet i seksjonen.

Det andre avsnittet i seksjonen.

- Et listepunkt
- Et annet listepunkt

## Hvordan den rapporterer

Et avsnitt under den andre overskriften.

### Detalj

Et avsnitt ett nivå dypere.
`

	sourceNames := namesInOrder(t, source)
	targetNames := namesInOrder(t, translated)
	require.Len(t, targetNames, len(sourceNames))
	assert.NotEqual(t, sourceNames, targetNames,
		"the readable names are written in each document's own language — if they "+
			"already agreed there would be nothing for the address to fix")

	sourceAddrs := addressesInOrder(t, source)
	targetAddrs := addressesInOrder(t, translated)
	assert.Equal(t, sourceAddrs, targetAddrs,
		"the invariant address must not depend on any block's words")

	// The shape is the heading trail written as heading identities.
	assert.Equal(t, []string{
		"h",
		"h/p",
		"h/h",
		"h/h/p",
		"h/h/p#2",
		"h/h/list/item",
		"h/h/list/item#2",
		"h/h#2",
		"h/h#2/p",
		"h/h#2/h",
		"h/h#2/h/p",
	}, sourceAddrs)
}

func TestMarkdownAddress_NameStaysReadable(t *testing.T) {
	const doc = `# Getting started

## Install

Run the installer.
`
	names := namesByText(t, doc)
	assert.Equal(t, "getting-started/install/p", names["Run the installer."],
		"the name keeps its ancestors' words — it is what a translator reads in the "+
			"extracted file")
}

// A document with no headings composes no address at all: its names are already
// invariant, so carrying a second copy of them would be noise on every block.
func TestMarkdownAddress_AbsentWhenTheNameIsAlreadyInvariant(t *testing.T) {
	const doc = `A paragraph.

Another paragraph.

- An item
`
	for _, b := range readBlocksForReconcile(t, doc) {
		assert.Empty(t, b.StructuralAddress(), "block %q", b.Name)
	}
}

// Two headings with the same words under one parent are told apart by the same
// ordinal on both sides, so their sections stay distinct in the address.
func TestMarkdownAddress_RepeatedHeadingsStayDistinct(t *testing.T) {
	const doc = `## Notes

First.

## Notes

Second.
`
	blocks := readBlocksForReconcile(t, doc)
	byText := map[string]*model.Block{}
	for _, b := range blocks {
		byText[b.SourceText()] = b
	}
	assert.Equal(t, "h/p", byText["First."].StructuralAddress())
	assert.Equal(t, "h#2/p", byText["Second."].StructuralAddress())
}
