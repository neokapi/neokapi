package host

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/neokapi/neokapi/core/check"
	"github.com/neokapi/neokapi/core/contextop"
	"github.com/neokapi/neokapi/core/project"
)

// postEditHookInput is the subset of the Claude Code PostToolUse-hook stdin
// payload the edit hook reads. The hook is wired with a matcher of
// Edit|Write|MultiEdit, so ToolInput.FilePath is the file the tool wrote.
// ToolUseID names the tool call, and the run's record is keyed by it, so the
// hook running twice for one edit records one run.
type postEditHookInput struct {
	CWD       string `json:"cwd"`
	SessionID string `json:"session_id"`
	ToolName  string `json:"tool_name"`
	ToolUseID string `json:"tool_use_id"`
	ToolInput struct {
		FilePath string `json:"file_path"`
	} `json:"tool_input"`
}

// PostEditHookDecision is the JSON the edit hook writes to stdout when the
// edited file fails its check. Claude Code reads a PostToolUse "block" as
// feedback: the edit has already happened, and Reason is shown to Claude next
// to the tool's result.
type PostEditHookDecision struct {
	Decision string `json:"decision"`
	Reason   string `json:"reason"`
}

// PostEditHookResult is what `kapi hook post-edit --json` prints: the
// decision, empty for a clean file, and the id of the operation that records
// the run in the project's log, empty when the run was not recorded.
type PostEditHookResult struct {
	Decision string `json:"decision,omitempty"`
	Reason   string `json:"reason,omitempty"`
	Record   string `json:"record,omitempty"`
}

// RunHookPostEdit runs the project's writing check on the file an edit wrote
// and writes the PostToolUse decision. It returns nil on every expected path
// (the verdict is the JSON on stdout, not the exit code); only an unexpected
// write failure is an error.
func (a *App) RunHookPostEdit(cmd Command) error {
	const hook = "kapi hook post-edit"

	var in postEditHookInput
	if because := readHookPayload(cmd.InOrStdin(), &in); because != "" {
		return hookUnheard(cmd, hook, because)
	}
	file := strings.TrimSpace(in.ToolInput.FilePath)
	if file == "" {
		return nil // a well-formed payload with no file path: nothing to check
	}

	// The hook process may start elsewhere, so move into the session's
	// directory before the upward walk that finds the project, and before a
	// relative file path is resolved.
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
		return nil // no project here: nothing to check
	}
	abs, err := filepath.Abs(file)
	if err != nil {
		return hookUnheard(cmd, hook, fmt.Sprintf("%s could not be resolved to an absolute path: %v", file, err))
	}

	// A Claude Code hook's stderr reaches the person as hook output, and
	// reading content may start a plugin that logs there. Silence it while
	// the check runs, and restore it before any notice.
	restore := silenceStderr()
	run, skip, err := a.postEditCheck(cmd, projectPath, abs)
	restore()
	switch {
	case err != nil:
		return hookUnheard(cmd, hook, err.Error())
	case skip:
		return nil
	}

	record := a.recordPostEditRun(cmd, in, projectPath, run)
	report := run.report
	switch {
	case report.Verdict == check.VerdictDidNotRun && report.DidNotRunCause == check.CauseCheckerInvalid:
		return hookUntrusted(cmd, hook, fmt.Sprintf("the check of %s: %s", run.display, strings.Join(report.DidNotRun, "; ")))
	case report.Verdict == check.VerdictDidNotRun && report.DidNotRunCause != check.CauseNothingToCheck:
		return hookUnheard(cmd, hook, fmt.Sprintf("the check of %s did not run: %s (%s)", run.display, check.CauseSummary(report.DidNotRunCause), report.DidNotRunCause))
	}

	var dec PostEditHookDecision
	if report.Verdict == check.VerdictFailed && report.Summary.Failing > 0 {
		dec = PostEditHookDecision{Decision: "block", Reason: postEditBlockReason(run.display, report)}
	}
	enc := json.NewEncoder(cmd.OutOrStdout())
	enc.SetEscapeHTML(false)
	if asJSON, _ := cmd.Flags().GetBool("json"); asJSON {
		return enc.Encode(PostEditHookResult{Decision: dec.Decision, Reason: dec.Reason, Record: record})
	}
	if dec.Decision == "" {
		return nil
	}
	return enc.Encode(dec)
}

// postEditRun is one edit hook check: the file, as the project and the
// session name it, and the report.
type postEditRun struct {
	root    string
	rel     string // project-relative, slash-separated
	display string // as the agent types it from the session's directory
	report  check.Report
}

// postEditCheck runs `kapi check <file>` on abs inside the project at
// projectPath. skip is true for a file the hook leaves alone: one outside the
// project, a generated target, a file the project's collections do not read,
// or one that is gone. err names why the check could not run.
func (a *App) postEditCheck(cmd Command, projectPath, abs string) (run postEditRun, skip bool, err error) {
	proj, err := project.LoadWithOptions(projectPath, project.LoadOptions{SkipRequiresCheck: true})
	if err != nil {
		return run, false, fmt.Errorf("the project at %s could not be loaded: %w", projectPath, err)
	}
	// Match in canonical (symlink-resolved) space: the project root comes
	// from the working directory, which may be the real path, while the
	// assistant names the file as it was given.
	root := NormalizeCheckoutPath(filepath.Dir(projectPath))
	target := NormalizeCheckoutPath(abs)
	rel, inside := projectRelPath(root, target)
	if !inside {
		return run, true, nil
	}
	if _, _, isTarget := matchTargetToSource(proj, root, target); isTarget {
		return run, true, nil
	}
	if info, serr := os.Stat(target); serr != nil || info.IsDir() {
		return run, true, nil
	}

	a.InitRegistries()
	pcmd := NewEnvCommand(CmdContext(cmd), "check")
	pcmd.SetOut(io.Discard)
	pcmd.SetErr(io.Discard)
	pcmd.Flags().String(projectFlagName, projectPath, "")
	formats, err := a.newCheckFormats(pcmd)
	if err != nil {
		return run, false, err
	}
	if _, declared := formats.lookup(target); !declared {
		return run, true, nil
	}

	report, err := a.ComputeCheck(pcmd, []string{target}) //nolint:contextcheck // ctx travels on pcmd (NewEnvCommand), which computeCheck reads through CmdContext
	if err != nil {
		return run, false, fmt.Errorf("the check of %s could not run: %w", rel, err)
	}
	display := rel
	if cwd, cerr := os.Getwd(); cerr == nil {
		display = filepath.ToSlash(relForReason(NormalizeCheckoutPath(cwd), target))
	}
	return postEditRun{root: root, rel: rel, display: display, report: report}, false, nil
}

// postEditBlockReason renders the failing findings as the feedback Claude
// reads beside the edit's result: one line per finding, with the rule's fix
// when it names one, and the command that shows the full report.
func postEditBlockReason(display string, report check.Report) string {
	var b strings.Builder
	fmt.Fprintf(&b, "kapi check found %d failing %s in %s. Fix %s in this file before you report the work done; kapi checks the file again on your next edit.\n\n",
		report.Summary.Failing, pluralWord(report.Summary.Failing, "finding", "findings"), display,
		pluralWord(report.Summary.Failing, "it", "them"))
	shown := 0
	for _, d := range report.Findings {
		if !d.Fails {
			continue
		}
		if shown >= hookBlockMaxFindings {
			break
		}
		loc := display
		switch {
		case d.Location.Lines != nil:
			loc += fmt.Sprintf(":%d", d.Location.Lines.First)
		case d.Location.Block != "":
			loc += " (" + d.Location.Block + ")"
		}
		fmt.Fprintf(&b, "  FAILS [%s] %s: %s", d.Rule, loc, d.Message)
		if d.Suggestion != "" {
			fmt.Fprintf(&b, " (%s)", d.Suggestion)
		}
		b.WriteByte('\n')
		shown++
	}
	if report.Summary.Failing > shown {
		fmt.Fprintf(&b, "  ... and %d more.\n", report.Summary.Failing-shown)
	}
	fmt.Fprintf(&b, "\nRun `kapi check %s` to see the full report.", display)
	return b.String()
}

// recordPostEditRun records an edit hook run in the project's operation log
// under the session's assistant and returns the operation's id. It never
// fails the hook: an error is written to stderr as a warning and the id comes
// back empty.
func (a *App) recordPostEditRun(cmd Command, in postEditHookInput, projectPath string, run postEditRun) string {
	restore := silenceStderr()
	written, err := a.recordHookRun(CmdContext(cmd), projectPath, postEditRunRecord(in, run))
	restore()
	if err != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "warning: kapi hook post-edit: the run is not recorded in the project's log: %v\n", err)
		return ""
	}
	return written.ID
}

// postEditRunRecord builds the check operation for one edit hook run. Its
// findings carry the file, rule and quoted wording the Stop hook's records
// carry, so a finding one hook reports and the other no longer finds counts
// as caught.
func postEditRunRecord(in postEditHookInput, run postEditRun) contextop.Record {
	report := run.report
	cr := &contextop.CheckRun{
		Hook:    contextop.CheckHookEdit,
		Verdict: string(report.Verdict),
		Files:   []string{run.rel},
		Edited:  true,
		Covered: 1,
	}
	rules := map[string]bool{}
	for _, d := range report.Findings {
		if !d.Fails {
			continue
		}
		f := contextop.CheckFinding{File: run.rel, Rule: d.Rule, Quote: d.Location.Snippet, Block: d.Location.Block}
		if f.Quote == "" {
			f.Message = d.Message
		}
		cr.Failing++
		rules[f.Rule] = true
		if len(cr.Findings) < contextop.CheckRunMaxFindings {
			cr.Findings = append(cr.Findings, f)
		}
	}
	cr.Clean = report.Verdict == check.VerdictPassed && cr.Failing == 0
	for rule := range rules {
		cr.Rules = append(cr.Rules, rule)
	}
	sort.Strings(cr.Rules)
	cr.Turn = strings.TrimSpace(in.ToolUseID)
	if cr.Turn == "" {
		// With no tool call to name, the run is an event of its own.
		cr.Turn = "edit:" + time.Now().UTC().Format(time.RFC3339Nano)
	}

	session := strings.TrimSpace(in.SessionID)
	if session == "" {
		session = agentSession(os.Getenv, AgentHostMarker{Name: stopHookAgent, Session: "CLAUDE_CODE_SESSION_ID"})
	}
	return contextop.Record{
		Actor: contextop.Actor{Kind: contextop.ActorAgent, Name: stopHookAgent, Session: session},
		Kind:  contextop.KindCheck,
		Check: cr,
	}
}
