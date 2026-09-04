package host

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/mattn/go-isatty"
	"github.com/neokapi/neokapi/core/convergence"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/host/output"
)

// formatPlanLine compresses the up-plan into the one-line header a run prints
// before executing — cost visibility before any tokens burn.
func formatPlanLine(plan UpPlanOutput) string {
	t := plan.Totals
	if plan.Monolingual {
		return "plan: no target languages, reconciling the source only"
	}
	if t.MissingTarget == 0 && t.Stale == 0 && t.Unanswered == 0 {
		if t.UnreadTargets > 0 {
			return fmt.Sprintf("plan: %d produced unit(s) not priced: the store has not read their committed "+
				"translations yet, and this run reads them first", t.UnreadTargets)
		}
		return "plan: every unit has a committed target the content memory answers, so this run verifies gates"
	}
	// The kinds of work the run will do, then what it costs. Each is named
	// separately because the reader is being told something different about it —
	// a stale unit is one a person had decided and this run is about to spend a
	// provider call replacing that decision's subject; an unanswered one holds a
	// translation the record does not stand behind, and the draft will replace it
	// — but the leverage and token figures cover all of them, because the run
	// does not treat them differently.
	var work []string
	if t.MissingTarget > 0 {
		work = append(work, fmt.Sprintf("%d unit(s) missing", t.MissingTarget))
	}
	if t.Stale > 0 {
		work = append(work, fmt.Sprintf("re-drafting %d stale unit(s): their source changed since the translation was decided", t.Stale))
	}
	if t.Unanswered > 0 {
		work = append(work, fmt.Sprintf("drafting %d unit(s) the content memory does not answer", t.Unanswered))
	}
	// Stored drafts are named the way the run's own per-locale line names them,
	// and only when there are any: a run that serves nothing from the store
	// reads as it always has.
	drafts := ""
	if t.Drafts > 0 {
		drafts = fmt.Sprintf(" · %d drafts", t.Drafts)
	}
	line := fmt.Sprintf("plan: %s · %d exact-content memory%s · %d AI · ≈%s tokens",
		strings.Join(work, " · "), t.MemoryExact, drafts, t.AIRemaining, compactTokens(t.TokenEstimate))
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
	cmd.Flags().Int("passes", 0, "maximum reconciliation passes (0 = loop until up to date or parked, capped at 5; 1 = single pass)")
	cmd.Flags().Int("jobs", 0, "how many languages to catch up concurrently per pass (0 = the recipe's defaults.jobs, else 4)")
	cmd.Flags().Bool("no-extract", false, "skip the pre-pass source-drift check and block-store re-extraction")
	cmd.Flags().Bool("no-checks", false, "skip the bound checks in the loop (produced units count as translated even when failing guardrails)")
	cmd.Flags().Bool("materialize", false, "after the loop, write target-language files from the project store for every shippable locale (forces defaults.materialize: on-converge)")
	cmd.Flags().Bool("plan", false, "dry run: report pending work, content-memory leverage, and a token estimate per (collection, locale), with no provider calls and no writes")
	cmd.Flags().Bool("json", false, "output the structured result as JSON")
}

// ServerRecipeURL reports whether the recipe at projectPath binds a
// convergence venue, and the declared URL when parsable. Best-effort: a load
// or decode error reports none — ExecuteUp surfaces real load failures.
func (a *App) ServerRecipeURL(projectPath string) (hasServer bool, url string) {
	proj, err := project.LoadWithOptions(projectPath, project.LoadOptions{SkipRequiresCheck: true})
	if err != nil {
		return false, ""
	}
	venue, ok := proj.Venue()
	return ok, venue.URL
}

// WarnIfServerRecipeConvergingLocally prints a one-line stderr warning when a
// recipe binds a convergence venue but the built-in up (no plugin providing
// the venue's plumbing) is about to converge it locally: the run spends the
// user's own AI provider and never pushes, so the server copy would silently
// go stale. Best-effort: any load error just skips the warning (ExecuteUp
// reports real load failures).
func (a *App) WarnIfServerRecipeConvergingLocally(cmd Command, projectPath string) {
	if a.Quiet {
		return
	}
	proj, err := project.LoadWithOptions(projectPath, project.LoadOptions{SkipRequiresCheck: true})
	if err != nil {
		return
	}
	venue, ok := proj.Venue()
	if !ok {
		return
	}
	fmt.Fprintf(cmd.ErrOrStderr(),
		"warning: this project declares a %s: block, but the bowrain plugin is not installed. "+
			"Running the loop locally on your own AI provider; results are NOT pushed to the server. "+
			"Install kapi-bowrain to run `kapi up` on the server (org keys, shared content memory, team review).\n",
		venue.Key)
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

	// Seed before the plan, not only before the loop: the plan's leverage figure
	// is read out of the project store, so a store left behind git reports less
	// leverage than the project has and a token estimate for translating a
	// corpus git already carries the reviewed wording for. The converge path
	// seeds again; the compile is digest-keyed, so the second call is a read.
	//
	// Except on a store that does not exist yet. A plan must not CREATE one —
	// that is a dry run with a side effect, and computeProjectPlan goes out of
	// its way to stat the store rather than open it for exactly that reason —
	// so a fresh clone's plan reports the no-leverage case an absent store
	// legitimately is, and the run that follows seeds it.
	if !BoolFlag(cmd, "plan") || projectStoreExists(projectPath) {
		if seeded, serr := a.SeedProjectContext(cmd.Context(), projectPath); serr != nil {
			return fmt.Errorf("seed committed context: %w", serr)
		} else if seeded.Compiled() && !a.Quiet {
			fmt.Fprintln(cmd.ErrOrStderr(), formatSeedLine(seeded))
		}
	}

	if BoolFlag(cmd, "plan") {
		return a.runUpPlan(cmd, proj, projectPath)
	}

	// First-run onboarding: a converge run needs an AI provider; when
	// none is configured anywhere and this is a terminal, walk through
	// the compact provider wizard inline, then continue. Non-TTY runs
	// keep the existing keys-only error path.
	//
	// A monolingual project is asked for none: it makes no provider calls, so
	// demanding a key would stop the front-door journey — and in CI, where the
	// wizard cannot run, stop it with an error about a provider nothing was
	// going to use.
	if proj.DeclaresTargetLanguages() {
		if err := a.EnsureAIProviderInteractive(cmd); err != nil {
			return err
		}
	}

	passes, _ := cmd.Flags().GetInt("passes")
	if passes < 0 {
		return fmt.Errorf("--passes must be >= 0 (0 = loop until up to date), got %d", passes)
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

	// The recipe's source language governs the run, whatever the run prints —
	// so it is adopted here, before the branch that chooses the output mode,
	// rather than inside one of its arms. The plan preamble below and the loop
	// itself then read the same language (host/sourcelang.go).
	a.ResolveSourceLang(proj.Defaults.SourceLanguage)

	// The run is live by default: --json streams the convergence
	// events as NDJSON (one event per line, a final result record) for
	// agents and CI; otherwise a renderer paints per-locale progress
	// on stderr — in place on a TTY, line-per-event on plain streams.
	// --quiet keeps today's summary-only behavior.
	jsonOut := BoolFlag(cmd, "json")
	var onEvent func(convergence.Event)
	var stream *output.NDJSONStream
	if jsonOut {
		// One stream for the whole NDJSON document — every progress line and the
		// closing result record — so a truncation anywhere in it is reported once,
		// at the end, instead of failing per event or vanishing.
		stream = output.NewNDJSONStream(cmd.OutOrStdout())
		onEvent = func(ev convergence.Event) { stream.Emit(ev) }
	} else if !a.Quiet {
		// Plan-first: one line of scope before any tokens burn (the
		// full dry-run table stays under --plan).
		//
		// The preamble is a courtesy line, so a plan that cannot be computed must
		// not stop a run that is otherwise fine — the converge below opens its own
		// content memory and does its own resolution. But it is REPORTED: the
		// preamble silently vanishing looks identical to a build where it was
		// never printed, and the usual cause (a memory store that will not open)
		// is the same fault that would make the plan's leverage figure wrong.
		if plan, perr := a.computeProjectPlan(cmd.Context(), proj, projectPath); perr != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "Warning: up: cannot show the plan preamble: %v (the run continues)\n", perr)
		} else {
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
	return PrintUpResultStream(cmd, stream, result)
}

// PrintUpResult renders a convergence result the way `kapi up` does. Callers that
// streamed events into an NDJSONStream should use PrintUpResultStream instead, so
// the closing record joins the same stream and a truncation anywhere in the
// document is reported once.
func PrintUpResult(cmd Command, result ConvergeOutput) error {
	return PrintUpResultStream(cmd, nil, result)
}

// PrintUpResultStream renders a convergence result: the text summary, or — under
// --json — the NDJSON stream's closing record (the structured result, flat,
// discriminated like every other line), written to stream. A nil stream gets a
// fresh one, so a caller with no event phase needs no ceremony. Exported for the
// plugin's up verb, whose remote venue produces the same ConvergeOutput.
//
// The returned error carries the stream's verdict, not just this record's: an
// encode that failed 200 events ago left the consumer with a truncated document
// and must fail the command, while a consumer that closed the pipe must not
// (`kapi up --json | head` is ordinary use).
func PrintUpResultStream(cmd Command, stream *output.NDJSONStream, result ConvergeOutput) error {
	if !BoolFlag(cmd, "json") {
		return result.FormatText(cmd.OutOrStdout())
	}
	if stream == nil {
		stream = output.NewNDJSONStream(cmd.OutOrStdout())
	}
	// The record's own failure is already recorded on the stream; Report renders
	// it — with the count of what got through — for every path.
	_ = stream.Encode(struct {
		Type string `json:"type"`
		ConvergeOutput
	}{Type: "result", ConvergeOutput: result})
	return stream.Report("kapi up --json")
}
