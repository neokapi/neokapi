package host

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestApplyEditsMCPContentWordingField(t *testing.T) {
	app := newToolboxApp(t)
	file := filepath.Join(t.TempDir(), "page.json")
	const source = `{"title":"Before","keep":"Unchanged"}`
	require.NoError(t, os.WriteFile(file, []byte(source), 0o600))
	server := mcp.NewServer(&mcp.Implementation{Name: "kapi", Version: "test"}, nil)
	registerEditMCPTools(server, app)
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(t.Context(), serverTransport, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = serverSession.Close() })
	client := mcp.NewClient(&mcp.Implementation{Name: "edit-input-test", Version: "test"}, nil)
	session, err := client.Connect(t.Context(), clientTransport, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = session.Close() })
	entry := map[string]any{
		"kind": "content", "file": file, "content_hash": model.ComputeContentHash("Before"),
		"replacement": "After",
	}
	bad, err := session.CallTool(t.Context(), &mcp.CallToolParams{
		Name: "apply_edits", Arguments: map[string]any{"changeset": []any{entry}},
	})
	require.NoError(t, err)
	require.True(t, bad.IsError)
	body, err := json.Marshal(bad)
	require.NoError(t, err)
	assert.Contains(t, string(body), "put the new wording")
	assert.Contains(t, string(body), "voice rules")
	unchanged, err := os.ReadFile(file)
	require.NoError(t, err)
	assert.Equal(t, source, string(unchanged))
	delete(entry, "replacement")
	entry["text"] = "After"
	good, err := session.CallTool(t.Context(), &mcp.CallToolParams{
		Name: "apply_edits", Arguments: map[string]any{"changeset": []any{entry}},
	})
	require.NoError(t, err)
	require.False(t, good.IsError)
	body, err = json.Marshal(good.StructuredContent)
	require.NoError(t, err)
	var result applyEditsMCPOutput
	require.NoError(t, json.Unmarshal(body, &result))
	assert.True(t, result.OK)
	assert.Len(t, result.Applied, 1)
	after, err := os.ReadFile(file)
	require.NoError(t, err)
	assert.Equal(t, `{"title":"After","keep":"Unchanged"}`, string(after))
}

func TestApplyCLIRejectsVoiceWordingFieldBeforeEditing(t *testing.T) {
	app := newToolboxApp(t)
	file := filepath.Join(t.TempDir(), "page.json")
	const source = `{"title":"Before","keep":"Unchanged"}`
	require.NoError(t, os.WriteFile(file, []byte(source), 0o600))
	entries := []changeEntry{
		{Kind: kindContent, File: file, ContentHash: model.ComputeContentHash("Before"), Text: "After"},
		{Kind: kindContent, File: file, Replacement: "Wrong field"},
	}
	body, err := json.Marshal(entries)
	require.NoError(t, err)
	changeset := filepath.Join(t.TempDir(), "edits.json")
	require.NoError(t, os.WriteFile(changeset, body, 0o600))
	err = app.RunApply(NewEnvCommand(t.Context(), "apply"), changeset, false, "", true)
	require.ErrorContains(t, err, "content entry 2")
	require.ErrorContains(t, err, `"text"`)
	after, err := os.ReadFile(file)
	require.NoError(t, err)
	assert.Equal(t, source, string(after))
	assert.NoError(t, validateContentWording([]changeEntry{{Kind: kindVoice, Replacement: "Preferred term"}}))
}
