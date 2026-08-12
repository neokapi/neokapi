package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/neokapi/neokapi/cli"
	"github.com/neokapi/neokapi/core/segment"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMCPToolSurfaceSnapshot locks the kapi MCP surface as a stable
// contract: the tool NAMES and INPUT SCHEMAS exposed by `kapi mcp` (ad-hoc
// mode — the full set) are snapshotted to testdata/mcp_tools.golden.json.
// Descriptions may evolve freely; names and input schemas may only be
// EXTENDED (new tools, new optional fields) — renaming a tool, removing one,
// or changing a field's type breaks agent integrations and needs an
// explicit, documented decision.
//
// To regenerate after an intentional change:
//
//	KAPI_UPDATE_GOLDEN=1 go test ./cmd/kapi -run TestMCPToolSurfaceSnapshot -tags fts5
//
// and note the change in web/docs/reference/cli-contract.md (MCP section).
func TestMCPToolSurfaceSnapshot(t *testing.T) {
	// Hermetic engine registry: this package's root.go init() runs
	// InitPluginHost at process start, so on a developer machine plugin
	// discovery registers machine-dependent segment engines (e.g. a
	// brew-installed kapi-sat) into the process-wide registry before any
	// test code — env isolation cannot help — and the segmentation tool's
	// schema would embed them into the golden, which CI (no plugins) can
	// never reproduce. The stable contract is the builtin binary's engines;
	// pin exactly those and restore the live registry afterwards.
	liveEngines := segment.SnapshotEnginesForTest()
	t.Cleanup(func() { segment.ReplaceEnginesForTest(liveEngines) })
	builtins := map[string]segment.EngineDescriptor{}
	for _, name := range []string{"srx", "uax29", "llm"} {
		if d, ok := liveEngines[name]; ok {
			builtins[name] = d
		}
	}
	segment.ReplaceEnginesForTest(builtins)

	app := &cli.App{}
	app.InitRegistries()

	server := mcp.NewServer(&mcp.Implementation{Name: "kapi", Version: "test"}, nil)
	cli.ApplyMCPToolFactories(server, app)

	// Enumerate the surface exactly as a client sees it: connect over an
	// in-memory transport pair and issue tools/list.
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(ctx, serverTransport, nil)
	require.NoError(t, err)
	defer serverSession.Close()

	client := mcp.NewClient(&mcp.Implementation{Name: "snapshot-test", Version: "test"}, nil)
	session, err := client.Connect(ctx, clientTransport, nil)
	require.NoError(t, err)
	defer session.Close()

	var tools []*mcp.Tool
	params := &mcp.ListToolsParams{}
	for {
		res, err := session.ListTools(ctx, params)
		require.NoError(t, err)
		tools = append(tools, res.Tools...)
		if res.NextCursor == "" {
			break
		}
		params.Cursor = res.NextCursor
	}
	require.NotEmpty(t, tools)

	type toolSnapshot struct {
		Name        string          `json:"name"`
		InputSchema json.RawMessage `json:"input_schema"`
	}
	snapshot := make([]toolSnapshot, 0, len(tools))
	for _, tool := range tools {
		schemaJSON, err := json.Marshal(tool.InputSchema)
		require.NoError(t, err, "tool %s input schema", tool.Name)
		snapshot = append(snapshot, toolSnapshot{Name: tool.Name, InputSchema: schemaJSON})
	}
	sort.Slice(snapshot, func(i, j int) bool { return snapshot[i].Name < snapshot[j].Name })

	got, err := json.MarshalIndent(snapshot, "", "  ")
	require.NoError(t, err)
	got = append(got, '\n')

	golden := filepath.Join("testdata", "mcp_tools.golden.json")
	if os.Getenv("KAPI_UPDATE_GOLDEN") != "" {
		require.NoError(t, os.MkdirAll(filepath.Dir(golden), 0o755))
		require.NoError(t, os.WriteFile(golden, got, 0o644))
		return
	}
	want, err := os.ReadFile(golden)
	require.NoError(t, err, "golden file missing — run with KAPI_UPDATE_GOLDEN=1 to create it")
	assert.Equal(t, string(want), string(got),
		"MCP tool surface drift — the tool names + input schemas are a stable contract; "+
			"if this change is intentional (additive), regenerate with KAPI_UPDATE_GOLDEN=1 "+
			"and document it in web/docs/reference/cli-contract.md")
}

// TestMCPResourceSurfaceSnapshot locks the addresses `kapi mcp` answers at.
//
// A resource URI is an address a caller writes into its own prompts and
// configuration, so it is a contract in exactly the way a tool name is: it may
// be added to, never renamed or dropped without an explicit decision. The mime
// type is snapshotted with it, because the rendering is a property of the
// address rather than a second one.
//
// Regenerate after an intentional change with:
//
//	KAPI_UPDATE_GOLDEN=1 go test ./cmd/kapi -run TestMCPResourceSurfaceSnapshot -tags fts5
func TestMCPResourceSurfaceSnapshot(t *testing.T) {
	app := &cli.App{}
	app.InitRegistries()

	server := mcp.NewServer(&mcp.Implementation{Name: "kapi", Version: "test"}, nil)
	cli.ApplyMCPToolFactories(server, app)

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(ctx, serverTransport, nil)
	require.NoError(t, err)
	defer serverSession.Close()

	client := mcp.NewClient(&mcp.Implementation{Name: "snapshot-test", Version: "test"}, nil)
	session, err := client.Connect(ctx, clientTransport, nil)
	require.NoError(t, err)
	defer session.Close()

	type resourceSnapshot struct {
		URITemplate string `json:"uri_template"`
		Name        string `json:"name"`
		MIMEType    string `json:"mime_type"`
	}
	var snapshot []resourceSnapshot
	params := &mcp.ListResourceTemplatesParams{}
	for {
		res, err := session.ListResourceTemplates(ctx, params)
		require.NoError(t, err)
		for _, tmpl := range res.ResourceTemplates {
			snapshot = append(snapshot, resourceSnapshot{
				URITemplate: tmpl.URITemplate, Name: tmpl.Name, MIMEType: tmpl.MIMEType,
			})
		}
		if res.NextCursor == "" {
			break
		}
		params.Cursor = res.NextCursor
	}
	require.NotEmpty(t, snapshot, "the context:// addresses are the by-location retrieval surface")
	sort.Slice(snapshot, func(i, j int) bool { return snapshot[i].URITemplate < snapshot[j].URITemplate })

	got, err := json.MarshalIndent(snapshot, "", "  ")
	require.NoError(t, err)
	got = append(got, '\n')

	golden := filepath.Join("testdata", "mcp_resources.golden.json")
	if os.Getenv("KAPI_UPDATE_GOLDEN") != "" {
		require.NoError(t, os.MkdirAll(filepath.Dir(golden), 0o755))
		require.NoError(t, os.WriteFile(golden, got, 0o644))
		return
	}
	want, err := os.ReadFile(golden)
	require.NoError(t, err, "golden file missing — run with KAPI_UPDATE_GOLDEN=1 to create it")
	assert.Equal(t, string(want), string(got),
		"MCP resource surface drift — the addresses are a stable contract; if this change is "+
			"intentional (additive), regenerate with KAPI_UPDATE_GOLDEN=1 and document it in "+
			"web/docs/reference/cli-contract.md")
}
