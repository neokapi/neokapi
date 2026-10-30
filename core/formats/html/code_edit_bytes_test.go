package html_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/format/spec"
	htmlfmt "github.com/neokapi/neokapi/core/formats/html"
	"github.com/neokapi/neokapi/core/model"
)

// applyAndWrite reads doc with its skeleton, applies op to the one block whose
// text holds pick, as a person with the capabilities the HTML writer declares,
// and writes the document back.
func applyAndWrite(t *testing.T, doc, pick string, op func(b *model.Block) change.Op) string {
	t.Helper()
	reader, writer := htmlfmt.NewReader(), htmlfmt.NewWriter()
	store, err := format.NewWiredSkeleton(reader, writer)
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })
	parts, err := spec.ReadParts(reader, []byte(doc))
	require.NoError(t, err)
	env := change.BlockEnv{Actor: change.Actor{Kind: change.ActorPerson}, Format: change.WriterCapabilities("html", writer)}
	applied := 0
	for _, p := range parts {
		if b, ok := p.Resource.(*model.Block); ok && strings.Contains(model.RunsText(b.SourceRuns()), pick) {
			res := change.ApplyBlock(b, []change.Op{op(b)}, env)
			require.Equal(t, change.OpApplied, res[0].Status, "%+v", res[0].Error)
			applied++
		}
	}
	require.Equal(t, 1, applied, "%q picks one block", pick)
	out, err := spec.WriteParts(writer, parts, []byte(doc))
	require.NoError(t, err)
	return string(out)
}

// A change to a block's codes keeps every byte of its text as the document
// spells it: a quote and a bare ampersand the encoding pass would respell, and
// the line breaks the reader normalized, stay; only the code changes.
func TestCodeEditKeepsTheBlocksBytes(t *testing.T) {
	cases := []struct {
		name, doc, pick string
		op              func(b *model.Block) change.Op
		from, to        string
	}{
		{
			name: "set_attribute on one line",
			doc:  `<html><body><p>Say "hi" & <a href="/x">go</a> now</p></body></html>`,
			pick: "Say",
			op: func(b *model.Block) change.Op {
				return change.Op{Kind: change.KindSetAttribute, At: change.Ref{Doc: "d", Block: b.ID}, IfMatch: model.EditionRevision(b, model.EditionKey{}),
					Body: &change.SetAttribute{Code: "1", Name: "href", Value: "/y"}}
			},
			from: `href="/x"`, to: `href="/y"`,
		},
		{
			name: "mark over wrapped text",
			doc:  "<html><body><p>Read the <a href=\"/x\">guide</a>\n    before you\n    order \"now\" & then.</p></body></html>",
			pick: "Read the",
			op: func(b *model.Block) change.Op {
				find := "order"
				return change.Op{Kind: change.KindMark, At: change.Ref{Doc: "d", Block: b.ID}, IfMatch: model.EditionRevision(b, model.EditionKey{}),
					Body: &change.Mark{Range: change.Selection{Find: &find}, Type: "fmt:italic"}}
			},
			from: "    order", to: "    <em>order</em>",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, 1, strings.Count(tc.doc, tc.from))
			assert.Equal(t, strings.Replace(tc.doc, tc.from, tc.to, 1), applyAndWrite(t, tc.doc, tc.pick, tc.op))
		})
	}
}

// replaceWord is a replace_text of one word of a block.
func replaceWord(find, with string) func(b *model.Block) change.Op {
	return func(b *model.Block) change.Op {
		f := find
		return change.Op{Kind: change.KindReplaceText, At: change.Ref{Doc: "d", Block: b.ID}, IfMatch: model.EditionRevision(b, model.EditionKey{}),
			Body: &change.ReplaceText{Edits: []change.TextEdit{{Find: &f, Text: with}}}}
	}
}

// A change to a block's text changes the words it replaces and nothing else:
// no meta is added to a head that declares no charset, the whitespace of the
// edited block and of every other block keeps its bytes, and the entities,
// quotes and tags around the edit stay as the document spells them.
func TestTextEditKeepsTheDocumentsBytes(t *testing.T) {
	cases := []struct {
		name, doc, pick string
		op              func(b *model.Block) change.Op
		from, to        string
	}{
		{
			name: "a word beside a paragraph with runs of spaces",
			doc:  "<!DOCTYPE html>\n<html>\n<head><title>Guide</title></head>\n<body>\n<p>We <b>utilize</b> the <a href=\"x\">widget</a>.</p>\n<p>Keep   this    spacing.</p>\n</body>\n</html>\n",
			pick: "We", op: replaceWord("utilize", "use"),
			from: "<b>utilize</b>", to: "<b>use</b>",
		},
		{
			name: "a word inside a paragraph with entities and runs of spaces",
			doc:  "<!DOCTYPE html>\n<html><head><title>utilize</title></head>\n<body>\n<p>We <b>utilize</b> the <a href=\"x\">widget</a>&nbsp;and   utilize&amp;more &eacute;.</p>\n<p>Send an e-<i>mail</i>.</p>\n</body></html>\n",
			pick: "widget", op: replaceWord("and   utilize", "and   use"),
			from: "and   utilize&amp;", to: "and   use&amp;",
		},
		{
			name: "a title",
			doc:  "<!DOCTYPE html>\n<html><head><title>We  utilize \"it\"</title></head>\n<body><p>Keep   this.</p></body></html>\n",
			pick: "it", op: replaceWord("utilize", "use"),
			from: "We  utilize", to: "We  use",
		},
		{
			name: "added text that needs escaping",
			doc:  "<html><body><p>Say   \"hi\" &amp; utilize it.</p><p>Keep   this.</p></body></html>",
			pick: "Say", op: replaceWord("utilize", "use <b> & co"),
			from: "utilize it", to: "use &lt;b> &amp; co it",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, 1, strings.Count(tc.doc, tc.from))
			assert.Equal(t, strings.Replace(tc.doc, tc.from, tc.to, 1), applyAndWrite(t, tc.doc, tc.pick, tc.op))
		})
	}
}
