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
	assert.Contains(t, string(body), "term entries")
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
