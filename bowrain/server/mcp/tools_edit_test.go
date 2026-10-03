package mcp

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/bowrain/changes"
	"github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/model"
)

// newEditMCPServer is an MCP server over a real PostgreSQL content store whose
// edit tools go through the stream home, with no policy of its own: the
// server's policy for an agent is tested where it is built.
func newEditMCPServer(t *testing.T) (*MCPServer, store.ContentStore) {
	t.Helper()
	cs := newTestContentStore(t)
	factory := func(ctx context.Context, _, projectID, stream string) (*change.Service, func(context.Context, *change.Result), error) {
		proj, err := cs.GetProject(ctx, projectID)
		if err != nil {
			return nil, nil, err
		}
		home := &changes.Home{Store: cs, ProjectID: proj.ID, Stream: stream, SourceLocale: proj.DefaultSourceLanguage, Locales: proj.TargetLanguages}
		return changes.NewService(home, nil), nil, nil
	}
	ms, err := NewMCPServerWithStore(&memVoiceStore{}, cs, Config{}, WithChangeService(factory))
	require.NoError(t, err)
	return ms, cs
}

// callWith is a tool call carrying args.
func callWith(t *testing.T, args any) *mcp.CallToolRequest {
	t.Helper()
	raw, err := json.Marshal(args)
	require.NoError(t, err)
	return &mcp.CallToolRequest{Params: &mcp.CallToolParamsRaw{Arguments: raw}}
}

// toolResult decodes a tool result's structured content.
func toolResult[T any](t *testing.T, res *mcp.CallToolResult) T {
	t.Helper()
	var out T
	raw, ok := res.StructuredContent.(json.RawMessage)
	require.True(t, ok, "the result carries its structured content")
	require.NoError(t, json.Unmarshal(raw, &out))
	return out
}

// An agent reads an item's blocks, edits a translation on the revision it
// read, and is refused with the current wording when the translation moved
// since. The edit it lands is recorded as the agent's.
func TestApplyEdits_EditsATranslationOnTheRevisionItRead(t *testing.T) {
	ms, cs := newEditMCPServer(t)
	ctx := t.Context()
	p := &store.Project{Name: "Edits", DefaultSourceLanguage: model.LocaleEnglish, TargetLanguages: []model.LocaleID{"fr"}}
	require.NoError(t, cs.CreateProject(ctx, p))
	require.NoError(t, cs.StoreItem(ctx, p.ID, "main", &store.Item{Name: "en.json", Format: "json"}))
	b := model.NewBlock("greeting", "Hello")
	b.Name = "greeting"
	b.SetTargetText("fr", "Bonjour")
	require.NoError(t, cs.StoreBlocksForItem(ctx, p.ID, "main", "en.json", []*model.Block{b}))

	res, err := ms.readBlocks(ctx, nil, readBlocksInput{ProjectID: p.ID, Doc: "en.json"})
	require.NoError(t, err)
	page := toolResult[change.Page](t, res)
	require.Len(t, page.Blocks, 1)
	block := page.Blocks[0]
	assert.Equal(t, "greeting", block.Ref.Block)
	fr := block.Editions["fr"]
	require.Equal(t, "Bonjour", fr.Text)

	edit := func(rev, text string) *mcp.CallToolResult {
		t.Helper()
		ref := block.Ref
		ref.Edition = model.EditionKey{Locale: "fr"}
		out, err := ms.applyEdits(ctx, callWith(t, map[string]any{
			"project_id": p.ID,
			"ops":        []change.Op{{Kind: change.KindSetContent, At: ref, IfMatch: rev, Body: &change.SetContent{Text: &text}}},
		}))
		require.NoError(t, err)
		return out
	}

	sb, err := cs.(store.BlockWriteStore).ItemBlocks(ctx, p.ID, "main", "en.json", nil)
	require.NoError(t, err)
	sb[0].Block.SetTargetText("fr", "Salut")
	require.NoError(t, cs.StoreBlocks(ctx, p.ID, "main", []*model.Block{sb[0].Block}))

	stale := edit(fr.Rev, "Coucou")
	assert.True(t, stale.IsError, "a refused change set is an error result")
	refused := toolResult[change.Result](t, stale)
	require.Equal(t, change.SetRefused, refused.Status)
	assert.Equal(t, change.CodeStale, refused.Ops[0].Error.Code)
	assert.Equal(t, "Salut", refused.Ops[0].Current.Text, "the refusal carries the wording that stands")

	landed := toolResult[change.Result](t, edit(refused.Ops[0].Current.Rev, "Coucou"))
	require.Equal(t, change.SetApplied, landed.Status, "%+v", landed.Ops)

	rows, err := cs.(store.BlockWriteStore).ItemBlocks(ctx, p.ID, "main", "en.json", nil)
	require.NoError(t, err)
	target := rows[0].Block.Target("fr")
	require.NotNil(t, target)
	assert.Equal(t, "Coucou", model.RunsText(target.Runs))
	assert.Equal(t, model.OriginAgent, target.Origin.Kind, "an agent's edit is recorded as the agent's")
}

// A change set that names no project, or arguments that are not a change set,
// are refused as invalid without reaching a stream.
func TestApplyEdits_ArgumentsThatAreNotAChangeSetAreRefused(t *testing.T) {
	ms, _ := newEditMCPServer(t)
	res, err := ms.applyEdits(t.Context(), &mcp.CallToolRequest{Params: &mcp.CallToolParamsRaw{Arguments: json.RawMessage(`[1]`)}})
	require.NoError(t, err)
	assert.True(t, res.IsError)
	refused := toolResult[change.Result](t, res)
	require.NotNil(t, refused.Error)
	assert.Equal(t, change.CodeInvalid, refused.Error.Code)
}
