package host

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/contextop"
	"github.com/neokapi/neokapi/core/profile"
)

// growthSession connects a client to a server carrying the context-growth
// tools, the way an assistant connects to `kapi mcp`.
func growthSession(t *testing.T, app *App) *mcp.ClientSession {
	t.Helper()
	server := mcp.NewServer(&mcp.Implementation{Name: "kapi", Version: "test"}, nil)
	registerContextGrowthMCPTools(server, app)
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(t.Context(), serverTransport, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = serverSession.Close() })
	client := mcp.NewClient(&mcp.Implementation{Name: "withdraw-test", Version: "test"}, nil)
	session, err := client.Connect(t.Context(), clientTransport, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = session.Close() })
	return session
}

// callRecord calls one growth tool and reads the operation it answers with.
func callRecord(t *testing.T, session *mcp.ClientSession, name string, args map[string]any) (contextRecordOutput, *mcp.CallToolResult) {
	t.Helper()
	res, err := session.CallTool(t.Context(), &mcp.CallToolParams{Name: name, Arguments: args})
	require.NoError(t, err)
	var out contextRecordOutput
	if !res.IsError {
		body, err := json.Marshal(res.StructuredContent)
		require.NoError(t, err)
		require.NoError(t, json.Unmarshal(body, &out))
	}
	return out, res
}

func resultText(res *mcp.CallToolResult) string {
	var text strings.Builder
	for _, c := range res.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			text.WriteString(tc.Text)
		}
	}
	return text.String()
}

// An agent that entered a correction backwards takes it back in the same
// session, and it stops advising.
func TestContextWithdrawTakesBackThisSessionsCandidate(t *testing.T) {
	app, _ := contextOpsApp(t)
	root := contextOpsProject(t, "withdraw-own")
	session := growthSession(t, app)

	recorded, res := callRecord(t, session, "context_correct", map[string]any{
		"from": "use", "to": "utilise", "path": "config/app.yaml", "propose": true,
		"project": recipeOf(root),
	})
	require.False(t, res.IsError, resultText(res))
	require.NotEmpty(t, recorded.Operation)

	withdrawn, res := callRecord(t, session, "context_withdraw", map[string]any{
		"operation": recorded.Operation, "note": "entered backwards", "project": recipeOf(root),
	})
	require.False(t, res.IsError, resultText(res))
	assert.Equal(t, string(contextop.KindDiscard), withdrawn.Kind)
	assert.Equal(t, "withdrew operation "+recorded.Operation, withdrawn.Recorded)

	log, err := app.ContextOperations(t.Context(), ContextLogRequest{Project: recipeOf(root), Status: contextop.StatusCandidate})
	require.NoError(t, err)
	for _, op := range log.Operations {
		assert.NotEqual(t, recorded.Operation, op.ID, "a withdrawn correction is no longer a candidate")
	}
}

// Another session's operation is not this session's to withdraw, and neither
// is a rule a person confirmed.
func TestContextWithdrawRefusesWhatIsNotThisSessions(t *testing.T) {
	app, _ := contextOpsApp(t)
	root := contextOpsProject(t, "withdraw-refused")
	session := growthSession(t, app)

	other, err := app.ProposeContextRule(t.Context(), ContextProposeRequest{
		Actor:    contextop.Actor{Kind: contextop.ActorAgent, Name: "codex", Session: "s-earlier"},
		Project:  recipeOf(root),
		Term:     &profile.TermRule{Term: "utilise", Replacement: "use"},
		Evidence: []contextop.Evidence{{Path: "config/app.yaml"}},
	})
	require.NoError(t, err)
	_, res := callRecord(t, session, "context_withdraw", map[string]any{
		"operation": other.ID, "project": recipeOf(root),
	})
	require.True(t, res.IsError, "another session's candidate is a person's to decide")
	assert.Contains(t, resultText(res), "another actor")

	mine, res := callRecord(t, session, "context_propose", map[string]any{
		"term": "widget", "use": "gadget", "path": "config/app.yaml", "project": recipeOf(root),
	})
	require.False(t, res.IsError, resultText(res))
	_, err = app.ConfirmContextOperation(t.Context(), ContextConfirmRequest{
		Actor:   contextop.Actor{Kind: contextop.ActorPerson, Name: "ada"},
		Project: recipeOf(root),
		ID:      mine.Operation,
	})
	require.NoError(t, err)
	_, res = callRecord(t, session, "context_withdraw", map[string]any{
		"operation": mine.Operation, "project": recipeOf(root),
	})
	require.True(t, res.IsError, "a confirmed rule is a person's to withdraw")
	assert.Contains(t, resultText(res), "confirmed")

	_, res = callRecord(t, session, "context_withdraw", map[string]any{"operation": " ", "project": recipeOf(root)})
	require.True(t, res.IsError)
	assert.Contains(t, resultText(res), "`operation`")
}
