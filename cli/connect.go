package cli

import (
	"github.com/spf13/cobra"

	"github.com/neokapi/neokapi/host"
	"github.com/neokapi/neokapi/host/output"
)

// connectShort is the one-line help of `kapi connect`, shared with its
// browser stand-in.
const connectShort = "Put an assistant on this project: rules as it starts, kapi check on every edit"

// NewConnectCmd returns `kapi connect [assistant]`: write the files each
// assistant reads in this project, through host.ConnectHosts.
func NewConnectCmd(a *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "connect [assistant]",
		Short:   connectShort,
		GroupID: "work",
		Long: `Put an assistant on this project. The assistant reads the project's rules
in AGENTS.md and CLAUDE.md as it starts, asks 'kapi context <path>' before it
writes a file, and runs 'kapi check' on each file it edited.

kapi connect writes what each assistant needs and lists every file as
created, updated or kept:

  kapi.yaml            the recipe, written as kapi init writes it when the
                       folder has none
  AGENTS.md, CLAUDE.md the Writing rules section; the rest of each file is
                       kept as you wrote it
  .mcp.json and the    the MCP server entry that starts 'kapi mcp' for this
  other hosts' files   project, and the kapi skill where the host reads skills

With no argument it connects Claude Code and every assistant whose folder the
project keeps (.cursor/, .vscode/, .codex/, .agents/), and lists the others.
Name one to connect it alone: claude, cursor, vscode, codex or agents.

Claude Code runs kapi check through two hooks of the kapi Claude Code
plugin: the edit hook checks each file an edit writes, and the Stop hook runs
the project's ship gates when Claude Code finishes. kapi connect reports both
hooks as pending, with the commands that install the plugin, until the
project's .claude/settings.json or your own Claude Code settings enable it.

Running it again rewrites nothing that is current. --dry-run lists the same
files with what it would do and writes none of them.`,
		Example: "  kapi connect\n" +
			"  kapi connect --dry-run\n" +
			"  kapi connect claude\n" +
			"  kapi connect codex --json",
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts := host.ConnectOptions{}
			opts.DryRun, _ = cmd.Flags().GetBool("dry-run")
			if len(args) == 1 {
				h, err := host.ParseConnectHost(args[0])
				if err != nil {
					return err
				}
				opts.Hosts = []host.AgentHost{h}
			}
			res, err := a.ConnectHosts(CmdContext(cmd), cmd, opts)
			if err != nil {
				return err
			}
			return output.Print(cmd, res)
		},
	}
	cmd.Flags().Bool("dry-run", false, "list the files kapi connect would write, and write none of them")
	AddProjectFlag(cmd)
	return cmd
}
