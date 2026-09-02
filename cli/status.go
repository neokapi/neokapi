package cli

import "github.com/spf13/cobra"

// NewStatusCmd creates `kapi status`: a project dashboard showing per-locale
// translation coverage and ship-gate standing — the informational counterpart
// to `kapi check --ship` (the gate). State is derived from the project's content ×
// target files, so it is always current with the working tree.
//
// The built-in owns the verb in every install (the no-shadowing rule): a
// server-connected project gets its sync section merged in via the plugin's
// hidden `server-status` plumbing (see host.appendServerStatus), never via a
// replacement command.
func NewStatusCmd(a *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "status",
		GroupID: "work",
		Short:   "Show per-locale translation coverage and ship-gate standing",
		Long: `Show, per target locale, how much of the project's tracked content is
translated and whether it clears its ship gate, a derived dashboard, like
git status. Coverage is recomputed from the content × target files on every run;
nothing is tracked as state.

This is the informational counterpart to 'kapi check --ship' (the quality gate). It
never fails: a locale that is behind is reported as pending rather than as an error.
Target-language drift is normal, expected work rather than a build break.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error { return a.RunStatus(cmd, args) },
	}
	AddProjectFlag(cmd)
	AddStatusFlags(cmd)
	return cmd
}
