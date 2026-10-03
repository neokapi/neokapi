//go:build !js

package host

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// callMCPUp runs the registered MCP `up` tool over a real session — the path an
// agent takes — and returns the structured result.
func callMCPUp(t *testing.T, a *App, in map[string]any) (*ConvergeOutput, error) {
	t.Helper()
	ctx := context.Background()
	server := mcp.NewServer(&mcp.Implementation{Name: "kapi-test", Version: "test"}, nil)
	registerUpMCPTools(server, a)

	clientT, serverT := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(ctx, serverT, nil)
	require.NoError(t, err)
	defer serverSession.Close()

	client := mcp.NewClient(&mcp.Implementation{Name: "agent", Version: "test"}, nil)
	session, err := client.Connect(ctx, clientT, nil)
	require.NoError(t, err)
	defer session.Close()

	res, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "up", Arguments: in})
	if err != nil {
		return nil, err
	}
	if res.IsError {
		return nil, toolErrorFrom(res)
	}
	var out ConvergeOutput
	require.NoError(t, json.Unmarshal(mustJSON(t, res.StructuredContent), &out))
	return &out, nil
}

// TestMCPUp_RunsAtTheServerVenue is the core regression: over MCP, a
// server-connected project's `up` reaches the venue plumbing — with the project
// and the structured stream it needs — and returns the run's result.
func TestMCPUp_RunsAtTheServerVenue(t *testing.T) {
	recipe := newVenueTestProject(t, true)
	stub := newServerUpStub(t)
	a := &App{PluginHost: stub.host}

	out, err := callMCPUp(t, a, map[string]any{"project": recipe, "passes": 2})
	require.NoError(t, err)

	require.NotNil(t, out)
	assert.Equal(t, "server-venue", out.Flow, "the result must be the venue run's, not a local loop's")
	assert.Equal(t, 2, out.Passes)
	assert.True(t, out.Converged)
	assert.Equal(t, 3, out.MaterializedFiles)

	args := stub.dispatchedArgs(t)
	require.NotEmpty(t, args, "the agent's up must dispatch to the venue plumbing")
	assert.Equal(t, []string{"command", serverUpCommand}, args[:2], "dispatch is a Mode-A command")
	assert.Contains(t, args, "--project="+recipe, "the plumbing must be pointed at this recipe, not a discovered one")
	assert.Contains(t, args, "--json", "an embedded run reads the structured document, and --json is also what stops the prompt")
	assert.Contains(t, args, "--passes=2")
}

// TestMCPUp_LocalProjectStillRunsHere is the control: without a bound venue
// nothing is dispatched, so a plain project keeps the local loop.
func TestMCPUp_LocalProjectStillRunsHere(t *testing.T) {
	recipe := newVenueTestProject(t, false)
	stub := newServerUpStub(t)
	a := &App{PluginHost: stub.host}
	a.SourceLang = "en"

	out, err := callMCPUp(t, a, map[string]any{"project": recipe, "passes": 1, "no_checks": true})
	require.NoError(t, err)
	require.NotNil(t, out)
	assert.Empty(t, stub.dispatchedArgs(t), "a project binding no venue must not reach the plumbing")
	assert.NotEqual(t, "server-venue", out.Flow)
}
