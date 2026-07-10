package host

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"

	"github.com/mattn/go-isatty"
	"github.com/neokapi/neokapi/core/convergence"
	"github.com/neokapi/neokapi/core/project"
)

// formatPlanLine compresses the up-plan into the one-line header a run prints
// before executing — cost visibility before any tokens burn.
func formatPlanLine(plan UpPlanOutput) string {
	t := plan.Totals
	if t.MissingTarget == 0 {
		return "plan: every unit has a committed target — verifying gates"
	}
	line := fmt.Sprintf("plan: %d unit(s) missing · %d exact-TM · %d AI · ≈%s tokens",
		t.MissingTarget, t.TMExact, t.AIRemaining, compactTokens(t.TokenEstimate))
	if plan.Provider != "" {
		if plan.Subscription {
			line += fmt.Sprintf(" · %s (your subscription)", plan.Provider)
		} else {
			line += " · " + plan.Provider
		}
	}
	return line
}

// compactTokens renders a token estimate at header scale (61k, 1.2M).
func compactTokens(n int) string {
	switch {
	case n >= 1_000_000:
		return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
	case n >= 1_000:
		return fmt.Sprintf("%dk", n/1_000)
	default:
		return strconv.Itoa(n)
	}
}

// AddUpFlags registers the `kapi up` flag set. Exported so a plugin that owns
// the up verb in a server-connected install (kapi-bowrain) presents the exact
// same local surface and delegates to ExecuteUp for the local venue.
func AddUpFlags(cmd Command) {
	cmd.Flags().Int("passes", 0, "maximum reconciliation passes (0 = loop until converged or parked, capped at 5; 1 = single pass)")
	cmd.Flags().Int("jobs", 0, "how many languages to converge concurrently per pass (0 = the recipe's defaults.jobs, else 4)")
	cmd.Flags().Bool("no-extract", false, "skip the pre-pass source-drift check and block-store re-extraction")
	cmd.Flags().Bool("no-checks", false, "skip the bound checks in the loop (produced units count as translated even when failing guardrails)")
	cmd.Flags().Bool("materialize", false, "after the loop, write localized files from the project store for every shippable locale (forces defaults.materialize: on-converge)")
	cmd.Flags().Bool("plan", false, "dry run: report pending work, TM leverage, and a token estimate per (collection, locale) — no provider calls, no writes")
	cmd.Flags().Bool("json", false, "output the structured result as JSON")
}

// ServerRecipeURL reports whether the recipe at projectPath declares a
// server: block, and the declared URL when parsable. Best-effort: a load or
// decode error reports no server — ExecuteUp surfaces real load failures.
// The server extension schema is owned by the bowrain plugin; this reads the
// raw extension node so the venue decision needs no plugin code.
func (a *App) ServerRecipeURL(projectPath string) (hasServer bool, url string) {
	proj, err := project.LoadWithOptions(projectPath, project.LoadOptions{SkipRequiresCheck: true})
	if err != nil {
		return false, ""
	}
	node, ok := proj.Extras["server"]
	if !ok {
		return false, ""
	}
	var spec struct {
		URL string `yaml:"url"`
	}
	_ = node.Decode(&spec)
	return true, spec.URL
}

// WarnIfServerRecipeConvergingLocally prints a one-line stderr warning when a
// recipe declares a server: block but the built-in up (no bowrain plugin) is
// about to converge it locally: the run spends the user's own AI provider and
// never pushes, so the server copy would silently go stale. Best-effort: any
// load error just skips the warning (ExecuteUp reports real load failures).
func (a *App) WarnIfServerRecipeConvergingLocally(cmd Command, projectPath string) {
	if a.Quiet {
		return
	}
	if hasServer, _ := a.ServerRecipeURL(projectPath); !hasServer {
		return
	}
	fmt.Fprintln(cmd.ErrOrStderr(),
		"warning: this project declares a server: block, but the bowrain plugin is not installed — "+
			"converging locally on your own AI provider; results are NOT pushed to the server. "+
			"Install kapi-bowrain to run `kapi up` on the server (org keys, shared TM, team review).")
}

// ExecuteUp is the local-venue `kapi up` execution behind the command: load
// the project, honor --plan, ensure an AI provider, then run the convergence
// loop with the live UX (plan-first header, per-locale TTY progress, NDJSON
// under --json). Exported so the kapi-bowrain plugin's `up` — which owns the
// verb in a connected install and adds the server venue — can delegate the
// local venue here and stay byte-identical with the built-in behavior.
func (a *App) ExecuteUp(cmd Command, projectPath string) error {
	proj, perr := a.LoadProjectInteractive(cmd.Context(), projectPath, LoadProjectInteractiveOptions{AssumeYes: a.AssumeYes})
	if perr != nil {
		return fmt.Errorf("load project: %w", perr)
	}
	a.InitRegistries()

	if BoolFlag(cmd, "plan") {
		return a.runUpPlan(cmd, proj, projectPath)
	}

	// First-run onboarding: a converge run needs an AI provider; when
	// none is configured anywhere and this is a terminal, walk through
	// the compact provider wizard inline, then continue. Non-TTY runs
	// keep the existing keys-only error path.
	if err := a.EnsureAIProviderInteractive(cmd); err != nil {
		return err
	}

	passes, _ := cmd.Flags().GetInt("passes")
	if passes < 0 {
		return fmt.Errorf("--passes must be >= 0 (0 = loop until converged), got %d", passes)
	}
	// --passes 1 is a single pass (no loop); 0 loops to the default cap;
	// N > 1 loops with N as the cap.
	untilGate := passes != 1
	maxPasses := passes
	if maxPasses == 0 {
		maxPasses = ConvergeMaxPassesDefault
	}
	jobs, _ := cmd.Flags().GetInt("jobs")
	if jobs <= 0 {
		jobs = proj.Defaults.Jobs
	}

	// The run is live by default: --json streams the convergence
	// events as NDJSON (one event per line, a final result record) for
	// agents and CI; otherwise a renderer paints per-locale progress
	// on stderr — in place on a TTY, line-per-event on plain streams.
	// --quiet keeps today's summary-only behavior.
	jsonOut := BoolFlag(cmd, "json")
	var onEvent func(convergence.Event)
	if jsonOut {
		enc := json.NewEncoder(cmd.OutOrStdout())
		onEvent = func(ev convergence.Event) { _ = enc.Encode(ev) }
	} else if !a.Quiet {
		// Plan-first: one line of scope before any tokens burn (the
		// full dry-run table stays under --plan).
		if !cmd.Flags().Changed("source-lang") && proj.Defaults.SourceLanguage != "" {
			a.SourceLang = string(proj.Defaults.SourceLanguage)
		}
		if plan, perr := a.computeProjectPlan(cmd.Context(), proj, projectPath); perr == nil {
			fmt.Fprintln(cmd.ErrOrStderr(), formatPlanLine(plan))
		}
		renderer := NewConvergeRenderer(cmd.ErrOrStderr(), isatty.IsTerminal(os.Stderr.Fd()))
		onEvent = renderer.OnEvent
	}

	var result ConvergeOutput
	if err := a.RunDefaultFlowConverge(cmd, proj, projectPath, ConvergeOptions{
		UntilGate:   untilGate,
		MaxPasses:   maxPasses,
		noExtract:   BoolFlag(cmd, "no-extract"),
		noChecks:    BoolFlag(cmd, "no-checks"),
		materialize: BoolFlag(cmd, "materialize"),
		jobs:        jobs,
		onEvent:     onEvent,
		capture:     &result,
	}); err != nil {
		return err
	}
	return PrintUpResult(cmd, result)
}

// PrintUpResult renders a convergence result the way `kapi up` does: the
// text summary, or — under --json — the NDJSON stream's closing record (the
// structured result, flat, discriminated like every other line). Exported for
// the plugin's up verb, whose remote venue produces the same ConvergeOutput.
func PrintUpResult(cmd Command, result ConvergeOutput) error {
	if BoolFlag(cmd, "json") {
		enc := json.NewEncoder(cmd.OutOrStdout())
		return enc.Encode(struct {
			Type string `json:"type"`
			ConvergeOutput
		}{Type: "result", ConvergeOutput: result})
	}
	return result.FormatText(cmd.OutOrStdout())
}
