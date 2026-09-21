package cli

import (
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/neokapi/neokapi/core/version"
	"github.com/neokapi/neokapi/host"
	"github.com/spf13/cobra"
)

// NewMCPCmd creates the shared `mcp` subcommand. It starts an MCP stdio
// server, populates it with tools from every registered MCPToolFactory,
// and serves until the connection closes.
//
// The implementation name is supplied by the caller so binaries can
// brand the server (e.g. "kapi" vs "bowrain"); both share the same
// underlying tool registry.
func NewMCPCmd(a *App, implName string) *cobra.Command {
	if implName == "" {
		implName = "kapi"
	}
	cmd := &cobra.Command{
		Use:   "mcp",
		Short: "Start MCP server (stdio) exposing the project's context and the loop around it",
		Long: `Start an MCP server over stdio.

The default surface is deliberately small: retrieving the project's context,
checking content against it, writing edits back, and running the loop. That is
what an assistant needs to work inside this project's context.

Everything else the tool registry can do is still reachable (pipeline steps,
format internals, one-off transforms) as an explicit opt-in, because a
surface nobody chose is how it grew to fifty-one tools.

  --all-tools   every CLI-visible registry tool
  --all-flows   the flow-running verbs
  --all         both`,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Resolve project vs ad-hoc mode once for the server's lifetime so
			// the tool factories can scope the exposed surface accordingly.
			if err := a.ResolveMCPProject(cmd); err != nil {
				return err
			}
			all, _ := cmd.Flags().GetBool("all")
			allTools, _ := cmd.Flags().GetBool("all-tools")
			allFlows, _ := cmd.Flags().GetBool("all-flows")
			a.MCPSurface = host.MCPSurface{
				AllTools: all || allTools,
				AllFlows: all || allFlows,
			}
			// The instructions reach every client at initialize, ahead of the
			// tool list and whether or not the host loads a skill. They are
			// the same for every binary that serves this surface, because
			// what they describe is the surface rather than the binary.
			server := mcp.NewServer(
				&mcp.Implementation{Name: implName, Version: version.Version},
				&mcp.ServerOptions{Instructions: host.MCPInstructions()},
			)
			ApplyMCPToolFactories(server, a)
			// One row in the workspace saying an agent is at work here, moved
			// forward on every request the server answers, so the desktop and
			// anything else watching the workspace can show it. Noted before
			// the first request as well: a server that starts and reads
			// nothing is still a session somebody may want to review.
			server.AddReceivingMiddleware(a.MCPSessionMiddleware())
			a.NoteMCPSession(host.CmdContext(cmd), a.MCPRecipePath(), "")
			// host.CmdContext, not cmd.Context: a caller that builds this
			// command and invokes RunE directly (the kapi-bowrain plugin's
			// mcp-server does) never went through cobra's Execute, so its
			// context is nil and server.Run segfaults on the first line.
			return server.Run(host.CmdContext(cmd), &mcp.StdioTransport{})
		},
	}
	AddProjectFlag(cmd)
	cmd.Flags().Bool("all-tools", false, "expose every CLI-visible registry tool, not the curated set")
	cmd.Flags().Bool("all-flows", false, "expose the flow-running verbs")
	cmd.Flags().Bool("all", false, "shorthand for --all-tools --all-flows")
	return cmd
}
