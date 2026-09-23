package host

import (
	"fmt"
	"io"

	"github.com/neokapi/neokapi/core/project"
)

// WarnInertRecipeFields prints a one-line stderr warning for each recipe
// field that is set but inert because the sibling key its extension depends
// on is missing (e.g. automations: without the venue block). Surfaced from
// the standing commands (kapi status, kapi check --ship) — never from
// project load, so a recipe carrying not-yet-active fields keeps loading
// everywhere else.
func (a *App) WarnInertRecipeFields(cmd Command, proj *project.KapiProject) {
	if a.Quiet || proj == nil {
		return
	}
	for _, in := range proj.InertProjectExtras() {
		msg := fmt.Sprintf("warning: recipe field %q has no effect without a %q block", in.Name, in.DependsOn)
		if project.IsVenueKey(in.DependsOn) {
			msg += ". Connect the project (kapi init --server <url>) to activate it"
		}
		fmt.Fprintln(cmd.ErrOrStderr(), msg)
	}
}

// UnsyncedCoordinatesWarning is what a run prints when a project binds a terms
// store to a profile (`profiles.<name>.termstore`) and is also connected to a
// server.
//
// A push carries every declared collection, its point and the voice governing
// it, so both venues resolve the same voice for the same content. A profile's
// termstore names a store this machine keeps, which a push does not carry, so
// the server checks that content against the workspace vocabulary instead: the
// same content checked against two vocabularies depending on where the loop
// ran.
const UnsyncedCoordinatesWarning = "warning: a profile's termstore: binding applies to local runs only. " +
	"This project is connected to a server, which checks terminology against the workspace vocabulary"

// WarnUnsyncedCoordinates writes UnsyncedCoordinatesWarning to w when the recipe
// binds terms per profile and binds a convergence venue. Surfaced from the run
// entry points, where the venue is decided — never from project load, so
// authoring a context space stays free of warnings everywhere else.
func (a *App) WarnUnsyncedCoordinates(w io.Writer, proj *project.KapiProject) {
	if a.Quiet || w == nil || proj == nil || !proj.BindsTermsByProfile() {
		return
	}
	if _, connected := proj.Venue(); !connected {
		return
	}
	fmt.Fprintln(w, UnsyncedCoordinatesWarning)
}
