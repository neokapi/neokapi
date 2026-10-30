package cli

import (
	"github.com/spf13/cobra"

	"github.com/neokapi/neokapi/host"
	"github.com/neokapi/neokapi/host/output"
)

// `kapi context suggest`: what kapi can see in the project's own files, filed
// as suggestions for a person to decide. It is the CLI half of
// host.SuggestFromFiles, which reads the files the recipe declares and calls
// no model.

func newContextSuggestCmd(a *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "suggest",
		Short: "File what kapi can see in the files as suggestions for you to decide",
		Long: `Read the files this project declares and file what kapi can see in them as
suggestions:

  - a word the files write more than one way, such as e-mail and email: the
    form written most often, instead of the others
  - in a project that binds no voice, the words a starter voice pack avoids,
    when the files use three or more of them
  - the files addressing the reader as you, when most blocks with a personal
    pronoun do and none says we

Each suggestion is noticed by kapi and waits for you in "kapi context review".
Checks report it and none fails on it until you keep it. Nothing else is
written: not the recipe, not a rules file. No AI model is used.

Running it again files nothing new. A suggestion you dropped stays dropped,
and one the files have changed is filed again as it now stands. --dry-run
prints what it would file and records nothing.`,
		Example: "  kapi context suggest\n" +
			"  kapi context suggest --dry-run\n" +
			"  kapi context suggest --json",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			projectPath, err := RequireProjectPath(cmd)
			if err != nil {
				return err
			}
			dry, _ := cmd.Flags().GetBool("dry-run")
			res, err := a.SuggestFromFiles(cmd.Context(), host.ContextSuggestRequest{Project: projectPath, DryRun: dry})
			if err != nil {
				return err
			}
			return output.Print(cmd, res)
		},
	}
	cmd.Flags().Bool("dry-run", false, "print what would be filed and record nothing")
	AddProjectFlag(cmd)
	return cmd
}
