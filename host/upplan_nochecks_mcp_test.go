//go:build !js

package host

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/project"
)

// callUpMCPTool calls one of the loop's MCP tools (up, up_plan) over a real
// session and decodes its structured result into out.
func callUpMCPTool(t *testing.T, a *App, name string, in map[string]any, out any) {
	t.Helper()
	ctx := context.Background()
	server := mcp.NewServer(&mcp.Implementation{Name: "kapi-test", Version: "test"}, nil)
	registerUpMCPTools(server, a)
	clientT, serverT := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(ctx, serverT, nil)
	require.NoError(t, err)
	defer serverSession.Close()
	session, err := mcp.NewClient(&mcp.Implementation{Name: "agent", Version: "test"}, nil).Connect(ctx, clientT, nil)
	require.NoError(t, err)
	defer session.Close()

	res, err := session.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: in})
	require.NoError(t, err)
	if res.IsError {
		t.Fatalf("%s: %v", name, toolErrorFrom(res))
	}
	require.NoError(t, json.Unmarshal(mustJSON(t, res.StructuredContent), out))
}

// TestUpPlanMCP_NoChecksPlansTheRunThatSkipsThem: over MCP, up_plan prices
// the run up makes with the same no_checks. The committed Norwegian
// translation drops the source's placeholder, so a checked run passes over
// the language, which the plan prices, and a run that skips the checks finds
// it up to date, so up_plan with no_checks prices nothing and up with
// no_checks makes no pass.
func TestUpPlanMCP_NoChecksPlansTheRunThatSkipsThem(t *testing.T) {
	t.Setenv("KAPI_CONFIG_DIR", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("KAPI_PLUGINS_DIR_ONLY", "1")
	t.Setenv("KAPI_PLUGINS_DIR", t.TempDir())
	t.Setenv("KAPI_NO_PROJECT", "1")
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "src"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "src", "en.json"), []byte(`{"greeting":"Hello %s, welcome."}`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "src", "nb.json"), []byte(`{"greeting":"Hei, velkommen."}`), 0o644))
	recipe := filepath.Join(dir, project.RecipeFileName)
	require.NoError(t, os.WriteFile(recipe, []byte(`version: v1
name: PlanChecks
defaults:
  source_language: en
  target_languages: [nb]
  translate_after: none
  flow: pseudo
collections:
  - name: app
    path: src/en.json
    target: src/{lang}.json
flows:
  pseudo:
    steps:
      - tool: pseudo-translate
`), 0o644))
	layout, err := project.LayoutFor(recipe)
	require.NoError(t, err)
	require.NoError(t, project.EnsureLayout(layout))
	app := func() *App {
		a := &App{}
		a.InitRegistries()
		t.Cleanup(a.Shutdown)
		return a
	}

	var run ConvergeOutput
	callUpMCPTool(t, app(), "up", map[string]any{"project": recipe, "no_checks": true}, &run)
	require.Zero(t, run.Passes, "without the checks the committed translation leaves nothing to do")

	var checked, unchecked UpPlanOutput
	callUpMCPTool(t, app(), "up_plan", map[string]any{"project": recipe}, &checked)
	callUpMCPTool(t, app(), "up_plan", map[string]any{"project": recipe, "no_checks": true}, &unchecked)
	assert.NotEmpty(t, checked.Scopes, "the placeholder the translation drops sends the language through a checked pass, which the plan prices")
	assert.Empty(t, unchecked.Scopes, "the run with no_checks passes over nothing, so its plan prices nothing")
}
