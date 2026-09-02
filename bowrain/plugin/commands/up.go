package commands

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/neokapi/neokapi/host/venue/transfer"

	"github.com/mattn/go-isatty"
	"github.com/neokapi/neokapi/bowrain/plugin/commands/output"
	"github.com/neokapi/neokapi/cli"
	"github.com/neokapi/neokapi/core/convergence"
	apiclient "github.com/neokapi/neokapi/host/venue/client"
	"github.com/neokapi/neokapi/host/venue/project"
	"github.com/spf13/cobra"
)

var (
	upLocal   bool
	upTimeout time.Duration
)

// upCmd is the hidden `server-up` plumbing behind `kapi up`'s server venue.
// The built-in up owns the verb in every install (the no-shadowing rule) and
// dispatches here — with the user's flags forwarded — when the recipe
// declares a server: block: the loop runs on the Bowrain server by default
// (org keys, shared content memory/terminology, always-on); --local runs it on this
// machine and then pushes the results so the server never goes stale. The
// local paths delegate to the same ExecuteUp as the built-in, so a
// disconnected recipe behaves byte-identically either way.
var upCmd = &cobra.Command{
	Use:    "server-up",
	Hidden: true,
	Short:  "Plumbing for kapi up's server venue: push, run the loop on the server, stream progress, pull results",
	Args:   cobra.NoArgs,
	Long: `Bring the project up to date against its ship gates: run the project's default
flow over all content across every target language, looping until every gated
scope ships or is parked for a human.

In a server-connected project (a recipe with a bowrain: block) the loop runs on
the Bowrain server by default, on the org's keys, against the org's Memory and
terminology, and this command pushes local changes, streams the server run's
live progress, and pulls the produced targets when the run finishes. Parked
units land in the team's review queue on the server.

The push phase carries the same payload as kapi push: content blocks, governed
terminology edits, and the recipe-bound voice profile (upserted into the
workspace's voice profiles by name; the recipe decides whether one is bound).

  --local   run the loop on this machine instead, then push the results so the
            server stays up to date.

Without a bowrain: block the command is the local loop, identical to kapi up in
the open-source binary.

--plan is always computed locally against the working tree (no server call): it
reports the pending work, content-memory leverage, and a token estimate per locale.`,
	Example: `  kapi up            # connected project: run the loop on the server, stream progress, pull results
  kapi up --local    # run the loop on this machine, then push the results to the server
  kapi up --plan     # dry run: pending work and a token estimate (computed locally)`,
	RunE: runUp,
}

func runUp(cmd *cobra.Command, _ []string) error {
	projectPath, err := cli.ResolveProjectPath(cmd)
	if err != nil {
		return err
	}
	if projectPath == "" {
		return errors.New("kapi up needs a project. Run inside a kapi project directory or pass -p <recipe>")
	}
	recipe, err := project.LoadRecipe(projectPath)
	if err != nil {
		return fmt.Errorf("load project: %w", err)
	}
	server := recipe.Server
	connected := server != nil && server.URL != ""

	// --plan is a local dry run in every venue: the server runs the same loop,
	// so the local plan against the working tree is the honest preview.
	// The local venue (no server, or --local) is byte-identical to built-in up.
	if !connected || upLocal || flagBool(cmd, "plan") {
		if err := app.ExecuteUp(cmd, projectPath); err != nil {
			return err
		}
		// With a server AND --local, push the freshly-produced targets so the
		// server is never left stale behind a local converge.
		if connected && upLocal && !flagBool(cmd, "plan") {
			if err := pushAfterLocalConverge(cmd, server); err != nil {
				return err
			}
		}
		return nil
	}

	return runServerUp(cmd, server)
}

// pushAfterLocalConverge uploads the results of a local converge to the server
// so the remote copy never lags behind a `kapi up --local`.
func pushAfterLocalConverge(cmd *cobra.Command, server *project.ServerSpec) error {
	if !app.Quiet {
		fmt.Fprintln(cmd.ErrOrStderr(), "Pushing produced results to the server...")
	}
	pr, conn, err := transfer.Push(cmd.Context(), app, transfer.PushOptions{})
	if err != nil {
		return fmt.Errorf("push after local run: %w", err)
	}
	defer conn.Close()
	// The declared context — collections, coordinates, voice — travelled
	// with the push itself; the push reports what its governance amounted to.
	bres := pr.Brand
	if proj, perr := project.FindProject(""); perr == nil {
		cres, cerr := conceptPush(cmd.Context(), proj, false)
		if cerr != nil {
			return cerr
		}
		if err := reportConceptPush(cmd, nil, cres, flagBool(cmd, "json")); err != nil {
			return err
		}
	}
	syncConvergePolicy(cmd.Context(), conn.Client(), server)
	if !app.Quiet {
		if pr.UpToDate {
			fmt.Fprintln(cmd.ErrOrStderr(), "Server already up to date.")
		} else {
			fmt.Fprintf(cmd.ErrOrStderr(), "Pushed %d block(s) to the server.\n", pr.BlocksPushed)
		}
	}
	return reportVoicePush(cmd, nil, bres, flagBool(cmd, "json"))
}

// reportConceptPush says what the terminology fold inside `kapi up`'s push
// phase amounted to.
//
// `up` ran the fold and discarded its result at both venues. A governed
// terminology edit — a term banned, a concept retired — became a submitted
// change-set waiting on a reviewer, and the run reported block counts and a
// convergence summary with no mention of it. The proposal was real; only the
// telling was missing.
//
// Under --json the same proposal travels as one discriminated NDJSON record
// alongside brand_profile's, carrying the review link as a field. That record is
// the whole of what a non-terminal consumer can see: a CI job summary is built
// from the run's structured output, not from its stderr, so a proposal that
// exists only as a stderr sentence is invisible to the surface most likely to
// be read by the person who has to review it.
func reportConceptPush(cmd *cobra.Command, stream *output.NDJSONStream, res *PushConceptsResult, jsonOut bool) error {
	if res == nil {
		return nil
	}
	if jsonOut {
		if res.ConceptsProposed == 0 {
			return nil
		}
		if stream == nil {
			stream = output.NewNDJSONStream(cmd.OutOrStdout())
		}
		return stream.Encode(struct {
			Type      string `json:"type"`
			Proposed  int    `json:"concepts_proposed"`
			ID        string `json:"changeset_id,omitempty"`
			ReviewURL string `json:"changeset_url,omitempty"`
		}{Type: "changeset", Proposed: res.ConceptsProposed, ID: res.ChangesetID, ReviewURL: res.ChangesetURL})
	}
	if app != nil && app.Quiet {
		return nil
	}
	w := cmd.ErrOrStderr()
	if res.ConceptsApplied > 0 || res.RelationsApplied > 0 {
		fmt.Fprintf(w, "Applied %d concept edit(s) and %d relation edit(s) directly.\n",
			res.ConceptsApplied, res.RelationsApplied)
	}
	if res.ConceptsProposed > 0 {
		fmt.Fprintf(w, "Proposed %d governed terminology edit(s) in change-set %s. They take effect when reviewed.\n",
			res.ConceptsProposed, res.ChangesetID)
		if res.ChangesetURL != "" {
			fmt.Fprintf(w, "Review it at %s\n", res.ChangesetURL)
		}
	}
	return nil
}

// runServerUp is the server-venue up: push local changes, start (or join) a
// server convergence run, stream its event feed through the same renderer the
// local venue uses, then pull the produced targets. The event protocol is
// identical to a local run, so a remote run is indistinguishable in the
// terminal (and as NDJSON under --json).
func runServerUp(cmd *cobra.Command, server *project.ServerSpec) error {
	ctx := cmd.Context()
	jsonOut := flagBool(cmd, "json")
	stderr := cmd.ErrOrStderr()

	// One stream for the whole NDJSON document — the brand-push record, every
	// re-emitted server event, and the closing result — so a write that fails
	// part-way is reported once at the end rather than leaving the consumer with a
	// truncated stream that looks complete. nil in text mode.
	var jsonStream *output.NDJSONStream
	if jsonOut {
		jsonStream = output.NewNDJSONStream(cmd.OutOrStdout())
	}

	// Self-seeding, before anything reads the store. The push carries the
	// project's terminology and reviewed wording out of the store, and on a
	// fresh CI checkout the store is empty — an unseeded run pushes no
	// terminology at all and then pulls the workspace's, leaving the store
	// holding the server's vocabulary and nothing git carried.
	if projectPath, perr := cli.ResolveProjectPath(cmd); perr == nil && projectPath != "" {
		if _, serr := app.SeedProjectContext(ctx, projectPath); serr != nil {
			return fmt.Errorf("seed committed context: %w", serr)
		}
	}

	// Recipe pre-push automations run before the push, exactly as they did for
	// the retired `kapi sync` (up subsumed sync — the hooks must not silently
	// stop firing for projects that migrated CI from sync to up).
	if proj := findProjectForAutomations(); proj != nil {
		if err := runLocalAutomations(cmd, proj, "pre-push"); err != nil {
			return fmt.Errorf("pre-push automation: %w", err)
		}
	}

	// Phase 1: transport — push local changes (pure, no implicit translate).
	if !app.Quiet && !jsonOut {
		fmt.Fprintln(stderr, "Pushing local changes...")
	}
	pr, conn, err := transfer.Push(ctx, app, transfer.PushOptions{})
	if err != nil {
		return fmt.Errorf("push: %w", err)
	}
	defer conn.Close()
	// The declared context — collections, coordinates, voice — travelled
	// with the push itself; the push reports what its governance amounted to.
	bres := pr.Brand
	if proj, perr := project.FindProject(""); perr == nil {
		cres, cerr := conceptPush(ctx, proj, false)
		if cerr != nil {
			return cerr
		}
		if err := reportConceptPush(cmd, jsonStream, cres, jsonOut); err != nil {
			return err
		}
	}
	client := conn.Client()
	if client == nil {
		return errors.New("bowrain: project is not connected to a server")
	}
	// Keep the server's convergence policy in step with the recipe before the
	// run, so an on-push project also converges when CI merely pushes.
	syncConvergePolicy(ctx, client, server)
	if !app.Quiet && !jsonOut {
		if pr.UpToDate {
			fmt.Fprintln(stderr, "Server already up to date.")
		} else {
			fmt.Fprintf(stderr, "Pushed %d block(s).\n", pr.BlocksPushed)
		}
	}
	if err := reportVoicePush(cmd, jsonStream, bres, jsonOut); err != nil {
		return err
	}

	// Phase 2: pre-flight — show the estimate (source readiness FIRST, then the
	// credit/scope estimate) and gate a large run behind confirmation. --yes and
	// --json skip the prompt; a non-TTY without --yes proceeds (CI must not hang).
	proceed, err := confirmServerRun(cmd, client, jsonOut)
	if err != nil {
		return err
	}
	if !proceed {
		if !app.Quiet && !jsonOut {
			fmt.Fprintln(stderr, "Aborted, no run started. Settle your source or add credits, then re-run kapi up.")
		}
		return nil
	}

	// Phase 2b: start (or join) the server convergence run. Scope defaults to
	// "all" (the CLI gates via the confirm above, not a subset); Confirmed records
	// that the estimate was shown and accepted.
	run, err := client.StartConvergenceRun(ctx, apiclient.StartConvergenceRunRequest{
		Trigger:   "cli",
		Confirmed: true,
	})
	if err != nil {
		return fmt.Errorf("start server run: %w", err)
	}
	if !app.Quiet && !jsonOut {
		fmt.Fprintf(stderr, "Server run %s started.\n", run.ID)
	}

	// Phase 3: subscribe to the run's event stream and re-emit it locally.
	var sink func(convergence.Event)
	if jsonOut {
		sink = func(ev convergence.Event) { jsonStream.Emit(ev) }
	} else if !app.Quiet {
		sink = cli.NewConvergeEventRenderer(stderr, isatty.IsTerminal(os.Stderr.Fd()))
	}
	acc := newRunAccumulator()
	onEvent := func(ev convergence.Event) {
		acc.observe(ev)
		if sink != nil {
			sink(ev)
		}
	}

	streamCtx := ctx
	var cancel context.CancelFunc
	if upTimeout > 0 {
		streamCtx, cancel = context.WithTimeout(ctx, upTimeout)
		defer cancel()
	}
	if err := client.StreamConvergenceRunEvents(streamCtx, run.ID, onEvent); err != nil {
		// A timeout is not a failure: pull whatever landed and report.
		if !errors.Is(err, context.DeadlineExceeded) {
			return fmt.Errorf("stream server run: %w", err)
		}
		if !app.Quiet && !jsonOut {
			fmt.Fprintln(stderr, "Timed out waiting for the run; pulling available results...")
		}
	}

	// Phase 4: transport — pull the produced targets back down.
	if !app.Quiet && !jsonOut {
		fmt.Fprintln(stderr, "Pulling results...")
	}
	if _, err := transfer.Pull(ctx, app, conn, nil, false, false); err != nil {
		return fmt.Errorf("pull: %w", err)
	}
	if proj, perr := project.FindProject(""); perr == nil {
		cres, baseline, cerr := conceptPull(ctx, proj, false)
		if cerr != nil {
			return cerr
		}
		if baseline != nil {
			conn.SetConceptBaseline(baseline)
		}
		if cres != nil {
			conn.ObserveTermsRef(cres.TermsRef)
		}
		// The terminology return leg: reviewed decisions the pull brought back
		// are merged into the committed terms source, so they are reviewable in
		// `git diff` rather than living only in the gitignored store. Silence
		// when the merge changed nothing, which is the ordinary night.
		if cres != nil && !app.Quiet && !jsonOut {
			if line := cli.FormatTermsProjection(cres.Projection); line != "" {
				fmt.Fprintln(stderr, line)
			}
		}
	}

	// Recipe post-pull automations run after the pull (sync parity: e.g.
	// reformat/regenerate the pulled locale files).
	if proj := findProjectForAutomations(); proj != nil {
		if err := runLocalAutomations(cmd, proj, "post-pull"); err != nil {
			return fmt.Errorf("post-pull automation: %w", err)
		}
	}

	// Phase 5: final result — prefer the run's authoritative final standing,
	// falling back to what the event stream accumulated.
	final, gerr := client.GetConvergenceRun(ctx, run.ID)
	if gerr != nil {
		final = run
	}
	if err := cli.PrintUpResultStream(cmd, jsonStream, acc.output(final)); err != nil {
		return err
	}
	// A run that ended failed/canceled is not ordinary parked work: surface it
	// as a non-zero exit so CI on `kapi up` does not read a broken run as
	// success. The summary above still prints for context.
	return acc.terminalError(final)
}

// reportVoicePush surfaces the voice profile result of up's push phase the way
// `kapi push` reports it: the same footer line (via output.PushOutput) next to
// the push messages on stderr, or — under --json — one discriminated NDJSON
// line on stdout carrying the same field names as push's JSON output. Silent
// when no profile travelled (nil result) or, for text, under --quiet.
//
// stream is the caller's NDJSON document, so this record shares the run's
// sticky-error accounting; a nil stream gets one of its own (the local-venue
// push, which happens after the local run has closed its stream).
func reportVoicePush(cmd *cobra.Command, stream *output.NDJSONStream, res *transfer.PushVoiceResult, jsonOut bool) error {
	if res == nil {
		return nil
	}
	if jsonOut {
		if stream == nil {
			stream = output.NewNDJSONStream(cmd.OutOrStdout())
		}
		// Unlike host's PrintUpResult this record's error is returned, not
		// dropped. Encode returns nil when the consumer merely went away.
		return stream.Encode(struct {
			Type    string `json:"type"`
			Profile string `json:"voice_profile"`
			Action  string `json:"voice_profile_action"`
			Version int    `json:"voice_profile_version,omitempty"`
			Reason  string `json:"voice_profile_reason,omitempty"`
		}{Type: "voice_profile", Profile: res.Name, Action: res.Action, Version: res.Version, Reason: res.Reason})
	}
	if app != nil && app.Quiet {
		return nil
	}
	out := output.PushOutput{
		VoiceProfile: res.Name,
		VoiceAction:  res.Action,
		VoiceVersion: res.Version,
		VoiceReason:  res.Reason,
	}
	out.FormatVoice(cmd.ErrOrStderr())
	return nil
}

// syncConvergePolicy pushes the recipe's server.converge value to the server so
// its continuous-convergence clock matches the project's declared policy. Any
// failure degrades to a one-line stderr note — the run itself is unaffected.
func syncConvergePolicy(ctx context.Context, client *apiclient.BowrainClient, server *project.ServerSpec) {
	if client == nil || server == nil {
		return
	}
	if err := client.SetConvergePolicy(ctx, string(server.ResolvedConverge())); err != nil && !app.Quiet {
		fmt.Fprintf(os.Stderr, "warning: could not update the server's converge policy (server.converge): %v\n", err)
	}
}

func init() {
	// The flow-run flags need the live *cli.App (AddFlowRunFlags is a method),
	// which isn't captured until the app initializer fires — after init(). So
	// wire the app-independent flags now and add the flow-run flags in the
	// command factory, which receives the App.
	cli.AddProjectFlag(upCmd)
	cli.AddUpFlags(upCmd)
	upCmd.Flags().BoolVar(&upLocal, "local", false, "run the loop on this machine instead of the server, then push the results")
	upCmd.Flags().DurationVar(&upTimeout, "timeout", 15*time.Minute, "maximum time to wait for a server run to finish before pulling available results")
	cli.RegisterCommandFactory(func(parent *cobra.Command, a *cli.App) {
		// Match the built-in `kapi up` flag surface exactly (NewUpCmd adds the
		// flow-run flags): without --provider/--model/--memory/--target-lang/… a
		// documented invocation would break the moment the plugin is installed,
		// since kapi dispatches raw argv to this cobra tree.
		if a != nil {
			a.AddFlowRunFlags(upCmd)
		}
		parent.AddCommand(upCmd)
	})
}

// flagBool reads a bool flag, defaulting to false when the flag is absent.
func flagBool(cmd *cobra.Command, name string) bool {
	v, _ := cmd.Flags().GetBool(name)
	return v
}

// confirmAITokenThreshold is the AI-work size above which `kapi up` asks for
// confirmation even when the balance covers it — so a large paid run is never
// started blind. Below it, a covered run proceeds without a prompt.
const confirmAITokenThreshold = 50_000

// confirmServerRun fetches the provider-free pre-flight estimate and shows it
// before a server run: source readiness FIRST ("N blocks held on source: settle
// your source first"), then the credit/scope estimate. It returns whether the
// run should proceed. A run is gated behind an interactive y/N confirmation when
// the AI work exceeds the workspace balance OR a size threshold; --yes, --quiet,
// and --json skip the prompt (assume yes), and a non-interactive terminal
// without --yes proceeds rather than hanging CI. A best-effort estimate error
// never blocks the run (the run itself is the source of truth).
func confirmServerRun(cmd *cobra.Command, client *apiclient.BowrainClient, jsonOut bool) (bool, error) {
	stderr := cmd.ErrOrStderr()
	est, err := client.EstimateConvergence(cmd.Context())
	if err != nil {
		// Estimate is advisory: an older server or a transient error must not stop
		// the run. Proceed silently (the run still gates source-first server-side).
		return true, nil
	}

	if !jsonOut && !app.Quiet {
		printEstimate(stderr, est)
	}

	if !runNeedsConfirm(est) || app.AssumeYes || jsonOut {
		return true, nil
	}
	// Non-interactive without --yes: proceed rather than hang (CI safety), but say
	// so, so the surprise-spend guard is at least visible in logs.
	if !isatty.IsTerminal(os.Stdin.Fd()) {
		fmt.Fprintln(stderr, "Proceeding without confirmation (non-interactive; pass --yes to silence this, or run interactively to confirm).")
		return true, nil
	}
	fmt.Fprintf(stderr, "\nStart this run? [y/N] ")
	reader := bufio.NewReader(cmd.InOrStdin())
	line, _ := reader.ReadString('\n')
	ans := strings.ToLower(strings.TrimSpace(line))
	return ans == "y" || ans == "yes", nil
}

// runNeedsConfirm reports whether the estimate warrants an explicit y/N
// confirmation: the run does paid AI work (ViaAI > 0) AND either the workspace
// balance does not cover it OR the AI work is large (over the token threshold).
// A run with no AI work (all recycled, or all source held) never prompts. Pure
// so the gating decision is unit-tested without a live server.
func runNeedsConfirm(est *apiclient.ConvergenceEstimate) bool {
	if est == nil || est.Totals.ViaAI == 0 {
		return false
	}
	exceedsBalance := est.Credits != nil && !est.Credits.CoversAllAI
	large := est.Totals.TokenEstimate >= confirmAITokenThreshold
	return exceedsBalance || large
}

// printEstimate renders the pre-flight estimate to w: source readiness first,
// then the per-locale/credit estimate for the ready source.
func printEstimate(w io.Writer, est *apiclient.ConvergenceEstimate) {
	src := est.Source
	fmt.Fprintln(w, "\nPre-flight estimate:")
	// Source readiness FIRST (epic 019): held source is the honest, cheap message.
	if src.Held > 0 {
		fmt.Fprintf(w, "  Source: %d of %d blocks ready; %d held on source. Settle your source first (terminology, brand, source checks), or set defaults.source_gate: none to translate anyway.\n",
			src.Ready, src.Total, src.Held)
	} else if src.Total > 0 {
		fmt.Fprintf(w, "  Source: all %d blocks ready.\n", src.Total)
	}

	if est.Totals.Pending == 0 {
		fmt.Fprintln(w, "  Translate: nothing pending over the ready source.")
		return
	}
	fmt.Fprintf(w, "  Translate (ready source): %d pending · content memory %d (free) · AI %d · ~%d tokens.\n",
		est.Totals.Pending, est.Totals.ViaMemory, est.Totals.ViaAI, est.Totals.TokenEstimate)
	if c := est.Credits; c != nil {
		fmt.Fprintf(w, "  Credits: ~%d credits (~$%.2f) for the AI work; balance %d",
			c.EstimatedCredits, c.EstimatedUSD, c.Balance)
		if c.CoversAllAI {
			fmt.Fprintln(w, ", covering all AI work.")
		} else {
			fmt.Fprintf(w, ", covering ~%d of %d AI units. Add credits to translate the rest.\n", c.CoversAIUnits, est.Totals.ViaAI)
		}
	}
}
