package host

// What the Stop hook caught.
//
// Each run of `kapi hook stop` is recorded in the project's operation log as a
// check operation (contextop.KindCheck): the session's assistant ran the
// project's checks as its turn ended, and the record says what they covered,
// what failed and whether the run was clean. "Caught" is never stored. It is
// read from two runs of one session in a row: a failing finding that run N
// reported and run N+1 does not, on the same file, was caught by the hook and
// fixed by the assistant before it finished.

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/neokapi/neokapi/core/check"
	"github.com/neokapi/neokapi/core/contextop"
)

// stopHookAgent is the name a Stop hook run is recorded under. The hook
// protocol is Claude Code's, so the assistant it runs for is Claude Code.
const stopHookAgent = "claude-code"

// SessionOutcomes is what one assistant session's check runs found.
type SessionOutcomes struct {
	// Session is the session the runs were recorded under.
	Session string `json:"session"`
	// Checked counts the check runs the session recorded.
	Checked int `json:"checked"`
	// Failing counts what the session left failing: the failing findings of
	// its runs that no later run checked again.
	Failing int `json:"failing"`
	// Caught counts the failing findings one run reported and the session's
	// next run on the same file no longer found.
	Caught int `json:"caught"`
	// Files counts the distinct files the session's turns edited, as their
	// transcripts name them. A run with no transcript adds none.
	Files int `json:"files"`
}

// SessionOutcomes reports what one session's check runs found in a project.
// A session that ran no check reports zero runs rather than an error.
func (a *App) SessionOutcomes(ctx context.Context, req ContextSessionRequest) (SessionOutcomes, error) {
	if strings.TrimSpace(req.Session) == "" {
		return SessionOutcomes{}, errors.New("no session named")
	}
	s, err := a.contextOps(ctx, req.Project)
	if err != nil {
		return SessionOutcomes{}, err
	}
	runs, err := s.ledger.Records(ctx, contextop.Filter{Project: s.key, Session: req.Session, Kinds: []contextop.Kind{contextop.KindCheck}})
	if err != nil {
		return SessionOutcomes{}, err
	}
	out := SessionOutcomesOf(runs)[req.Session]
	out.Session = req.Session
	return out, nil
}

// LatestSession is the session a caller that names none reads in a project's
// log.
type LatestSession struct {
	// Session is the id the session recorded under.
	Session string
	// Checked is true when Session holds the project's latest check run, and
	// false when no session holds a check run and Session is the one that
	// recorded last.
	Checked bool
	// Last is when the session last recorded, in any operation.
	Last time.Time
}

// LatestContextSession names the session a caller that knows no session id
// reads: the session whose latest Stop hook check run is the project's most
// recent one. When no session holds a check run, it is the session that
// recorded last in any operation. ok is false when the log holds no operation
// under a session.
//
// The MCP tools record notes under the server's own session and the Stop hook
// records its runs under the assistant host's. A check run outranks a later
// note, so the hook's outcomes reach an agent that has just written a note.
func (a *App) LatestContextSession(ctx context.Context, project string) (latest LatestSession, ok bool, err error) {
	s, err := a.contextOps(ctx, project)
	if err != nil {
		return LatestSession{}, false, err
	}
	records, err := s.ledger.Records(ctx, contextop.Filter{Project: s.key})
	if err != nil {
		return LatestSession{}, false, err
	}
	// The records arrive newest first, so on equal instants the record the log
	// accepted later wins.
	last := map[string]time.Time{}
	var anyOp, checkRun contextop.Record
	for _, r := range records {
		if r.Actor.Session == "" {
			continue
		}
		if t, seen := last[r.Actor.Session]; !seen || r.At.After(t) {
			last[r.Actor.Session] = r.At
		}
		if anyOp.Actor.Session == "" || r.At.After(anyOp.At) {
			anyOp = r
		}
		if r.Kind == contextop.KindCheck && (checkRun.Actor.Session == "" || r.At.After(checkRun.At)) {
			checkRun = r
		}
	}
	switch {
	case checkRun.Actor.Session != "":
		return LatestSession{Session: checkRun.Actor.Session, Checked: true, Last: last[checkRun.Actor.Session]}, true, nil
	case anyOp.Actor.Session != "":
		return LatestSession{Session: anyOp.Actor.Session, Last: last[anyOp.Actor.Session]}, true, nil
	}
	return LatestSession{}, false, nil
}

// SessionOutcomesOf derives every session's outcomes from check records, in
// any order. Runs are compared within one project and one session, in the
// order the log accepted them, as CheckSequences and CaughtMarks pair them.
func SessionOutcomesOf(records []contextop.Record) map[string]SessionOutcomes {
	out := map[string]SessionOutcomes{}
	files := map[string]map[string]bool{}
	for _, seq := range CheckSequences(records) {
		session := seq[0].Actor.Session
		o := out[session]
		o.Session = session
		if files[session] == nil {
			files[session] = map[string]bool{}
		}
		marks := CaughtMarks(seq)
		for i, r := range seq {
			run := *r.Check
			o.Checked++
			// A run with no transcript lists the files the check read,
			// which is the project, not what the session touched.
			if run.Edited {
				for _, f := range run.Files {
					files[session][string(r.Project)+"\x00"+f] = true
				}
			}
			o.Caught += countCaught(marks[i])
		}
		o.Failing += leftFailing(seq)
		o.Files = len(files[session])
		out[session] = o
	}
	return out
}

// CheckSequences groups check records into the sequences caught is read
// from: one per project and session, each in log order, the order of the
// records' ids. The records may arrive in any order; a record that is not a
// check run, or names no session, belongs to no sequence. The sequences are
// ordered by project, then by session.
//
// Only the runs of a sequence that reached a verdict are paired for caught,
// each with the sequence's next run that reached one. CaughtMarks applies
// that pairing.
func CheckSequences(records []contextop.Record) [][]contextop.Record {
	groups := map[string][]contextop.Record{}
	var order []string
	for _, r := range records {
		if r.Kind != contextop.KindCheck || r.Check == nil || r.Actor.Session == "" {
			continue
		}
		key := string(r.Project) + "\x00" + r.Actor.Session
		if _, ok := groups[key]; !ok {
			order = append(order, key)
		}
		groups[key] = append(groups[key], r)
	}
	sort.Strings(order)
	out := make([][]contextop.Record, 0, len(order))
	for _, key := range order {
		seq := groups[key]
		sort.SliceStable(seq, func(i, j int) bool { return seq[i].ID < seq[j].ID })
		out = append(out, seq)
	}
	return out
}

// CaughtMarks marks the findings of one sequence from CheckSequences that
// its later runs caught. The result has one entry per record of seq: for a
// run that reached a verdict and is followed by another that did, one mark
// per finding, true for a finding a later run caught; for every other run,
// nil. The marks of a session's sequences add up to its SessionOutcomes
// caught count.
//
// Each finding is compared with the sequence's next run that reached a
// verdict and covers it (contextop.CheckRun.Covers): the next run of the
// ship gates, or the next edit hook run on the finding's file. Between two
// Stop hook runs that is the next run, as CaughtFindings pairs them.
func CaughtMarks(seq []contextop.Record) [][]bool {
	by := caughtBy(seq)
	out := make([][]bool, len(seq))
	for i, catchers := range by {
		if catchers == nil {
			continue
		}
		out[i] = make([]bool, len(catchers))
		for k, j := range catchers {
			out[i][k] = j >= 0
		}
	}
	return out
}

// caughtBy answers, for each finding of each run of seq that reached a
// verdict and is followed by another that did, the index in seq of the run
// that caught it, or -1. Runs with no later run that reached a verdict, and
// runs that reached none, answer nil.
func caughtBy(seq []contextop.Record) [][]int {
	out := make([][]int, len(seq))
	last := -1
	for i, r := range seq {
		if r.Check.Reached() {
			last = i
		}
	}
	// remaining holds, per later run, the findings it lists that no earlier
	// finding has been matched to yet, so findings with one key match one
	// to one.
	remaining := map[int]map[string]int{}
	for i, r := range seq {
		if !r.Check.Reached() || i >= last {
			continue
		}
		out[i] = make([]int, len(r.Check.Findings))
		for k, f := range r.Check.Findings {
			out[i][k] = -1
			j := nextCovering(seq, i, f)
			if j < 0 || !seq[j].Check.Complete() {
				continue
			}
			rem, ok := remaining[j]
			if !ok {
				rem = map[string]int{}
				for _, g := range seq[j].Check.Findings {
					rem[g.Key()]++
				}
				remaining[j] = rem
			}
			if rem[f.Key()] > 0 {
				rem[f.Key()]--
				continue
			}
			out[i][k] = j
		}
	}
	return out
}

// nextCovering is the index of the first run after i in seq that reached a
// verdict and covers f, or -1.
func nextCovering(seq []contextop.Record, i int, f contextop.CheckFinding) int {
	for j := i + 1; j < len(seq); j++ {
		if seq[j].Check.Reached() && seq[j].Check.Covers(f) {
			return j
		}
	}
	return -1
}

// leftFailing counts the failing findings a sequence ends with: each run's
// listed findings that no later run that reached a verdict covers. A Stop
// hook run covers every run before it, so a sequence of Stop hook runs ends
// with its last run's failing count. A finding a run counted and did not
// list is counted until a later run checks everything the run checked.
func leftFailing(seq []contextop.Record) int {
	failing := 0
	for i, r := range seq {
		run := r.Check
		if !run.Reached() {
			continue
		}
		for _, f := range run.Findings {
			if nextCovering(seq, i, f) < 0 {
				failing++
			}
		}
		if unlisted := run.Failing - len(run.Findings); unlisted > 0 && !coveredLater(seq, i) {
			failing += unlisted
		}
	}
	return failing
}

// coveredLater reports whether a run after i in seq that reached a verdict
// checked everything run i checked: a later run of the ship gates, or, for an
// edit hook run, a later edit hook run on the same file.
func coveredLater(seq []contextop.Record, i int) bool {
	run := seq[i].Check
	for j := i + 1; j < len(seq); j++ {
		next := seq[j].Check
		if !next.Reached() {
			continue
		}
		if !next.OneFile() {
			return true
		}
		if run.OneFile() && len(run.Files) > 0 && len(next.Files) > 0 && run.Files[0] == next.Files[0] {
			return true
		}
	}
	return false
}

// CaughtFindings marks which of prev's listed findings next no longer
// reports: the result has one entry per finding of prev, true for a finding
// next caught. A finding is the same finding by its file, its rule and the
// wording it quotes (contextop.CheckFinding.Key), so one that moved to another
// block of the file is still there. Findings with one key match one to one, so
// a finding reported twice and fixed once is caught once. When next lists
// fewer findings than it counted, what it left out is unknown, and no finding
// is caught.
//
// prev and next are consecutive runs of one session in one project that
// reached a verdict; CaughtMarks pairs no others.
func CaughtFindings(prev, next contextop.CheckRun) []bool {
	out := make([]bool, len(prev.Findings))
	if !next.Complete() {
		return out
	}
	remaining := map[string]int{}
	for _, f := range next.Findings {
		remaining[f.Key()]++
	}
	for i, f := range prev.Findings {
		if remaining[f.Key()] > 0 {
			remaining[f.Key()]--
			continue
		}
		out[i] = true
	}
	return out
}

// countCaught counts the findings marks names caught.
func countCaught(marks []bool) int {
	caught := 0
	for _, c := range marks {
		if c {
			caught++
		}
	}
	return caught
}

// runsCaught answers, for each check record, how many findings it no longer
// found that the session's earlier runs did.
func runsCaught(records []contextop.Record) map[string]int {
	out := map[string]int{}
	for _, seq := range CheckSequences(records) {
		for _, catchers := range caughtBy(seq) {
			for _, j := range catchers {
				if j >= 0 {
					out[seq[j].ID]++
				}
			}
		}
	}
	return out
}

// hookRunRecord builds the check operation for one Stop hook run.
//
// Two gates can report one finding: the terminology gate and the checks gate
// both hold a source block to its word rules. Each gate's findings are counted
// by file, rule, wording and block, and the record holds the largest count any
// gate reported for each, so the second gate's copy adds nothing and two uses
// of the same words in one block stay two findings.
func hookRunRecord(in stopHookInput, root string, out verifyOutput) contextop.Record {
	run := &contextop.CheckRun{
		Hook:    "stop",
		Verdict: string(out.Verdict),
		Covered: len(out.covered),
	}
	most := map[string]int{}
	first := map[string]contextop.CheckFinding{}
	var order []string
	for _, g := range out.Gates {
		counts := map[string]int{}
		for _, f := range g.Findings {
			if !f.Fails {
				continue
			}
			finding := contextop.CheckFinding{File: f.File, Rule: f.Rule, Quote: f.Quote, Block: f.Block}
			if finding.Rule == "" {
				finding.Rule = g.Gate
			}
			if finding.Quote == "" {
				finding.Message = f.Message
			}
			key := finding.Key() + "\x00" + finding.Block
			if _, ok := first[key]; !ok {
				first[key] = finding
				order = append(order, key)
			}
			counts[key]++
		}
		for key, n := range counts {
			most[key] = max(most[key], n)
		}
	}
	rules := map[string]bool{}
	for _, key := range order {
		finding := first[key]
		rules[finding.Rule] = true
		for range most[key] {
			run.Failing++
			if len(run.Findings) < contextop.CheckRunMaxFindings {
				run.Findings = append(run.Findings, finding)
			}
		}
	}
	run.Clean = out.Verdict == check.VerdictPassed && run.Failing == 0
	for rule := range rules {
		run.Rules = append(run.Rules, rule)
	}
	sort.Strings(run.Rules)

	turn, edited := readTranscriptTurn(in.TranscriptPath, root)
	if len(edited) > 0 {
		run.Files, run.Edited = edited, true
	} else {
		run.Files = slices.Clone(out.covered)
	}
	if len(run.Files) > contextop.CheckRunMaxFiles {
		run.Files = run.Files[:contextop.CheckRunMaxFiles]
	}
	run.Turn = turn
	if run.Turn == "" {
		// With no transcript to say which turn this is, the run is a turn of
		// its own.
		run.Turn = "run:" + time.Now().UTC().Format(time.RFC3339Nano)
	}

	session := strings.TrimSpace(in.SessionID)
	if session == "" {
		session = agentSession(os.Getenv, AgentHostMarker{Name: stopHookAgent, Session: "CLAUDE_CODE_SESSION_ID"})
	}
	return contextop.Record{
		Actor: contextop.Actor{Kind: contextop.ActorAgent, Name: stopHookAgent, Session: session},
		Kind:  contextop.KindCheck,
		Check: run,
	}
}

// recordHookRun records one Stop hook run in the project's operation log and
// returns the operation as the log holds it. A run the log already holds for
// the same session and transcript state is returned in place of a new one.
func (a *App) recordHookRun(ctx context.Context, projectPath string, r contextop.Record) (contextop.Record, error) {
	s, err := a.contextOps(ctx, projectPath)
	if err != nil {
		return contextop.Record{}, err
	}
	r.Project = s.key
	s.noteAgentSession(ctx, r.Actor)
	return s.ledger.Append(ctx, r)
}

// transcriptLine is the part of one line of a Claude Code transcript the hook
// reads.
type transcriptLine struct {
	Type    string `json:"type"`
	UUID    string `json:"uuid"`
	IsMeta  bool   `json:"isMeta"`
	Message struct {
		Content json.RawMessage `json:"content"`
	} `json:"message"`
}

// transcriptItem is one item of a message's content.
type transcriptItem struct {
	Type  string `json:"type"`
	Name  string `json:"name"`
	Input struct {
		FilePath     string `json:"file_path"`
		NotebookPath string `json:"notebook_path"`
	} `json:"input"`
}

// editTools are the tools whose calls write a file.
var editTools = []string{"Edit", "Write", "MultiEdit", "NotebookEdit"}

// readTranscriptTurn reads the session's transcript for the Stop a hook ran
// at. The turn is the id of the transcript's last entry. Claude Code writes an
// entry after every Stop hook, so the next Stop sees a different last entry.
// The files are those inside root that the assistant's edits since the
// person's last message wrote, project-relative and each once. An unreadable
// or missing transcript answers with nothing.
func readTranscriptTurn(path, root string) (turn string, edited []string) {
	if path == "" {
		return "", nil
	}
	f, err := os.Open(path)
	if err != nil {
		return "", nil
	}
	defer func() { _ = f.Close() }()
	root = NormalizeCheckoutPath(root)
	r := bufio.NewReader(f)
	var since []string
	for {
		line, rerr := r.ReadBytes('\n')
		if len(bytes.TrimSpace(line)) > 0 {
			turn, since = readTranscriptLine(line, root, turn, since)
		}
		if rerr != nil {
			if rerr != io.EOF {
				return "", nil
			}
			break
		}
	}
	return turn, since
}

// readTranscriptLine folds one transcript line into the turn read so far.
func readTranscriptLine(line []byte, root, turn string, edited []string) (string, []string) {
	var entry transcriptLine
	if json.Unmarshal(line, &entry) != nil {
		return turn, edited
	}
	if entry.UUID != "" {
		turn = entry.UUID
	}
	switch entry.Type {
	case "user":
		if !entry.IsMeta && isPrompt(entry.Message.Content) {
			edited = nil
		}
	case "assistant":
		var items []transcriptItem
		if json.Unmarshal(entry.Message.Content, &items) != nil {
			return turn, edited
		}
		for _, it := range items {
			if it.Type != "tool_use" || !slices.Contains(editTools, it.Name) {
				continue
			}
			path := it.Input.FilePath
			if path == "" {
				path = it.Input.NotebookPath
			}
			if rel, ok := projectRelPath(root, NormalizeCheckoutPath(path)); ok && path != "" && !slices.Contains(edited, rel) {
				edited = append(edited, rel)
			}
		}
	}
	return turn, edited
}

// isPrompt reports whether a user message's content is a person's message
// rather than the results of the assistant's tool calls.
func isPrompt(content json.RawMessage) bool {
	var text string
	if json.Unmarshal(content, &text) == nil {
		return true
	}
	var items []transcriptItem
	if json.Unmarshal(content, &items) != nil {
		return false
	}
	for _, it := range items {
		if it.Type == "tool_result" {
			return false
		}
	}
	return len(items) > 0
}

// describeCheckRun renders a check run the way a log line shows it:
// "checked 3 files · 2 failing (terms.vocabulary)", or "… · clean", with what
// the run caught when it caught anything.
func describeCheckRun(run contextop.CheckRun, caught int) string {
	parts := []string{fmt.Sprintf("checked %d %s", run.Covered, pluralWord(run.Covered, "file", "files"))}
	switch {
	case !run.Reached():
		parts = append(parts, "did not run")
	case run.Clean:
		parts = append(parts, "clean")
	default:
		failing := fmt.Sprintf("%d failing", run.Failing)
		if len(run.Rules) > 0 {
			failing += " (" + strings.Join(run.Rules, ", ") + ")"
		}
		parts = append(parts, failing)
	}
	if caught > 0 {
		parts = append(parts, fmt.Sprintf("%d caught", caught))
	}
	return strings.Join(parts, " · ")
}
