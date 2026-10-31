package contextop

import "strings"

// CheckRunMaxFiles bounds how many file paths one check operation lists, and
// CheckRunMaxFindings how many failing findings. The log is read whole on
// every fold, so a run over a large project keeps its counts exact and lists
// only this many of each.
const (
	CheckRunMaxFiles    = 200
	CheckRunMaxFindings = 500
)

// CheckRun is what one run of a project's checks found, as a check operation
// carries it.
type CheckRun struct {
	// Hook names what ran the checks: CheckHookStop for the Claude Code Stop
	// hook, which runs the ship gates over the project, or CheckHookEdit for
	// its PostToolUse hook, which runs the writing check on the one file an
	// edit wrote. A record that names no hook was written by the Stop hook.
	Hook string `json:"hook,omitempty"`
	// Turn identifies the event the run answered. For a Stop hook run it is
	// the id of the last entry in the session's transcript: Claude Code
	// writes an entry after every Stop hook, so each Stop gets a turn of its
	// own, and the hook running twice on one transcript state shares one. For
	// an edit hook run it is the id of the tool call that wrote the file. The
	// operation's id is derived from the session and the turn.
	Turn string `json:"turn"`
	// Verdict is the run's outcome: passed, failed or did_not_run.
	Verdict string `json:"verdict"`
	// Clean reports a run that reached a verdict with nothing failing.
	Clean bool `json:"clean"`
	// Files are the project-relative files the turn edited, as the session's
	// transcript names them, with Edited set. A run whose transcript names no
	// edit lists the files the check read. At most CheckRunMaxFiles are listed.
	Files []string `json:"files,omitempty"`
	// Edited reports that Files are the files the turn edited.
	Edited bool `json:"edited,omitempty"`
	// Covered counts the files the check read.
	Covered int `json:"covered"`
	// Failing counts the failing findings.
	Failing int `json:"failing"`
	// Rules are the distinct rule ids of the failing findings, sorted.
	Rules []string `json:"rules,omitempty"`
	// Findings are the failing findings, at most CheckRunMaxFindings of
	// them.
	Findings []CheckFinding `json:"findings,omitempty"`
}

// The hooks a check run names.
const (
	// CheckHookStop is the Claude Code Stop hook: the ship gates over the
	// whole project as a turn ends.
	CheckHookStop = "stop"
	// CheckHookEdit is the Claude Code PostToolUse hook: the writing check
	// on the one file an edit wrote.
	CheckHookEdit = "edit"
)

// OneFile reports a run that checked a single file, the one an edit wrote:
// Files names it. Every other run checked the whole project.
func (c CheckRun) OneFile() bool { return c.Hook == CheckHookEdit }

// Covers reports whether the run would have reported f again had f still
// been there. A run over the whole project covers every finding. A run on
// one file covers the findings on that file whose rule the writing check
// reports, which a rule id of the form "<check>.<category>" names; a finding
// only a ship gate reports names the gate instead, and an edit's run cannot
// see it.
func (c CheckRun) Covers(f CheckFinding) bool {
	if !c.OneFile() {
		return true
	}
	return len(c.Files) > 0 && c.Files[0] == f.File && strings.Contains(f.Rule, ".")
}

// Complete reports that Findings lists every failing finding the run
// counted.
func (c CheckRun) Complete() bool { return len(c.Findings) == c.Failing }

// Reached reports a run that reached a verdict: one whose findings say what
// the content held when it ran.
func (c CheckRun) Reached() bool { return c.Verdict == "passed" || c.Verdict == "failed" }

// CheckFinding is one failing finding of a check run.
type CheckFinding struct {
	// File is the project-relative file the finding is in.
	File string `json:"file,omitempty"`
	// Rule is the rule id the check reports it under, or the gate's name for
	// a finding that names no rule.
	Rule string `json:"rule"`
	// Quote is the wording the finding is about, when the check quotes it.
	Quote string `json:"quote,omitempty"`
	// Message is the finding's message, kept for a finding that quotes
	// nothing, so two of them on one file stay apart.
	Message string `json:"message,omitempty"`
	// Block is the block the finding sits in, when the check names one.
	Block string `json:"block,omitempty"`
}

// Key is what a finding is the same finding by from one run to the next: its
// file, its rule and the wording it quotes. The block is left out, so a
// finding that moved to another block of the file is still the same finding.
func (f CheckFinding) Key() string {
	what := f.Quote
	if what == "" {
		what = f.Message
	}
	return f.File + "\x00" + f.Rule + "\x00" + what
}
