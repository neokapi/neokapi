package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

// RootRunE is the RunE for a bare root invocation (`kapi` with no
// subcommand). Inside a project it prints the status summary (the same text
// renderer as `kapi status`) followed by a hint pointing at `kapi up`;
// outside a project it falls back to the standard cobra help. --help output
// is untouched (cobra handles the help flag before RunE runs).
func RootRunE(a *App, cmd *cobra.Command, _ []string) error {
	projectPath, err := ResolveProjectPath(cmd)
	if err != nil || projectPath == "" {
		return cmd.Help()
	}

	// Reuse the status command's renderer against the discovered project.
	// The fresh command carries status's own flags at their defaults; output
	// goes to the root's configured streams.
	scmd := NewStatusCmd(a)
	scmd.SetContext(CmdContext(cmd))
	scmd.SetOut(cmd.OutOrStdout())
	scmd.SetErr(cmd.ErrOrStderr())
	if serr := a.RunStatus(scmd, nil); serr != nil {
		// A project that cannot be summarized (broken recipe, unreadable
		// content) should not make the bare root fail: surface the problem
		// and fall back to help.
		fmt.Fprintf(cmd.ErrOrStderr(), "warning: %v\n", serr)
		return cmd.Help()
	}

	out := cmd.OutOrStdout()
	fmt.Fprintln(out)
	fmt.Fprintln(out, "run `kapi up` to reconcile")
	fmt.Fprintln(out, "kapi --help for all commands")
	return nil
}
