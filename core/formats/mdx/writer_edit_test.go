package mdx_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/format/spec"
	"github.com/neokapi/neokapi/core/formats/mdx"
	"github.com/neokapi/neokapi/core/model"
)

// editMDXSource reads doc through the skeleton round-trip `kapi apply` uses,
// edits the block whose edit text is from to the edit text to, and writes the
// document back with no locale.
func editMDXSource(t *testing.T, doc, from, to string) string {
	t.Helper()
	reader, writer := mdx.NewReader(), mdx.NewWriter()
	store, err := format.NewWiredSkeleton(reader, writer)
	require.NoError(t, err)
	require.NotNil(t, store)
	t.Cleanup(func() { _ = store.Close() })

	parts, err := spec.ReadParts(reader, []byte(doc))
	require.NoError(t, err)
	edited := 0
	var seen []string
	for _, p := range parts {
		b, ok := p.Resource.(*model.Block)
		if !ok || !b.Translatable {
			continue
		}
		seen = append(seen, model.RunsEditText(b.Source))
		if model.RunsEditText(b.Source) != from {
			continue
		}
		b.EditSourceRuns(model.ParseRunsEditText(to, b.Source))
		edited++
	}
	require.Equal(t, 1, edited, "exactly one block reads %q; blocks: %q", from, seen)

	out, err := spec.WriteParts(writer, parts, []byte(doc))
	require.NoError(t, err)
	return string(out)
}

// An MDX expression is JavaScript the docs build runs, and JSX and ES module
// statements are code too. An edit's wording is text, so markup it adds is
// escaped, while expressions and markup the block already held stay as the
// document spelled them.
func TestWriter_EditedSourceAddsNoMarkup(t *testing.T) {
	tests := []struct {
		name string
		doc  string
		from string
		to   string
		want string // the edited block as written
	}{
		{
			name: "an expression and a script element",
			doc:  "# Title\n\nHello world\n",
			from: "Hello world",
			to:   "x {process.exit(1)} <script>alert(1)</script>",
			want: "x \\{process.exit(1)\\} \\<script>alert(1)\\</script>",
		},
		{
			name: "a component",
			doc:  "Hello world\n",
			from: "Hello world",
			to:   "a <Danger/> and <>fragment</>",
			want: "a \\<Danger/> and \\<>fragment\\</>",
		},
		{
			name: "an export after a blank line",
			doc:  "Hello world\n",
			from: "Hello world",
			to:   "x\n\nexport const a = process.exit(1)",
			want: "x\n\n&#101;xport const a = process.exit(1)",
		},
		{
			name: "an import opening the block",
			doc:  "Hello world\n",
			from: "Hello world",
			to:   "import fs from 'fs'",
			want: "&#105;mport fs from 'fs'",
		},
		{
			name: "a link to a script",
			doc:  "Hello world\n",
			from: "Hello world",
			to:   "[x](javascript:alert(1))",
			want: "\\[x](javascript:alert(1))",
		},
		{
			name: "an expression the block held is kept",
			doc:  "Hello {props.name} there\n",
			from: "Hello {props.name} there",
			to:   "Hi {props.name}, welcome",
			want: "Hi {props.name}, welcome",
		},
		{
			name: "an expression the edit changes is text",
			doc:  "Hello {props.name} there\n",
			from: "Hello {props.name} there",
			to:   "Hi {props.name.constructor('x')()}",
			want: "Hi \\{props.name.constructor('x')()\\}",
		},
		{
			name: "a brace the edit adds after a kept expression",
			doc:  "Hello {props.name} there\n",
			from: "Hello {props.name} there",
			to:   "Hello {props.name} {",
			want: "Hello {props.name} \\{",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			out := editMDXSource(t, tc.doc, tc.from, tc.to)
			assert.Contains(t, out, tc.want+"\n")
		})
	}
}

// A block nobody edited is written as it was read, expressions, components
// and module statements included.
func TestWriter_EditKeepsUntouchedMDXByteExact(t *testing.T) {
	const doc = "import { Foo } from './foo'\n\n" +
		"# Title\n\n" +
		"Hello {props.name} and <Foo bar=\"x\">inner</Foo> see [1].\n\n" +
		"<Callout>Review &amp; approve {x}</Callout>\n\n" +
		"Edit me\n"

	out := editMDXSource(t, doc, "Edit me", "Edited <b>")

	assert.Equal(t, strings.Replace(doc, "Edit me", "Edited \\<b>", 1), out)
}
