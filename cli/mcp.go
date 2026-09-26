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
		Short: "Start an MCP server (stdio) for assistants writing in this project",
		Long: `Start an MCP server over stdio.

--tools picks the tool sets it serves, as a comma-separated list:

  writing       (default) context_read and the context:// resources,
                context_search, the recording tools (context_observe,
                context_correct, context_withdraw), context_session_summary
                and check_file
  content       check_text, voice_check, voice_rewrite, term-check,
                extract_content, detect_format, apply_edits, redact
  translation   translate, up, up_plan, stats
  review        review_queue, review_unit, pre_review_unit
  all           every set

Two flags add what no set holds, for debugging:

  --all-tools   every CLI-visible registry tool (pipeline steps, format
                internals, one-off transforms)
  --all-flows   the flow-running verbs
  --all         every set and both of these`,
		Example: "  kapi mcp\n" +
			"  kapi mcp --tools writing,translation\n" +
			"  kapi mcp --tools all",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			all, _ := cmd.Flags().GetBool("all")
			allTools, _ := cmd.Flags().GetBool("all-tools")
			allFlows, _ := cmd.Flags().GetBool("all-flows")
			names, _ := cmd.Flags().GetStringSlice("tools")
			sets, err := host.ParseMCPToolSets(names)
			if err != nil {
				return err
			}
			if all {
				sets = host.AllMCPToolSets()
			}
			// Resolve project vs ad-hoc mode once for the server's lifetime so
			// the tool factories can scope the exposed surface accordingly.
			if err := a.ResolveMCPProject(cmd); err != nil {
				return err
			}
			a.MCPSurface = host.MCPSurface{
				Sets:     sets,
				AllTools: all || allTools,
				AllFlows: all || allFlows,
			}
			// The instructions reach every client at initialize, ahead of the
			// tool list and whether or not the host loads a skill. They are
			// the same for every binary that serves this surface, because
			// what they describe is the surface rather than the binary.
			server := mcp.NewServer(
				&mcp.Implementation{Name: implName, Version: version.Version},
				&mcp.ServerOptions{Instructions: host.MCPInstructionsFor(sets)},
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
	cmd.Flags().Bool("all", false, "every tool set, plus --all-tools and --all-flows")
	cmd.Flags().StringSlice("tools", nil, "tool sets to serve: writing (default), content, translation, review, all")
	return cmd
}
