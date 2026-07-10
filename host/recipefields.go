package host

import (
	"fmt"

	"github.com/neokapi/neokapi/core/project"
)

// WarnInertRecipeFields prints a one-line stderr warning for each recipe
// field that is set but inert because the sibling key its extension depends
// on is missing (e.g. automations: without a server: block). Surfaced from
// the standing commands (kapi status, kapi check --ship) — never from
// project load, so a recipe carrying not-yet-active fields keeps loading
// everywhere else.
func (a *App) WarnInertRecipeFields(cmd Command, proj *project.KapiProject) {
	if a.Quiet || proj == nil {
		return
	}
	for _, in := range proj.InertProjectExtras() {
		msg := fmt.Sprintf("warning: recipe field %q has no effect without a %q block", in.Name, in.DependsOn)
		if in.DependsOn == "server" {
			msg += " — connect the project (kapi init --server <url>) to activate it"
		}
		fmt.Fprintln(cmd.ErrOrStderr(), msg)
	}
}
