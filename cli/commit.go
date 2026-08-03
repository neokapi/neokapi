package cli

import "github.com/spf13/cobra"

// NewCommitCmd writes staged decisions into the project's committed record.
func NewCommitCmd(a *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "commit",
		GroupID: "work",
		Short:   "Write staged decisions into the project's committed record",
		Long: `Write decisions recorded since the last commit into the project's committed
record — the JSON Lines shards under the state directory that git tracks.

Recording a decision and publishing it are separate acts. A decision is durable
the moment it is made; committing is what puts it in the record a reviewer reads,
so a run of automated approvals does not land in the tracked record before anyone
has looked at it.

'kapi status' reports what is staged.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error { return a.RunCommit(cmd, args) },
	}
	AddProjectFlag(cmd)
	return cmd
}
