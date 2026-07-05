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

// hookBlockMaxFindings caps how many findings the block reason lists, so the
// instruction stays focused even on a project with many issues.
const hookBlockMaxFindings = 25

// RunHookStop evaluates the project's verify gates and writes the Stop-hook
// decision. It returns nil on every expected path (the decision is the JSON on
// stdout, not the exit code); only an unexpected write failure is an error.
func (a *App) RunHookStop(cmd Command) error {
	in := readStopHookInput(cmd.InOrStdin())

	// Evaluate the project in the session's working directory. The hook process
	// may start elsewhere, so move into cwd before resolving the project and its
	// relative content globs. If we can't, fail open (let Claude stop).
	if in.CWD != "" {
		if err := os.Chdir(in.CWD); err != nil {
			return nil
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
	out, err := a.computeVerify(vcmd, nil)
	restore()
	if err != nil {
		// No .kapi project here, or an operational error — nothing to gate on.
		// Don't trap the assistant; let it stop.
		return nil
	}
	if out.Pass {
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
	in := readPreEditHookInput(cmd.InOrStdin())

	file := strings.TrimSpace(in.ToolInput.FilePath)
	if file == "" {
		return nil // nothing to guard
	}

	// Resolve the project in the session's working directory. The hook process
	// may start elsewhere, so move into cwd before the git-style upward walk and
	// before resolving the project's relative content globs. Fail open if we
	// can't.
	if in.CWD != "" {
		if err := os.Chdir(in.CWD); err != nil {
			return nil
		}
	}

	projectPath, err := ResolveProjectPath(cmd)
	if err != nil || projectPath == "" {
		return nil // no project here → nothing to guard
	}

	// Silence any gate chatter to stderr while we load and match: a PreToolUse
	// hook's stderr on exit 0 surfaces to the user as a notice, and the verdict
	// belongs in the decision JSON, not in stray logging.
	restore := silenceStderr()
	proj, err := project.LoadWithOptions(projectPath, project.LoadOptions{SkipRequiresCheck: true})
	if err != nil {
		restore()
		return nil // unloadable project (e.g. requires a plugin we lack) → fail open
	}
	abs, err := filepath.Abs(file)
	if err != nil {
		restore()
		return nil
	}
	// Match in canonical (symlink-resolved) space. The project root comes from
	// os.Getwd (which may return the real path) while the edited file_path comes
	// from the assistant verbatim; on macOS those differ (/var vs /private/var),
	// and a project under any symlinked path would mismatch otherwise. Reuse the
	// canonical forms for the reason so its paths render relative consistently.
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
		PermissionDecisionReason: preEditDenyReason(root, target, source, locale),
	}}
	enc := json.NewEncoder(cmd.OutOrStdout())
	enc.SetEscapeHTML(false)
	return enc.Encode(dec)
}

// readPreEditHookInput parses the PreToolUse-hook JSON from r. Missing or
// malformed input yields a zero value (safe defaults). When r is an interactive
// terminal (the command was run by hand with no piped payload) it does not block
// on a read that would never return.
func readPreEditHookInput(r io.Reader) preEditHookInput {
	var in preEditHookInput
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

// preEditDenyReason renders the instruction fed back to Claude when it tries to
// edit a generated target. It names the target, its source and locale, and the
// round-trip to use instead, mirroring the skill's localize guidance.
func preEditDenyReason(root, targetAbs, sourceAbs, locale string) string {
	target := relForReason(root, targetAbs)
	source := relForReason(root, sourceAbs)
	return fmt.Sprintf(
		"%s is a generated kapi translation target. kapi writes it from %s [%s] and "+
			"overwrites it on the next `kapi merge`, so editing it by hand is discarded and "+
			"never passes through kapi's terminology, placeholder, and brand-voice gates.\n\n"+
			"Don't edit the target directly. To revise the %s translation, route it through kapi: "+
			"`kapi extract --target-lang %s` → fill the targets (follow `kapi brand guide` and the "+
			"glossary, keep placeholders intact) → `kapi merge -i out/*.xliff` → `kapi verify`. "+
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
