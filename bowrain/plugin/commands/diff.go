package commands

import (
	"fmt"

	"github.com/neokapi/neokapi/bowrain/core/project"
	"github.com/neokapi/neokapi/bowrain/plugin/commands/output"
	bconn "github.com/neokapi/neokapi/bowrain/plugin/connector"
	"github.com/neokapi/neokapi/cli"
	"github.com/spf13/cobra"
)

// diffCmd lives under the plugin group (`kapi bowrain diff`): the built-in
// `kapi diff` (the format-aware kdiff file differ) keeps the top-level verb
// under the no-shadowing rule, and pluginattach mounts this colliding name
// group-only.
var diffCmd = &cobra.Command{
	Use:   "diff [paths...]",
	Short: "Show differences between local and remote",
	Long: `Display the blocks that changed locally relative to the last sync.

Like 'git diff --stat', but for translation content: it reports, per file,
how many blocks were added, changed, or removed locally versus the
last-synced server state, plus the count of remote changes available to pull.

Use --verbose to list the changed block ids/keys with a source preview.

Examples:
  kapi bowrain diff
  kapi bowrain diff src/locales/
  kapi bowrain diff --verbose`,
	RunE: runDiff,
}

func runDiff(cmd *cobra.Command, args []string) error {
	proj, err := project.FindProject("")
	if err != nil {
		return fmt.Errorf("find project: %w (run 'kapi init' to create a project)", err)
	}

	verbose, _ := cmd.Flags().GetBool("verbose")

	out := output.DiffOutput{
		Project: output.ProjectInfo{
			Root:      proj.Root,
			ConfigDir: proj.RecipePath(),
		},
		Verbose: verbose,
	}

	// With a server: compute local deltas AND query the server for pending
	// remote changes. Without a server: still compute local-vs-cache deltas
	// so the command stays useful offline.
	var conn *bconn.BowrainSourceConnector
	if proj.Recipe.HasServer() {
		conn, err = bconn.NewSourceConnector(proj, app.FormatReg)
		if err != nil {
			return err
		}
		out.Connected = true
		out.Project.Server = proj.Recipe.Server.ServerURL()
		out.Project.ProjectID = proj.Recipe.Server.ProjectID()
		out.Stream = conn.Stream()
	} else {
		conn = bconn.NewLocalConnector(proj, app.FormatReg)
	}
	defer conn.Close()

	diff, err := conn.Diff(cmd.Context(), args)
	if err != nil {
		return err
	}

	out.Added = diff.Added
	out.Changed = diff.Changed
	out.Removed = diff.Removed
	out.PendingPull = diff.PendingPull
	out.Files = make([]output.DiffFileEntry, 0, len(diff.Files))
	for _, f := range diff.Files {
		entry := output.DiffFileEntry{
			Path:    f.Path,
			Format:  f.Format,
			Added:   f.Added,
			Changed: f.Changed,
			Removed: f.Removed,
		}
		entry.Blocks = make([]output.DiffBlockEntry, 0, len(f.Blocks))
		for _, b := range f.Blocks {
			entry.Blocks = append(entry.Blocks, output.DiffBlockEntry{
				BlockID: b.BlockID,
				Name:    b.Name,
				Preview: b.Preview,
				Change:  b.Change,
			})
		}
		out.Files = append(out.Files, entry)
	}

	return output.Print(cmd, out)
}

func init() {
	diffCmd.Flags().BoolP("verbose", "v", false, "list changed block ids/keys with a source preview")
	cli.RegisterCommandFactory(func(parent *cobra.Command, _ *cli.App) { parent.AddCommand(diffCmd) })
}
