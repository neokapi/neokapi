package filehome_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/change"
)

// A refused change set reports nothing it did not write: the refused
// operation carries no revision or position, and the documents it read are
// listed as not written, each with the digest it was read at.
func TestFileHome_ARefusedChangeSetListsTheDocumentsItRead(t *testing.T) {
	f := newFixture(t, map[string]string{"a.json": `{"greeting": "Hello there", "farewell": "Goodbye now"}` + "\n"})
	ctx := context.Background()
	page, err := f.svc.Read(ctx, change.ReadRequest{Doc: "a.json"})
	require.NoError(t, err)
	require.Len(t, page.Blocks, 2)
	b := page.Blocks[0]
	for name, op := range map[string]change.Op{
		"stale": {Kind: change.KindReplaceText, At: b.Ref, IfMatch: "r:0000000000000000",
			Body: &change.ReplaceText{Edits: []change.TextEdit{{Find: new("there"), Text: "world"}}}},
		"not found": {Kind: change.KindReplaceText, At: b.Ref, IfMatch: b.Rev,
			Body: &change.ReplaceText{Edits: []change.TextEdit{{Find: new("nowhere"), Text: "world"}}}},
	} {
		t.Run(name, func(t *testing.T) {
			res, err := f.svc.Apply(ctx, change.Set{Ops: []change.Op{op}}, person)
			require.NoError(t, err)
			require.Equal(t, change.SetRefused, res.Status)
			r := res.Ops[0]
			require.Equal(t, change.OpRefused, r.Status)
			assert.Empty(t, r.After)
			assert.Empty(t, r.Before)
			assert.Nil(t, r.Resolved)
			require.Len(t, res.Docs, 1, "the document the change set read")
			d := res.Docs[0]
			assert.Equal(t, "a.json", d.Doc)
			assert.False(t, d.Written)
			assert.Nil(t, d.After)
			assert.Equal(t, page.Head, d.Before, "the digest the document was read at")
			assert.Nil(t, d.Findings, "no check ran outside a project")
			raw, err := json.Marshal(res)
			require.NoError(t, err)
			assert.NotContains(t, string(raw), `"findings"`)
			assert.NotContains(t, string(raw), `"resolved"`)
		})
	}
}
