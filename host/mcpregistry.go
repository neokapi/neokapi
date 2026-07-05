package host

import (
	"sync"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

var (
	mcpRegMu         sync.RWMutex
	mcpToolFactories []MCPToolFactory
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

// ApplyMCPToolFactories invokes every registered MCPToolFactory.
func ApplyMCPToolFactories(server *mcp.Server, app *App) {
	mcpRegMu.RLock()
	fs := append([]MCPToolFactory(nil), mcpToolFactories...)
	mcpRegMu.RUnlock()
	for _, f := range fs {
		f(server, app)
	}
}

// ResetMCPToolFactoriesForTest clears the MCP tool factory registry.
// Only for use in tests.
func ResetMCPToolFactoriesForTest() {
	mcpRegMu.Lock()
	defer mcpRegMu.Unlock()
	mcpToolFactories = nil
}
