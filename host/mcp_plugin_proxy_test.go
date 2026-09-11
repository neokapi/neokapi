package host

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/neokapi/neokapi/core/plugin/manifest"
	"github.com/neokapi/neokapi/host/pluginhost"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// helperEnv names the mode a re-executed test binary serves as a fake plugin.
const helperEnv = "KAPI_TEST_PLUGIN_MCP"

// TestPluginMCPHelperProcess is the fake plugin binary. It is a real MCP server
// over stdio, spawned by the proxy exactly as a plugin is, so the tests exercise
// the subprocess path rather than a stub of it. Outside a helper run it does
// nothing.
func TestPluginMCPHelperProcess(t *testing.T) {
	mode := os.Getenv(helperEnv)
	if mode == "" {
		t.Skip("not a helper process")
	}
	serveFakePlugin(mode)
}

// serveFakePlugin runs the fake plugin's MCP surface and exits without
// returning, so the test framework never prints over the transport.
func serveFakePlugin(mode string) {
	switch mode {
	case "crash":
		fmt.Fprintln(os.Stderr, "fake plugin: refusing to start")
		os.Exit(1)
	case "hang":
		select {}
	}

	server := mcp.NewServer(&mcp.Implementation{Name: "fake-plugin", Version: "1"}, nil)
	objectSchema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{"type": "string"},
		},
	}

	echo := func(name string) mcp.ToolHandler {
		return func(_ context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return &mcp.CallToolResult{Content: []mcp.Content{
				&mcp.TextContent{Text: fmt.Sprintf("%s handled %s", name, string(req.Params.Arguments))},
			}}, nil
		}
	}

	if mode != "missing" {
		server.AddTool(&mcp.Tool{
			Name:        "project_status",
			Description: "Show sync status for a fake project",
			InputSchema: objectSchema,
		}, echo("project_status"))
	}
	// A tool the plugin serves but never declared: its own business, and off
	// the unified surface.
	server.AddTool(&mcp.Tool{
		Name:        "undeclared_tool",
		Description: "Not in the manifest",
		InputSchema: objectSchema,
	}, echo("undeclared_tool"))
	// A tool whose name kapi already serves.
	server.AddTool(&mcp.Tool{
		Name:        "up",
		Description: "The plugin's idea of up",
		InputSchema: objectSchema,
	}, echo("plugin up"))

	if err := server.Run(context.Background(), &mcp.StdioTransport{}); err != nil {
		fmt.Fprintln(os.Stderr, "fake plugin:", err)
	}
	os.Exit(0)
}

// fakePlugin writes a launcher script and a manifest, and returns the discovered
// plugin the proxy will spawn. declared lists the manifest's mcp_tools.
func fakePlugin(t *testing.T, name, mode string, declared ...string) *pluginhost.Plugin {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the fake plugin launcher is a shell script")
	}
	dir := t.TempDir()
	binary := filepath.Join(dir, "kapi-"+name)

	self, err := os.Executable()
	require.NoError(t, err)
	script := fmt.Sprintf("#!/bin/sh\n%s=%s exec %q -test.run='^TestPluginMCPHelperProcess$' --\n",
		helperEnv, mode, self)
	require.NoError(t, os.WriteFile(binary, []byte(script), 0o755))

	tools := make([]manifest.MCPTool, 0, len(declared))
	for _, d := range declared {
		tools = append(tools, manifest.MCPTool{Name: d, Description: "declared " + d})
	}
	return &pluginhost.Plugin{
		Dir:        dir,
		BinaryPath: binary,
		Manifest: &manifest.Manifest{
			Plugin:  name,
			Version: "0.0.0-test",
			Binary:  filepath.Base(binary),
			Capabilities: manifest.Capabilities{
				MCPTools: tools,
			},
		},
	}
}

// appWithPlugins builds an App whose plugin host carries the given plugins.
func appWithPlugins(t *testing.T, plugins ...*pluginhost.Plugin) *App {
	t.Helper()
	a := &App{}
	a.InitRegistries()
	a.PluginHost = pluginhost.NewHost(plugins, nil)
	t.Cleanup(a.closePluginMCPSessions)
	return a
}

// surfaceNames lists what a server serves, over an in-memory session.
func surfaceNames(t *testing.T, server *mcp.Server) map[string]*mcp.Tool {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	st, ct := mcp.NewInMemoryTransports()
	ss, err := server.Connect(ctx, st, nil)
	require.NoError(t, err)
	defer func() { _ = ss.Close() }()

	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1"}, nil)
	cs, err := client.Connect(ctx, ct, nil)
	require.NoError(t, err)
	defer func() { _ = cs.Close() }()

	out := map[string]*mcp.Tool{}
	for tool, err := range cs.Tools(ctx, nil) {
		require.NoError(t, err)
		out[tool.Name] = tool
	}
	return out
}

// coreServer is a server carrying one tool kapi itself owns, standing in for
// the core factories that have already run when the late proxy factory does.
func coreServer(t *testing.T) *mcp.Server {
	t.Helper()
	server := mcp.NewServer(&mcp.Implementation{Name: "kapi", Version: "test"}, nil)
	server.AddTool(&mcp.Tool{
		Name:        "up",
		Description: "kapi's own loop verb",
		InputSchema: map[string]any{"type": "object"},
	}, func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "core up"}}}, nil
	})
	return server
}

// TestPluginMCPProxy_DeclaredToolsReachTheSurface is the core of #1845: an agent
// connected to kapi reaches a plugin's declared tools without configuring a
// second MCP server.
func TestPluginMCPProxy_DeclaredToolsReachTheSurface(t *testing.T) {
	a := appWithPlugins(t, fakePlugin(t, "fake", "ok", "project_status"))
	server := coreServer(t)

	registerPluginMCPTools(server, a)

	tools := surfaceNames(t, server)
	require.Contains(t, tools, "project_status", "the plugin's declared tool is on kapi's surface")
	assert.Equal(t, "Show sync status for a fake project", tools["project_status"].Description,
		"the description comes from the plugin's live server, not the manifest")
	assert.NotNil(t, tools["project_status"].InputSchema, "the plugin's own input schema is published")
	assert.Contains(t, tools, "up", "kapi's own tools are untouched")
}

// TestPluginMCPProxy_UndeclaredToolsStayOff keeps the manifest as the list of
// what a plugin contributes. The plugin's own server carries the shared kapi
// surface too, so proxying everything it lists would duplicate the whole thing.
func TestPluginMCPProxy_UndeclaredToolsStayOff(t *testing.T) {
	a := appWithPlugins(t, fakePlugin(t, "fake", "ok", "project_status"))
	server := coreServer(t)

	registerPluginMCPTools(server, a)

	tools := surfaceNames(t, server)
	assert.Contains(t, tools, "project_status")
	assert.NotContains(t, tools, "undeclared_tool", "served by the plugin, not declared, so not proxied")
}

// TestPluginMCPProxy_ForwardsCalls checks the call reaches the plugin process
// with its arguments and the plugin's answer comes back.
func TestPluginMCPProxy_ForwardsCalls(t *testing.T) {
	a := appWithPlugins(t, fakePlugin(t, "fake", "ok", "project_status"))
	server := coreServer(t)

	registerPluginMCPTools(server, a)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	st, ct := mcp.NewInMemoryTransports()
	ss, err := server.Connect(ctx, st, nil)
	require.NoError(t, err)
	defer func() { _ = ss.Close() }()
	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1"}, nil)
	cs, err := client.Connect(ctx, ct, nil)
	require.NoError(t, err)
	defer func() { _ = cs.Close() }()

	res, err := cs.CallTool(ctx, &mcp.CallToolParams{
		Name:      "project_status",
		Arguments: map[string]any{"path": "docs/guide.md"},
	})
	require.NoError(t, err)
	require.Len(t, res.Content, 1)
	text, ok := res.Content[0].(*mcp.TextContent)
	require.True(t, ok)
	assert.Contains(t, text.Text, "project_status handled")
	assert.Contains(t, text.Text, "docs/guide.md", "arguments reached the plugin")
}

// TestPluginMCPProxy_CoreToolWinsACollision: a plugin cannot take over a name
// kapi already serves.
func TestPluginMCPProxy_CoreToolWinsACollision(t *testing.T) {
	a := appWithPlugins(t, fakePlugin(t, "fake", "ok", "project_status", "up"))
	server := coreServer(t)

	registerPluginMCPTools(server, a)

	tools := surfaceNames(t, server)
	require.Contains(t, tools, "up")
	assert.Equal(t, "kapi's own loop verb", tools["up"].Description, "kapi keeps the name")
	assert.Contains(t, tools, "project_status", "the plugin's other tool still lands")
}

// TestPluginMCPProxy_PluginThatFailsToStart leaves the rest of the surface
// intact rather than failing the session.
func TestPluginMCPProxy_PluginThatFailsToStart(t *testing.T) {
	a := appWithPlugins(t,
		fakePlugin(t, "broken", "crash", "broken_tool"),
		fakePlugin(t, "working", "ok", "project_status"),
	)
	server := coreServer(t)

	registerPluginMCPTools(server, a)

	tools := surfaceNames(t, server)
	assert.NotContains(t, tools, "broken_tool", "a plugin that will not start contributes nothing")
	assert.Contains(t, tools, "project_status", "the working plugin is unaffected")
	assert.Contains(t, tools, "up")
}

// TestPluginMCPProxy_DeclaredButNotServed covers a manifest that promises more
// than the binary delivers.
func TestPluginMCPProxy_DeclaredButNotServed(t *testing.T) {
	a := appWithPlugins(t, fakePlugin(t, "fake", "missing", "project_status"))
	server := coreServer(t)

	registerPluginMCPTools(server, a)

	tools := surfaceNames(t, server)
	assert.NotContains(t, tools, "project_status", "declared, not served, so not published")
}

// TestPluginMCPProxy_NoPluginHost and the empty case: nothing to proxy, nothing
// happens, and the core surface is untouched.
func TestPluginMCPProxy_NothingToProxy(t *testing.T) {
	t.Run("no plugin host", func(t *testing.T) {
		server := coreServer(t)
		registerPluginMCPTools(server, &App{})
		assert.Len(t, surfaceNames(t, server), 1)
	})

	t.Run("no plugin declares mcp tools", func(t *testing.T) {
		a := appWithPlugins(t, fakePlugin(t, "quiet", "ok"))
		server := coreServer(t)
		registerPluginMCPTools(server, a)
		assert.Len(t, surfaceNames(t, server), 1)
	})
}

// TestPluginMCPProxy_ShutdownClosesSessions: the spawned plugin servers are the
// session's, and they go when it does.
func TestPluginMCPProxy_ShutdownClosesSessions(t *testing.T) {
	a := appWithPlugins(t, fakePlugin(t, "fake", "ok", "project_status"))
	server := coreServer(t)

	registerPluginMCPTools(server, a)
	a.mcpPluginMu.Lock()
	count := len(a.mcpPluginSessions)
	a.mcpPluginMu.Unlock()
	require.Equal(t, 1, count, "the spawned session is tracked")

	a.closePluginMCPSessions()
	a.mcpPluginMu.Lock()
	defer a.mcpPluginMu.Unlock()
	assert.Empty(t, a.mcpPluginSessions, "shutdown released them")
}

// TestPluginInputSchema substitutes an open object schema for a plugin that
// publishes none, which AddTool would otherwise panic on.
func TestPluginInputSchema(t *testing.T) {
	assert.Equal(t, map[string]any{"type": "object"}, pluginInputSchema(&mcp.Tool{}))

	declared := map[string]any{"type": "object", "properties": map[string]any{}}
	assert.Equal(t, declared, pluginInputSchema(&mcp.Tool{InputSchema: declared}))
}

// TestPluginMCPProxy_SlowPluginDoesNotBlockForever bounds the startup wait. The
// proxy's own constant is minutes-free on purpose; here the deadline is the
// test's, and what matters is that a hanging plugin returns control.
func TestPluginMCPProxy_SlowPluginDoesNotBlockForever(t *testing.T) {
	restore := pluginMCPStartupTimeout
	pluginMCPStartupTimeout = 2 * time.Second
	t.Cleanup(func() { pluginMCPStartupTimeout = restore })

	a := appWithPlugins(t, fakePlugin(t, "slow", "hang", "slow_tool"))
	server := coreServer(t)

	done := make(chan struct{})
	go func() {
		defer close(done)
		registerPluginMCPTools(server, a)
	}()

	select {
	case <-done:
	case <-time.After(pluginMCPStartupTimeout + 30*time.Second):
		t.Fatal("a hanging plugin held up the MCP session past the startup bound")
	}
	assert.NotContains(t, surfaceNames(t, server), "slow_tool")
}

// TestDeclaredToolsAndPluginOrder covers the two ordering helpers directly, so
// a surface built from several plugins is reproducible.
func TestDeclaredToolsAndPluginOrder(t *testing.T) {
	zed := &pluginhost.Plugin{Manifest: &manifest.Manifest{Plugin: "zed"}}
	ace := &pluginhost.Plugin{Manifest: &manifest.Manifest{Plugin: "ace"}}
	routes := []*pluginhost.MCPRoute{
		{Plugin: zed, Tool: manifest.MCPTool{Name: "z_second"}},
		{Plugin: ace, Tool: manifest.MCPTool{Name: "a_tool"}},
		{Plugin: zed, Tool: manifest.MCPTool{Name: "z_first"}},
	}

	order := pluginsInRouteOrder(routes)
	require.Len(t, order, 2)
	assert.Equal(t, "ace", order[0].Name())
	assert.Equal(t, "zed", order[1].Name())

	tools := declaredTools(routes, zed)
	require.Len(t, tools, 2)
	assert.Equal(t, "z_first", tools[0].Name)
	assert.Equal(t, "z_second", tools[1].Name)
}
