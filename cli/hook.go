package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

// stopHookInput is the subset of the Claude Code Stop-hook stdin payload the
// verify hook reads. Unknown fields (transcript_path, effort, …) are ignored.
type stopHookInput struct {
	CWD            string `json:"cwd"`
	SessionID      string `json:"session_id"`
	StopHookActive bool   `json:"stop_hook_active"`
}

// stopHookDecision is the JSON a Stop hook writes to stdout. An omitted
// Decision lets Claude stop; "block" keeps it working, with Reason fed back as
// the instruction for what to do next. We always exit 0 — the verdict lives in
// the JSON, not the exit code.
type stopHookDecision struct {
	Decision string `json:"decision,omitempty"`
	Reason   string `json:"reason,omitempty"`
}

// hookBlockMaxFindings caps how many findings the block reason lists, so the
// instruction stays focused even on a project with many issues.
const hookBlockMaxFindings = 25

// NewHookCmd creates the hidden `kapi hook` group: integration glue for AI
// coding assistants. The commands read an assistant's hook payload on stdin and
// emit the assistant's expected response on stdout; they are not part of a
// human workflow, so the group is hidden from help.
func (a *App) NewHookCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "hook",
		Short:   "Integration hooks for AI coding assistants (Claude Code)",
		GroupID: "management",
		Hidden:  true,
		Long: `Glue commands for AI coding assistants. Each subcommand reads the
assistant's hook payload on stdin and writes the assistant's expected response
on stdout. These are wired up by the assistant's plugin, not run by hand.`,
	}
	cmd.AddCommand(a.newHookStopCmd())
	return cmd
}

// newHookStopCmd implements the Claude Code `Stop` hook: it runs the current
// project's `kapi verify` gates and, when they fail, tells Claude to keep
// working (with the findings) instead of finishing. This turns the verify gate
// into a hard guardrail — the assistant cannot end a turn with a project that
// is off-brand, off-terminology, or with broken placeholders.
func (a *App) newHookStopCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "stop",
		Short: "Claude Code Stop hook: keep working until the project's kapi verify gates pass",
		Long: `Claude Code Stop hook. Reads the Stop-event JSON on stdin, runs the
verify gates for the project in the session's working directory, and:

  - emits nothing (exit 0) when the project passes, when there is no .kapi
    project, or when verify cannot run — Claude is free to finish; or
  - emits {"decision":"block","reason":"…findings…"} (exit 0) when a gate
    fails, so Claude keeps working and fixes the findings before stopping.

Wire it up via the kapi Claude Code plugin (hooks/hooks.json). It fails open:
anything other than a clean gate failure lets Claude stop, so a missing project
or a verify error never traps the assistant. Claude Code caps consecutive
blocks, so an unfixable finding cannot loop forever.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.runHookStop(cmd)
		},
	}
}

// runHookStop evaluates the project's verify gates and writes the Stop-hook
// decision. It returns nil on every expected path (the decision is the JSON on
// stdout, not the exit code); only an unexpected write failure is an error.
func (a *App) runHookStop(cmd *cobra.Command) error {
	in := readStopHookInput(cmd.InOrStdin())

	// Evaluate the project in the session's working directory. The hook process
	// may start elsewhere, so move into cwd before resolving the project and its
	// relative content globs. If we can't, fail open (let Claude stop).
	if in.CWD != "" {
		if err := os.Chdir(in.CWD); err != nil {
			return nil
		}
	}

	out, err := a.computeVerify(a.NewVerifyCmd(), nil)
	if err != nil {
		// No .kapi project here, or an operational error — nothing to gate on.
		// Don't trap the assistant; let it stop.
		return nil
	}
	if out.Pass {
		return nil
	}

	dec := stopHookDecision{Decision: "block", Reason: hookBlockReason(out)}
	enc := json.NewEncoder(cmd.OutOrStdout())
	enc.SetEscapeHTML(false)
	return enc.Encode(dec)
}

// readStopHookInput parses the Stop-hook JSON from r. Missing or malformed
// input yields a zero value (safe defaults). When r is an interactive terminal
// (the command was run by hand with no piped payload) it does not block on a
// read that would never return.
func readStopHookInput(r io.Reader) stopHookInput {
	var in stopHookInput
	if f, ok := r.(*os.File); ok {
		if fi, err := f.Stat(); err == nil && (fi.Mode()&os.ModeCharDevice) != 0 {
			return in
		}
	}
	data, err := io.ReadAll(r)
	if err != nil || len(bytes.TrimSpace(data)) == 0 {
		return in
	}
	_ = json.Unmarshal(data, &in)
	return in
}

// hookBlockReason renders the failing gates' findings as the instruction fed
// back to Claude. It mirrors what the assistant would see from `kapi verify`
// so the guidance is consistent however the assistant arrives at it.
func hookBlockReason(out VerifyOutput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "kapi verify has not passed yet — %d of %d gate(s) failing. ",
		out.Summary.Failed, out.Summary.Gates)
	b.WriteString("Fix the findings below in the affected files, then finish; ")
	b.WriteString("kapi verify re-checks before you can stop.\n\n")

	shown := 0
	for _, g := range out.Gates {
		if g.Pass {
			continue
		}
		for _, f := range g.Findings {
			if shown >= hookBlockMaxFindings {
				break
			}
			loc := f.File
			if f.Locale != "" {
				loc = strings.TrimSpace(loc + " [" + f.Locale + "]")
			}
			if loc != "" {
				loc = " " + loc
			}
			fmt.Fprintf(&b, "  %s [%s]%s: %s", strings.ToUpper(f.Severity), g.Gate, loc, f.Message)
			if f.Suggestion != "" {
				fmt.Fprintf(&b, " — %s", f.Suggestion)
			}
			b.WriteByte('\n')
			shown++
		}
	}
	if out.Summary.Findings > shown {
		fmt.Fprintf(&b, "  … and %d more.\n", out.Summary.Findings-shown)
	}

	b.WriteString("\nRun `kapi verify` to see the full report. ")
	b.WriteString("A finding on a source file means the source needs fixing too, not just the translation.")
	return b.String()
}
