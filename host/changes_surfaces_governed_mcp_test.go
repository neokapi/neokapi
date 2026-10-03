//go:build !js

package host

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/projector"
)

func TestMCPApplyEdits_RunsTheCommitCheckAndRecordsTheEdit(t *testing.T) {
	f := newCommitFixture(t)
	f.app.InitRegistries()
	const doc = "docs/guide.md"
	original := readFile(t, f.recipe, doc)
	session := editSession(t, f.app, "governed-agent")

	var page mcpPage
	isErr, body := callEditTool(t, session, "read_blocks", map[string]any{"doc": doc, "project": f.recipe}, &page)
	require.False(t, isErr, body)
	ref, rev, _ := page.blockWith(t, "every day")
	setContent := func(text string) map[string]any {
		return map[string]any{"project": f.recipe, "ops": []any{map[string]any{"op": "set_content", "at": ref, "if_match": rev, "text": text}}}
	}

	var res change.Result
	isErr, body = callEditTool(t, session, "apply_edits", setContent("We utilize the widget every day."), &res)
	assert.True(t, isErr, "a refused change set is an error result")
	require.Equal(t, change.SetRefused, res.Status, body)
	require.NotNil(t, res.Ops[0].Error)
	assert.Equal(t, change.CodeGateFailed, res.Ops[0].Error.Code)
	assert.Contains(t, findingRules(res.Ops[0].Findings), "terms.vocabulary")
	assert.Nil(t, res.Record)
	assert.Equal(t, original, readFile(t, f.recipe, doc), "a refused edit writes nothing")
	assert.Empty(t, editOps(t, f.app, f.root), "a refused edit records nothing")

	res = change.Result{}
	isErr, body = callEditTool(t, session, "apply_edits", setContent("We use the widget each day."), &res)
	require.False(t, isErr, body)
	require.Equal(t, change.SetApplied, res.Status, body)
	assert.Equal(t, "# Guide\n\nWe use the widget each day.\n", readFile(t, f.recipe, doc))
	require.NotNil(t, res.Record, "apply_edits records what it landed")
	ops := editOps(t, f.app, f.root)
	require.Len(t, ops, 1)
	assert.Equal(t, projector.KindEdit, ops[0].Kind)
	assert.Equal(t, *res.Record, ops[0].ID)
	e := editPayload(t, ops[0].Payload)
	assert.Equal(t, change.Actor{Kind: change.ActorAgent, Name: "governed-agent", Session: MCPSessionID()}, e.Actor)
	assert.Equal(t, mcpChangeOrigin, e.Origin.By)
	assert.NotEmpty(t, e.Fingerprint)
}
