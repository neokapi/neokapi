package cli

import (
	"fmt"

	"github.com/neokapi/neokapi/host/output"
	"github.com/spf13/cobra"
)

// RootRunE is the RunE for a bare root invocation (`kapi` with no
// subcommand). Inside a project it prints the Map, the same text `kapi look`
// prints: the project's files, blocks, rules, findings and waiting
// suggestions, ending on the next command to run. A line naming where the
// commands are listed follows it. Outside a project it falls back to the
// standard cobra help. --help output is untouched (cobra handles the help flag
// before RunE runs).
func RootRunE(a *App, cmd *cobra.Command, _ []string) error {
	projectPath, err := ResolveProjectPath(cmd)
	if err != nil || projectPath == "" {
		return cmd.Help()
	}

	m, lerr := a.LookProject(cmd)
	if lerr != nil {
		// A project that cannot be read (broken recipe, unreadable content)
		// should not make the bare root fail: surface the problem and fall
		// back to help.
		fmt.Fprintf(cmd.ErrOrStderr(), "warning: %v\n", lerr)
		return cmd.Help()
	}
	if err := output.Print(cmd, m); err != nil {
		return err
	}
	if output.ResolveFormat(cmd) != output.FormatText {
		return nil
	}
	out := cmd.OutOrStdout()
	fmt.Fprintln(out)
	fmt.Fprintln(out, "kapi --help for the commands")
	return nil
}
