package host

import (
	"sync"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

var (
	mcpRegMu         sync.RWMutex
	mcpToolFactories []MCPToolFactory
	mcpLateFactories []MCPToolFactory
)

// MCPToolFactory registers MCP tools on the shared `mcp` command's
// server. Plugins register factories via init(); the shared mcp command
// walks them when starting the stdio MCP server.
type MCPToolFactory func(server *mcp.Server, app *App)

// RegisterMCPToolFactory queues an MCP tool factory. Safe from init().
func RegisterMCPToolFactory(f MCPToolFactory) {
	mcpRegMu.Lock()
	defer mcpRegMu.Unlock()
	mcpToolFactories = append(mcpToolFactories, f)
}

// RegisterLateMCPToolFactory queues a factory that runs after every ordinary
// one. Init order within a package follows filename, and factories registered
// from other packages append in dependency order, so a factory that must see
// the finished surface cannot get there by being named late. The plugin proxy
// needs it: a name is only free once every core factory has claimed what it
// wants. Safe from init().
func RegisterLateMCPToolFactory(f MCPToolFactory) {
	mcpRegMu.Lock()
	defer mcpRegMu.Unlock()
	mcpLateFactories = append(mcpLateFactories, f)
}

// ApplyMCPToolFactories invokes every registered MCPToolFactory, then every
// late one.
func ApplyMCPToolFactories(server *mcp.Server, app *App) {
	mcpRegMu.RLock()
	fs := append([]MCPToolFactory(nil), mcpToolFactories...)
	late := append([]MCPToolFactory(nil), mcpLateFactories...)
	mcpRegMu.RUnlock()
	for _, f := range fs {
		f(server, app)
	}
	for _, f := range late {
		f(server, app)
	}
}

// ResetMCPToolFactoriesForTest clears the MCP tool factory registry.
// Only for use in tests.
func ResetMCPToolFactoriesForTest() {
	mcpRegMu.Lock()
	defer mcpRegMu.Unlock()
	mcpToolFactories = nil
	mcpLateFactories = nil
}
