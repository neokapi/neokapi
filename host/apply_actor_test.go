package host

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/neokapi/neokapi/core/contextop"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// personApplies is a person applying a change-set from a terminal, the actor
// `kapi apply` stamps when the environment names no agent.
var personApplies = changeActor{Actor: contextop.Actor{Kind: contextop.ActorPerson}, Note: "applied with `kapi apply`"}

// apply_edits stamps every entry with the calling agent and its session, so the
// context policy refuses the term, memory and recipe entries an agent sends:
// writing one directly is a person's decision, and an agent records a
// suggestion instead. The content entry in the same change-set still lands.
func TestApplyEditsMCPStampsTheCallingAgent(t *testing.T) {
	app, _ := contextOpsApp(t)
	app.InitRegistries()
	root := contextOpsProject(t, "mcp-apply-actor")
	recipeBefore, err := os.ReadFile(recipeOf(root))
	require.NoError(t, err)

	server := mcp.NewServer(&mcp.Implementation{Name: "kapi", Version: "test"}, nil)
	registerEditMCPTools(server, app)
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(t.Context(), serverTransport, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = serverSession.Close() })
	client := mcp.NewClient(&mcp.Implementation{Name: "actor-test-agent", Version: "test"}, nil)
	session, err := client.Connect(t.Context(), clientTransport, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = session.Close() })

	content := filepath.Join(root, "config", "app.yaml")
	res, err := session.CallTool(t.Context(), &mcp.CallToolParams{
		Name: "apply_edits",
		Arguments: map[string]any{
			"project": recipeOf(root),
			"changeset": []any{
				map[string]any{
					"kind": "content", "file": content,
					"content_hash": model.ComputeContentHash("We utilise the widget every day."),
					"text":         "We use the widget every day.",
				},
				map[string]any{"kind": "term", "op": "upsert", "term": "leverage", "replacement": "use", "locale": "en", "status": "forbidden"},
				map[string]any{"kind": "memory", "op": "add", "source": "Goodbye", "target": "Ha det", "source_locale": "en", "target_locale": "nb"},
				map[string]any{"kind": "recipe", "op": "set", "path": "defaults.coordinates.brand"},
			},
		},
	})
	require.NoError(t, err)
	require.False(t, res.IsError, "%s", toolErrorText(res))
	body, err := json.Marshal(res.StructuredContent)
	require.NoError(t, err)
	var out applyEditsMCPOutput
	require.NoError(t, json.Unmarshal(body, &out))

	assert.Len(t, out.Applied, 1, "an agent applies content edits")
	assert.False(t, out.OK, "a refused entry means the change-set did not fully land")
	require.Len(t, out.Assets, 3)
	for _, asset := range out.Assets {
		assert.Equal(t, "error", asset.Status, "%s entry", asset.Kind)
		assert.Contains(t, asset.Detail, "agent actor-test-agent/"+MCPSessionID()+" may not edit",
			"the refusal names the calling agent and its session")
	}
	assert.Contains(t, out.Assets[0].Detail, "context_observe", "the refusal says what an agent does instead")

	assert.NotContains(t, projectTerms(t, app, root), "leverage")
	log, err := app.ContextOperations(t.Context(), ContextLogRequest{Project: recipeOf(root)})
	require.NoError(t, err)
	for _, op := range log.Operations {
		assert.NotEqual(t, contextop.KindEdit, op.Kind, "nothing was recorded as an edit")
	}
	recipeAfter, err := os.ReadFile(recipeOf(root))
	require.NoError(t, err)
	assert.Equal(t, string(recipeBefore), string(recipeAfter), "the recipe entry wrote nothing")
}

// toolErrorText is the text a failed tool call returned.
func toolErrorText(res *mcp.CallToolResult) string {
	var out strings.Builder
	for _, c := range res.Content {
		if text, ok := c.(*mcp.TextContent); ok {
			out.WriteString(text.Text)
		}
	}
	return out.String()
}
