// Package projflow lists the project-defined flows under .kapi/flows/ for the
// plugin's command and MCP surfaces, so the two stay decoupled from each other.
// The files themselves are read by the framework (core/project.ListDirFlows),
// which is also what the project runner resolves `kapi run <flow>` through.
package projflow

import (
	coreproj "github.com/neokapi/neokapi/core/project"
	clioutput "github.com/neokapi/neokapi/host/output"
	"github.com/neokapi/neokapi/host/venue/project"
)

// List returns the flow info entries in the project's .kapi/flows/ directory.
// Returns nil when no project is found. A file that does not describe a
// runnable flow is listed with its problem as the description, so it is
// visible where its author looks for it.
func List() []clioutput.FlowInfo {
	proj, err := project.FindProject("")
	if err != nil {
		return nil
	}

	var flows []clioutput.FlowInfo
	for _, def := range coreproj.ListDirFlows(proj.Layout) {
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
