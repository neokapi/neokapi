package workhome_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/change/filehome"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/workhome"
)

// The block history keeps, for each write to an edition the workspace home
// keeps, the kinds of the operations that made it and the tool that made it,
// as it keeps them for a write to a file: a flow's draft names its tool and
// the kind change.Diff would send, a change set names the kinds it sent, and
// a delivery's release removes the edition from the workspace home.

// historyStep is one row of a block's history: the kinds and the tool.
type historyStep struct {
	ops  []string
	tool string
}

// stepsOf reads the history of one block of the German edition of a.json,
// oldest first.
func stepsOf(t *testing.T, m *machine, block string) []historyStep {
	t.Helper()
	rows, err := m.st.History.Edition(context.Background(), "a.json", block, "de", 0)
	require.NoError(t, err)
	out := make([]historyStep, len(rows))
	for i, r := range rows {
		out[len(rows)-1-i] = historyStep{ops: r.Ops, tool: r.Tool}
	}
	return out
}

func TestWorkspaceHome_RecordsTheOperationKindsAndTheTool(t *testing.T) {
	ctx := context.Background()
	m := newMachine(t, t.TempDir())
	f := newKeptFixture(t, m, map[string]string{"a.json": `{"greeting": "Hello there", "farewell": "Goodbye now"}` + "\n"}, filehome.Options{})

	// A flow drafts the greeting, then drafts it again with more words.
	draft := func(text, before string) {
		t.Helper()
		res, err := m.home.Produce(ctx, workhome.Produce{Doc: "a.json", Edition: de,
			Actor: change.Actor{Kind: change.ActorTool, Name: "translate"}, Origin: "flow:translate",
			Blocks: []workhome.Produced{{Block: "greeting", Before: before, Tool: "ai-translate",
				Edition: model.Edition{Runs: []model.Run{model.TextR(text)}, Status: model.Status(model.TargetStatusDraft),
					Origin: model.Origin{Kind: model.OriginAI, Tool: "ai-translate"}}}}})
		require.NoError(t, err)
		require.Equal(t, 1, res.Written)
	}
	draft("Hallo", model.AbsentRevision)
	held, err := m.home.Edition(ctx, "a.json", de)
	require.NoError(t, err)
	draft("Hallo zusammen", model.RunsRevision(de, held.Blocks["greeting"].Runs))

	// A person sets it, then changes a word of it.
	setGerman(t, f, "greeting", "Servus")
	page, err := f.svc.Read(ctx, change.ReadRequest{Doc: "de/a.json", Blocks: []string{"greeting"}})
	require.NoError(t, err)
	find := "Servus"
	res, err := f.svc.Apply(ctx, change.Set{Ops: []change.Op{{Kind: change.KindReplaceText, At: page.Blocks[0].Ref,
		IfMatch: page.Blocks[0].Rev, Body: &change.ReplaceText{Edits: []change.TextEdit{{Find: &find, Text: "Grüß Gott"}}}}}}, person)
	require.NoError(t, err)
	require.Equal(t, change.SetApplied, res.Status, "%+v", res.Ops)

	assert.Equal(t, []historyStep{
		{ops: []string{"set_content"}, tool: "ai-translate"},
		{ops: []string{"replace_text"}, tool: "ai-translate"},
		{ops: []string{"set_content"}},
		{ops: []string{"replace_text"}},
	}, stepsOf(t, m, "greeting"))

	// A person drafts the farewell, and a delivery that wrote it into its
	// file releases it from the workspace home.
	f.draft(t, "a.json", map[string]string{"farewell": "Tschüss"})
	delivered, err := m.home.Read(ctx, "a.json", de)
	require.NoError(t, err)
	_, err = m.home.Release(ctx, "a.json", de, workhome.Release{Token: delivered.Token, Blocks: []string{"farewell"}, Actor: merge, Origin: "merge"})
	require.NoError(t, err)
	assert.Equal(t, []historyStep{
		{ops: []string{"set_content"}},
		{ops: []string{"remove_edition"}},
	}, stepsOf(t, m, "farewell"))
}
