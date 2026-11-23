package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"

	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/host/output"
)

// CheckCommandState prints, before a command runs, the one-time lines about
// the state of the checkout the command resolves (project.NoticeState). It
// runs in the root command's PersistentPreRunE.
//
// It points project.StateNotice at the command's error stream when a person
// reads it there: the command concerns a project (ProjectDeclaration), its
// standard error is a terminal, its output is text and it is not quiet. For
// any other command the notices are shown to nobody and stay unrecorded, and
// no project is resolved here, so a command such as `kapi version` reads and
// writes nothing of the project's. A command that resolves no project, or
// whose project does not resolve, is left to say so itself.
func CheckCommandState(cmd *cobra.Command) error {
	shown := stateNoticesShown(cmd)
	errOut := cmd.ErrOrStderr()
	project.StateNotice = func(msg string) bool {
		if !shown {
			return false
		}
		fmt.Fprintln(errOut, msg)
		return true
	}
	if !shown {
		return nil
	}
	path, err := ResolveProjectPath(cmd)
	if err != nil || path == "" {
		return nil
	}
	// The one-time lines about this checkout's state: state kept for another
	// folder at this location, and state an earlier kapi left inside it.
	project.NoticeState(project.LayoutAt(filepath.Dir(path)))
	return nil
}

// stateNoticesShown reports whether a line cmd writes to its error stream
// reaches a person: see CheckCommandState.
func stateNoticesShown(cmd *cobra.Command) bool {
	if uses, _ := ProjectDeclaration(cmd); !uses {
		return false
	}
	if quiet, _ := cmd.Flags().GetBool("quiet"); quiet {
		return false
	}
	if output.ResolveFormat(cmd) != output.FormatText {
		return false
	}
	return writerIsTerminal(cmd.ErrOrStderr())
}

// writerIsTerminal reports whether w is a terminal.
var writerIsTerminal = func(w io.Writer) bool {
	f, ok := w.(*os.File)
	return ok && (isatty.IsTerminal(f.Fd()) || isatty.IsCygwinTerminal(f.Fd()))
}

// AssumeTerminal makes CheckCommandState take any error stream for a
// terminal, as it is when a person runs kapi, and returns the function that
// undoes it. The command tree tests use it to stand in for that person.
func AssumeTerminal() (restore func()) {
	prev := writerIsTerminal
	writerIsTerminal = func(io.Writer) bool { return true }
	return func() { writerIsTerminal = prev }
}
