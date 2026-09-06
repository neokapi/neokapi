package mdx

import (
	"bytes"
	"context"
	"io"
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// readPartsExtract runs the MDX reader over src with a skeleton store and the
// extractNonTranslatableContent flag set to `on`, returning the ordered
// Block/Data parts plus the populated skeleton store (ready to replay).
func readPartsExtract(t *testing.T, src []byte, on bool) ([]*model.Part, *format.SkeletonStore) {
	t.Helper()
	r := NewReader()
	r.cfg.SetExtractNonTranslatableContent(on)
	store, err := format.NewSkeletonStore()
	require.NoError(t, err)
	r.SetSkeletonStore(store)
	doc := &model.RawDocument{
		Reader:       io.NopCloser(bytes.NewReader(src)),
		SourceLocale: model.LocaleEnglish,
	}
	require.NoError(t, r.Open(context.Background(), doc))

	var parts []*model.Part
	for pr := range r.Read(context.Background()) {
		require.NoError(t, pr.Error)
		switch pr.Part.Type {
		case model.PartBlock, model.PartData:
			parts = append(parts, pr.Part)
		}
	}
	return parts, store
}

// contentBlocks returns blocks of the given Type.
func contentBlocks(parts []*model.Part, blockType string) []*model.Block {
	var out []*model.Block
	for _, p := range parts {
		if p.Type != model.PartBlock {
			continue
		}
		if b := p.Resource.(*model.Block); b.Type == blockType {
			out = append(out, b)
		}
	}
	return out
}

// opaqueData returns the verbatim content of every opaque Data part with the
// given mdx-* name.
func opaqueData(parts []*model.Part, name string) []string {
	var out []string
	for _, p := range parts {
		if p.Type != model.PartData {
			continue
		}
		if d := p.Resource.(*model.Data); d.Name == name {
			out = append(out, d.Properties["content"])
		}
	}
	return out
}

// --- Treatment A: block-level JSX text children (#928) ---

// TestJSXChildrenSurfacedWhenOn verifies that with the flag ON, a block-level
// JSX element's text children are surfaced as content blocks (RoleCode-free
// plain strings, PreserveWhitespace, single verbatim run) with the
// tags/attributes/{expressions} kept in the skeleton, and that the untranslated
// round-trip stays byte-for-byte.
//
// <Callout> is not in the W3C translatability table, so it is a container, and
// a container holding direct text is promoted: its prose is a component's
// body, and dropping it is how a page ends up half translated.
func TestJSXChildrenSurfacedWhenOn(t *testing.T) {
	src := []byte(`# Title

<Callout type="warning" data={{ a: 1 }}>
  Read the {docsURL} guide first.
  Then run the command.
</Callout>

Done.
`)
	parts, store := readPartsExtract(t, src, true)

	children := contentBlocks(parts, "jsx-text")
	require.NotEmpty(t, children, "expected JSX text children surfaced as content blocks")
	for _, b := range children {
		assert.True(t, b.Translatable,
			"an unclassified component's text is promoted, not dropped")
		assert.True(t, b.PreserveWhitespace, "JSX text children ride verbatim")
		assert.Len(t, b.Source, 1, "JSX text child must be a single verbatim run")
		assert.NotContains(t, b.SourceText(), "<", "no tag bytes in a content block")
		assert.NotContains(t, b.SourceText(), "Callout", "no component name in a content block")
	}
	texts := blockTextsByType(parts, "jsx-text")
	// An inline {docsURL} expression stays in the skeleton and splits the
	// surrounding text into two child runs (the trailing run keeps its
	// internal newline + indentation verbatim).
	assert.Contains(t, texts, "Read the")
	assert.Contains(t, texts, "guide first.\n  Then run the command.")
	for _, b := range children {
		assert.NotContains(t, b.SourceText(), "docsURL", "inline expression leaked into a content block")
	}
	// The opaque JSX Data part is NOT emitted on the surfaced path.
	assert.Empty(t, opaqueData(parts, "mdx-jsx"), "surfaced JSX must not also emit opaque Data")

	out := writeParts(t, parts, store, "")
	assert.Equal(t, string(src), string(out), "JSX child surfacing must round-trip byte-for-byte")
}

// TestJSXChildrenOpaqueWhenOff verifies that with the flag OFF the JSX region
// stays a single opaque Data part — the exact pre-#928 part stream — so parity
// (which forces the flag off) is byte-identical.
func TestJSXChildrenOpaqueWhenOff(t *testing.T) {
	src := []byte(`# Title

<Callout type="warning">
  Read the guide first.
</Callout>

Done.
`)
	parts, store := readPartsExtract(t, src, false)

	assert.Empty(t, contentBlocks(parts, "jsx-text"), "no JSX children surfaced when flag off")
	jsx := opaqueData(parts, "mdx-jsx")
	require.Len(t, jsx, 1, "JSX region stays a single opaque Data part when flag off")
	assert.Contains(t, jsx[0], "Read the guide first.")

	out := writeParts(t, parts, store, "")
	assert.Equal(t, string(src), string(out))
}

// TestSelfClosingJSXStaysOpaque verifies a self-closing element (no text
// children) never surfaces a content block even with the flag on.
func TestSelfClosingJSXStaysOpaque(t *testing.T) {
	src := []byte("# Title\n\n<Widget id=\"x\" count={3} />\n\nDone.\n")
	parts, store := readPartsExtract(t, src, true)
	assert.Empty(t, contentBlocks(parts, "jsx-text"), "self-closing JSX has no children to surface")
	require.Len(t, opaqueData(parts, "mdx-jsx"), 1, "self-closing JSX stays opaque")
	out := writeParts(t, parts, store, "")
	assert.Equal(t, string(src), string(out))
}

// TestNestedJSXChildrenSurfaced verifies deeply-nested block JSX (the
// real-world walkthroughs-index shape: <div><Link><h3>…</h3><p>…</p>…) surfaces
// each text child while keeping every tag in the skeleton and round-tripping
// byte-for-byte.
func TestNestedJSXChildrenSurfaced(t *testing.T) {
	src := []byte(`<div className="row">
  <div className="col">
    <Link to="/a">
      <h3>Tour title</h3>
      <p>A short description sentence.</p>
    </Link>
  </div>
</div>
`)
	parts, store := readPartsExtract(t, src, true)
	texts := blockTextsByType(parts, "jsx-text")
	assert.Contains(t, texts, "Tour title")
	assert.Contains(t, texts, "A short description sentence.")
	for _, b := range contentBlocks(parts, "jsx-text") {
		assert.NotContains(t, b.SourceText(), "className", "attribute leaked into content block")
		assert.NotContains(t, b.SourceText(), "Link", "component name leaked into content block")
	}
	out := writeParts(t, parts, store, "")
	assert.Equal(t, string(src), string(out), "nested JSX child surfacing must round-trip byte-for-byte")
}

// --- Treatment A: GFM table cell prose (#928) ---

// TestTableCellsAreTranslatableBlocks pins what delegating a GFM table to the
// markdown reader buys: each cell is an ordinary translatable block with its
// inline markup parsed into runs, the header row's cells carry the header role,
// the delimiter row and the pipes and padding around every cell stay in the
// skeleton, and the untranslated round-trip is byte-for-byte.
func TestTableCellsAreTranslatableBlocks(t *testing.T) {
	src := []byte(`# Title

| Name       | Value            |
| ---------- | ---------------- |
| **alpha**  | ` + "`first`" + `          |
| beta       | second           |

After.
`)
	parts, store := readPartsExtract(t, src, true)

	cells := contentBlocks(parts, "table-cell")
	require.Len(t, cells, 6, "two header cells and four body cells")
	for _, b := range cells {
		assert.True(t, b.Translatable, "a table cell holds prose a reader reads")
		assert.NotContains(t, b.SourceText(), "|", "pipe leaked into a cell block")
	}
	assert.Equal(t, model.RoleTableHeader, cells[0].SemanticRole())
	assert.Equal(t, model.RoleTableHeader, cells[1].SemanticRole())
	assert.Equal(t, model.RoleTableCell, cells[2].SemanticRole())

	texts := blockTextsByType(parts, "table-cell")
	assert.Equal(t, []string{"Name", "Value", "alpha", "first", "beta", "second"}, texts,
		"a cell's inline markup is parsed, so its text is the prose inside it")
	assert.NotEmpty(t, cells[2].Source, "bold cell text arrives as runs")
	assert.NotContains(t, texts, "----------", "the delimiter row is not a cell")
	assert.Empty(t, opaqueData(parts, "mdx-table"), "a delegated table emits no opaque Data")

	out := writeParts(t, parts, store, "")
	assert.Equal(t, string(src), string(out), "a delegated table must round-trip byte-for-byte")
}

// TestTableCellsTranslateWhateverTheSurfacingFlagSays is the other half. The
// flag governs NON-translatable content surfacing (code fences, JSX text); a
// table's cells are prose either way, so turning it off must not take them
// away or send the table back to an opaque region.
func TestTableCellsTranslateWhateverTheSurfacingFlagSays(t *testing.T) {
	src := []byte(`# Title

| A    | B    |
| ---- | ---- |
| 1    | 2    |

After.
`)
	parts, store := readPartsExtract(t, src, false)
	assert.Len(t, contentBlocks(parts, "table-cell"), 4)
	assert.Empty(t, opaqueData(parts, "mdx-table"), "the table is not opaque with the flag off")
	out := writeParts(t, parts, store, "")
	assert.Equal(t, string(src), string(out))
}

// TestTableRowsRideGroups pins the grid a cross-format export needs: the
// markdown reader brackets a table's cells in a table group and one row group
// per row, and MDX forwards them with the IDs renumbered into its own
// namespace rather than dropping them with the sub-reader's layer parts.
func TestTableRowsRideGroups(t *testing.T) {
	src := []byte("| A | B |\n| - | - |\n| 1 | 2 |\n")

	r := NewReader()
	store, err := format.NewSkeletonStore()
	require.NoError(t, err)
	defer func() { _ = store.Close() }()
	r.SetSkeletonStore(store)
	require.NoError(t, r.Open(context.Background(), &model.RawDocument{
		Reader:       io.NopCloser(bytes.NewReader(src)),
		SourceLocale: model.LocaleEnglish,
	}))

	var shape []string
	for pr := range r.Read(context.Background()) {
		require.NoError(t, pr.Error)
		switch v := pr.Part.Resource.(type) {
		case *model.GroupStart:
			shape = append(shape, "start:"+v.Type+":"+v.ID)
		case *model.GroupEnd:
			shape = append(shape, "end:"+v.ID)
		case *model.Block:
			shape = append(shape, "block:"+v.ID)
		}
	}

	assert.Equal(t, []string{
		"start:table:g1",
		"start:table-row:g2", "block:tu1", "block:tu2", "end:g2",
		"start:table-row:g3", "block:tu3", "block:tu4", "end:g3",
		"end:g1",
	}, shape)
}

// --- Treatment A: markdown-opaque fallback blocks (#928) ---

// TestMarkdownOpaqueFallbackSurfacesBlocks verifies that when a Markdown span
// fails byte-exact reconstruction (here a strikethrough whose tilde runs differ
// in length, which the reader spells from the parser's delimiter length)
// the span is kept verbatim opaque
// AND, when the flag is on, the prose the markdown sub-reader already parsed
// is surfaced as non-translatable
// content blocks (no skeleton ref → no round-trip impact). With the flag off,
// only the opaque Data is emitted (identical pre-#928 part stream). Both
// directions round-trip byte-for-byte.
func TestMarkdownOpaqueFallbackSurfacesBlocks(t *testing.T) {
	// A strikethrough whose tilde runs differ in length: the reader spells the
	// pair from the delimiter length the parser reports rather than from the
	// source bytes, so the markdown reader does not reconstruct it
	// byte-for-byte and the opaque fallback takes over (#2500).
	src := []byte("line one ~~two~ three\n")

	// Flag ON: opaque Data + a non-translatable block carrying the prose.
	onParts, onStore := readPartsExtract(t, src, true)
	require.Len(t, opaqueData(onParts, "mdx-markdown-opaque"), 1,
		"the non-reconstructable span must still be preserved verbatim opaque")
	var surfaced []*model.Block
	for _, p := range onParts {
		if p.Type == model.PartBlock {
			surfaced = append(surfaced, p.Resource.(*model.Block))
		}
	}
	require.NotEmpty(t, surfaced, "flag on must surface the parsed prose as content blocks")
	for _, b := range surfaced {
		assert.False(t, b.Translatable, "markdown-opaque fallback blocks must be non-translatable")
	}
	assert.Equal(t, string(src), string(writeParts(t, onParts, onStore, "")),
		"surfacing the fallback blocks must not change the byte round-trip")

	// Flag OFF: opaque Data only (no blocks) — identical pre-#928 part stream.
	offParts, offStore := readPartsExtract(t, src, false)
	require.Len(t, opaqueData(offParts, "mdx-markdown-opaque"), 1)
	for _, p := range offParts {
		assert.NotEqual(t, model.PartBlock, p.Type, "no blocks surfaced on the flag-off fallback path")
	}
	assert.Equal(t, string(src), string(writeParts(t, offParts, offStore, "")))
}

// --- Config flag (#928) ---

// TestExtractNonTranslatableContentConfig verifies the inverted-private-field
// flag idiom: default ON, ApplyMap parsing, the Set toggle, and that the flag
// is NOT forwarded to the delegated markdown reader (an mdx-level toggle that
// leaves embedded code-fence behaviour unchanged).
func TestExtractNonTranslatableContentConfig(t *testing.T) {
	c := &Config{}
	c.Reset()
	assert.True(t, c.ExtractNonTranslatableContent(), "default is ON")

	require.NoError(t, c.ApplyMap(map[string]any{"extractNonTranslatableContent": false}))
	assert.False(t, c.ExtractNonTranslatableContent(), "ApplyMap can turn it off")
	require.NoError(t, c.ApplyMap(map[string]any{"extractNonTranslatableContent": true}))
	assert.True(t, c.ExtractNonTranslatableContent(), "ApplyMap can turn it back on")

	c.SetExtractNonTranslatableContent(false)
	assert.False(t, c.ExtractNonTranslatableContent(), "Set toggles the flag")

	// Wrong type is rejected.
	require.Error(t, c.ApplyMap(map[string]any{"extractNonTranslatableContent": "nope"}))

	// The flag is NOT forwarded to markdown: a code fence inside an MDX
	// markdown span stays opaque (Data) even with the mdx flag ON, because
	// embedded markdown surfacing is governed separately and kept off.
	src := []byte("# Title\n\n```js\nconst secret = 1;\n```\n")
	parts, store := readPartsExtract(t, src, true)
	for _, p := range parts {
		if p.Type == model.PartBlock {
			assert.NotContains(t, p.Resource.(*model.Block).SourceText(), "const secret",
				"code fence must stay opaque (mdx flag is not forwarded to markdown)")
		}
	}
	assert.Equal(t, string(src), string(writeParts(t, parts, store, "")))
}

// blockTextsByType returns the source text of every block of the given Type.
func blockTextsByType(parts []*model.Part, blockType string) []string {
	var out []string
	for _, b := range contentBlocks(parts, blockType) {
		out = append(out, b.SourceText())
	}
	return out
}
