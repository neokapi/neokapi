package cli

import (
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/neokapi/neokapi/core/project"
)

// StateAdoptAnnotation marks the command a person runs to vouch for a
// project's state (`kapi trust allow --state`). CheckCommandState asks it only
// whether git tracks the state, since its purpose is to answer the refusal of
// state this machine did not write.
const StateAdoptAnnotation = "kapi.state.adopt"

// CheckCommandState asks, before a command runs, whether the project it
// resolves holds state that is not this machine's own
// (project.CheckStateOwned): state git tracks, or state kapi on this machine
// did not write. A command that resolves no project, or whose project does not
// resolve, is left to say so itself. It runs in the root command's
// PersistentPreRunE, so every command that acts on a project has asked before
// it reads any state.
func CheckCommandState(cmd *cobra.Command) error {
	path, err := ResolveProjectPath(cmd)
	if err != nil || path == "" {
		return nil
	}
	layout := project.LayoutAt(filepath.Dir(path))
	if cmd.Annotations[StateAdoptAnnotation] != "" {
		return project.CheckStateTracked(cmd.Context(), layout)
	}
	return project.CheckStateOwned(cmd.Context(), layout)
}
