package html_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/format"
	htmlfmt "github.com/neokapi/neokapi/core/formats/html"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
)

// HTML has two readers — a DOM walk (convert / inspect) and a tokenizer that
// keeps the skeleton for byte-exact write-back. Both name blocks by DOM path,
// and both are exercised here, because a block whose name depends on which
// engine read it has no identity at all.
//
// A block's Name is folded into its context hash (core/reconcile), so what these
// tests pin is: an edit in one subtree leaves names in every other subtree
// alone, two blocks with identical text are still told apart, and two reads of
// the same bytes agree.

type htmlReaderMode struct {
	name    string
	newRead func(t *testing.T) *htmlfmt.Reader
}

// htmlModes returns both reader engines. The tokenizer engine is selected by
// giving the reader a skeleton store.
func htmlModes() []htmlReaderMode {
	return []htmlReaderMode{
		{
			name:    "dom",
			newRead: func(*testing.T) *htmlfmt.Reader { return htmlfmt.NewReader() },
		},
		{
			name: "tokenizer",
			newRead: func(t *testing.T) *htmlfmt.Reader {
				t.Helper()
				r := htmlfmt.NewReader()
				store, err := format.NewSkeletonStore()
				require.NoError(t, err)
				t.Cleanup(func() { _ = store.Close() })
				r.SetSkeletonStore(store)
				return r
			},
		},
	}
}

func htmlNames(t *testing.T, mode htmlReaderMode, content string) []string {
	t.Helper()
	ctx := t.Context()
	reader := mode.newRead(t)
	require.NoError(t, reader.Open(ctx, testutil.RawDocFromString(content, model.LocaleEnglish)))
	defer reader.Close()

	var out []string
	for _, b := range testutil.CollectBlocks(t, reader.Read(ctx)) {
		out = append(out, b.Name)
	}
	return out
}

func htmlNamesByText(t *testing.T, mode htmlReaderMode, content string) map[string]string {
	t.Helper()
	ctx := t.Context()
	reader := mode.newRead(t)
	require.NoError(t, reader.Open(ctx, testutil.RawDocFromString(content, model.LocaleEnglish)))
	defer reader.Close()

	out := map[string]string{}
	for _, b := range testutil.CollectBlocks(t, reader.Read(ctx)) {
		if text := b.SourceText(); text != "" {
			out[text] = b.Name
		}
	}
	return out
}

func TestStructuralName_DeletionInAnotherSubtreeDoesNotRename(t *testing.T) {
	const v1 = `<html><body>` +
		`<div class="intro"><p>Alpha</p><p>Bravo</p></div>` +
		`<div class="main"><p>Charlie</p><p>Delta</p></div>` +
		`</body></html>`
	// Bravo goes. Nothing in the second <div> moved.
	const v2 = `<html><body>` +
		`<div class="intro"><p>Alpha</p></div>` +
		`<div class="main"><p>Charlie</p><p>Delta</p></div>` +
		`</body></html>`

	for _, mode := range htmlModes() {
		t.Run(mode.name, func(t *testing.T) {
			before := htmlNamesByText(t, mode, v1)
			after := htmlNamesByText(t, mode, v2)

			require.Equal(t, "div/p", before["Alpha"])
			require.Equal(t, "div/p[2]", before["Bravo"])
			require.Equal(t, "div[2]/p", before["Charlie"])
			require.Equal(t, "div[2]/p[2]", before["Delta"])

			assert.Equal(t, before["Charlie"], after["Charlie"],
				"a paragraph in an untouched subtree must keep its name — the ordinal is scoped to its own parent")
			assert.Equal(t, before["Delta"], after["Delta"])
			assert.Equal(t, before["Alpha"], after["Alpha"],
				"removing a LATER sibling must not rename an earlier one")
		})
	}
}

func TestStructuralName_IdenticalTextGetsDistinctNames(t *testing.T) {
	const doc = `<html><body>` +
		`<div><p>Same</p><p>Same</p></div>` +
		`<section><p>Same</p></section>` +
		`</body></html>`

	for _, mode := range htmlModes() {
		t.Run(mode.name, func(t *testing.T) {
			names := htmlNames(t, mode, doc)
			assert.Equal(t, []string{"div/p", "div/p[2]", "section/p"}, names)

			seen := map[string]bool{}
			for _, n := range names {
				assert.False(t, seen[n], "duplicate block name %q — two blocks would share one identity", n)
				seen[n] = true
			}
		})
	}
}

func TestStructuralName_StableAcrossTwoReads(t *testing.T) {
	const doc = `<html><body><h1>Title</h1><div><p>Alpha</p><p>Bravo</p></div><p>Tail</p></body></html>`

	for _, mode := range htmlModes() {
		t.Run(mode.name, func(t *testing.T) {
			assert.Equal(t, htmlNames(t, mode, doc), htmlNames(t, mode, doc),
				"two reads of identical bytes must produce identical names")
			assert.Equal(t, []string{"h1", "div/p", "div/p[2]", "p"}, htmlNames(t, mode, doc))
		})
	}
}

// An id is a natural key the author controls: it names the element, and it
// anchors the path of everything beneath it, so moving the subtree elsewhere in
// the document renames nothing inside it.
func TestStructuralName_IDAnchorsThePath(t *testing.T) {
	const shallow = `<html><body><div id="nav"><p>Home</p><p>About</p></div></body></html>`
	const buried = `<html><body><header><section><div id="nav"><p>Home</p><p>About</p></div></section></header></body></html>`

	for _, mode := range htmlModes() {
		t.Run(mode.name, func(t *testing.T) {
			assert.Equal(t, []string{"nav-id/p", "nav-id/p[2]"}, htmlNames(t, mode, shallow))
			assert.Equal(t, htmlNames(t, mode, shallow), htmlNames(t, mode, buried),
				"an id anchors the path, so relocating the subtree renames nothing inside it")
		})
	}
}

// A translatable attribute is a separate block from the element's text, and is
// addressed as such.
func TestStructuralName_AttributeBlocksAreAddressed(t *testing.T) {
	const doc = `<html><body><p><img src="a.png" alt="First"><img src="b.png" alt="Second"></p></body></html>`

	for _, mode := range htmlModes() {
		t.Run(mode.name, func(t *testing.T) {
			names := htmlNames(t, mode, doc)
			assert.Contains(t, names, "p/img@alt")
			assert.Contains(t, names, "p/img[2]@alt")
		})
	}
}

// html/head/body are dropped from the path: the parser synthesizes them when the
// source omits them, so keeping them would give a fragment and a full document
// different names for the same paragraph.
func TestStructuralName_ImplicitWrappersAreNotPartOfThePath(t *testing.T) {
	for _, mode := range htmlModes() {
		t.Run(mode.name, func(t *testing.T) {
			assert.Equal(t,
				htmlNames(t, mode, `<html><body><p>Alpha</p></body></html>`),
				htmlNames(t, mode, `<p>Alpha</p>`),
				"a fragment and the document it expands to must name the paragraph the same")
		})
	}
}
