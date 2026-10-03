package cli

import (
	"fmt"
	"os"

	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/version"
	"github.com/neokapi/neokapi/host/config"
	"github.com/spf13/cobra"
)

// FilesAliasName is the multi-call name of the project-free surface that the
// WP5 paired agent evaluation measures (docs/internals/evals.md): the same
// binary, exposing only inspect, apply, formats and the toolbox, with project
// discovery off. The name says what the surface works on and stays inside the
// kapi namespace, so it decides nothing about a product name (decision D14 in
// docs/internals/edit-model.md). `make build` links no such name, kapi's help
// lists none, and the command reference documents none: the evaluation
// installs the link in each of its cells.
const FilesAliasName = "kapi-files"

// filesProjectFlag is the flag host.AddProjectFlag registers on a
// project-aware command.
const filesProjectFlag = "project"

// newFilesRoot builds the root command BusyboxRoot returns for FilesAliasName.
// Every command resolves no project: a -p is refused, KAPI_PROJECT is
// dropped, and KAPI_NO_PROJECT turns the upward walk off, so a file is read
// with the format detection finds and named as the caller named it, wherever
// a kapi.yaml sits. Without a project nothing governs an edit: the commit
// check, the voice and the terms are what a project adds.
func newFilesRoot(app *App) *cobra.Command {
	root := &cobra.Command{
		Use:     FilesAliasName,
		Short:   "Read and edit the content inside files of any format, with no project",
		Version: version.Version,
		Long: `Read any format into content blocks, edit them through a change set, and
write each file back in its own format. Every command works on the files it
is given and resolves no project: no recipe, no terms, no voice and no check
at commit.

  inspect   read files into blocks, with the reference and revision an edit names
  apply     apply a kapi.change/v1 change set
  formats   list the formats and what each can do
  kcat, kgrep, ksed, kdiff, kconv
            read, search, rewrite, compare and convert the content of files`,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.CompletionOptions.DisableDefaultCmd = true
	AddPersistentFlags(app, root)
	root.PersistentPreRunE = func(cmd *cobra.Command, _ []string) error {
		if f := cmd.Flags().Lookup(filesProjectFlag); f != nil && f.Changed {
			return WithExitCode(ExitUsage, fmt.Errorf("%s works without a project and takes no -p", FilesAliasName))
		}
		if err := os.Setenv(project.NoProjectEnvVar, "1"); err != nil {
			return err
		}
		if err := os.Unsetenv(project.ProjectEnvVar); err != nil {
			return err
		}
		app.Config = config.NewAppConfig()
		if err := app.Init(); err != nil {
			return err
		}
		ApplyAppInitializers(app)
		return nil
	}
	root.PersistentPostRun = func(*cobra.Command, []string) { app.Shutdown() }

	cmds := []*cobra.Command{NewInspectCmd(app), NewApplyCmd(app), NewFormatsCmd(app)}
	for _, proxy := range NewToolboxProxies(app) {
		proxy.Hidden = false
		cmds = append(cmds, proxy)
	}
	for _, c := range cmds {
		c.GroupID = ""
		if f := c.Flags().Lookup(filesProjectFlag); f != nil {
			f.Hidden = true
		}
		root.AddCommand(c)
	}
	return root
}
