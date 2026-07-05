package commands

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/mattn/go-isatty"
	apiclient "github.com/neokapi/neokapi/bowrain/core/client"
	"github.com/neokapi/neokapi/bowrain/core/connector"
	"github.com/neokapi/neokapi/bowrain/core/project"
	"github.com/neokapi/neokapi/cli"
	"github.com/neokapi/neokapi/core/convergence"
	"github.com/spf13/cobra"
)

var (
	upLocal   bool
	upTimeout time.Duration
)

// upCmd owns the `up` verb in a bowrain-connected install: `kapi up` reconciles
// the project toward its ship gates. In a connected project the loop runs on
// the Bowrain server by default (org keys, shared TM/terminology, always-on);
// --local runs it on this machine and then pushes the results so the server
// never goes stale. When the recipe has no server: block, up is purely local —
// identical to the built-in `kapi up`.
//
// The plugin declares `up` in its manifest, so kapi dispatches `kapi up` here
// (the plugin wins the verb) once kapi-bowrain is installed.
var upCmd = &cobra.Command{
	Use:   "up",
	Short: "Reconcile the project toward its ship gates (runs on the server when connected)",
	Args:  cobra.NoArgs,
	Long: `Reconcile the project toward its ship gates: treat the recipe as the desired
state and run the project's default flow over all content across every target
language, looping until every gated scope ships or is parked for a human.

In a server-connected project (a recipe with a server: block) the loop runs on
the Bowrain server by default — on the org's keys, against the org's TM and
terminology — and this command pushes local changes, streams the server run's
live progress, and pulls the produced targets when it converges. Parked units
land in the team's review queue on the server.

  --local   run the loop on this machine instead, then push the results so the
            server stays up to date.

Without a server: block the command is the local loop, identical to kapi up in
the open-source binary.

--plan is always computed locally against the working tree (no server call): it
reports the pending work, TM leverage, and a token estimate per locale.`,
	Example: `  kapi up            # connected project: converge on the server, stream progress, pull results
  kapi up --local    # converge on this machine, then push the results to the server
  kapi up --plan     # dry run: pending work and a token estimate (computed locally)`,
	RunE: runUp,
}

func runUp(cmd *cobra.Command, _ []string) error {
	projectPath, err := cli.ResolveProjectPath(cmd)
	if err != nil {
		return err
	}
	if projectPath == "" {
		return errors.New("kapi up needs a project — run inside a kapi project directory or pass -p <recipe>")
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
		fmt.Fprintln(cmd.ErrOrStderr(), "Pushing converged results to the server...")
	}
	pr, conn, err := doPush(cmd.Context(), connector.PushOptions{}, nil)
	if err != nil {
		return fmt.Errorf("push after local converge: %w", err)
	}
	defer conn.Close()
	if proj, perr := project.FindProject(""); perr == nil {
		if _, cerr := conceptPush(cmd.Context(), proj, false); cerr != nil {
			return cerr
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
	pr, conn, err := doPush(ctx, connector.PushOptions{}, nil)
	if err != nil {
		return fmt.Errorf("push: %w", err)
	}
	defer conn.Close()
	if proj, perr := project.FindProject(""); perr == nil {
		if _, cerr := conceptPush(ctx, proj, false); cerr != nil {
			return cerr
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

	// Phase 2: start (or join) the server convergence run.
	run, err := client.StartConvergenceRun(ctx, apiclient.StartConvergenceRunRequest{Trigger: "cli"})
	if err != nil {
		return fmt.Errorf("start convergence run: %w", err)
	}
	if !app.Quiet && !jsonOut {
		fmt.Fprintf(stderr, "Server run %s started.\n", run.ID)
	}

	// Phase 3: subscribe to the run's event stream and re-emit it locally.
	var sink func(convergence.Event)
	if jsonOut {
		enc := json.NewEncoder(cmd.OutOrStdout())
		sink = func(ev convergence.Event) { _ = enc.Encode(ev) }
	} else if !app.Quiet {
		sink = cli.NewConvergeEventRenderer(stderr, isatty.IsTerminal(os.Stderr.Fd()))
	}
	acc := &runAccumulator{}
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
			return fmt.Errorf("stream convergence run: %w", err)
		}
		if !app.Quiet && !jsonOut {
			fmt.Fprintln(stderr, "Timed out waiting for the run; pulling available results...")
		}
	}

	// Phase 4: transport — pull the produced targets back down.
	if !app.Quiet && !jsonOut {
		fmt.Fprintln(stderr, "Pulling results...")
	}
	if _, err := doPull(ctx, conn, nil, false, false); err != nil {
		return fmt.Errorf("pull: %w", err)
	}
	if proj, perr := project.FindProject(""); perr == nil {
		if _, baseline, cerr := conceptPull(ctx, proj, false); cerr != nil {
			return cerr
		} else if baseline != nil {
			conn.SetConceptBaseline(baseline)
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
	if err := cli.PrintUpResult(cmd, acc.output(final)); err != nil {
		return err
	}
	// A run that ended failed/canceled is not ordinary parked work: surface it
	// as a non-zero exit so CI on `kapi up` does not read a broken run as
	// success. The summary above still prints for context.
	return acc.terminalError(final)
}

// syncConvergePolicy pushes the recipe's server.converge value to the server so
// its continuous-convergence clock matches the project's declared policy. Any
// failure degrades to a one-line stderr note — the run itself is unaffected.
func syncConvergePolicy(ctx context.Context, client *apiclient.BowrainClient, server *project.ServerSpec) {
	if client == nil || server == nil {
		return
	}
	if err := client.SetConvergePolicy(ctx, string(server.ResolvedConverge())); err != nil && !app.Quiet {
		fmt.Fprintf(os.Stderr, "warning: could not update the server's convergence policy: %v\n", err)
	}
}

func init() {
	// The flow-run flags need the live *cli.App (AddFlowRunFlags is a method),
	// which isn't captured until the app initializer fires — after init(). So
	// wire the app-independent flags now and add the flow-run flags in the
	// command factory, which receives the App.
	cli.AddProjectFlag(upCmd)
	cli.AddUpFlags(upCmd)
	upCmd.Flags().BoolVar(&upLocal, "local", false, "converge on this machine instead of the server, then push the results")
	upCmd.Flags().DurationVar(&upTimeout, "timeout", 15*time.Minute, "maximum time to wait for a server run to finish before pulling available results")
	cli.RegisterCommandFactory(func(parent *cobra.Command, a *cli.App) {
		// Match the built-in `kapi up` flag surface exactly (NewUpCmd adds the
		// flow-run flags): without --provider/--model/--tm/--target-lang/… a
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
