package cli

import (
	"github.com/spf13/cobra"

	"github.com/neokapi/neokapi/host/output"
)

// The sharing half of the context surface (AD C-11). A recipe declares where
// the project's context is shared (`context.backend`); pull and push move the
// operations between this machine and that backend, and backend reports or
// changes the choice for this machine.

func newContextPullCmd(a *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "pull",
		Short: "Merge the context other machines pushed",
		Long: `Read what other machines pushed to the project's context backend and
merge it into this machine's context: every suggestion, decision, term and
approved wording, with its history.

A pull reads only what this machine has not seen, merges it by operation id and
brings the stores up to date. Running it twice changes nothing.

The backend is declared in kapi.yaml:

  context:
    backend: git      # local, file, git or s3

It exits with status 5 when the backend cannot be reached, and nothing changes.`,
		Example: "  kapi context pull\n" +
			"  kapi context pull --json",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			projectPath, err := RequireProjectPath(cmd)
			if err != nil {
				return err
			}
			res, err := a.PullProjectContext(cmd.Context(), projectPath)
			if err != nil {
				return err
			}
			return output.Print(cmd, res)
		},
	}
	AddProjectFlag(cmd)
	return cmd
}

func newContextPushCmd(a *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "push",
		Short: "Share the context recorded on this machine",
		Long: `Write what this machine recorded in the project's context to the context
backend, where every other machine's next pull reads it.

A push adds files and never changes one another machine wrote, so two machines
pushing at once both succeed. When the backend has gained many operations since
its last checkpoint, the push adds one, so a new machine's first pull starts
from it rather than replaying everything.

Withheld originals and the rules you widened to every project stay on this
machine.

It exits with status 5 when the backend cannot be reached; the operations stay
queued for the next push.`,
		Example: "  kapi context push\n" +
			"  kapi context pull && kapi context push",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			projectPath, err := RequireProjectPath(cmd)
			if err != nil {
				return err
			}
			res, err := a.PushProjectContext(cmd.Context(), projectPath)
			if err != nil {
				return err
			}
			return output.Print(cmd, res)
		},
	}
	AddProjectFlag(cmd)
	return cmd
}

func newContextBackendCmd(a *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "backend [local | file <dir> | recipe]",
		Short: "Show or choose where this machine shares the project's context",
		Long: `Show which backend this project's context is shared through, where that
was decided, and how far this machine and the backend were apart at the last
pull or push.

Name a backend to use another one on this machine only. The choice is kept in
this machine's configuration under the project's id, and the recipe is left
alone:

  local         keep this project's context on this machine
  file <dir>    share it through a directory, such as a mounted team share
  recipe        use the backend kapi.yaml declares again

A git or S3 backend is declared in kapi.yaml, where every checkout reads it.`,
		Example: "  kapi context backend\n" +
			"  kapi context backend local\n" +
			"  kapi context backend file /Volumes/team/kapi/docs\n" +
			"  kapi context backend recipe",
		Args: cobra.RangeArgs(0, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			projectPath, err := RequireProjectPath(cmd)
			if err != nil {
				return err
			}
			if len(args) == 0 {
				res, err := a.ContextBackend(cmd.Context(), projectPath)
				if err != nil {
					return err
				}
				return output.Print(cmd, res)
			}
			path := ""
			if len(args) == 2 {
				path = args[1]
			}
			res, err := a.SetContextBackend(cmd.Context(), projectPath, args[0], path)
			if err != nil {
				return err
			}
			return output.Print(cmd, res)
		},
	}
	AddProjectFlag(cmd)
	return cmd
}
