package cli

import (
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/neokapi/neokapi/core/version"
	"github.com/spf13/cobra"
)

// NewMCPCmd creates the shared `mcp` subcommand. It starts an MCP stdio
// server, populates it with tools from every registered MCPToolFactory,
// and serves until the connection closes.
//
// The implementation name is supplied by the caller so binaries can
// brand the server (e.g. "kapi" vs "bowrain"); both share the same
// underlying tool registry.
func (a *App) NewMCPCmd(implName string) *cobra.Command {
	if implName == "" {
		implName = "kapi"
	}
	return &cobra.Command{
		Use:           "mcp",
		Short:         "Start MCP server (stdio) exposing project tools",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Resolve project vs ad-hoc mode once for the server's lifetime so
			// the tool factories can scope the exposed surface accordingly.
			a.resolveMCPProject(cmd)
			server := mcp.NewServer(
				&mcp.Implementation{Name: implName, Version: version.Version},
				nil,
			)
			ApplyMCPToolFactories(server, a)
			return server.Run(cmd.Context(), &mcp.StdioTransport{})
		},
	}
}
