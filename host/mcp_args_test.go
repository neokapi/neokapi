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
)

// fullSurfaceSession connects a client to a server carrying every tool kapi's
// host registers, the widest surface `kapi mcp --all` serves, with the roots
// middleware in front.
func fullSurfaceSession(t *testing.T, app *App) (*mcp.ClientSession, context.Context) {
	t.Helper()
	app.MCPSurface = MCPSurface{AllTools: true, AllFlows: true}
	return rootsSession(t, app, nil)
}

// Every argument that can carry a string is declared as a location or as not
// one, so the roots middleware checks every location on every call.
func TestEveryMCPPathArgumentIsDeclared(t *testing.T) {
	isolateCheckExecution(t)
	alpha := scopeProject(t, "alpha", "translation memory", "content memory", "nb")
	t.Chdir(alpha)
	session, ctx := fullSurfaceSession(t, scopeApp(t, alpha))
	undeclared, err := UndeclaredMCPPathArgs(ctx, session)
	require.NoError(t, err)
	assert.Empty(t, undeclared, "declare each in the schema (path=file, path=directory or path=none in a "+
		"framework tool's schema tag) or with DeclareMCPPathArgs (host/mcp_args.go)")
}

// auditTool lists one tool with schema and returns what the audit reports for
// it.
func auditTool(t *testing.T, name string, schema map[string]any) []string {
	t.Helper()
	server := mcp.NewServer(&mcp.Implementation{Name: "audit-test", Version: "test"}, nil)
	server.AddTool(&mcp.Tool{Name: name, InputSchema: schema}, func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return &mcp.CallToolResult{}, nil
	})
	ctx := t.Context()
	st, ct := mcp.NewInMemoryTransports()
	ss, err := server.Connect(ctx, st, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = ss.Close() })
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "audit-test", Version: "test"}, nil).Connect(ctx, ct, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = cs.Close() })
	got, err := UndeclaredMCPPathArgs(ctx, cs)
	require.NoError(t, err)
	return got
}

// The audit decides by declaration, never by name: a neutrally named string,
// a string inside an object, a string array, a map of strings, an open
// object and a value of any type each need a declaration, and a declaration
// of an argument the schema lacks is reported.
func TestMCPAuditDecidesByDeclarationNotName(t *testing.T) {
	str := map[string]any{"type": "string"}
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"subject":  str,
			"count":    map[string]any{"type": "integer"},
			"tags":     map[string]any{"type": "array", "items": str},
			"labels":   map[string]any{"type": "object", "additionalProperties": str},
			"options":  map[string]any{"type": "object"},
			"anything": map[string]any{"description": "untyped"},
			"spec": map[string]any{"type": "object", "properties": map[string]any{
				"where": str,
				"what":  map[string]any{"type": "string", "x-not-location": true},
			}},
			"src":   map[string]any{"type": "string", "x-path": map[string]any{"type": "file"}},
			"words": map[string]any{"type": "array", "items": str, "x-not-location": true},
		},
	}
	assert.Equal(t, []string{
		"audit_neutral.anything", "audit_neutral.labels.*", "audit_neutral.options",
		"audit_neutral.spec.where", "audit_neutral.subject", "audit_neutral.tags",
	}, auditTool(t, "audit_neutral", schema))

	DeclareMCPPathArgs("audit_declared", MCPArgPath, "subject", "spec.where")
	DeclareMCPPathArgs("audit_declared", MCPArgNotLocation, "tags", "labels", "options", "anything", "gone")
	assert.Equal(t, []string{"audit_declared.gone (declared, not in the schema)"}, auditTool(t, "audit_declared", schema))
}

// A location declared inside a structure is checked where the path leads,
// through arrays and map values, and a location declared on an open object
// covers every string in it.
func TestMCPChecksANestedDeclaredLocation(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "secret.txt")
	ctx := WithMCPRoots(t.Context(), []string{root})
	str := map[string]any{"type": "string"}
	schema := readMCPInputSchema(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"ops": map[string]any{"type": "array", "items": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"at":   map[string]any{"type": "object", "properties": map[string]any{"doc": str, "block": str}},
					"doc":  map[string]any{"type": "string", "x-not-location": true},
					"bag":  map[string]any{"type": "object"},
					"tags": map[string]any{"type": "object", "additionalProperties": map[string]any{"type": "string", "x-path": map[string]any{}}},
				},
			}},
		},
	})
	DeclareMCPPathArgs("nested_check", MCPArgProjectRelative, "ops.at.doc")
	DeclareMCPPathArgs("nested_check", MCPArgPath, "ops.bag")
	DeclareMCPPathArgs("nested_check", MCPArgNotLocation, "ops.at.block")

	call := func(op map[string]any) error {
		raw, err := json.Marshal(map[string]any{"ops": []any{map[string]any{"at": map[string]any{"doc": "a.md"}}, op}})
		require.NoError(t, err)
		return checkMCPCallArgs(ctx, "nested_check", schema, raw)
	}
	require.NoError(t, call(map[string]any{"at": map[string]any{"doc": "docs/b.md", "block": "../not-a-path"}, "doc": outside}),
		"a block key and a field declared as no location are not checked")

	err := call(map[string]any{"at": map[string]any{"doc": "../b.md"}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ops.at.doc")

	err = call(map[string]any{"bag": map[string]any{"deep": []any{outside}}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ops.bag")

	err = call(map[string]any{"tags": map[string]any{"any": outside}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ops.tags.*")
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
