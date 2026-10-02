package markdown_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/format/spec"
	"github.com/neokapi/neokapi/core/formats/markdown"
	"github.com/neokapi/neokapi/core/model"
)

// applyToCell reads doc with its skeleton, applies op to the one block whose
// text holds pick, as a person with the capabilities the Markdown writer
// declares, and writes the document back.
func applyToCell(t *testing.T, doc, pick string, op func(b *model.Block) change.Op) (string, change.OpResult) {
	t.Helper()
	reader, writer := markdown.NewReader(), markdown.NewWriter()
	store, err := format.NewWiredSkeleton(reader, writer)
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })
	parts, err := spec.ReadParts(reader, []byte(doc))
	require.NoError(t, err)
	env := change.BlockEnv{Actor: change.Actor{Kind: change.ActorPerson}, Format: change.WriterCapabilities("markdown", writer)}
	var res []change.OpResult
	for _, p := range parts {
		if b, ok := p.Resource.(*model.Block); ok && strings.Contains(model.RunsText(b.Source), pick) {
			require.Nil(t, res, "%q picks one block", pick)
			res = change.ApplyBlock(b, []change.Op{op(b)}, env)
		}
	}
	require.Len(t, res, 1)
	out, err := spec.WriteParts(writer, parts, []byte(doc))
	require.NoError(t, err)
	return string(out), res[0]
}

// cellLink returns the opening half of the link in the block whose text holds
// pick, as doc reads.
func cellLink(t *testing.T, doc, pick string) *model.PcOpenRun {
	t.Helper()
	parts, err := spec.ReadParts(markdown.NewReader(), []byte(doc))
	require.NoError(t, err)
	for _, p := range parts {
		if b, ok := p.Resource.(*model.Block); ok && strings.Contains(model.RunsText(b.Source), pick) {
			for _, r := range b.Source {
				if r.PcOpen != nil && r.PcOpen.Type == "link:hyperlink" {
					return r.PcOpen
				}
			}
		}
	}
	t.Fatalf("no link in a block holding %q", pick)
	return nil
}

// A table row spells a pipe in a cell as `\|`. The reader reads a link
// destination in a cell without that backslash, so a destination written with
// a pipe reads back as the value set, and setting it again is unchanged.
func TestTableCellLinkDestinationReadsBackAsSet(t *testing.T) {
	const doc = "| Item | Where |\n| --- | --- |\n| Herbs | [the shop](https://a.example/p) |\n| Spices | the market |\n"
	const value = "https://a.example/p|q"

	set := func(b *model.Block) change.Op {
		return change.Op{Kind: change.KindSetAttribute, At: change.Ref{Doc: "d", Block: b.ID}, IfMatch: model.EditionRevision(b, model.EditionKey{}),
			Body: &change.SetAttribute{Code: "1", Name: "href", Value: value}}
	}
	out, res := applyToCell(t, doc, "the shop", set)
	require.Equal(t, change.OpApplied, res.Status, "%+v", res.Error)
	assert.Contains(t, out, `[the shop](https://a.example/p\|q)`, "the row spells the pipe escaped")
	assert.Equal(t, value, cellLink(t, out, "the shop").Attrs["href"], "the destination reads back as the value set")

	_, res = applyToCell(t, out, "the shop", set)
	assert.Equal(t, change.OpUnchanged, res.Status, "the same value again changes nothing")

	find := "the market"
	out, res = applyToCell(t, doc, find, func(b *model.Block) change.Op {
		return change.Op{Kind: change.KindMark, At: change.Ref{Doc: "d", Block: b.ID}, IfMatch: model.EditionRevision(b, model.EditionKey{}),
			Body: &change.Mark{Range: change.Selection{Find: &find}, Type: "link:hyperlink", Attrs: map[string]string{"href": value}}}
	})
	require.Equal(t, change.OpApplied, res.Status, "%+v", res.Error)
	assert.Contains(t, out, `[the market](https://a.example/p\|q)`)
	assert.Equal(t, value, cellLink(t, out, "the market").Attrs["href"], "a new link in a cell reads back as the value set")
}
