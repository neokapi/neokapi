package asciidoc

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/format/spec"
	"github.com/neokapi/neokapi/core/model"
)

// editAsciidocSource reads doc through the skeleton round-trip `kapi apply`
// uses, edits the block whose edit text is from to the edit text to, and
// writes the document back with no locale.
func editAsciidocSource(t *testing.T, doc, from, to string) string {
	t.Helper()
	reader, writer := NewReader(), NewWriter()
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

// An edit's wording is text. AsciiDoc passes a raw passthrough and the pass
// macro to the page as HTML, and reads macros, attribute references and block
// attribute lines from text, so markup an edit adds is written with its
// opening character as a character reference, while markup the block already
// held stays as the document spelled it.
func TestWriter_EditedSourceAddsNoMarkup(t *testing.T) {
	tests := []struct {
		name string
		doc  string
		from string
		to   string
		want string // the edited block as written
	}{
		{
			name: "a raw passthrough and the pass macro",
			doc:  "= T\n\nHello world\n",
			from: "Hello world",
			to:   "x +++<script>alert(1)</script>+++ y pass:[<img src=x onerror=alert(1)>]",
			want: "x &#43;&#43;&#43;<script>alert(1)</script>&#43;&#43;&#43; y pass:&#91;<img src=x onerror=alert(1)>]",
		},
		{
			name: "a link macro with a script target",
			doc:  "Hello world\n",
			from: "Hello world",
			to:   "click xlink:javascript:alert(1)[here]",
			want: "click xlink:javascript:alert(1)&#91;here]",
		},
		{
			name: "an include directive after a blank line",
			doc:  "Hello world\n",
			from: "Hello world",
			to:   "x\n\ninclude::/etc/passwd[]",
			want: "x\n\ninclude::/etc/passwd&#91;]",
		},
		{
			name: "a passthrough block style",
			doc:  "Hello world\n",
			from: "Hello world",
			to:   "x\n\n[pass]\n<b>raw</b>",
			want: "x\n\n&#91;pass]\n<b>raw</b>",
		},
		{
			name: "a passthrough block",
			doc:  "Hello world\n",
			from: "Hello world",
			to:   "x\n\n++++\n<script>alert(1)</script>",
			want: "x\n\n&#43;&#43;&#43;&#43;\n<script>alert(1)</script>",
		},
		{
			name: "an attribute reference and a cross reference",
			doc:  "Hello world\n",
			from: "Hello world",
			to:   "see {secret} and <<other>>",
			want: "see &#123;secret} and &#60;<other>>",
		},
		{
			name: "a passthrough the block held is kept",
			doc:  "Use +++<kbd>K</kbd>+++ here.\n",
			from: "Use +++<kbd>K</kbd>+++ here.",
			to:   "Press +++<kbd>K</kbd>+++ now.",
			want: "Press +++<kbd>K</kbd>+++ now.",
		},
		{
			name: "brackets and pluses that open nothing stay as typed",
			doc:  "Hello world\n",
			from: "Hello world",
			to:   "see [1] and C++ and 1 + 2",
			want: "see [1] and C++ and 1 + 2",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			out := editAsciidocSource(t, tc.doc, tc.from, tc.to)
			assert.Contains(t, out, tc.want+"\n")
		})
	}
}

// A block nobody edited is written as it was read, passthroughs and macros
// included.
func TestWriter_EditKeepsUntouchedAsciidocByteExact(t *testing.T) {
	const doc = "= Title\n\n" +
		"Hello {name} and *bold* and C++ and pass:[<b>raw</b>] and +++<i>r</i>+++.\n\n" +
		"Plain paragraph with [1] and <<sec,Section>>.\n\n" +
		"Edit me\n"

	out := editAsciidocSource(t, doc, "Edit me", "Edited")

	assert.Equal(t, strings.Replace(doc, "Edit me", "Edited", 1), out)
}
