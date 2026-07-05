package cli

import (
	"fmt"
	"strconv"

	"github.com/spf13/cobra"
)

// NewVerifyCmd creates the `kapi verify` command: one project-aware quality
// gate that runs a project's bound brand, terminology, and QA checks in a
// single shot. It returns a single structured pass/fail plus actionable
// findings and exits non-zero on failure, so both CI and an AI assistant can
// loop on it: produce content, run verify, read findings, fix, re-run.
//
// Since #1078 (C1) verify is a hidden one-release alias: its porcelain home is
// `kapi check --ship`, which routes through the same engine (computeVerify).
// The alias keeps its full flag surface and prints a one-line pointer.
func NewVerifyCmd(a *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:    "verify [files...]",
		Short:  "Run a project's bound quality gates (brand, terminology, QA) in one shot",
		Hidden: true,
		Long: `Deprecated: use 'kapi check --ship', which absorbs this command.

Run a project's bound quality gates in a single shot and
return a single structured pass/fail plus actionable findings.

Gates (each runs only when the project binds the resource it needs):

  brand        If a brand voice is bound (defaults.brand_voice), score the
               source-language content against it. Fails when the score is
               below the threshold (default ` + strconv.Itoa(DefaultBrandMinScore) + `, override with --min-score).
  terminology  If a termbase is bound (defaults.termbase), check that target
               files use the required translations from the project glossary.
  qa           For translated target files, check placeholder/tag integrity
               against the source and flag untranslated/empty targets.

With no file arguments, verify inspects the project's content: brand on the
source files, terminology and QA on the target files derived from each
content item's target template and the project target languages.

Pass file paths to verify just those files instead.

Selecting gates (--brand/--terms/--qa) and missing bindings: with no gate flag,
every gate runs and a gate whose binding is missing (no defaults.brand_voice, no
defaults.termbase) is skipped silently — there is nothing to check. But when you
explicitly request a gate whose binding is missing, verify does not skip it: it
fails the gate with a clear "misconfigured" finding, so a CI run that asked for
--brand or --terms cannot pass by silently doing nothing. Bind the resource in
the .kapi project, drop the flag, or pass --no-fail to keep it report-only.

Exit codes: 0 pass, 3 when any gate fails (including a requested-but-unbound
gate), 1 for operational errors. Exit 3 means "not on-spec yet", not a crash — in
an assistant fix-loop, read the findings and fix. Pass --no-fail to always exit 0
(report mode) when looping; omit it for CI gating.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// One-release deprecation pointer (#1078): verify forwards to the
			// same engine `kapi check --ship` uses.
			fmt.Fprintln(cmd.ErrOrStderr(), "note: `kapi verify` is deprecated — use `kapi check --ship`; this alias will be removed in a future release.")
			return a.RunVerify(cmd, args)
		},
	}

	AddProjectFlag(cmd)
	AddVerifyFlags(cmd)
	return cmd
}
