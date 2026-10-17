package cli

import (
	"github.com/neokapi/neokapi/host"
	"github.com/neokapi/neokapi/host/output"
	"github.com/spf13/cobra"
)

// newContextAtlasCmd is `kapi context atlas`: the whole of a project's context
// in one read, for exploring. The terminal prints a short summary; --json
// prints the document the desktop's Explore view draws (host.ContextAtlas).
func newContextAtlasCmd(a *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "atlas",
		Short: "Lay out everything the project's context holds, for exploring",
		Long: `Read everything this project's context holds in one pass: each rule,
note, content-memory pair and term, at the point it holds on the project's
axes, with the statuses it has held over time, and each event on two clocks,
when something was recorded and when a window opens or closes. Content cells
count the project's units by collection, language and state.

The terminal shows a summary by axis. --json prints the full document, the one
the desktop's Explore view draws.`,
		Example: "  kapi context atlas\n" +
			"  kapi context atlas --json > atlas.json\n" +
			"  kapi context atlas --all-projects --json",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			all, _ := cmd.Flags().GetBool("all-projects")
			noContent, _ := cmd.Flags().GetBool("no-content")
			req := host.ContextAtlasRequest{AllProjects: all, Content: !noContent && !all}
			if !all {
				projectPath, err := RequireProjectPath(cmd)
				if err != nil {
					return err
				}
				req.Project = projectPath
			}
			res, err := a.ContextAtlas(cmd.Context(), req)
			if err != nil {
				return err
			}
			return output.Print(cmd, res)
		},
	}
	cmd.Flags().Bool("all-projects", false, "read every project in the workspace (content is then left out)")
	cmd.Flags().Bool("no-content", false, "leave out the content cells, which read every file of the project")
	AddProjectFlag(cmd)
	return cmd
}
