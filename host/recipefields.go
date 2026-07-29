package host

import (
	"fmt"
	"io"

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

// UnsyncedCoordinatesWarning is what a run prints when a project governs its
// content by coordinates and is also connected to a server. The recipe is the
// authoring surface for governance, but only one venue reads it at a time: a
// local run resolves the coordinates, while the server has no coordinate rows
// to resolve them from and falls back to the project-wide voice. Saying so is
// the whole point — the same content coming back in two different voices
// depending on where the loop ran is the failure this warning exists to
// pre-empt.
const UnsyncedCoordinatesWarning = "warning: coordinate governance (coordinates:/profiles:) applies to local runs only — " +
	"this project is connected to a server, which governs by defaults.brand_voice until the coordinates are synced"

// WarnUnsyncedCoordinates writes UnsyncedCoordinatesWarning to w when the recipe
// both governs by coordinates and carries a server: block. Surfaced from the run
// entry points, where the venue is decided — never from project load, so
// authoring a context space stays free of warnings everywhere else.
func (a *App) WarnUnsyncedCoordinates(w io.Writer, proj *project.KapiProject) {
	if a.Quiet || w == nil || proj == nil || !proj.GovernsByCoordinates() {
		return
	}
	if _, connected := proj.Extras["server"]; !connected {
		return
	}
	fmt.Fprintln(w, UnsyncedCoordinatesWarning)
}
