package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

// The first-meeting notice on the command line.
//
// A clone that arrived with context files under `.kapi/` and a store that has
// never held this project's context is a checkout where nothing in those files
// is in force. Every command that resolves a project says so once, on standard
// error, so the answer a person is reading comes with the reason it is empty.
// `kapi context import` is what reads them.

// AttachContextNotice makes every command under root carry the notice. It
// chains onto whatever the root already does before a command runs, so the
// notice is printed after the app is ready and the flags are parsed.
func AttachContextNotice(a *App, root *cobra.Command) {
	inner := root.PersistentPreRunE
	root.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		if inner != nil {
			if err := inner(cmd, args); err != nil {
				return err
			}
		}
		printContextFilesNotice(a, cmd)
		return nil
	}
}

// printContextFilesNotice writes the notice to standard error, which keeps it
// clear of a `--json` reader's standard output.
func printContextFilesNotice(a *App, cmd *cobra.Command) {
	if a == nil || a.Quiet || skipsContextNotice(cmd) {
		return
	}
	path, err := ResolveProjectPath(cmd)
	if err != nil || path == "" {
		return
	}
	notice, unread := a.ContextFilesUnread(CmdContext(cmd), path)
	if !unread {
		return
	}
	fmt.Fprintln(cmd.ErrOrStderr(), "note: "+notice.Message())
}

// skipsContextNotice reports the commands that would only repeat themselves.
// The context commands that read and write a layout name the files in their own
// report, and the commands that answer questions about kapi rather than about
// the project have no project to say it about.
func skipsContextNotice(cmd *cobra.Command) bool {
	path := cmd.CommandPath()
	for _, verb := range []string{"context import", "context snapshot", "context export", "context restore"} {
		if strings.HasSuffix(path, verb) {
			return true
		}
	}
	switch cmd.Name() {
	case "version", "completion", "help", "update":
		return true
	}
	return false
}
