//go:build !js

package host

import "github.com/modelcontextprotocol/go-sdk/mcp"

// pruneMCPToolSets removes from the server every tool that no selected set
// lists, and the resources when their set is not selected. A nil selection
// serves every set, for a caller that builds a server without going through
// `kapi mcp`.
func pruneMCPToolSets(server *mcp.Server, selected map[string]bool) {
	if selected == nil {
		return
	}
	served := map[string]bool{}
	for set, tools := range mcpToolSets {
		if selected[set] {
			for _, tool := range tools {
				served[tool] = true
			}
		}
	}
	for set, tools := range mcpToolSets {
		if selected[set] {
			continue
		}
		for _, tool := range tools {
			if !served[tool] {
				server.RemoveTools(tool)
			}
		}
	}
	if !selected[mcpResourceSet] {
		server.RemoveResourceTemplates(contextLocationTemplate, contextProfileTemplate)
	}
}
