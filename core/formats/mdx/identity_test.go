package mdx

import (
	"bytes"
	"context"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
)

// An MDX block's name is its structural address, exactly as in markdown — the
// heading trail it sits under plus the structures it sits inside. See
// core/model/structural.go for why a positional counter is worthless as an
// identity signal, and core/formats/markdown/naming.go for the scheme.
//
// MDX has one hazard markdown does not: it reads a document as several spans,
// delegating each prose span to its own markdown.Reader and handling the JSX and
// ESM regions between them itself. All of them share one markdown.NamingState,
// and these tests are what proves it — without sharing, every span would restart
// the heading trail and two paragraphs of one document would both be named "p".

// mdxNames reads src with content surfacing on and maps each block's source text
// to its name.
func mdxNames(t *testing.T, src string) map[string]string {
	t.Helper()

	r := NewReader()
	r.cfg.SetExtractNonTranslatableContent(true)
	store, err := format.NewSkeletonStore()
	require.NoError(t, err)
	r.SetSkeletonStore(store)
	require.NoError(t, r.Open(context.Background(), &model.RawDocument{
		Reader:       io.NopCloser(bytes.NewReader([]byte(src))),
		SourceLocale: model.LocaleEnglish,
	}))

	out := map[string]string{}
	for pr := range r.Read(context.Background()) {
		require.NoError(t, pr.Error)
		if pr.Part == nil || pr.Part.Type != model.PartBlock {
			continue
		}
		if b, ok := pr.Part.Resource.(*model.Block); ok && b != nil {
			if text := b.SourceText(); text != "" {
				out[text] = b.Name
			}
		}
	}
	return out
}

// mdxNamesInOrder reads src and returns each block's name in document order.
func mdxNamesInOrder(t *testing.T, src string) []string {
	t.Helper()

	r := NewReader()
	r.cfg.SetExtractNonTranslatableContent(true)
	store, err := format.NewSkeletonStore()
	require.NoError(t, err)
	r.SetSkeletonStore(store)
	require.NoError(t, r.Open(context.Background(), &model.RawDocument{
		Reader:       io.NopCloser(bytes.NewReader([]byte(src))),
		SourceLocale: model.LocaleEnglish,
	}))

	var out []string
	for pr := range r.Read(context.Background()) {
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

// The scheme, on a document whose prose is broken up by the MDX constructs that
// force it into separate spans.
func TestMDXNames_AreStructuralAddresses(t *testing.T) {
	const doc = `import {Note} from './note'

# Getting Started

Welcome.

<Note>Careful here.</Note>

## Install

Run the installer.

| Name | Value |
| --- | --- |
| alpha | 1 |

Then restart.
`

	got := mdxNames(t, doc)

	assert.Equal(t, "h", got["Getting Started"])
	assert.Equal(t, "getting-started/p", got["Welcome."])
	assert.Equal(t, "getting-started/jsx/text", got["Careful here."],
		"a JSX element scopes its own text children, under the heading it follows")
	assert.Equal(t, "getting-started/h", got["Install"],
		"a heading hangs off its parent trail, never off its own text")
	assert.Equal(t, "getting-started/install/p", got["Run the installer."])
	assert.Equal(t, "getting-started/install/table/row/cell", got["Name"])
	assert.Equal(t, "getting-started/install/table/row#2/cell", got["alpha"])

	// The paragraph after the table is in the same section as the one before
	// it, even though a table and a JSX element split the document into three
	// markdown spans between them.
	assert.Equal(t, "getting-started/install/p#2", got["Then restart."])
}

// Property 1 — a deletion in one section does not touch names in another, across
// the span boundaries MDX introduces.
func TestMDXNames_SurviveADeletionInAnotherSection(t *testing.T) {
	const v1 = `## Install

Alpha

Bravo

<Note>Aside.</Note>

## Configure

Charlie

Delta
`
	const v2 = `## Install

Alpha

<Note>Aside.</Note>

## Configure

Charlie

Delta
`

	before, after := mdxNames(t, v1), mdxNames(t, v2)

	require.Contains(t, before, "Bravo")
	require.NotContains(t, after, "Bravo")

	assert.Equal(t, before["Alpha"], after["Alpha"])
	assert.Equal(t, before["Aside."], after["Aside."],
		"the JSX element is addressed by the section it sits in, not by how much prose preceded it")
	assert.Equal(t, before["Charlie"], after["Charlie"])
	assert.Equal(t, before["Delta"], after["Delta"])
}

// Property 2 — the same honest gap as markdown: two paragraphs in one section
// are separated only by an ordinal, so deleting the first re-addresses the
// second. Pinned so the boundary is a known one. See
// markdown.TestMarkdownNames_ShiftWithinTheSameSection for why content is not an
// option.
func TestMDXNames_ShiftWithinTheSameSection(t *testing.T) {
	before := mdxNames(t, "## Install\n\nAlpha\n\nBravo\n\nCharlie\n")
	after := mdxNames(t, "## Install\n\nAlpha\n\nCharlie\n")

	assert.Equal(t, "install/p#3", before["Charlie"])
	assert.Equal(t, "install/p#2", after["Charlie"],
		"a later sibling in the same section still shifts")
}

// Property 3 — editing a heading re-addresses what is beneath it, including the
// MDX regions in that section. That is what a structural edit means.
func TestMDXNames_ChangeWhenTheHeadingChanges(t *testing.T) {
	before := mdxNames(t, "## Install\n\n<Note>Aside.</Note>\n")
	after := mdxNames(t, "## Installation\n\n<Note>Aside.</Note>\n")

	assert.Equal(t, "install/jsx/text", before["Aside."])
	assert.Equal(t, "installation/jsx/text", after["Aside."])

	// The heading itself does not move, so it grades as an edit and keeps its
	// history. See markdown.TestMarkdownNames_HeadingSurvivesBeingReworded.
	assert.Equal(t, "h", before["Install"])
	assert.Equal(t, "h", after["Installation"])
}

// Property 4 — identical text in one section gets distinct names, and it must
// stay distinct across the span boundary between them. This is the one that
// fails outright if the spans do not share a naming state: both would be "p".
func TestMDXNames_AreUniqueAcrossSpans(t *testing.T) {
	const doc = `## Install

See below.

<Note>See below.</Note>

See below.

<Note>See below.</Note>
`

	names := mdxNamesInOrder(t, doc)
	require.NotEmpty(t, names)

	seen := map[string]bool{}
	for _, n := range names {
		assert.False(t, seen[n], "duplicate block name %q — spans are not sharing a naming state", n)
		assert.NotEmpty(t, n)
		seen[n] = true
	}
	assert.Equal(t, []string{
		"h",
		"install/p",
		"install/jsx/text",
		"install/p#2",
		"install/jsx#2/text",
	}, names)
}

// Property 5 — reading the same bytes twice gives the same names.
func TestMDXNames_AreStableAcrossReads(t *testing.T) {
	const doc = `import {Note} from './note'

# Title

Alpha

<Note>Aside.</Note>

## Section

| a | b |
| --- | --- |
| c | d |
`

	first := mdxNamesInOrder(t, doc)
	require.NotEmpty(t, first)
	assert.Equal(t, first, mdxNamesInOrder(t, doc))
}
