package main

import (
	"github.com/neokapi/neokapi/cli"

	// The bowrain plugin's commands package init() registers the
	// CommandFactories that build the bowrain command tree on top of the
	// shared cli.App. Blank-importing the anchor pulls in schema +
	// commands + MCP tool registrations in one go.
	_ "github.com/neokapi/neokapi/bowrain/plugin"
)

func main() {
	cli.Run(rootCmd, app.Shutdown)
}
