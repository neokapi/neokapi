// Package projflow lists the project's file-per-flow definitions, in the
// directory the recipe names with `flows_dir:`, for the plugin's MCP surface.
// The files themselves are read by the framework (core/project.ListDirFlows),
// which is also what the project runner resolves `kapi run <flow>` through and
// what `kapi flows` lists them with (host.ListFlows).
package projflow

import (
	coreproj "github.com/neokapi/neokapi/core/project"
	clioutput "github.com/neokapi/neokapi/host/output"
	"github.com/neokapi/neokapi/host/venue/project"
)

// List returns the flow info entries in the recipe's `flows_dir:`.
// Returns nil when no project is found. A file that does not describe a
// runnable flow is listed with its problem as the description, so it is
// visible where its author looks for it.
func List() []clioutput.FlowInfo {
	proj, err := project.FindProject("")
	if err != nil {
		return nil
	}

	var flows []clioutput.FlowInfo
	for _, def := range coreproj.ListDirFlows(proj.FlowsDirPath()) {
		info := clioutput.FlowInfo{
			Name:        def.Name,
			Description: def.Description,
			Path:        def.Path,
		}
		if def.Err != nil {
			info.Description = def.Err.Error()
		} else {
			info.Steps = len(def.Spec.Steps)
		}
		flows = append(flows, info)
	}
	return flows
}
