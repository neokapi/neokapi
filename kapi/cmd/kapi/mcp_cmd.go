package main

import (
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/neokapi/neokapi/core/version"
	"github.com/spf13/cobra"
)

func newMCPCmd() *cobra.Command {
	return &cobra.Command{
		Use:           "mcp",
		Short:         "Start MCP server for file processing",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			server := mcp.NewServer(
				&mcp.Implementation{Name: "kapi", Version: version.Version},
				nil,
			)
			registerKapiTools(server, app)
			return server.Run(cmd.Context(), &mcp.StdioTransport{})
		},
	}
}
