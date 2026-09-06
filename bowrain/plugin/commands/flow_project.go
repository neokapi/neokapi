package commands

import (
	"github.com/neokapi/neokapi/bowrain/plugin/internal/projflow"
	clioutput "github.com/neokapi/neokapi/host/output"
)

// listProjectFlows returns flow info entries from .kapi/flows/, which the
// plugin registers as the CLI's extra-flows source so `kapi flows` lists a
// project's own flows beside the built-in catalog.
//
// Running one of them is the project runner's job: `kapi run <flow>` resolves
// a .kapi/flows/ definition the same way it resolves an inline `flows:` entry
// on the recipe (host.RunFromProject), over the recipe's collections, with its
// format bindings, locale passes and standing bindings.
func listProjectFlows() []clioutput.FlowInfo {
	return projflow.List()
}
