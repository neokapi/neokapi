//go:build js

package cli

import "github.com/spf13/cobra"

// NewMCPCmd is the browser build's mcp command: an MCP server serves a
// long-lived stdio session, which the browser has no peer for, so the command
// explains that the way every other unavailable verb does. The server itself
// (mcp.go) and the MCP SDK are built for every other target.
func NewMCPCmd(*App, string) *cobra.Command { return newBrowserGapCmd("mcp") }
