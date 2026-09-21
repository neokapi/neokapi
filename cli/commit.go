package cli

import "github.com/spf13/cobra"

// NewCommitCmd writes the project's unit state into its committed record.
func NewCommitCmd(a *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "commit",
		GroupID: "work",
		Short:   "Write the project's unit state into its committed record",
		Long: `Write this checkout's unit state into the project's committed record: the JSON
Lines shards under .kapi/state/ that git tracks.

Every decision is already durable the moment it is made. This writes the record
out in the form git tracks and a reviewer reads in a diff: for each unit the
checkout holds, what was decided about the source and the translation it holds
now. A unit whose wording has moved on since a decision was made keeps that
decision and reads as stale, and the decision applies again if the wording comes
back.

Running it twice over an unchanged project writes the same bytes and leaves the
files alone.

'--dry-run' reports what would be written and writes nothing.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error { return a.RunCommit(cmd, args) },
	}
	AddProjectFlag(cmd)
	cmd.Flags().Bool("dry-run", false, "report what would be committed and write nothing")
	return cmd
}
