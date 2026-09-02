package host

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/neokapi/neokapi/core/project"
)

// stopHookInput is the subset of the Claude Code Stop-hook stdin payload the
// verify hook reads. Unknown fields (transcript_path, effort, …) are ignored.
type stopHookInput struct {
	CWD            string `json:"cwd"`
	SessionID      string `json:"session_id"`
	StopHookActive bool   `json:"stop_hook_active"`
}

// StopHookDecision is the JSON a Stop hook writes to stdout. An omitted
// Decision lets Claude stop; "block" keeps it working, with Reason fed back as
// the instruction for what to do next. We always exit 0 — the verdict lives in
// the JSON, not the exit code.
type StopHookDecision struct {
	Decision string `json:"decision,omitempty"`
	Reason   string `json:"reason,omitempty"`
}

// HookNotice is what a kapi hook writes when it could not evaluate its guard at
// all. It deliberately carries no decision — so the assistant's normal flow is
// untouched and the hook still fails open — plus a systemMessage, Claude Code's
// documented channel for a warning shown to the user.
//
// The shape exists because fail-open without a signal is unobservable: the zero
// value of a hook payload is indistinguishable from a legitimate one, so a guard
// that never ran looked exactly like a guard that passed. An unheard hook is
// never silent.
type HookNotice struct {
	SystemMessage string `json:"systemMessage"`
}

// hookUnheard reports that hook could not evaluate its guard, then allows.
//
// The message goes to both of a hook's channels because they reach different
// readers: stderr is the shell/CI channel (and the only one when the command is
// run by hand), while the systemMessage on stdout is the in-session channel
// Claude Code surfaces to the user. Both name the hook, so an inert guard is
// attributable.
//
// The returned error is the stdout write's, matching the decision paths: a hook
// that cannot write its own stdout has genuinely failed, and Claude Code reads a
// non-zero exit (other than 2) as a non-blocking hook error — it shows stderr to
// the user and lets the operation proceed. So even that path stays fail-open.
func hookUnheard(cmd Command, hook, because string) error {
	msg := fmt.Sprintf("%s: the guard did not run: %s. Allowing, because a kapi hook fails open. "+
		"Treat the result as unverified: the guard never ran.", hook, because)
	fmt.Fprintln(cmd.ErrOrStderr(), "warning: "+msg)
	enc := json.NewEncoder(cmd.OutOrStdout())
	enc.SetEscapeHTML(false)
	return enc.Encode(HookNotice{SystemMessage: msg})
}

// readHookPayload decodes an assistant's hook payload from r into out. It
// returns an empty string when out is usable, and otherwise a clause naming why
// the payload could not be turned into a decision input, so the caller can fail
// open *and say so*.
//
// The three failures are kept apart on purpose — the previous code conflated an
// unreadable stdin with an empty one and then discarded the JSON error entirely,
// which is how an inert guard became invisible. The documented no-payload case
// is not one of them: when r is a character device (a terminal, or /dev/null)
// the command was run by hand with nothing piped in, so there is nothing to read
// and nothing to warn about — and reading would block on a tty forever.
func readHookPayload(r io.Reader, out any) string {
	if f, ok := r.(*os.File); ok {
		if fi, err := f.Stat(); err == nil && (fi.Mode()&os.ModeCharDevice) != 0 {
			return ""
		}
	}
	data, err := io.ReadAll(r)
	if err != nil {
		return fmt.Sprintf("stdin could not be read: %v", err)
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return "the payload on stdin was empty"
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Sprintf("the payload on stdin is not the JSON this hook expects: %v", err)
	}
	return ""
}

// hookBlockMaxFindings caps how many findings the block reason lists, so the
// instruction stays focused even on a project with many issues.
const hookBlockMaxFindings = 25

// RunHookStop evaluates the project's verify gates and writes the Stop-hook
// decision. It returns nil on every expected path (the decision is the JSON on
// stdout, not the exit code); only an unexpected write failure is an error.
func (a *App) RunHookStop(cmd Command) error {
	const hook = "kapi hook stop"

	var in stopHookInput
	if because := readHookPayload(cmd.InOrStdin(), &in); because != "" {
		return hookUnheard(cmd, hook, because)
	}

	// Evaluate the project in the session's working directory. The hook process
	// may start elsewhere, so move into cwd before resolving the project and its
	// relative content globs. If we can't, fail open — but say so: from the wrong
	// directory the upward walk finds a different project, or none, and the gate
	// silently evaluates nothing.
	if in.CWD != "" {
		if err := os.Chdir(in.CWD); err != nil {
			return hookUnheard(cmd, hook, fmt.Sprintf("the session directory %s could not be entered: %v", in.CWD, err))
		}
	}

	// A Stop hook's only channel is the decision JSON on stdout; anything the
	// assistant sees on stderr is treated as a hook error. Reading content may
	// start the okapi-bridge subprocess, which logs to stderr — silence it (and
	// any other gate chatter) so a clean gate failure never looks like a crash.
	restore := silenceStderr()
	vcmd := NewEnvCommand(context.Background(), "verify")
	AddProjectFlag(vcmd)
	AddVerifyFlags(vcmd)
	// Resolve the project before evaluating, so "there is no project here" (the
	// ordinary case, and genuinely nothing to gate) stays apart from "there is a
	// project and its gates could not be evaluated" (an installed-but-inert
	// guard, which must be reported). computeVerify returns one error for both.
	projectPath, perr := ResolveProjectPath(vcmd)
	var out verifyOutput
	var verr error
	if perr == nil && projectPath != "" {
		out, verr = a.computeVerify(vcmd, nil)
	}
	restore()

	switch {
	case perr != nil:
		return hookUnheard(cmd, hook, fmt.Sprintf("the project could not be located: %v", perr))
	case projectPath == "":
		return nil // no kapi project here → nothing to gate; Claude may stop
	case verr != nil:
		return hookUnheard(cmd, hook, fmt.Sprintf("the gates for %s could not be evaluated: %v", projectPath, verr))
	case out.Pass:
		return nil
	}

	dec := StopHookDecision{Decision: "block", Reason: hookBlockReason(out)}
	enc := json.NewEncoder(cmd.OutOrStdout())
	enc.SetEscapeHTML(false)
	return enc.Encode(dec)
}

// silenceStderr redirects os.Stderr to /dev/null and returns a function that
// restores it. Subprocesses launched while it is in effect (the okapi-bridge,
// via cmd.Stderr = os.Stderr) inherit the null stderr for their lifetime, so
// their logging never reaches the assistant as hook-error output.
func silenceStderr() func() {
	orig := os.Stderr
	devnull, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		return func() {}
	}
	os.Stderr = devnull
	return func() {
		os.Stderr = orig
		_ = devnull.Close()
	}
}

// hookBlockReason renders the failing gates' findings as the instruction fed
// back to Claude. It mirrors what the assistant would see from `kapi check --ship`
// so the guidance is consistent however the assistant arrives at it.
func hookBlockReason(out verifyOutput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "kapi check --ship has not passed yet: %d of %d gate(s) failing. ",
		out.Summary.Failed, out.Summary.Gates)
	b.WriteString("Fix the findings below in the affected files, then finish; ")
	b.WriteString("kapi check --ship re-checks before you can stop.\n\n")

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
				fmt.Fprintf(&b, " (%s)", f.Suggestion)
			}
			b.WriteByte('\n')
			shown++
		}
	}
	if out.Summary.Findings > shown {
		fmt.Fprintf(&b, "  … and %d more.\n", out.Summary.Findings-shown)
	}

	b.WriteString("\nRun `kapi check --ship` to see the full report. ")
	b.WriteString("A finding on a source file means the source needs fixing too, not just the translation.")
	return b.String()
}

// preEditHookInput is the subset of the Claude Code PreToolUse-hook stdin
// payload the edit guard reads. The hook is wired with a matcher of
// Edit|Write|MultiEdit, so ToolInput.FilePath is the file about to be written.
type preEditHookInput struct {
	CWD       string `json:"cwd"`
	ToolName  string `json:"tool_name"`
	ToolInput struct {
		FilePath string `json:"file_path"`
	} `json:"tool_input"`
}

// preToolUseHookOutput is the PreToolUse decision payload. permissionDecision
// "deny" blocks the tool call and feeds permissionDecisionReason back to Claude
// as the reason; an empty (unwritten) payload leaves the normal permission flow
// untouched (it does NOT auto-approve), so we only ever emit this to deny.
type preToolUseHookOutput struct {
	HookEventName            string `json:"hookEventName"`
	PermissionDecision       string `json:"permissionDecision"`
	PermissionDecisionReason string `json:"permissionDecisionReason"`
}

// PreToolUseDecision wraps the decision under hookSpecificOutput, the shape
// Claude Code expects from a PreToolUse hook on exit 0.
type PreToolUseDecision struct {
	HookSpecificOutput preToolUseHookOutput `json:"hookSpecificOutput"`
}

// RunHookPreEdit evaluates whether the edited file is a generated target and
// writes the deny decision when it is. It returns nil on every expected path
// (the verdict is the JSON on stdout, not the exit code); only an unexpected
// write failure is an error.
func (a *App) RunHookPreEdit(cmd Command) error {
	const hook = "kapi hook pre-edit"

	var in preEditHookInput
	if because := readHookPayload(cmd.InOrStdin(), &in); because != "" {
		return hookUnheard(cmd, hook, because)
	}

	file := strings.TrimSpace(in.ToolInput.FilePath)
	if file == "" {
		return nil // a well-formed payload with no file path → nothing to guard
	}

	// Resolve the project in the session's working directory. The hook process
	// may start elsewhere, so move into cwd before the git-style upward walk and
	// before resolving the project's relative content globs. Fail open if we
	// can't — but say so: from the wrong directory the walk finds a different
	// project, or none, and the guard matches nothing.
	if in.CWD != "" {
		if err := os.Chdir(in.CWD); err != nil {
			return hookUnheard(cmd, hook, fmt.Sprintf("the session directory %s could not be entered: %v", in.CWD, err))
		}
	}

	projectPath, err := ResolveProjectPath(cmd)
	if err != nil {
		return hookUnheard(cmd, hook, fmt.Sprintf("the project could not be located: %v", err))
	}
	if projectPath == "" {
		return nil // no project here → nothing to guard
	}

	// Silence any gate chatter to stderr while we load and match: a PreToolUse
	// hook's stderr on exit 0 surfaces to the user as a notice, and the verdict
	// belongs in the decision JSON, not in stray logging. Restore it before any
	// warning, so an un-evaluated guard is still heard.
	restore := silenceStderr()
	proj, lerr := project.LoadWithOptions(projectPath, project.LoadOptions{SkipRequiresCheck: true})
	restore()
	if lerr != nil {
		// An unloadable project (e.g. it requires a plugin we lack) leaves the
		// guard installed but inert — exactly the state that must not be silent.
		return hookUnheard(cmd, hook, fmt.Sprintf("the project at %s could not be loaded: %v", projectPath, lerr))
	}
	abs, aerr := filepath.Abs(file)
	if aerr != nil {
		return hookUnheard(cmd, hook, fmt.Sprintf("%s could not be resolved to an absolute path: %v", file, aerr))
	}
	// Match in canonical (symlink-resolved) space. The project root comes from
	// os.Getwd (which may return the real path) while the edited file_path comes
	// from the assistant verbatim; on macOS those differ (/var vs /private/var),
	// and a project under any symlinked path would mismatch otherwise. Reuse the
	// canonical forms for the reason so its paths render relative consistently.
	restore = silenceStderr()
	root := canonicalPath(filepath.Dir(projectPath))
	target := canonicalPath(abs)
	source, locale, isTarget := matchTargetToSource(proj, root, target)
	restore()

	if !isTarget {
		return nil // not a generated target → allow the edit normally
	}

	dec := PreToolUseDecision{HookSpecificOutput: preToolUseHookOutput{
		HookEventName:            "PreToolUse",
		PermissionDecision:       "deny",
		PermissionDecisionReason: preEditDenyReason(root, target, source.Path, locale),
	}}
	enc := json.NewEncoder(cmd.OutOrStdout())
	enc.SetEscapeHTML(false)
	return enc.Encode(dec)
}

// preEditDenyReason renders the instruction fed back to Claude when it tries to
// edit a generated target. It names the target, its source and locale, and the
// round-trip to use instead, mirroring the skill's localize guidance.
func preEditDenyReason(root, targetAbs, sourceAbs, locale string) string {
	target := relForReason(root, targetAbs)
	source := relForReason(root, sourceAbs)
	return fmt.Sprintf(
		"%s is a generated kapi translation target. kapi writes it from %s [%s] and "+
			"overwrites it on the next `kapi merge`, so editing it by hand is discarded and "+
			"never passes through kapi's terminology, placeholder, and voice-profile gates.\n\n"+
			"Don't edit the target directly. To revise the %s translation, route it through kapi: "+
			"`kapi extract --target-lang %s` → fill the targets (follow `kapi voice guide` and the "+
			"terms, keep placeholders intact) → `kapi merge -i out/*.xliff` → `kapi check --ship`. "+
			"To change the meaning for every language, edit the source %s instead and re-run the round-trip.",
		target, source, locale, locale, locale, source,
	)
}

// canonicalPath resolves symlinks in p so two paths to the same file compare
// equal regardless of representation (e.g. macOS /var vs /private/var). When p
// itself does not exist yet (a Write to a new target), it resolves the nearest
// existing parent and rejoins the remainder, so the result is still canonical.
func canonicalPath(p string) string {
	if resolved, err := filepath.EvalSymlinks(p); err == nil {
		return resolved
	}
	dir, base := filepath.Split(filepath.Clean(p))
	if dir == "" {
		return p
	}
	if resolvedDir, err := filepath.EvalSymlinks(filepath.Clean(dir)); err == nil {
		return filepath.Join(resolvedDir, base)
	}
	return p
}

// relForReason renders abs relative to root for human-readable messages,
// falling back to the absolute path when it lies outside root.
func relForReason(root, abs string) string {
	if rel, err := filepath.Rel(root, abs); err == nil && !strings.HasPrefix(rel, "..") {
		return rel
	}
	return abs
}
