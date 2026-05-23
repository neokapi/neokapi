package mdx

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestScanSegmentsGapFree verifies the segmenter always produces a
// gap-free, in-order partition of the body (concatenation reproduces the
// input exactly) for a variety of inputs.
func TestScanSegmentsGapFree(t *testing.T) {
	inputs := []string{
		"",
		"plain prose only\n",
		"import X from \"x\";\n\n# H\n",
		"# H\n\n<Tag />\n\nmore\n",
		"{expr}\n\ntext\n\n<>\nfrag\n</>\n",
		"export const a = {\n b: 1,\n};\n\ntext\n",
		"text\n<Comp prop={value}>\n  child\n</Comp>\ntext after\n",
	}
	for _, in := range inputs {
		body := []byte(in)
		segs := scanSegments(body)
		var rebuilt []byte
		prevEnd := 0
		for _, s := range segs {
			require.Equal(t, prevEnd, s.start, "segments must be gap-free for %q", in)
			require.LessOrEqual(t, s.start, s.end)
			rebuilt = append(rebuilt, body[s.start:s.end]...)
			prevEnd = s.end
		}
		if len(segs) > 0 {
			require.Equal(t, len(body), prevEnd, "segments must cover the whole body for %q", in)
		}
		assert.Equal(t, in, string(rebuilt), "segment concatenation must reproduce input")
	}
}

// TestScanSegmentsClassification checks each construct maps to the right
// segment kind.
func TestScanSegmentsClassification(t *testing.T) {
	cases := []struct {
		body string
		want segmentKind
	}{
		{"import X from \"x\";\n", segESM},
		{"export const a = 1;\n", segESM},
		{"<Component />\n", segJSX},
		{"<div>\nhi\n</div>\n", segJSX},
		{"<>\nfrag\n</>\n", segJSX},
		{"{value}\n", segExpr},
		{"Just prose.\n", segMarkdown},
		{"    indented < not jsx\n", segMarkdown},
		{"- list with import keyword inside\n", segMarkdown},
	}
	for _, c := range cases {
		segs := scanSegments([]byte(c.body))
		require.NotEmpty(t, segs, "expected a segment for %q", c.body)
		assert.Equal(t, c.want, segs[0].kind, "wrong kind for %q", c.body)
	}
}

// TestSplitMarkdownTables verifies table isolation within a Markdown span.
func TestSplitMarkdownTables(t *testing.T) {
	span := []byte("intro\n\n| A | B |\n| - | - |\n| 1 | 2 |\n\noutro\n")
	subs := splitMarkdownTables(span)

	var tableCount int
	var rebuilt []byte
	prevEnd := 0
	for _, s := range subs {
		require.Equal(t, prevEnd, s.start, "table sub-spans must be gap-free")
		rebuilt = append(rebuilt, span[s.start:s.end]...)
		prevEnd = s.end
		if s.isTable {
			tableCount++
			assert.Contains(t, string(span[s.start:s.end]), "| A | B |")
		}
	}
	assert.Equal(t, len(span), prevEnd)
	assert.Equal(t, string(span), string(rebuilt))
	assert.Equal(t, 1, tableCount, "expected exactly one table sub-span")
}

// TestSplitMarkdownTablesNoTable verifies a table-free span yields a single
// non-table sub-span.
func TestSplitMarkdownTablesNoTable(t *testing.T) {
	span := []byte("just prose\n\nand a pipe | not a table\n")
	subs := splitMarkdownTables(span)
	for _, s := range subs {
		assert.False(t, s.isTable, "no table expected")
	}
}
