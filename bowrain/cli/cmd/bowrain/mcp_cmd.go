package main

import (
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/neokapi/neokapi/core/version"
	"github.com/spf13/cobra"
)

var mcpCmd = &cobra.Command{
	Use:           "mcp",
	Short:         "Start MCP server for project management",
	SilenceUsage:  true,
	SilenceErrors: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		server := mcp.NewServer(
			&mcp.Implementation{Name: "bowrain", Version: version.Version},
			nil,
		)
		registerBowrainTools(server, app)
		return server.Run(cmd.Context(), &mcp.StdioTransport{})
	},
}

func init() {
	rootCmd.AddCommand(mcpCmd)
}
