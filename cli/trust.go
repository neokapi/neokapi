package cli

import (
	"errors"

	"github.com/neokapi/neokapi/host/output"
	"github.com/spf13/cobra"
)

// NewTrustCmd creates the trust command group: the front door to the
// execution-trust record host/exectrust.go keeps.
func NewTrustCmd(a *App) *cobra.Command {
	trustCmd := &cobra.Command{
		Use:     "trust",
		Short:   "List, inspect and change which projects may run commands",
		GroupID: "advanced",
		Long: `List, inspect and change which projects may run commands.

A recipe that names an exec-class step (external-command, script, or the exec
format) runs code the recipe chooses, with your privileges and your
environment. kapi asks once per project before running it and remembers the
answer, allow or deny, against a fingerprint of the commands shown. A recipe
edit that changes what would run asks again. A comment edit's project
formatter is decided the same way and kept in the same record, keyed by the
configuration file that selects it.

These commands read and change that record, which is kept in the kapi config
directory, not in the project. They are about execution only: plugin installs
and package adoption are confirmed at the time and keep no record.

KAPI_TRUST_EXEC=1 grants trust to one process without consulting or writing
the record; it is for automation and is not shown here as a decision.`,
		Example: `  kapi trust list
  kapi trust show -p ./app
  kapi trust allow -p ./app
  kapi trust revoke ./app`,
	}

	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List every recorded decision and whether it still applies",
		Long: `List every recorded decision: the path it is keyed by, allow or deny, when it
was recorded, and how it stands against what is at the path now.

  current     the recipe still runs what was decided; the next run honours it
  changed     the recipe runs something else now; the next run asks again
  missing     nothing is at the path any more
  unreadable  the recipe at the path does not load
  unverified  a formatter decision; kapi apply checks it at the edit`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return output.Print(cmd, a.ExecTrustList())
		},
	}

	showCmd := &cobra.Command{
		Use:   "show",
		Short: "Show what a recipe would run and the decision that applies",
		Long: `Show the commands a recipe would run and the decision recorded for them,
without starting a run. Inside a project no flag is needed; -p names one.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			recipePath, err := RequireProjectPath(cmd)
			if err != nil {
				return err
			}
			out, err := a.ExecTrustShow(recipePath)
			if err != nil {
				return err
			}
			return output.Print(cmd, out)
		},
	}
	AddProjectFlag(showCmd)

	allowCmd := &cobra.Command{
		Use:   "allow",
		Short: "Record that a project may run the commands its recipe names",
		Long: `Record an allow for what the recipe runs now, without starting a run. The
commands are shown first and confirmed at the terminal. With no terminal
attached nothing is recorded unless --yes is given.

An allow is keyed to the commands shown: a recipe edit that changes them asks
again. To grant one process trust without recording anything, set
KAPI_TRUST_EXEC=1 instead.`,
		Example: `  kapi trust allow -p ./app
  kapi trust allow -p ./app --yes`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			recipePath, err := RequireProjectPath(cmd)
			if err != nil {
				return err
			}
			out, err := a.ExecTrustAllow(cmd, recipePath)
			if err != nil {
				return err
			}
			return output.Print(cmd, out)
		},
	}
	AddProjectFlag(allowCmd)

	revokeCmd := &cobra.Command{
		Use:   "revoke [path]",
		Short: "Withdraw the decision recorded for a project",
		Long: `Withdraw the decision recorded for a project, allow or deny, so the next run
asks again. The path names the recipe or its directory; a formatter decision
is named by the configuration file kapi trust list shows for it. Withdrawing
a decision nobody recorded reports that and succeeds.`,
		Example: `  kapi trust revoke ./app
  kapi trust revoke -p ./app`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var path string
			if len(args) == 1 {
				path = args[0]
			} else {
				resolved, err := ResolveProjectPath(cmd)
				if err != nil {
					return err
				}
				if resolved == "" {
					return errors.New("name the project to revoke: kapi trust revoke <path>, or -p <path>")
				}
				path = resolved
			}
			out, err := a.ExecTrustRevoke(path)
			if err != nil {
				return err
			}
			return output.Print(cmd, out)
		},
	}
	AddProjectFlag(revokeCmd)

	trustCmd.AddCommand(listCmd, showCmd, allowCmd, revokeCmd)
	return trustCmd
}
