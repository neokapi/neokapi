package mdx

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// translatableTexts returns the source text of every translatable block the
// reader emitted for src.
func translatableTexts(t *testing.T, src []byte) []string {
	t.Helper()
	parts, _ := readParts(t, src)
	var out []string
	for _, p := range parts {
		if p.Type != model.PartBlock {
			continue
		}
		if b := p.Resource.(*model.Block); b.Translatable {
			out = append(out, b.SourceText())
		}
	}
	return out
}

// TestOpenFenceAt covers which lines open a CommonMark code fence.
func TestOpenFenceAt(t *testing.T) {
	cases := []struct {
		name       string
		line       string
		wantOK     bool
		wantChar   byte
		wantLength int
	}{
		{name: "backtick fence", line: "```\n", wantOK: true, wantChar: '`', wantLength: 3},
		{name: "backtick fence with info string", line: "```bash\n", wantOK: true, wantChar: '`', wantLength: 3},
		{name: "long backtick fence", line: "`````\n", wantOK: true, wantChar: '`', wantLength: 5},
		{name: "tilde fence", line: "~~~\n", wantOK: true, wantChar: '~', wantLength: 3},
		{name: "tilde fence with backticks in info", line: "~~~`x`\n", wantOK: true, wantChar: '~', wantLength: 3},
		{name: "three spaces of indent", line: "   ```\n", wantOK: true, wantChar: '`', wantLength: 3},
		{name: "four spaces is indented code", line: "    ```\n"},
		{name: "tab is four columns", line: "\t```\n"},
		{name: "two backticks are a code span", line: "``x``\n"},
		{name: "backtick in the info string", line: "``` `x`\n"},
		{name: "ordinary prose", line: "Some prose.\n"},
		{name: "JSX element", line: "<Callout>\n"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f, ok := openFenceAt([]byte(c.line), 0)
			require.Equal(t, c.wantOK, ok)
			if c.wantOK {
				assert.Equal(t, c.wantChar, f.char)
				assert.Equal(t, c.wantLength, f.length)
			}
		})
	}
}

// TestFencedBlockEnd covers where a fenced block ends: at its closing fence,
// or at end of document when the fence is left open.
func TestFencedBlockEnd(t *testing.T) {
	cases := []struct {
		name string
		body string
		want int
	}{
		{name: "closed fence", body: "```\ncode\n```\nafter\n", want: len("```\ncode\n```\n")},
		{name: "closing fence may be longer", body: "```\ncode\n`````\nafter\n", want: len("```\ncode\n`````\n")},
		{name: "shorter run does not close", body: "````\n```\ncode\n````\nafter\n", want: len("````\n```\ncode\n````\n")},
		{name: "tilde does not close a backtick fence", body: "```\n~~~\n```\nafter\n", want: len("```\n~~~\n```\n")},
		{name: "info string on the closing fence does not close", body: "```\n```js\n```\nafter\n", want: len("```\n```js\n```\n")},
		{name: "unclosed fence runs to end of document", body: "```\ncode\n", want: len("```\ncode\n")},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			end, ok := fencedBlockEnd([]byte(c.body), 0)
			require.True(t, ok)
			assert.Equal(t, c.want, end)
		})
	}
}

// TestFenceOpaqueToSegmenter is the #1860 regression at the scanner level: no
// byte inside a fenced code block may open an MDX region, whatever it looks
// like. Each body must scan to Markdown only.
func TestFenceOpaqueToSegmenter(t *testing.T) {
	bodies := map[string]string{
		"angle-bracket token":   "# T\n\n```\n<binary> version\n```\n\nAfter.\n",
		"info string":           "# T\n\n```bash\n<binary> doctor\n```\n\nAfter.\n",
		"tilde fence":           "# T\n\n~~~\n<placeholder>\n~~~\n\nAfter.\n",
		"import line":           "# T\n\n```js\nimport X from \"x\";\n```\n\nAfter.\n",
		"export line":           "# T\n\n```js\nexport const a = 1;\n```\n\nAfter.\n",
		"expression line":       "# T\n\n```js\n{ value }\n```\n\nAfter.\n",
		"jsx fragment":          "# T\n\n```jsx\n<>\nchild\n</>\n```\n\nAfter.\n",
		"indented fence":        "- item\n\n  ```\n  <token> here\n  ```\n\n- second\n",
		"pipes that look table": "# T\n\n```\n| a | b |\n| - | - |\n```\n\nAfter.\n",
	}
	for name, body := range bodies {
		t.Run(name, func(t *testing.T) {
			segs := scanSegments([]byte(body))
			for _, s := range segs {
				assert.Equal(t, segMarkdown, s.kind,
					"fenced content must not open an MDX region: %q", body[s.start:s.end])
				assert.Equal(t, defectNone, s.defect)
			}
		})
	}
}

// TestWrappedCodeSpanIsNotJSX covers the other way code puts an angle-bracket
// token at column 0: an inline code span that wraps across a line break. The
// continuation line is prose, not a JSX element.
func TestWrappedCodeSpanIsNotJSX(t *testing.T) {
	bodies := map[string]string{
		"wrapped placeholder": "Every command below is invoked as `kapi\n<command>` (e.g. `kapi init`). See\n[Installation](/install).\n",
		"wrapped in a note":   ":::note\nRun `kapi translate --provider\n<name>` when a provider is needed.\n:::\n",
		"wrapped brace":       "Set the value with `kapi config set\n{key}` before the run.\n",
		"three-line span":     "Run `go run ./scripts/scan -src\n<dir>/src -class\n<Class> -out gen.go`.\n",
	}
	for name, body := range bodies {
		t.Run(name, func(t *testing.T) {
			segs := scanSegments([]byte(body))
			for _, s := range segs {
				assert.Equal(t, segMarkdown, s.kind,
					"a wrapped code span must not open an MDX region: %q", body[s.start:s.end])
			}
			assert.Equal(t, body, string(roundTrip(t, []byte(body))))
		})
	}
}

// TestCodeSpanOpacityStopsAtABlankLine verifies the paragraph bound: a code
// span cannot contain a blank line, so an unclosed backtick does not make the
// rest of the document opaque to JSX detection.
func TestCodeSpanOpacityStopsAtABlankLine(t *testing.T) {
	body := []byte("A stray ` backtick.\n\n<Callout>\n\nchild\n\n</Callout>\n")
	segs := scanSegments(body)
	var sawJSX bool
	for _, s := range segs {
		if s.kind == segJSX {
			sawJSX = true
			assert.Equal(t, defectNone, s.defect)
		}
	}
	assert.True(t, sawJSX, "the element after the blank line is still JSX")
}

// TestStrayClosingTagReported verifies a closing tag with no opening tag is
// named as such — the region is that tag alone, not the document's tail.
func TestStrayClosingTagReported(t *testing.T) {
	src := []byte("# Title\n\nProse.\n\n</content>\n")
	err := readErr(t, src)
	var se *StrayClosingTagError
	require.ErrorAs(t, err, &se)
	assert.Equal(t, 5, se.Line)
	assert.Equal(t, "</content>", se.Tag)
	assert.Contains(t, se.Error(), "has no opening tag")
}

// TestFencedAngleBracketDocumentReadsCompletely is the #1860 regression at the
// reader level: the document in the issue must yield every prose block, not
// just the ones before the fence.
func TestFencedAngleBracketDocumentReadsCompletely(t *testing.T) {
	src := []byte("# Title\n\nFirst paragraph.\n\n```\n<binary> version\n```\n\nSecond paragraph.\n\n## Later heading\n\nThird paragraph.\n")

	texts := translatableTexts(t, src)
	assert.Equal(t, []string{
		"Title",
		"First paragraph.",
		"Second paragraph.",
		"Later heading",
		"Third paragraph.",
	}, texts)

	assert.Equal(t, string(src), string(roundTrip(t, src)),
		"the fixed document must still round-trip byte-for-byte")
}

// TestFenceInJSXChildrenKeepsElementBalanced verifies a fence inside a JSX
// element's children cannot unbalance the element: the region ends at the
// closing tag and the prose after it is read normally.
func TestFenceInJSXChildrenKeepsElementBalanced(t *testing.T) {
	src := []byte("<Callout>\n\n```\n<binary> version\n```\n\n</Callout>\n\nAfter the element.\n")

	segs := scanSegments(src)
	require.NotEmpty(t, segs)
	assert.Equal(t, segJSX, segs[0].kind)
	assert.Equal(t, defectNone, segs[0].defect, "the fence must not swallow the closing tag")
	assert.Equal(t, len("<Callout>\n\n```\n<binary> version\n```\n\n</Callout>\n"), segs[0].end)

	assert.Contains(t, translatableTexts(t, src), "After the element.")
	assert.Equal(t, string(src), string(roundTrip(t, src)))
}

// TestFenceFixtureCoverage asserts the two fixtures added for #1860 read
// through to their closing prose — the property that was lost when a fence
// could open a JSX region.
func TestFenceFixtureCoverage(t *testing.T) {
	cases := []struct {
		file string
		last string
	}{
		{file: "fenced-angle-tokens.mdx", last: "Closing prose, which must be read as prose."},
		{
			file: "plugin-protocol-shape.mdx",
			last: "A plugin that exits non-zero fails the step that called it. A plugin that\nwrites malformed frames fails the read with the offending frame's offset, which\nis the only way a truncated stream is distinguishable from a short one.",
		},
	}
	for _, c := range cases {
		t.Run(c.file, func(t *testing.T) {
			src, err := os.ReadFile(filepath.Join("testdata", c.file))
			require.NoError(t, err)
			texts := translatableTexts(t, src)
			require.NotEmpty(t, texts)
			assert.Equal(t, c.last, texts[len(texts)-1],
				"the document's final prose block must be reached")
		})
	}
}
