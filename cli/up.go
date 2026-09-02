package cli

import (
	"errors"
	"fmt"
	"time"

	"github.com/neokapi/neokapi/host"
	"github.com/neokapi/neokapi/host/pluginhost"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// NewUpCmd creates `kapi up`: bring the project up to date.
// It is the porcelain home of the kapi loop (the convergence model, issue
// #1078 C1): the recipe is the desired state, and `up` runs the project's
// default flow over all content across every target language, looping until
// every gated scope ships or is parked for a human. It reuses the same engine
// as the no-argument `kapi run` (RunDefaultFlowConverge), with until-gate
// looping ON by default.
//
// up owns the verb in every install (the no-shadowing rule). The venue is
// resolved here: when the recipe binds a convergence venue and a plugin
// provides the hidden `server-up` plumbing (kapi-bowrain), the run is
// dispatched there — push, converge on the server, stream progress, pull
// results. Otherwise the loop runs locally via ExecuteUp. The resolved venue
// is printed up front whenever a bound venue makes the choice ambiguous.
func NewUpCmd(a *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "up",
		Short:   "Bring the project up to date (run the default flow until every gated scope ships or parks)",
		GroupID: "work",
		Args:    cobra.NoArgs,
		Long: `Bring the project up to date: the recipe declares the languages, the flow, and
the gates that decide shippable; kapi up runs the project's default flow
(defaults.flow) over every target language, looping until every gated scope is
shippable or parked for a human.

Without defaults.flow, up runs the built-in default flow, Memory reuse (recycle)
followed by AI translate, so a recipe needs no flow YAML at all to catch up.
Setting defaults.flow replaces the built-in default.

Before each pass, up re-syncs the project block store with the working tree:
edited source files, a store written by another kapi version, or a missing
store trigger a re-extraction (the same shared path behind the desktop's
Re-extract). --no-extract opts out.

Each pass re-derives coverage from the working tree, runs the flow only for
the locales still short of their gate, and stops when everything ships, a pass
makes no progress (the remainder parks, because it needs a human), or the pass cap is
reached. After each pass the project's bound checks run over what was
produced: a unit with failing findings (dropped placeholders, terminology
violations) still counts as translated, which it is, and holds its locale out of
shipping until the finding is fixed. 'kapi status' runs the same checks and
reports the same verdict. --no-checks opts out.

The materialize policy decides whether the run owns delivery of the
target-language files. Under 'defaults.materialize: on-converge' (or the
--materialize flag) it does, and the ship gate holds it back: each pass drafts
into a run-local tree, and only a locale whose gated scopes are all shippable
has its files written where the recipe points, and a parked locale's files are
absent, not merely unblessed. The default ('manual') leaves delivery to
'kapi merge' and claims no gate.

Venue: in a server-connected project (a recipe with a bowrain: block, with the
bowrain plugin installed) the loop runs on the Bowrain server by default, on
the org's keys, against the org's shared Memory and terminology, and this command
pushes local changes, streams the server run's live progress, and pulls the
produced targets. --local keeps the loop on this machine and then pushes the
results so the server never goes stale; --server fails rather than falling
back to a local run. The resolved venue is printed first whenever the recipe
connects to a server. Without that block, up is purely local.

--plan is a dry run in every venue: instead of running anything, up reports
the pending work per (collection, locale): units missing a target, exact Memory
leverage, the remaining AI work, and a rough token estimate, computed locally
against the working tree, with no provider calls and no writes. Combine with
--json for agents.

up never fails the build on target drift: parked, pending target content is
normal toil, reported rather than thrown. Use 'kapi status' to inspect
standing without running anything, and 'kapi check --ship' to enforce the
gates (e.g. before a release tag).

--passes 1 runs a single pass (the behavior of the bare 'kapi run');
--passes N caps the loop at N passes.`,
		Example: `  kapi up                # loop the default flow until every gated scope ships or parks
  kapi up --plan         # dry run: pending work, content-memory leverage, and a token estimate per locale
  kapi up --passes 1     # a single pass over every locale that needs work
  kapi up --materialize  # also write target-language files for the shippable locales
  kapi up --local        # connected project: run the loop on this machine, then push the results
  kapi up -p kapi.yaml   # bring an explicit project recipe up to date`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			projectPath, err := ResolveProjectPath(cmd)
			if err != nil {
				return err
			}
			if projectPath == "" {
				return errors.New("kapi up needs a project. Pass -p <recipe> or run from inside a kapi project directory")
			}
			return runUpWithVenue(a, cmd, projectPath)
		},
	}

	AddProjectFlag(cmd)
	a.AddFlowRunFlags(cmd)
	AddUpFlags(cmd)
	cmd.Flags().Bool("local", false, "run the loop on this machine even when the recipe declares a server (the results are then pushed so the server stays current)")
	cmd.Flags().Bool("server", false, "require the server venue: fail rather than run the loop locally when the recipe has no server or the server plumbing is unavailable")
	cmd.Flags().Duration("timeout", 15*time.Minute, "server venue: maximum time to wait for the server run to finish before pulling available results")
	return cmd
}

// runUpWithVenue executes the run at the venue host.ResolveUpVenue picks — the
// same decision every surface offering the verb makes, an agent over MCP
// included. What is the terminal's own is here: the resolved-venue line, the
// user's flags re-rendered as argv for the plumbing subprocess, and the live
// stdio the subprocess inherits.
func runUpWithVenue(a *App, cmd *cobra.Command, projectPath string) error {
	local := BoolFlag(cmd, "local")
	forceServer := BoolFlag(cmd, "server")
	if local && forceServer {
		return errors.New("--local and --server are mutually exclusive")
	}

	dec, err := a.ResolveUpVenue(projectPath, upVenueOptions(cmd))
	if err != nil {
		return err
	}

	if dec.Venue == host.UpVenueServer {
		printUpVenue(a, cmd, local, dec.ServerURL)
		return pluginhost.ExecPluginCommand(cmd.Context(), dec.Route, forwardedFlagArgs(cmd.Flags(), "server"))
	}

	if dec.HasServer && dec.Route == nil && !BoolFlag(cmd, "plan") {
		a.WarnIfServerRecipeConvergingLocally(cmd, projectPath)
	}
	return a.ExecuteUp(cmd, projectPath)
}

// upVenueOptions reads the venue knobs off the command's flags.
func upVenueOptions(cmd *cobra.Command) host.UpVenueOptions {
	return host.UpVenueOptions{
		Local:       BoolFlag(cmd, "local"),
		ForceServer: BoolFlag(cmd, "server"),
		Plan:        BoolFlag(cmd, "plan"),
	}
}

// printUpVenue names the resolved venue on stderr before the run starts.
// Printed only when a bound venue makes the choice ambiguous — a plain
// local project gets no banner. Suppressed under --quiet and --json (the
// NDJSON stream is the machine surface; agents read the plan/events).
func printUpVenue(a *App, cmd *cobra.Command, local bool, serverURL string) {
	if a.Quiet || BoolFlag(cmd, "json") {
		return
	}
	target := serverURL
	if target == "" {
		target = "server"
	}
	if local {
		fmt.Fprintf(cmd.ErrOrStderr(), "Venue: local (--local), running the loop on this machine; results push to %s.\n", target)
		return
	}
	fmt.Fprintf(cmd.ErrOrStderr(), "Venue: server, running the loop on %s (use --local to run it on this machine).\n", target)
}

// forwardedFlagArgs re-renders the flags the user set as argv for the plugin
// plumbing subprocess, which parses the same flag surface itself. Flags named
// in except are venue-resolution flags the core owns and the plumbing does
// not know.
func forwardedFlagArgs(fs *pflag.FlagSet, except ...string) []string {
	skip := map[string]bool{}
	for _, name := range except {
		skip[name] = true
	}
	var args []string
	fs.Visit(func(f *pflag.Flag) {
		if skip[f.Name] {
			return
		}
		if sv, ok := f.Value.(pflag.SliceValue); ok {
			for _, v := range sv.GetSlice() {
				args = append(args, "--"+f.Name, v)
			}
			return
		}
		// --name=value survives every value shape (bools, negatives,
		// empty strings) without argv ambiguity.
		args = append(args, "--"+f.Name+"="+f.Value.String())
	})
	return args
}
