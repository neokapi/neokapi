package cli

import (
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/neokapi/neokapi/core/project"
)

// CheckCommandState prints, before a command runs, the one-time lines about
// the state of the checkout the command resolves (project.NoticeState). A
// command that resolves no project, or whose project does not resolve, is
// left to say so itself. It runs in the root command's PersistentPreRunE.
func CheckCommandState(cmd *cobra.Command) error {
	path, err := ResolveProjectPath(cmd)
	if err != nil || path == "" {
		return nil
	}
	// The one-time lines about this checkout's state: state kept for another
	// folder at this location, and state an earlier kapi left inside it.
	project.NoticeState(project.LayoutAt(filepath.Dir(path)))
	return nil
}
