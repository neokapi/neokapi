package cli

import (
	"errors"

	"github.com/spf13/cobra"
)

// NewUpCmd creates `kapi up`: reconcile the project toward its ship gates.
// It is the porcelain home of convergence (issue #1078 C1): the recipe is the
// desired state, and `up` runs the project's default flow over all content
// across every target language, looping until every gated scope ships or is
// parked for a human. It reuses the same engine as the no-argument `kapi run`
// (RunDefaultFlowConverge), with until-gate looping ON by default.
func NewUpCmd(a *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "up",
		Short:   "Reconcile the project toward its ship gates (run the default flow until converged)",
		GroupID: "work",
		Args:    cobra.NoArgs,
		Long: `Reconcile the project toward its ship gates: treat the recipe as the desired
state and run the project's default flow (defaults.flow) over all content
across every target language, looping until every gated scope is shippable or
parked for a human.

Without defaults.flow, up runs the built-in default flow — TM reuse (recycle)
followed by AI translate — so a recipe needs no flow YAML at all to converge.
Setting defaults.flow replaces the built-in default.

Before each pass, up re-syncs the project block store with the working tree:
edited source files, a store written by another kapi version, or a missing
store trigger a re-extraction (the same shared path behind the desktop's
Re-extract). --no-extract opts out.

Each pass re-derives coverage from the working tree, runs the flow only for
the locales still short of their gate, and stops when everything ships, a pass
makes no progress (the remainder parks — it needs a human), or the pass cap is
reached. After each pass the project's bound checks run over what was
produced: a unit with failing findings (dropped placeholders, glossary
violations) counts as drafted, not translated, so it cannot lift its locale
over the gate until fixed. --no-checks opts out.

When the loop ends, the materialize policy decides whether localized files
are written from the project store: 'defaults.materialize: on-converge' (or
the --materialize flag) writes them for every locale whose gated scopes are
all shippable; the default ('manual') leaves that to 'kapi merge'.

--plan is a dry run: instead of running anything, up reports the pending work
per (collection, locale) — units missing a target, exact TM leverage, the
remaining AI work, and a rough token estimate — with no provider calls and no
writes. Combine with --json for agents.

up never fails the build on target drift: parked, pending target content is
normal toil, reported rather than thrown. Use 'kapi status' to inspect
standing without running anything, and 'kapi check --ship' to enforce the
gates (e.g. before a release tag).

--passes 1 runs a single pass (the behavior of the bare 'kapi run');
--passes N caps the loop at N passes.`,
		Example: `  kapi up                # loop the default flow until every gated scope ships or parks
  kapi up --plan         # dry run: pending work, TM leverage, and a token estimate per locale
  kapi up --passes 1     # a single pass over every locale that needs work
  kapi up --materialize  # also write localized files for the shippable locales
  kapi up -p app.kapi    # reconcile an explicit project recipe`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			projectPath, err := ResolveProjectPath(cmd)
			if err != nil {
				return err
			}
			if projectPath == "" {
				return errors.New("kapi up needs a project — pass -p <recipe> or run from inside a kapi project directory")
			}
			// This RunE only runs when no plugin owns `up` (kapi/cmd/kapi
			// skips the built-in when a plugin declares the verb). So if the
			// recipe declares a server: block, the bowrain plugin that would
			// run it on the server is absent: warn that this converges LOCALLY
			// on the user's own AI keys and does NOT push, rather than silently
			// diverging from the connected behavior. --plan is a read-only dry
			// run, so it needs no warning.
			if !BoolFlag(cmd, "plan") {
				a.WarnIfServerRecipeConvergingLocally(cmd, projectPath)
			}
			return a.ExecuteUp(cmd, projectPath)
		},
	}

	AddProjectFlag(cmd)
	a.AddFlowRunFlags(cmd)
	AddUpFlags(cmd)
	return cmd
}
