package formats_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/change/filehome"
	"github.com/neokapi/neokapi/core/formats"
	"github.com/neokapi/neokapi/core/registry"
)

// An edit of one block changes the bytes of the words it replaces and no
// other byte of the file. The operations matrix (opsmatrix_test.go) asserts
// this over templates through change.ApplyBlock. This table drives the change
// service itself, the read and the write `kapi apply` makes. Its documents
// hold what a writer can respell: escapes, literal slashes and a byte order
// mark in JSON; runs of spaces, character references and a head with no
// charset declaration in HTML; CRLF line ends, trailing spaces and a table in
// Markdown.
//
// A format that cannot make the claim yet is listed with expectFail and the
// reason, and the test asserts that it still fails, so a fix that makes it pass
// has to remove the entry.
func TestEditOfOneBlockLeavesEveryOtherByte(t *testing.T) {
	const markdownGuide = "# Guide\r\n\r\nWe **utilize** the [utilize link](https://example.com/utilize)  and `utilize` code.\r\n\r\n" +
		"* Simply send an e-**mail** or an e-mail.   \r\n\r\n<!-- utilize comment -->\r\n\r\n| a | b |\r\n|---|---|\r\n| utilize | x |\r\n"
	const htmlPage = "<!DOCTYPE html>\n<html><head><title>utilize</title></head>\n<body>\n" +
		"<p>We <b>utilize</b> the <a href=\"x\">widget</a>&nbsp;and   utilize&amp;more &eacute;.</p>\n" +
		"<p>Send an e-<i>mail</i>.</p>\n</body></html>\n"
	const htmlSpacing = "<!DOCTYPE html>\n<html>\n<head><title>Guide</title></head>\n<body>\n" +
		"<p>We <b>utilize</b> the <a href=\"x\">widget</a>.</p>\n<p>Keep   this    spacing.</p>\n</body>\n</html>\n"
	const jsonStrings = "{\n    \"a\": \"Hello {name}, utilize it\",\n  \"b\" :\"caf\\u00e9 utilize <b>e-mail</b>\",\n  \"c\": \"nothing\"\n}"

	cases := []struct {
		name, file, doc string
		// pick names the block to edit by text it holds; find, occurrence
		// and with are the replace_text.
		pick, find string
		occurrence int
		with       string
		// from and to are the bytes the edit must change, and the only ones.
		from, to string
		// expectFail is why the format cannot pass yet.
		expectFail string
	}{
		{
			name: "markdown emphasis beside a link and code", file: "guide.md", doc: markdownGuide,
			pick: "code.", find: "utilize", occurrence: 1, with: "use",
			from: "We **utilize** the", to: "We **use** the",
		},
		{
			name: "markdown list item with trailing spaces", file: "guide.md", doc: markdownGuide,
			pick: "Simply send", find: "e-mail", occurrence: 2, with: "email",
			from: "or an e-mail.   \r\n", to: "or an email.   \r\n",
		},
		{
			name: "markdown table cell", file: "guide.md", doc: markdownGuide,
			pick: "utilize", find: "utilize", with: "use",
			from: "| utilize | x |", to: "| use | x |",
		},
		{
			name: "json value with an escape", file: "strings.json", doc: jsonStrings,
			pick: "café", find: "utilize", with: "use",
			from: "\"caf\\u00e9 utilize <b>", to: "\"caf\\u00e9 use <b>",
		},
		{
			name: "json value beside escaped slashes", file: "slashes.json",
			doc:  "{\n  \"link\": \"https:\\/\\/example.com\\/guide\",\n  \"text\": \"We utilize <b>it<\\/b>\\tnow\"\n}\n",
			pick: "We", find: "utilize", with: "use",
			from: `"We utilize <b>`, to: `"We use <b>`,
		},
		{
			name: "json with a byte order mark", file: "bom.json",
			doc:  "\xef\xbb\xbf{\r\n  \"a\": \"We utilize it\"\r\n}\r\n",
			pick: "We", find: "utilize", with: "use",
			from: "We utilize it", to: "We use it",
		},
		{
			name: "html bold word beside a paragraph with runs of spaces", file: "spacing.html", doc: htmlSpacing,
			pick: "widget", find: "utilize", with: "use",
			from: "<b>utilize</b>", to: "<b>use</b>",
		},
		{
			name: "html word between character references", file: "page.html", doc: htmlPage,
			pick: "widget", find: "utilize", occurrence: 2, with: "use",
			from: "and   utilize&amp;more", to: "and   use&amp;more",
		},
		{
			name: "html title in a head with no charset declaration", file: "page.html", doc: htmlPage,
			pick: "utilize", find: "utilize", with: "use",
			from: "<title>utilize</title>", to: "<title>use</title>",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, 1, strings.Count(tc.doc, tc.from), "the bytes to change are one span of the document")
			got := applyOneEdit(t, tc.file, tc.doc, tc.pick, change.TextEdit{Find: &tc.find, Occurrence: tc.occurrence, Text: tc.with})
			want := strings.Replace(tc.doc, tc.from, tc.to, 1)
			if tc.expectFail != "" {
				assert.NotEqual(t, want, got, "listed as failing (%s), and passes: remove the entry", tc.expectFail)
				return
			}
			assert.Equal(t, want, got)
		})
	}
}

// applyOneEdit writes doc to a directory as file, reads it through the change
// service, applies a replace_text of the one block whose text equals pick or
// holds it, and returns the file's bytes after the write.
func applyOneEdit(t *testing.T, file, doc, pick string, edit change.TextEdit) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, file)
	require.NoError(t, os.WriteFile(path, []byte(doc), 0o600))
	reg := registry.NewFormatRegistry()
	formats.RegisterAll(reg)
	home := filehome.New(filehome.DirLayout{Root: dir, Formats: reg, SourceLocale: "en"}, filehome.Options{LockDir: t.TempDir()})
	svc := change.NewService(filehome.Formats{Registry: reg}, change.OneHome(home))

	read, err := svc.Read(t.Context(), change.ReadRequest{Doc: file})
	require.NoError(t, err)
	var block *change.BlockRead
	for i, b := range read.Blocks {
		if b.Text == pick {
			block = &read.Blocks[i]
			break
		}
		if block == nil && strings.Contains(b.Text, pick) {
			block = &read.Blocks[i]
		}
	}
	require.NotNil(t, block, "a block holds %q: %+v", pick, read.Blocks)
	op := change.Op{Kind: change.KindReplaceText, At: block.Ref, IfMatch: block.Rev, Body: &change.ReplaceText{Edits: []change.TextEdit{edit}}}
	res, err := svc.Apply(t.Context(), change.Set{Ops: []change.Op{op}}, change.Actor{Kind: change.ActorPerson, Name: "tester"})
	require.NoError(t, err)
	require.Equal(t, change.SetApplied, res.Status, "%+v", res.Ops)
	out, err := os.ReadFile(path)
	require.NoError(t, err)
	return string(out)
}
