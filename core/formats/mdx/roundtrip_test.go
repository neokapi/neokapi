package mdx

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// readParts runs the MDX reader over src with a skeleton store and returns
// the ordered Block/Data parts plus the populated skeleton store. The
// store is left flushed (ready for reading) so a writer can replay it.
func readParts(t *testing.T, src []byte) ([]*model.Part, *format.SkeletonStore) {
	t.Helper()
	r := NewReader()
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

// writeParts replays the skeleton store + parts through the MDX writer for
// the given target locale (empty = source) and returns the output bytes.
func writeParts(t *testing.T, parts []*model.Part, store *format.SkeletonStore, locale model.LocaleID) []byte {
	t.Helper()
	w := NewWriter()
	var out bytes.Buffer
	require.NoError(t, w.SetOutputWriter(&out))
	if !locale.IsEmpty() {
		w.SetLocale(locale)
	}
	w.SetSkeletonStore(store)

	ch := make(chan *model.Part, len(parts)+1)
	for _, p := range parts {
		ch <- p
	}
	close(ch)
	require.NoError(t, w.Write(context.Background(), ch))
	return out.Bytes()
}

// roundTrip reads then writes src untranslated and returns the result.
func roundTrip(t *testing.T, src []byte) []byte {
	t.Helper()
	parts, store := readParts(t, src)
	return writeParts(t, parts, store, "")
}

// testdataFiles returns every .mdx file under testdata/.
func testdataFiles(t *testing.T) []string {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join("testdata", "*.mdx"))
	require.NoError(t, err)
	require.NotEmpty(t, matches, "expected .mdx fixtures under testdata/")
	return matches
}

// TestRoundTripByteFaithful is the PRIMARY acceptance bar: every real-world
// .mdx fixture must round-trip read→write byte-for-byte when nothing is
// translated.
func TestRoundTripByteFaithful(t *testing.T) {
	for _, path := range testdataFiles(t) {
		t.Run(filepath.Base(path), func(t *testing.T) {
			src, err := os.ReadFile(path)
			require.NoError(t, err)
			out := roundTrip(t, src)
			assert.True(t, bytes.Equal(out, src),
				"byte round-trip mismatch for %s (src=%d out=%d)", path, len(src), len(out))
		})
	}
}

// TestESMPreservedOpaque verifies ESM import/export statements are emitted
// as opaque Data (never translatable Blocks) and round-trip verbatim,
// including multi-line and side-effect imports.
func TestESMPreservedOpaque(t *testing.T) {
	src := []byte(`import A from "a";
import { B, C } from "b";
import "./side-effect";
export const meta = {
  x: 1,
  y: [2, 3],
};
export default function X() {}

# Heading
`)
	parts, store := readParts(t, src)

	var esmData []string
	for _, p := range parts {
		if p.Type == model.PartBlock {
			b := p.Resource.(*model.Block)
			txt := strings.TrimSpace(b.Source[0].Text())
			assert.False(t, strings.HasPrefix(txt, "import ") || strings.HasPrefix(txt, "export "),
				"ESM statement leaked into a translatable block: %q", txt)
		}
		if p.Type == model.PartData {
			if d := p.Resource.(*model.Data); strings.HasPrefix(d.Name, "mdx-esm") {
				esmData = append(esmData, d.Properties["content"])
			}
		}
	}
	require.NotEmpty(t, esmData, "expected ESM regions to be emitted as opaque Data")

	out := writeParts(t, parts, store, "")
	assert.Equal(t, string(src), string(out))
	assert.Contains(t, string(out), `export const meta = {`)
	assert.Contains(t, string(out), `import "./side-effect";`)
}

// TestJSXPreservedOpaque verifies block-level JSX (elements with children,
// self-closing elements, fragments) is opaque and round-trips verbatim,
// and that component/attribute names are never extracted.
func TestJSXPreservedOpaque(t *testing.T) {
	src := []byte(`# Title

<ThemedVideo
  sources={{ light: "/v.webm" }}
  maxWidth="900px"
/>

<Callout type="warning">
  Children prose stays inside the opaque region in v1.
</Callout>

<>
  Fragment children.
</>

Done.
`)
	parts, store := readParts(t, src)

	var jsxData []string
	for _, p := range parts {
		if p.Type == model.PartBlock {
			b := p.Resource.(*model.Block)
			txt := b.Source[0].Text()
			assert.NotContains(t, txt, "ThemedVideo", "component name leaked into block")
			assert.NotContains(t, txt, "maxWidth", "attribute name leaked into block")
		}
		if p.Type == model.PartData {
			if d := p.Resource.(*model.Data); strings.HasPrefix(d.Name, "mdx-jsx") {
				jsxData = append(jsxData, d.Properties["content"])
			}
		}
	}
	require.Len(t, jsxData, 3, "expected 3 opaque JSX regions (self-closing, element-with-children, fragment)")

	out := writeParts(t, parts, store, "")
	assert.Equal(t, string(src), string(out))
}

// TestExpressionPreservedOpaque verifies top-level `{ … }` expression
// blocks (e.g. JSX comments) are opaque and round-trip verbatim.
func TestExpressionPreservedOpaque(t *testing.T) {
	src := []byte(`# Title

{/* a comment expression */}

{ someValue }

Prose.
`)
	parts, store := readParts(t, src)

	var exprData int
	for _, p := range parts {
		if p.Type == model.PartData {
			if d := p.Resource.(*model.Data); d.Name == "mdx-expression" {
				exprData++
			}
		}
	}
	assert.Equal(t, 2, exprData, "expected 2 opaque expression regions")

	out := writeParts(t, parts, store, "")
	assert.Equal(t, string(src), string(out))
}

// TestProseExtracted verifies plain Markdown prose, headings, and list
// items ARE extracted as translatable Blocks.
func TestProseExtracted(t *testing.T) {
	src := []byte(`import X from "x";

# Heading One

A paragraph of prose.

- item one
- item two
`)
	parts, _ := readParts(t, src)

	var texts []string
	for _, p := range parts {
		if p.Type == model.PartBlock {
			texts = append(texts, p.Resource.(*model.Block).Source[0].Text())
		}
	}
	assert.Contains(t, texts, "Heading One")
	assert.Contains(t, texts, "A paragraph of prose.")
	assert.Contains(t, texts, "item one")
	assert.Contains(t, texts, "item two")
}

// TestCodeFenceNotTranslated verifies fenced code blocks are not extracted
// as translatable Blocks (default config) and round-trip verbatim.
func TestCodeFenceNotTranslated(t *testing.T) {
	src := []byte("# Title\n\n```js\nconst secret = 1; // do not translate\n```\n\nProse.\n")
	parts, store := readParts(t, src)

	for _, p := range parts {
		if p.Type == model.PartBlock {
			txt := p.Resource.(*model.Block).Source[0].Text()
			assert.NotContains(t, txt, "const secret", "code fence content leaked into a translatable block")
		}
	}

	out := writeParts(t, parts, store, "")
	assert.Equal(t, string(src), string(out))
	assert.Contains(t, string(out), "const secret = 1; // do not translate")
}

// TestTablePreservedOpaque verifies GFM tables (which the markdown reader
// would normalise) are kept verbatim as opaque regions, preserving the
// source cell alignment byte-for-byte.
func TestTablePreservedOpaque(t *testing.T) {
	src := []byte(`# Title

| Name       | Value     |
| ---------- | --------- |
| alpha      | first     |
| beta       | second    |

After the table.
`)
	parts, store := readParts(t, src)

	for _, p := range parts {
		if p.Type == model.PartBlock {
			txt := p.Resource.(*model.Block).Source[0].Text()
			assert.NotContains(t, txt, "|", "table row leaked into a translatable block")
		}
	}

	out := writeParts(t, parts, store, "")
	assert.Equal(t, string(src), string(out), "table alignment must round-trip verbatim")
	assert.Contains(t, string(out), "| alpha      | first     |", "source padding preserved")
}

// TestTranslationSplicesOnlyProse verifies that translating a prose block
// changes only that block's bytes, leaving ESM, JSX, and tables untouched.
func TestTranslationSplicesOnlyProse(t *testing.T) {
	src := []byte(`import { Widget } from "@scope/widget";

# Heading

The quick brown fox.

<Widget id="x" />

| A    | B    |
| ---- | ---- |
| 1    | 2    |
`)
	parts, store := readParts(t, src)

	// Translate the prose paragraph "The quick brown fox.".
	var translated bool
	for _, p := range parts {
		if p.Type != model.PartBlock {
			continue
		}
		b := p.Resource.(*model.Block)
		if b.Source[0].Text() == "The quick brown fox." {
			b.Targets[model.LocaleID("fr-FR")] = []*model.Segment{
				model.NewRunsSegment("s1", []model.Run{{Text: &model.TextRun{Text: "Le renard brun rapide."}}}),
			}
			translated = true
		}
	}
	require.True(t, translated, "expected to find the prose block to translate")

	out := writeParts(t, parts, store, model.LocaleID("fr-FR"))
	s := string(out)

	assert.Contains(t, s, "Le renard brun rapide.", "translation must be spliced in")
	assert.NotContains(t, s, "The quick brown fox.", "source prose must be replaced")
	// Everything else stays byte-identical.
	assert.Contains(t, s, `import { Widget } from "@scope/widget";`)
	assert.Contains(t, s, `<Widget id="x" />`)
	assert.Contains(t, s, "| 1    | 2    |")
}

// TestFrontMatterPreserved verifies YAML front matter round-trips verbatim
// under the default config (front matter not translated).
func TestFrontMatterPreserved(t *testing.T) {
	src := []byte(`---
title: "Hello"
description: A test.
---

import X from "x";

# Body
`)
	out := roundTrip(t, src)
	assert.Equal(t, string(src), string(out))
	assert.Contains(t, string(out), `title: "Hello"`)
}

// TestEmptyAndWhitespace verifies degenerate inputs do not panic and
// round-trip exactly.
func TestEmptyAndWhitespace(t *testing.T) {
	for _, src := range [][]byte{
		[]byte(""),
		[]byte("\n"),
		[]byte("   \n\n"),
		[]byte("# Just a heading\n"),
		[]byte("import X from \"x\";\n"),
		[]byte("<Self />\n"),
		[]byte("{expr}\n"),
	} {
		out := roundTrip(t, src)
		assert.Equal(t, string(src), string(out), "round-trip mismatch for %q", string(src))
	}
}

// TestConfigForwardsToMarkdown verifies an MDX config parameter (e.g.
// translateCodeBlocks) is forwarded to the delegated markdown reader.
func TestConfigForwardsToMarkdown(t *testing.T) {
	src := []byte("# Title\n\n```js\nconst x = 1;\n```\n")

	r := NewReader()
	cfg := r.Config().(*Config)
	cfg.Reset()
	require.NoError(t, cfg.ApplyMap(map[string]any{"translateCodeBlocks": true}))

	store, err := format.NewSkeletonStore()
	require.NoError(t, err)
	r.SetSkeletonStore(store)
	doc := &model.RawDocument{Reader: io.NopCloser(bytes.NewReader(src)), SourceLocale: model.LocaleEnglish}
	require.NoError(t, r.Open(context.Background(), doc))

	var codeBlockExtracted bool
	for pr := range r.Read(context.Background()) {
		require.NoError(t, pr.Error)
		if pr.Part.Type == model.PartBlock {
			if pr.Part.Resource.(*model.Block).Type == "code-block" {
				codeBlockExtracted = true
			}
		}
	}
	assert.True(t, codeBlockExtracted, "translateCodeBlocks=true should extract the code block as a translatable Block")
}

// TestConfigRejectsUnknownKey verifies the config validates parameter names
// against the markdown schema.
func TestConfigRejectsUnknownKey(t *testing.T) {
	cfg := &Config{}
	cfg.Reset()
	err := cfg.ApplyMap(map[string]any{"notARealParam": true})
	assert.Error(t, err)
}
