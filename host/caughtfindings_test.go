package host

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/neokapi/neokapi/core/contextop"
	"github.com/neokapi/neokapi/core/workspace"
)

// caughtMarks returns the CaughtMarks of every sequence by record id, leaving
// out the runs that were paired with none.
func caughtMarks(records []contextop.Record) map[string][]bool {
	out := map[string][]bool{}
	for _, seq := range CheckSequences(records) {
		for i, m := range CaughtMarks(seq) {
			if m != nil {
				out[seq[i].ID] = m
			}
		}
	}
	return out
}

// TestCaughtFindings_MarksAddUpToTheSessionsCount: each case is one session's
// check records, the findings CaughtFindings marks caught on each run, and
// the session's outcomes. The marks add up to SessionOutcomesOf's caught
// count in every case, so a surface that lists caught findings and one that
// counts them agree.
func TestCaughtFindings_MarksAddUpToTheSessionsCount(t *testing.T) {
	at := time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)
	logIn := contextop.CheckFinding{File: "docs/guide.md", Rule: "terms.vocabulary", Quote: "Log in", Block: "p#2"}
	moved := logIn
	moved.Block = "p#5"
	inOther := logIn
	inOther.File = "docs/other.md"
	missing := contextop.CheckFinding{File: "locales/fr/app.json", Rule: "completeness", Message: "greeting has no translation"}
	otherMissing := missing
	otherMissing.Message = "farewell has no translation"

	failed := func(findings ...contextop.CheckFinding) contextop.CheckRun {
		return contextop.CheckRun{Verdict: "failed", Failing: len(findings), Findings: findings}
	}
	clean := contextop.CheckRun{Verdict: "passed", Clean: true}
	rec := func(id string, project workspace.ProjectKey, minute int, run contextop.CheckRun) contextop.Record {
		return contextop.Record{
			ID:      id,
			Project: project,
			Kind:    contextop.KindCheck,
			At:      at.Add(time.Duration(minute) * time.Minute),
			Actor:   contextop.Actor{Kind: contextop.ActorAgent, Name: "claude-code", Session: "s1"},
			Check:   &run,
		}
	}

	var many []contextop.CheckFinding
	for i := range 30 {
		many = append(many, contextop.CheckFinding{File: "docs/guide.md", Rule: "terms.vocabulary", Quote: fmt.Sprintf("word %02d", i)})
	}
	allCaught := make([]bool, len(many))
	for i := range allCaught {
		allCaught[i] = true
	}
	cut := failed()
	cut.Failing = 3

	cases := []struct {
		name    string
		records []contextop.Record
		marks   map[string][]bool
		caught  int
		failing int
	}{
		{
			name:    "a finding that moved to another block of the same file is still there",
			records: []contextop.Record{rec("1", "p", 0, failed(logIn)), rec("2", "p", 1, failed(moved))},
			marks:   map[string][]bool{"1": {false}},
			failing: 1,
		},
		{
			name: "a run that reached no verdict between two that did compares nothing",
			records: []contextop.Record{
				rec("1", "p", 0, failed(logIn)),
				rec("2", "p", 1, contextop.CheckRun{Verdict: "did_not_run"}),
				rec("3", "p", 2, failed(logIn)),
			},
			marks:   map[string][]bool{"1": {false}},
			failing: 1,
		},
		{
			name:    "the same rule and quote in two files with one fixed",
			records: []contextop.Record{rec("1", "p", 0, failed(logIn, inOther)), rec("2", "p", 1, failed(inOther))},
			marks:   map[string][]bool{"1": {true, false}},
			caught:  1,
			failing: 1,
		},
		{
			name: "reported, absent and back again is caught once",
			records: []contextop.Record{
				rec("1", "p", 0, failed(logIn)),
				rec("2", "p", 1, clean),
				rec("3", "p", 2, failed(logIn)),
			},
			marks:   map[string][]bool{"1": {true}, "2": {}},
			caught:  1,
			failing: 1,
		},
		{
			name:    "one wording twice in a file and fixed once is caught once",
			records: []contextop.Record{rec("1", "p", 0, failed(logIn, moved)), rec("2", "p", 1, failed(moved))},
			marks:   map[string][]bool{"1": {false, true}},
			caught:  1,
			failing: 1,
		},
		{
			name:    "a next run that lists fewer findings than it counted catches nothing",
			records: []contextop.Record{rec("1", "p", 0, failed(logIn)), rec("2", "p", 1, cut)},
			marks:   map[string][]bool{"1": {false}},
			failing: 3,
		},
		{
			name: "a finding with no quote is the same finding by its message",
			records: []contextop.Record{
				rec("1", "p", 0, failed(missing, otherMissing)),
				rec("2", "p", 1, failed(missing)),
			},
			marks:   map[string][]bool{"1": {false, true}},
			caught:  1,
			failing: 1,
		},
		{
			name: "two projects in one session compare within their project, in any record order",
			records: []contextop.Record{
				rec("4", "q", 3, failed(logIn)),
				rec("1", "p", 0, failed(logIn)),
				rec("3", "p", 2, clean),
				rec("2", "q", 1, failed(logIn)),
			},
			marks:   map[string][]bool{"1": {true}, "2": {false}},
			caught:  1,
			failing: 1,
		},
		{
			name: "runs with one timestamp compare in the order of their ids",
			records: []contextop.Record{
				rec("op-2", "p", 0, clean),
				rec("op-1", "p", 0, failed(logIn)),
			},
			marks:  map[string][]bool{"op-1": {true}},
			caught: 1,
		},
		{
			name:    "every caught finding counts, however many a surface lists",
			records: []contextop.Record{rec("1", "p", 0, failed(many...)), rec("2", "p", 1, clean)},
			marks:   map[string][]bool{"1": allCaught},
			caught:  len(many),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			marks := caughtMarks(tc.records)
			assert.Equal(t, tc.marks, marks)
			sum := 0
			for _, m := range marks {
				for _, c := range m {
					if c {
						sum++
					}
				}
			}
			got := SessionOutcomesOf(tc.records)["s1"]
			assert.Equal(t, tc.caught, got.Caught)
			assert.Equal(t, sum, got.Caught, "the marks add up to the session's count")
			assert.Equal(t, tc.failing, got.Failing)
			assert.Equal(t, len(tc.records), got.Checked)
		})
	}
}

// TestCaughtFindings_OneMarkPerFindingOfTheEarlierRun: the result has one
// entry per finding of the earlier run, whatever the later run held.
func TestCaughtFindings_OneMarkPerFindingOfTheEarlierRun(t *testing.T) {
	logIn := contextop.CheckFinding{File: "docs/guide.md", Rule: "terms.vocabulary", Quote: "Log in"}
	prev := contextop.CheckRun{Verdict: "failed", Failing: 2, Findings: []contextop.CheckFinding{logIn, logIn}}
	clean := contextop.CheckRun{Verdict: "passed", Clean: true}
	cut := contextop.CheckRun{Verdict: "failed", Failing: 1}

	assert.Equal(t, []bool{true, true}, CaughtFindings(prev, clean))
	assert.Equal(t, []bool{false, false}, CaughtFindings(prev, cut), "an incomplete run catches none of them")
	assert.Equal(t, []bool{}, CaughtFindings(clean, cut), "a run with no findings has no marks")
}

// TestCheckSequences_OnePerProjectAndSessionInLogOrder: the records arrive in
// any order and come back grouped by project and session, ordered by project
// then session, each group by record id. A record that is not a check run or
// names no session is in no sequence.
func TestCheckSequences_OnePerProjectAndSessionInLogOrder(t *testing.T) {
	run := contextop.CheckRun{Verdict: "passed", Clean: true}
	rec := func(id string, project workspace.ProjectKey, session string) contextop.Record {
		return contextop.Record{ID: id, Project: project, Kind: contextop.KindCheck,
			Actor: contextop.Actor{Kind: contextop.ActorAgent, Name: "claude-code", Session: session}, Check: &run}
	}
	noCheck := rec("op-9", "p", "s1")
	noCheck.Check = nil
	observe := rec("op-8", "p", "s1")
	observe.Kind = contextop.KindObserve
	records := []contextop.Record{
		rec("op-5", "q", "s1"),
		rec("op-3", "p", "s2"),
		rec("op-4", "p", "s1"),
		rec("op-1", "p", "s1"),
		rec("op-2", "p", ""),
		noCheck,
		observe,
	}

	var ids [][]string
	for _, seq := range CheckSequences(records) {
		var group []string
		for _, r := range seq {
			group = append(group, r.ID)
		}
		ids = append(ids, group)
	}
	assert.Equal(t, [][]string{{"op-1", "op-4"}, {"op-3"}, {"op-5"}}, ids)
	assert.Empty(t, CheckSequences(nil))
}

// TestCaughtMarks_PairsOnlyRunsThatReachedAVerdict: a run that did not reach
// a verdict is skipped, so the runs on either side of it are the pair; it and
// the sequence's last run that reached a verdict carry no marks.
func TestCaughtMarks_PairsOnlyRunsThatReachedAVerdict(t *testing.T) {
	logIn := contextop.CheckFinding{File: "docs/guide.md", Rule: "terms.vocabulary", Quote: "Log in"}
	rec := func(id string, run contextop.CheckRun) contextop.Record {
		return contextop.Record{ID: id, Project: "p", Kind: contextop.KindCheck,
			Actor: contextop.Actor{Kind: contextop.ActorAgent, Name: "claude-code", Session: "s1"}, Check: &run}
	}
	seq := []contextop.Record{
		rec("op-1", contextop.CheckRun{Verdict: "failed", Failing: 1, Findings: []contextop.CheckFinding{logIn}}),
		rec("op-2", contextop.CheckRun{}),
		rec("op-3", contextop.CheckRun{Verdict: "passed", Clean: true}),
		rec("op-4", contextop.CheckRun{}),
	}
	assert.Equal(t, [][]bool{{true}, nil, nil, nil}, CaughtMarks(seq))
	assert.Equal(t, 1, SessionOutcomesOf(seq)["s1"].Caught)
}

// TestCaughtMarks_AnEditRunCatchesOnlyOnItsFile: an edit hook run checks one
// file, so it catches a finding on that file and leaves the findings on every
// other file to the next run that checks them. A finding only a ship gate
// reports is left to the next Stop hook run, and the session's failing count
// keeps what no later run checked again.
func TestCaughtMarks_AnEditRunCatchesOnlyOnItsFile(t *testing.T) {
	onGuide := contextop.CheckFinding{File: "docs/guide.md", Rule: "terms.vocabulary", Quote: "Log in"}
	onOther := contextop.CheckFinding{File: "docs/other.md", Rule: "terms.vocabulary", Quote: "Log in"}
	gateOnly := contextop.CheckFinding{File: "docs/guide.md", Rule: "completeness", Message: "greeting has no translation"}
	rec := func(id string, run contextop.CheckRun) contextop.Record {
		return contextop.Record{ID: id, Project: "p", Kind: contextop.KindCheck,
			Actor: contextop.Actor{Kind: contextop.ActorAgent, Name: "claude-code", Session: "s1"}, Check: &run}
	}
	edit := func(file string, findings ...contextop.CheckFinding) contextop.CheckRun {
		run := contextop.CheckRun{Hook: contextop.CheckHookEdit, Files: []string{file}, Edited: true, Covered: 1,
			Verdict: "passed", Clean: len(findings) == 0, Failing: len(findings), Findings: findings}
		if len(findings) > 0 {
			run.Verdict = "failed"
		}
		return run
	}
	stop := contextop.CheckRun{Hook: contextop.CheckHookStop, Verdict: "failed", Failing: 3,
		Findings: []contextop.CheckFinding{onGuide, onOther, gateOnly}}

	seq := []contextop.Record{
		rec("op-1", stop),
		rec("op-2", edit("docs/other.md", onOther)),
		rec("op-3", edit("docs/guide.md")),
	}
	assert.Equal(t, [][]bool{{true, false, false}, {false}, nil}, CaughtMarks(seq),
		"op-3 catches the finding on guide.md; op-2 still reports the one on other.md; the gate's finding waits for a Stop hook run")
	o := SessionOutcomesOf(seq)["s1"]
	assert.Equal(t, 1, o.Caught)
	assert.Equal(t, 2, o.Failing, "the finding on other.md and the gate's finding are left failing")
	assert.Equal(t, map[string]int{"op-3": 1}, runsCaught(seq))

	// An edit run on another file catches nothing on the first.
	seq = []contextop.Record{
		rec("op-1", edit("docs/guide.md", onGuide)),
		rec("op-2", edit("docs/other.md")),
	}
	assert.Equal(t, [][]bool{{false}, nil}, CaughtMarks(seq))
	assert.Equal(t, 1, SessionOutcomesOf(seq)["s1"].Failing)

	// A Stop hook run after an edit run checks every file again.
	seq = []contextop.Record{
		rec("op-1", edit("docs/guide.md", onGuide)),
		rec("op-2", contextop.CheckRun{Hook: contextop.CheckHookStop, Verdict: "passed", Clean: true}),
	}
	assert.Equal(t, [][]bool{{true}, nil}, CaughtMarks(seq))
	assert.Equal(t, 0, SessionOutcomesOf(seq)["s1"].Failing)
}
