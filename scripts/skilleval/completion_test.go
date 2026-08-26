package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestCompletionIsNotScoredOnTriggering.
//
// The bug this catches shipped for exactly one sweep. Completion mode reported
// "17 scenarios, 17 pass" on a run where only three scenarios carried a gate
// and two of those three came back red: the verdict was still computed from
// whether the agent reached for kapi, which in completion mode is the starting
// line rather than the finish.
//
// A scenario with no gate has no definition of done. Calling that a pass is the
// most flattering thing this page could do, so it gets its own verdict instead.
func TestCompletionIsNotScoredOnTriggering(t *testing.T) {
	green := &GateResult{Command: "true", ExitCode: 0}
	red := &GateResult{Command: "false", ExitCode: 1}

	cases := []struct {
		name string
		gate string
		runs []Run
		want string
	}{
		{
			name: "triggered but nothing verified",
			runs: []Run{{Triggered: true}, {Triggered: true}},
			want: "no gate",
		},
		{
			name: "gate green every pass",
			gate: "test -f out.json",
			runs: []Run{{Triggered: true, Gate: green}, {Triggered: true, Gate: green}},
			want: "pass",
		},
		{
			name: "triggered and the gate is red",
			gate: "test -f out.json",
			runs: []Run{{Triggered: true, Gate: red}, {Triggered: true, Gate: red}},
			want: "fail",
		},
		{
			name: "green once, red once",
			gate: "test -f out.json",
			runs: []Run{{Triggered: true, Gate: green}, {Triggered: true, Gate: red}},
			want: "flaky",
		},
		{
			name: "never reached kapi and the gate is red anyway",
			gate: "test -f out.json",
			runs: []Run{{Triggered: false, Gate: red}},
			want: "fail",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := Result{Scenario: Scenario{Kind: positive, CompletionGate: c.gate}, Runs: c.runs}
			r.score(modeCompletion)
			assert.Equal(t, c.want, r.Verdict)
		})
	}
}

// TestUngatedScenariosAreCountedSeparately: the summary must show how much of a
// completion sweep was actually checkable, or "17 pass" reads as coverage it
// does not have.
func TestUngatedScenariosAreCountedSeparately(t *testing.T) {
	results := []Result{
		{Scenario: Scenario{Kind: positive}, Runs: []Run{{Triggered: true}}},
		{Scenario: Scenario{Kind: positive}, Runs: []Run{{Triggered: true}}},
		{
			Scenario: Scenario{Kind: positive, CompletionGate: "true"},
			Runs:     []Run{{Triggered: true, Gate: &GateResult{ExitCode: 0}}},
		},
	}
	for i := range results {
		results[i].score(modeCompletion)
	}
	s := summarize(results)
	assert.Equal(t, 1, s.Pass, "only the gated scenario was verified")
	assert.Equal(t, 2, s.Ungated, "the other two had no definition of done")
	assert.Contains(t, s.Line(), "no gate to check")
}
