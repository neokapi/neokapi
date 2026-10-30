package cli

import "github.com/spf13/cobra"

// NewStatusCmd creates `kapi status`: a project dashboard showing source
// readiness and gate standing, with per-locale coverage for a project that
// names target languages. It is the informational counterpart to `kapi check
// --ship` (the gate). State is derived from the project's content and target
// files, so it is always current with the working tree.
//
// The built-in owns the verb in every install (the no-shadowing rule): a
// server-connected project gets its sync section merged in via the plugin's
// hidden `server-status` plumbing (see host.appendServerStatus), never via a
// replacement command.
func NewStatusCmd(a *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "status",
		GroupID: "work",
		Short:   "Show how ready the project's content is against its gates",
		Long: `Show how far the project's content has progressed and whether it clears the
project's gates: a derived dashboard, like git status. The standing is
recomputed from the working tree on every run; nothing is tracked as state.

A project with target languages also gets per-language coverage: for each
target locale, how much of the content is translated and whether it clears
its ship gate. A locale that is behind is reported as pending rather than as
an error, because target-language drift is normal, expected work.

This is the informational counterpart to 'kapi check --ship' (the quality
gate), and it never fails.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error { return a.RunStatus(cmd, args) },
	}
	AddProjectFlag(cmd)
	AddStatusFlags(cmd)
	return cmd
}
