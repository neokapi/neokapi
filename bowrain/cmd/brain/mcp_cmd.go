package main

import (
	"github.com/gokapi/gokapi/core/version"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spf13/cobra"
)

var mcpCmd = &cobra.Command{
	Use:           "mcp",
	Short:         "Start MCP server for project management",
	SilenceUsage:  true,
	SilenceErrors: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		server := mcp.NewServer(
			&mcp.Implementation{Name: "brain", Version: version.Version},
			nil,
		)
		registerBrainTools(server, app)
		return server.Run(cmd.Context(), &mcp.StdioTransport{})
	},
}

func init() {
	rootCmd.AddCommand(mcpCmd)
}
