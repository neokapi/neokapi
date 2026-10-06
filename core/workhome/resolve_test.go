package workhome_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/change"
)

// conflicted is two machines that each wrote the German of one block from
// the same head, with the first machine's log merged from the second's: the
// write that sorts first holds the block, and the other is a conflict.
func conflicted(t *testing.T) *keptFixture {
	t.Helper()
	a, b := twoMachines(t, map[string]string{"thanks": "Danke"})
	setGerman(t, a, "thanks", "Vielen Dank")
	setGerman(t, b, "thanks", "Danke schön")
	mergeInto(t, a.m, b.m)
	conflicts, err := a.m.home.Conflicts(context.Background())
	require.NoError(t, err)
	require.Len(t, conflicts, 1)
	return a
}

// keepGerman sends the German of block as text (the wording a read finds
// when text is empty), guarded by the revision that read finds.
func keepGerman(t *testing.T, f *keptFixture, block, text string) *change.Result {
	t.Helper()
	ctx := context.Background()
	page, err := f.svc.Read(ctx, change.ReadRequest{Doc: "de/a.json", Blocks: []string{block}})
	require.NoError(t, err)
	require.Len(t, page.Blocks, 1)
	if text == "" {
		text = page.Blocks[0].Text
	}
	res, err := f.svc.Apply(ctx, change.Set{Ops: []change.Op{{Kind: change.KindSetContent, At: page.Blocks[0].Ref,
		IfMatch: page.Blocks[0].Rev, Body: &change.SetContent{Text: &text}}}}, person)
	require.NoError(t, err)
	require.Equal(t, change.SetApplied, res.Status, "%+v", res.Ops)
	return res
}

// TestWorkspaceHome_KeepingTheContestedWordingSettlesTheConflict: a person
// who sends the wording the head holds for a contested block decides it. The
// operation changes nothing, the workspace home records the decision, and the
// conflict is gone.
func TestWorkspaceHome_KeepingTheContestedWordingSettlesTheConflict(t *testing.T) {
	a := conflicted(t)
	held := german(t, a)["thanks"]
	res := keepGerman(t, a, "thanks", "")
	assert.Equal(t, change.OpUnchanged, res.Ops[0].Status)
	require.NotNil(t, res.Record, "the decision to keep the wording is recorded")
	assert.Equal(t, held, german(t, a)["thanks"])
	conflicts, err := a.m.home.Conflicts(context.Background())
	require.NoError(t, err)
	assert.Empty(t, conflicts)
}

// TestWorkspaceHome_TakingTheOtherWordingSettlesTheConflict: a person who
// sends the wording of the write that did not land lands it, and the conflict
// is gone.
func TestWorkspaceHome_TakingTheOtherWordingSettlesTheConflict(t *testing.T) {
	a := conflicted(t)
	other := "Danke schön"
	if german(t, a)["thanks"] == other {
		other = "Vielen Dank"
	}
	res := keepGerman(t, a, "thanks", other)
	assert.Equal(t, change.OpApplied, res.Ops[0].Status)
	assert.Equal(t, other, german(t, a)["thanks"])
	conflicts, err := a.m.home.Conflicts(context.Background())
	require.NoError(t, err)
	assert.Empty(t, conflicts)
}

// TestWorkspaceHome_KeepingAnUncontestedWordingRecordsNothing: the same
// wording sent for a block no conflict names changes nothing and records
// nothing.
func TestWorkspaceHome_KeepingAnUncontestedWordingRecordsNothing(t *testing.T) {
	a, _ := twoMachines(t, map[string]string{"thanks": "Danke"})
	res := keepGerman(t, a, "thanks", "")
	assert.Equal(t, change.OpUnchanged, res.Ops[0].Status)
	assert.Nil(t, res.Record)
}
