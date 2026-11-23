package cli

import (
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/neokapi/neokapi/core/project"
)

// CheckCommandState asks, before a command runs, whether git tracks kapi's
// own state in the project the command resolves (project.CheckStateTracked).
// A command that resolves no project, or whose project does not resolve, is
// left to say so itself. It runs in the root command's PersistentPreRunE, so
// every command that acts on a project has asked before it reads any state.
func CheckCommandState(cmd *cobra.Command) error {
	path, err := ResolveProjectPath(cmd)
	if err != nil || path == "" {
		return nil
	}
	return project.CheckStateTracked(cmd.Context(), project.LayoutAt(filepath.Dir(path)))
}
