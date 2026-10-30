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
	// Failing counts the failing findings of the session's latest run that
	// reached a verdict: what the session left failing.
	Failing int `json:"failing"`
	// Caught counts the failing findings one run reported and the session's
	// next run no longer found on the same file.
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

// SessionOutcomesOf derives every session's outcomes from check records, in
// any order. Runs are compared within one project and one session, in the
// order the log accepted them.
func SessionOutcomesOf(records []contextop.Record) map[string]SessionOutcomes {
	out := map[string]SessionOutcomes{}
	files := map[string]map[string]bool{}
	for _, seq := range checkSequences(records) {
		session := seq[0].Actor.Session
		o := out[session]
		o.Session = session
		if files[session] == nil {
			files[session] = map[string]bool{}
		}
		var prev *contextop.CheckRun
		for _, r := range seq {
			run := *r.Check
			o.Checked++
			// A run with no transcript lists the files the check read,
			// which is the project, not what the session touched.
			if run.Edited {
				for _, f := range run.Files {
					files[session][string(r.Project)+"\x00"+f] = true
				}
			}
			if !run.Reached() {
				continue
			}
			if prev != nil {
				o.Caught += caughtBetween(*prev, run)
			}
			prev = &run
		}
		if prev != nil {
			o.Failing += prev.Failing
		}
		o.Files = len(files[session])
		out[session] = o
	}
	return out
}

// checkSequences groups check records by project and session, each group
// oldest first.
func checkSequences(records []contextop.Record) [][]contextop.Record {
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

// caughtBetween counts the failing findings of prev that next no longer
// reports. A finding is the same finding by its file, its rule and the wording
// it quotes, so one that moved to another block of the file is still there.
// When next lists fewer findings than it counted, what it left out is
// unknown, and nothing is counted as caught.
func caughtBetween(prev, next contextop.CheckRun) int {
	if !next.Complete() {
		return 0
	}
	remaining := map[string]int{}
	for _, f := range next.Findings {
		remaining[f.Key()]++
	}
	caught := 0
	for _, f := range prev.Findings {
		if remaining[f.Key()] > 0 {
			remaining[f.Key()]--
			continue
		}
		caught++
	}
	return caught
}

// runsCaught answers, for each check record, how many findings it no longer
// found that the session's previous run did.
func runsCaught(records []contextop.Record) map[string]int {
	out := map[string]int{}
	for _, seq := range checkSequences(records) {
		var prev *contextop.CheckRun
		for _, r := range seq {
			run := *r.Check
			if !run.Reached() {
				continue
			}
			if prev != nil {
				if n := caughtBetween(*prev, run); n > 0 {
					out[r.ID] = n
				}
			}
			prev = &run
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

// readTranscriptTurn reads the session's transcript for the turn a Stop hook
// ran at the end of: the id of the transcript's last entry, which names the
// Stop event (Claude Code writes an entry after every Stop hook, so the next
// Stop sees a different last entry), and the files inside root that the assistant's edits since the
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
