package html_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/format/spec"
	htmlfmt "github.com/neokapi/neokapi/core/formats/html"
	"github.com/neokapi/neokapi/core/model"
)

// writeForLocale reads doc through the skeleton round trip and writes it for
// locale. When copySource is set, each translatable block gets a target equal
// to its source, the way a tool that leaves a block untranslated fills it.
func writeForLocale(t *testing.T, doc string, locale model.LocaleID, copySource bool) string {
	t.Helper()
	reader, writer := htmlfmt.NewReader(), htmlfmt.NewWriter()
	store, err := format.NewWiredSkeleton(reader, writer)
	require.NoError(t, err)
	require.NotNil(t, store)
	t.Cleanup(func() { _ = store.Close() })

	parts, err := spec.ReadParts(reader, []byte(doc))
	require.NoError(t, err)
	if copySource {
		for _, p := range parts {
			if b, ok := p.Resource.(*model.Block); ok && b.Translatable {
				b.SetTargetRuns(locale, model.ParseRunsEditText(model.RunsEditText(b.SourceRuns()), b.SourceRuns()))
			}
		}
	}
	writer.SetLocale(locale)
	out, err := spec.WriteParts(writer, parts, []byte(doc))
	require.NoError(t, err)
	return string(out)
}

// An untranslated block keeps the document's own spelling when the document is
// written for another locale: line breaks inside inline text, the whitespace
// between a text run and the tag after it, and quotes in text. The fixture's
// text runs sit in containers whose first child is an inline element, so the
// reader extracts them as bare text blocks rather than as leaf blocks.
func TestWriter_UntranslatedBlocksKeepSourceSpelling(t *testing.T) {
	doc := "<html lang=\"en\"><body>\n" +
		"<center><b><font size=\"+1\">News Reference Facts Information\n" +
		"Sources Haven Works <a href=\"http://x/\">!</a>-)</font></b></center>\n" +
		"<center><b><a href=\"/al\">AL</a></b>\n" +
		"- <b><a href=\"/ar\">AR</a></b> \"quoted\"\n" +
		"- <b><a href=\"/az\">AZ</a></b></center>\n" +
		"<div><a href=\"#a\">A</a>-<a href=\"#b\">B</a>\n&nbsp;\n<div>y</div></div>\n" +
		"<div><a href=\"#c\">C</a></div>\n&nbsp;\n<div>w</div>\n" +
		"<p>Two\nlines</p>\n" +
		"<ul>&nbsp;\n<center>\n<p>x</p></center></ul>\n" +
		"<br>Haven <a href=\"/\"><img src=\"h.gif\" alt=\"Haven logo\"></a></center>\n\n<div>z</div>\n" +
		"</body></html>\n"
	want := "<html lang=\"fr\">" + doc[len("<html lang=\"en\">"):]

	for _, copySource := range []bool{false, true} {
		name := "no target"
		if copySource {
			name = "target equals source"
		}
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, want, writeForLocale(t, doc, "fr", copySource))
		})
	}
}
