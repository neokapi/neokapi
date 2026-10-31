package backend

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/contextop"
	"github.com/neokapi/neokapi/host"
)

// TestASessionListsItsCheckRuns: a group carries its session's check runs,
// newest first, each with its files and result, and a finding the next run no
// longer found is marked caught on the run that last reported it. The marks
// add up to the session's caught count.
func TestASessionListsItsCheckRuns(t *testing.T) {
	app := newWorkspaceApp(t)
	recipe, key := openFeedProject(t, app, "KapiMart")
	engine := agentProcess(t, app)
	agentSuggest(t, engine, recipe, "sess-1", "sign in", "log in", "docs/a.md", "Sign in.")

	utilise := contextop.CheckFinding{File: "docs/a.md", Rule: "terms.vocabulary", Quote: "utilise"}
	two := contextop.CheckRun{Verdict: "failed", Failing: 2, Rules: []string{"terms.vocabulary"}, Findings: []contextop.CheckFinding{logInFinding, utilise}}
	start := time.Now().UTC().Add(-time.Hour).Truncate(time.Second)
	hookRun(t, app, key, "sess-1", "t1", start, two)
	hookRun(t, app, key, "sess-1", "t2", start.Add(time.Minute), contextop.CheckRun{Verdict: "did_not_run"})
	hookRun(t, app, key, "sess-1", "t3", start.Add(2*time.Minute), failingRun())
	hookRun(t, app, key, "sess-1", "t4", start.Add(3*time.Minute), cleanRun())
	hookRun(t, app, key, "sess-2", "t1", start, failingRun())

	feed, err := app.ContextFeed(key, 0)
	require.NoError(t, err)
	groups := feedGroups(feed)
	one := groups["sess-1"]
	require.Len(t, one.Runs, 4)
	ats := []string{}
	for _, run := range one.Runs {
		ats = append(ats, run.At)
		assert.Equal(t, key, run.ProjectKey)
		assert.Equal(t, []string{"docs/a.md"}, run.Files)
		assert.True(t, run.Edited)
		assert.Equal(t, 1, run.FileCount)
	}
	assert.Equal(t, []string{
		start.Add(3 * time.Minute).Format(time.RFC3339),
		start.Add(2 * time.Minute).Format(time.RFC3339),
		start.Add(time.Minute).Format(time.RFC3339),
		start.Format(time.RFC3339),
	}, ats, "newest first")

	clean, logIn, notRun, first := one.Runs[0], one.Runs[1], one.Runs[2], one.Runs[3]
	assert.True(t, clean.Reached)
	assert.True(t, clean.Clean)
	assert.Empty(t, clean.Findings)

	assert.Equal(t, 1, logIn.Failing)
	assert.Equal(t, []string{"terms.vocabulary"}, logIn.Rules)
	assert.Equal(t, []ContextCheckFinding{{File: "docs/a.md", Rule: "terms.vocabulary", Quote: "Log in", Caught: true}}, logIn.Findings,
		"the clean run no longer found it, so the run that last reported it caught it")
	assert.Equal(t, 1, logIn.Caught)

	assert.False(t, notRun.Reached, "a run that reached no verdict takes no part")
	assert.Zero(t, notRun.Caught)

	assert.Equal(t, 2, first.Failing)
	assert.Equal(t, []ContextCheckFinding{
		{File: "docs/a.md", Rule: "terms.vocabulary", Quote: "Log in"},
		{File: "docs/a.md", Rule: "terms.vocabulary", Quote: "utilise", Caught: true},
	}, first.Findings, "the next run that reached a verdict still found one of the two")
	assert.Equal(t, 1, first.Caught)

	caught := 0
	for _, run := range one.Runs {
		caught += run.Caught
	}
	assert.Equal(t, one.Caught, caught, "the marks add up to the session's count")

	alone := groups["sess-2"]
	require.Len(t, alone.Runs, 1)
	assert.Equal(t, []ContextCheckFinding{{File: "docs/a.md", Rule: "terms.vocabulary", Quote: "Log in"}}, alone.Runs[0].Findings,
		"a session's last run caught nothing")
}

// TestAPersonsDayListsNoRuns: a group with no session lists no runs, as an
// empty list rather than none.
func TestAPersonsDayListsNoRuns(t *testing.T) {
	at := time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)
	person := contextop.Record{
		ID:      "op-1",
		Project: "kapimart",
		Actor:   contextop.Actor{Kind: contextop.ActorPerson, Name: "ana"},
		Kind:    contextop.KindObserve,
		At:      at,
	}
	groups := groupContextFeed([]contextop.Record{person}, nil, &contextProjectIndex{}, at, time.Time{})
	require.Len(t, groups, 1)
	assert.NotNil(t, groups[0].Runs)
	assert.Empty(t, groups[0].Runs)
}

// checkRecord is one check run with an id that orders it, for the run listing.
func checkRecord(id, session string, at time.Time, run contextop.CheckRun) contextop.Record {
	return contextop.Record{
		ID:      id,
		Project: "kapimart",
		Actor:   contextop.Actor{Kind: contextop.ActorAgent, Name: "claude-code", Session: session},
		Kind:    contextop.KindCheck,
		At:      at,
		Check:   &run,
	}
}

// TestAGroupListsItsLatestRuns: a group lists at most contextFeedMaxRuns
// runs, the newest, and its counts still cover every run.
func TestAGroupListsItsLatestRuns(t *testing.T) {
	start := time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)
	var records []contextop.Record
	for i := range contextFeedMaxRuns + 5 {
		records = append(records, checkRecord(fmt.Sprintf("op-%03d", i), "sess-1", start.Add(time.Duration(i)*time.Minute), failingRun()))
	}
	runs := sessionRunsOf(records)["sess-1"]
	require.Len(t, runs, contextFeedMaxRuns)
	assert.Equal(t, fmt.Sprintf("op-%03d", contextFeedMaxRuns+4), runs[0].ID, "the newest run first")
	assert.Equal(t, "op-005", runs[len(runs)-1].ID, "the oldest runs are left out")

	groups := groupContextFeed(nil, records, &contextProjectIndex{}, start, time.Time{})
	require.Len(t, groups, 1)
	assert.Equal(t, contextFeedMaxRuns+5, groups[0].Checked)
	assert.Len(t, groups[0].Runs, contextFeedMaxRuns)
}

// TestARunListingFewerFindingsCatchesNothing: when the next run lists fewer
// findings than it counted, what it left out is unknown and nothing is marked
// caught, as host.SessionOutcomesOf counts it.
func TestARunListingFewerFindingsCatchesNothing(t *testing.T) {
	start := time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)
	cut := contextop.CheckRun{Verdict: "failed", Failing: 3, Rules: []string{"terms.vocabulary"}}
	records := []contextop.Record{
		checkRecord("op-1", "sess-1", start, failingRun()),
		checkRecord("op-2", "sess-1", start.Add(time.Minute), cut),
	}
	runs := sessionRunsOf(records)["sess-1"]
	require.Len(t, runs, 2)
	require.Len(t, runs[1].Findings, 1)
	assert.False(t, runs[1].Findings[0].Caught)
	assert.Zero(t, runs[1].Caught)
	assert.Zero(t, host.SessionOutcomesOf(records)["sess-1"].Caught)
}

// TestARunListsWhatItCaughtFirst: a run with more findings than it lists
// keeps every caught one in the list, in the run's order, and counts them all.
func TestARunListsWhatItCaughtFirst(t *testing.T) {
	start := time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)
	var many []contextop.CheckFinding
	for i := range contextRunMaxFindings + 10 {
		many = append(many, contextop.CheckFinding{File: "docs/a.md", Rule: "terms.vocabulary", Quote: fmt.Sprintf("word %02d", i)})
	}
	first := contextop.CheckRun{Verdict: "failed", Failing: len(many), Rules: []string{"terms.vocabulary"}, Findings: many}
	// The next run no longer found the last finding and found every other.
	next := contextop.CheckRun{Verdict: "failed", Failing: len(many) - 1, Rules: []string{"terms.vocabulary"}, Findings: many[:len(many)-1]}
	records := []contextop.Record{
		checkRecord("op-1", "sess-1", start, first),
		checkRecord("op-2", "sess-1", start.Add(time.Minute), next),
	}
	runs := sessionRunsOf(records)["sess-1"]
	require.Len(t, runs, 2)
	listed := runs[1].Findings
	require.Len(t, listed, contextRunMaxFindings)
	assert.Equal(t, 1, runs[1].Caught)
	last := listed[len(listed)-1]
	assert.True(t, last.Caught)
	assert.Equal(t, fmt.Sprintf("word %02d", len(many)-1), last.Quote)
	assert.Equal(t, "word 00", listed[0].Quote, "the run's order holds")
	assert.Equal(t, host.SessionOutcomesOf(records)["sess-1"].Caught, runs[1].Caught)
}
