package host

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/neokapi/neokapi/core/plugin/manifest"
	"github.com/neokapi/neokapi/host/pluginhost"
)

// init registers the plugin MCP proxy as a LATE factory: it can only tell a
// free tool name from one a core factory has taken after the core factories
// have run.
func init() {
	RegisterLateMCPToolFactory(registerPluginMCPTools)
}

// A plugin declares the tools it contributes to the agent surface in its
// manifest under `mcp_tools`, and serves them from its own MCP-over-stdio
// server. `kapi mcp` spawns that server once per session and forwards calls to
// it, so an agent connected to kapi reaches a plugin's tools without being told
// to configure a second server. What a surface offers an agent is what it
// offers a human, and `kapi push` has a human form already.
//
// kapi links no plugin's code. The proxy talks to a subprocess over stdio, so a
// plugin's dependencies and its licence stay inside its own binary.
const (
	// pluginMCPSubcommand is the subcommand a plugin binary serves its MCP
	// surface from. It is a convention of the plugin model rather than a
	// manifest field: every plugin binary that declares `mcp_tools` answers on
	// it, and a plugin that does not is reported and skipped.
	pluginMCPSubcommand = "mcp-server"

	// pluginMCPCallTimeout bounds one forwarded tool call. Pull and push move
	// content over a network, so it is generous compared with the startup bound.
	pluginMCPCallTimeout = 10 * time.Minute
)

// pluginMCPStartupTimeout bounds a plugin's handshake and tool listing. A
// plugin that has not answered by then is left off the surface rather than
// holding up the session: an agent waiting on `kapi mcp` to start has no way to
// know a plugin is the reason. A var so the test that proves the bound can cut
// it to something a CI run should pay.
var pluginMCPStartupTimeout = 15 * time.Second

// registerPluginMCPTools adds one proxy tool per declared, live plugin MCP tool.
func registerPluginMCPTools(server *mcp.Server, a *App) {
	if a == nil || a.PluginHost == nil {
		return
	}
	routes := a.PluginHost.MCPRoutes()
	if len(routes) == 0 {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), pluginMCPStartupTimeout)
	defer cancel()

	taken := serverToolNames(ctx, server)

	for _, p := range pluginsInRouteOrder(routes) {
		declared := declaredTools(routes, p)
		session, live, err := a.dialPluginMCP(ctx, p)
		if err != nil {
			warnPlugin(p, fmt.Sprintf("its MCP server did not start (%v); its tools are not on this surface", err))
			continue
		}

		added := 0
		for _, decl := range declared {
			if taken[decl.Name] {
				warnPlugin(p, fmt.Sprintf("tool %q collides with a tool kapi already serves and was skipped", decl.Name))
				continue
			}
			tool, ok := live[decl.Name]
			if !ok {
				warnPlugin(p, fmt.Sprintf("tool %q is declared in the manifest but its MCP server does not offer it", decl.Name))
				continue
			}
			desc := tool.Description
			if desc == "" {
				desc = decl.Description
			}
			server.AddTool(&mcp.Tool{
				Name:        decl.Name,
				Description: desc,
				InputSchema: pluginInputSchema(tool),
			}, pluginMCPHandler(session, decl.Name))
			taken[decl.Name] = true
			added++
		}

		if added == 0 {
			// Nothing was registered, so nothing will ever call this session.
			_ = session.Close()
			continue
		}
		a.trackPluginMCPSession(session)
	}
}

// pluginInputSchema returns the schema to publish for a proxied tool. AddTool
// panics on a nil schema, and a plugin is another program: an open object
// schema keeps a plugin that publishes none off the crash path, and the plugin
// still validates its own arguments.
func pluginInputSchema(t *mcp.Tool) any {
	if t.InputSchema == nil {
		return map[string]any{"type": "object"}
	}
	return t.InputSchema
}

// pluginMCPHandler forwards one tool call to the plugin's own MCP server and
// returns its result verbatim. The plugin owns the answer; kapi carries it.
func pluginMCPHandler(session *mcp.ClientSession, name string) mcp.ToolHandler {
	return func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		ctx, cancel := context.WithTimeout(ctx, pluginMCPCallTimeout)
		defer cancel()

		var args any
		if len(req.Params.Arguments) > 0 {
			args = req.Params.Arguments
		}
		res, err := session.CallTool(ctx, &mcp.CallToolParams{
			Name:      name,
			Arguments: args,
		})
		if err != nil {
			return nil, fmt.Errorf("plugin tool %q: %w", name, err)
		}
		return res, nil
	}
}

// dialPluginMCP spawns the plugin's MCP server and lists what it serves.
func (a *App) dialPluginMCP(ctx context.Context, p *pluginhost.Plugin) (*mcp.ClientSession, map[string]*mcp.Tool, error) {
	cmd := exec.Command(p.BinaryPath, pluginMCPSubcommand)
	cmd.Stderr = os.Stderr
	cmd.Env = os.Environ()

	client := mcp.NewClient(&mcp.Implementation{Name: "kapi", Version: "1"}, nil)
	session, err := client.Connect(ctx, &mcp.CommandTransport{Command: cmd}, nil)
	if err != nil {
		return nil, nil, err
	}

	live, err := listSessionTools(ctx, session)
	if err != nil {
		_ = session.Close()
		return nil, nil, fmt.Errorf("list tools: %w", err)
	}
	return session, live, nil
}

// listSessionTools drains a session's paginated tool list into a map by name.
func listSessionTools(ctx context.Context, session *mcp.ClientSession) (map[string]*mcp.Tool, error) {
	out := map[string]*mcp.Tool{}
	for tool, err := range session.Tools(ctx, nil) {
		if err != nil {
			return nil, err
		}
		out[tool.Name] = tool
	}
	return out, nil
}

// serverToolNames reads back what the server already serves, over an in-memory
// session it closes again. The SDK's AddTool replaces a tool of the same name
// without saying so, so the names have to be read rather than assumed: a plugin
// that declares `up` would otherwise silently take over the loop verb.
func serverToolNames(ctx context.Context, server *mcp.Server) map[string]bool {
	names := map[string]bool{}
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(ctx, serverTransport, nil)
	if err != nil {
		return names
	}
	defer func() { _ = serverSession.Close() }()

	client := mcp.NewClient(&mcp.Implementation{Name: "kapi-surface-probe", Version: "1"}, nil)
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		return names
	}
	defer func() { _ = clientSession.Close() }()

	live, err := listSessionTools(ctx, clientSession)
	if err != nil {
		return names
	}
	for name := range live {
		names[name] = true
	}
	return names
}

// pluginsInRouteOrder lists the distinct plugins behind a set of MCP routes,
// ordered by plugin name so a session's startup order is reproducible.
func pluginsInRouteOrder(routes []*pluginhost.MCPRoute) []*pluginhost.Plugin {
	seen := map[string]bool{}
	var out []*pluginhost.Plugin
	for _, r := range routes {
		if r == nil || r.Plugin == nil || seen[r.Plugin.Name()] {
			continue
		}
		seen[r.Plugin.Name()] = true
		out = append(out, r.Plugin)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name() < out[j].Name() })
	return out
}

// declaredTools lists one plugin's declared MCP tools, ordered by name.
func declaredTools(routes []*pluginhost.MCPRoute, p *pluginhost.Plugin) []manifest.MCPTool {
	var out []manifest.MCPTool
	for _, r := range routes {
		if r == nil || r.Plugin == nil || r.Plugin.Name() != p.Name() {
			continue
		}
		out = append(out, r.Tool)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// warnPlugin reports a plugin that contributed less than its manifest promised.
// It goes to stderr because stdout is the MCP transport.
func warnPlugin(p *pluginhost.Plugin, msg string) {
	fmt.Fprintf(os.Stderr, "kapi mcp: plugin %q: %s\n", p.Name(), msg)
}

// trackPluginMCPSession keeps a spawned plugin server alive for the session and
// hands its teardown to Shutdown.
func (a *App) trackPluginMCPSession(session *mcp.ClientSession) {
	a.mcpPluginMu.Lock()
	defer a.mcpPluginMu.Unlock()
	a.mcpPluginSessions = append(a.mcpPluginSessions, session)
}

// closePluginMCPSessions shuts down every plugin MCP server this App spawned.
// Closing the session closes the subprocess's stdin, which is how the plugin
// learns the session is over.
func (a *App) closePluginMCPSessions() {
	a.mcpPluginMu.Lock()
	sessions := a.mcpPluginSessions
	a.mcpPluginSessions = nil
	a.mcpPluginMu.Unlock()
	for _, s := range sessions {
		_ = s.Close()
	}
}
