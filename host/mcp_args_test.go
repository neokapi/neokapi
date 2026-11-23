//go:build !js

package host

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fullSurfaceSession connects a client to a server carrying every tool kapi's
// host registers, the widest surface `kapi mcp --all` serves, with the roots
// middleware in front.
func fullSurfaceSession(t *testing.T, app *App) (*mcp.ClientSession, context.Context) {
	t.Helper()
	app.MCPSurface = MCPSurface{AllTools: true, AllFlows: true}
	return rootsSession(t, app, nil)
}

// Every string argument whose name or description reads as a location is
// declared, so the roots middleware checks it on every call.
func TestEveryMCPPathArgumentIsDeclared(t *testing.T) {
	isolateCheckExecution(t)
	alpha := scopeProject(t, "alpha", "translation memory", "content memory", "nb")
	t.Chdir(alpha)
	session, ctx := fullSurfaceSession(t, scopeApp(t, alpha))
	undeclared, err := UndeclaredMCPPathArgs(ctx, session)
	require.NoError(t, err)
	assert.Empty(t, undeclared, "declare each with an x-path schema annotation (path=file) or DeclareMCPPathArgs")
}

// An argument a tool's schema does not list is refused before the tool sees
// it; a field the schema hides is never decoded into the tool's config.
func TestMCPRefusesAnArgumentTheSchemaDoesNotList(t *testing.T) {
	isolateCheckExecution(t)
	alpha := scopeProject(t, "alpha", "translation memory", "content memory", "nb")
	t.Chdir(alpha)
	outside := t.TempDir()
	session, ctx := fullSurfaceSession(t, scopeApp(t, alpha))

	vault := filepath.Join(outside, "newvault.json")
	res := callTool(t, ctx, session, "redact", map[string]any{"text": "Ada", "vault_path": vault})
	require.True(t, res.IsError)
	assert.Contains(t, resultText(res), "takes no argument vault_path")
	assert.NoFileExists(t, vault)
}

// A declared location in a framework tool's config is checked like any other.
func TestMCPChecksAToolConfigPath(t *testing.T) {
	isolateCheckExecution(t)
	alpha := scopeProject(t, "alpha", "translation memory", "content memory", "nb")
	t.Chdir(alpha)
	outside := t.TempDir()
	rules := filepath.Join(outside, "rules.yaml")
	require.NoError(t, os.WriteFile(rules, []byte("secret-opening-characters"), 0o600))
	session, ctx := fullSurfaceSession(t, scopeApp(t, alpha))

	res := callTool(t, ctx, session, "redact", map[string]any{"text": "Ada", "rules_path": rules})
	require.True(t, res.IsError)
	assert.Contains(t, resultText(res), "rules_path")
	assert.NotContains(t, resultText(res), "secret-opening")
}
