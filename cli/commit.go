package cli

import "github.com/spf13/cobra"

// NewCommitCmd writes staged unit state into the project's committed record.
func NewCommitCmd(a *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "commit",
		GroupID: "work",
		Short:   "Write staged unit state into the project's committed record",
		Long: `Write the unit state recorded since the last commit into the project's committed
record: the JSON Lines shards under .kapi/state/ that git tracks.

Recording a review and publishing it are separate acts. The state record is
durable the moment it is written; committing is what puts it in the record a
reviewer reads, so a run of automated approvals does not land in the tracked
record before anyone has looked at it.

'kapi status' reports what is staged.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error { return a.RunCommit(cmd, args) },
	}
	AddProjectFlag(cmd)
	return cmd
}
