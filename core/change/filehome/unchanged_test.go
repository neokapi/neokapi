package filehome_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/model"
)

// An edit that says what the file already holds, and differs from the read
// only in what the writer takes from the file (a code's native form, the
// do-not-translate mark on a code span's text), is unchanged. It is not a
// change the format would drop: the file already says it.
func TestFileHome_AnEditTheFileAlreadyHoldsIsUnchanged(t *testing.T) {
	const doc = "Keep your code, for example `HB-2041`, ready.\n"
	f := newFixture(t, map[string]string{"a.md": doc})
	ctx := context.Background()
	page, err := f.svc.Read(ctx, change.ReadRequest{Doc: "a.md"})
	require.NoError(t, err)
	require.Len(t, page.Blocks, 1)
	b := page.Blocks[0]
	require.Contains(t, b.Text, `<x id="1"/>HB-2041<x id="/1"/>`)

	// The runs a returned translation carries: the codes by id, with no
	// native form, and the code span's text with no do-not-translate mark.
	runs := []model.Run{
		model.TextR("Keep your code, for example "),
		model.PcOpenR(model.PcOpenRun{ID: "1"}),
		model.TextR("HB-2041"),
		model.PcCloseR(model.PcCloseRun{ID: "1"}),
		model.TextR(", ready."),
	}
	res, err := f.svc.Apply(ctx, change.Set{Ops: []change.Op{{
		Kind: change.KindSetContent, At: b.Ref, IfMatch: b.Rev, Body: &change.SetContent{Runs: runs},
	}}}, person)
	require.NoError(t, err)
	require.Equal(t, change.SetApplied, res.Status, "%+v", res.Ops[0].Error)
	assert.Equal(t, change.OpUnchanged, res.Ops[0].Status)
	assert.Equal(t, res.Ops[0].Before, res.Ops[0].After)
	assert.Equal(t, doc, f.read(t, "a.md"))
}
