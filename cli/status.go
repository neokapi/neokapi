package cli

import "github.com/spf13/cobra"

// NewStatusCmd creates `kapi status`: a project dashboard showing per-locale
// translation coverage and ship-gate standing — the informational counterpart
// to `kapi verify` (the gate). State is derived from the project's content ×
// target files, so it is always current with the working tree.
//
// When a plugin provides its own `status` (e.g. kapi-bowrain's sync status), the
// plugin's command takes precedence and this built-in is not registered (see the
// command wiring in cmd/kapi/root.go).
func NewStatusCmd(a *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "status",
		GroupID: "work",
		Short:   "Show per-locale translation coverage and ship-gate standing",
		Long: `Show, per target locale, how much of the project's tracked content is
translated and whether it clears its ship gate — a derived dashboard, like
git status. Coverage is recomputed from the content × target files on every run;
nothing is tracked as state.

This is the informational counterpart to 'kapi verify' (the quality gate). It
never fails: a locale that is behind is reported as pending, not an error —
target-language drift is normal, expected work, not a build break.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error { return a.RunStatus(cmd, args) },
	}
	AddProjectFlag(cmd)
	AddStatusFlags(cmd)
	return cmd
}
