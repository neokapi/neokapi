package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// coldStartEvidence lays out a saved drill the way a run leaves it, so the
// report can be rendered without a model call or a live host.
func coldStartEvidence(t *testing.T) (string, coldStartStudyRecord) {
	t.Helper()
	dir := t.TempDir()
	record := coldStartStudyRecord{
		Schema: coldStartSchema, Fingerprint: "fingerprint-1", Manifest: validColdStartManifest(),
		KapiVersion: "kapi version v1.2.0", KapiCommit: "abc1234", CreatedAt: time.Now().UTC(),
	}
	require.NoError(t, writePairedJSON(filepath.Join(dir, "study.json"), record))
	return dir, record
}

func writeColdStartAttempt(t *testing.T, dir, phase string, attempt coldStartAttempt, result *coldStartAttemptResult) {
	t.Helper()
	attemptDir := filepath.Join(dir, phase, attempt.Session.ID)
	require.NoError(t, os.MkdirAll(attemptDir, 0o700))
	require.NoError(t, writePairedJSON(filepath.Join(attemptDir, "started.json"), attempt))
	if result != nil {
		require.NoError(t, writePairedJSON(filepath.Join(attemptDir, "result.json"), *result))
	}
}

func coldStartStore(byKind map[string]int, withEvidence, withoutEvidence int) ColdStartStore {
	return ColdStartStore{Operations: []ColdStartOperation{}, ByKind: byKind, Candidates: []ColdStartOperation{},
		WithEvidence: withEvidence, WithoutEvidence: withoutEvidence}
}

func TestReportColdStartRendersThreeMeasures(t *testing.T) {
	dir, record := coldStartEvidence(t)
	session := ColdStartSession{
		ID: "empty-export-section-claude-one", Cell: "empty-export-section-claude", Stage: coldStartStageOne,
		Host: record.Manifest.Hosts[0], Task: "empty-export-section",
	}
	writeColdStartAttempt(t, dir, coldStartPhaseSmoke, coldStartAttempt{
		Schema: coldStartSchema, Session: session, Phase: coldStartPhaseSmoke, Fingerprint: record.Fingerprint,
		HostVersion: "2.1.278 (Claude Code)",
		Prepared:    ColdStartPrepared{Wiring: ColdStartWiring{Harness: []string{"codex config.toml: the kapi server copied across"}}},
	}, &coldStartAttemptResult{
		Status: "completed", IdentityStatus: "verified", UserDataUntouched: true,
		StoreBefore: coldStartStore(map[string]int{}, 0, 0),
		StoreAfter:  coldStartStore(map[string]int{"observe": 2, "correct": 1, "propose": 1}, 3, 1),
		Transcript: ColdStartTranscript{Status: "completed", AskedBeforeWriting: true, SkillLoaded: true, Calls: []ColdStartCall{
			{Order: 1, Surface: "mcp", Tool: "context_search", Kind: coldStartKindAsk},
			{Order: 2, Surface: "mcp", Tool: "context_observe", Kind: coldStartKindRecord},
		}},
		Changed: []string{"M docs/troubleshooting.md"},
	})

	require.NoError(t, reportColdStart(context.Background(), ColdStartOptions{Dir: dir}))

	rendered := coldStartLatest(t, dir, "report-*.md")
	assert.Contains(t, rendered, "## What the first session recorded")
	assert.Contains(t, rendered, "## What a person confirmed")
	assert.Contains(t, rendered, "## What the second session did")
	assert.Contains(t, rendered, "context_search, context_observe")
	assert.Contains(t, rendered, "observe 2, propose 1, correct 1")
	assert.Contains(t, rendered, "3 of 4")
	assert.Contains(t, rendered, "kapi version v1.2.0 (abc1234)")
	assert.Contains(t, rendered, "claude 2.1.278 (Claude Code)")
	assert.Contains(t, rendered, "## What the harness wired by hand")

	var report ColdStartReport
	require.NoError(t, readPairedJSON(coldStartLatestPath(t, dir, "report-*.json"), &report))
	assert.Equal(t, 1, report.Attempts)
	assert.Len(t, report.Rows, len(record.Manifest.Tasks)*len(record.Manifest.Hosts),
		"every cell the manifest describes is a row, run or not")
	for _, row := range report.Rows {
		if row.Cell != "empty-export-section-claude" {
			assert.False(t, row.One.Ran, "a cell that has not run reports no session")
			assert.Empty(t, coldStartYes(row.Two.AskedFirst, row.Two.Ran),
				"a second session that has not run leaves its column empty")
		}
	}
}

// A session that reached no kapi surface is the result that would end the
// premise, so it has to render as a row rather than be left out.
func TestReportColdStartKeepsASessionThatRecordedNothing(t *testing.T) {
	dir, record := coldStartEvidence(t)
	session := ColdStartSession{
		ID: "release-note-codex-one", Cell: "release-note-codex", Stage: coldStartStageOne,
		Host: record.Manifest.Hosts[1], Task: "release-note",
	}
	writeColdStartAttempt(t, dir, coldStartPhaseSessionOne, coldStartAttempt{
		Schema: coldStartSchema, Session: session, Phase: coldStartPhaseSessionOne, Fingerprint: record.Fingerprint,
		HostVersion: "codex-cli 0.155.1",
	}, &coldStartAttemptResult{
		Status: "completed", IdentityStatus: "verified", UserDataUntouched: true,
		StoreBefore: coldStartStore(map[string]int{}, 0, 0),
		StoreAfter:  coldStartStore(map[string]int{}, 0, 0),
		Transcript:  ColdStartTranscript{Status: "completed", Calls: []ColdStartCall{}},
		Changed:     []string{"?? docs/releases/2026-09.md"},
	})

	require.NoError(t, reportColdStart(context.Background(), ColdStartOptions{Dir: dir}))
	rendered := coldStartLatest(t, dir, "report-*.md")
	assert.Contains(t, rendered, "| release-note-codex | completed | none | 0 | nothing | 0 of 0 | yes |")
}

// An attempt that was reserved and never finished stays in the record, because
// dropping it would make a ceiling reached by failures look like a batch that
// was never run.
func TestReportColdStartKeepsAReservedAttempt(t *testing.T) {
	dir, record := coldStartEvidence(t)
	session := ColdStartSession{
		ID: "release-note-claude-one", Cell: "release-note-claude", Stage: coldStartStageOne,
		Host: record.Manifest.Hosts[0], Task: "release-note",
	}
	writeColdStartAttempt(t, dir, coldStartPhaseSessionOne, coldStartAttempt{
		Schema: coldStartSchema, Session: session, Phase: coldStartPhaseSessionOne, Fingerprint: record.Fingerprint,
	}, nil)

	require.NoError(t, reportColdStart(context.Background(), ColdStartOptions{Dir: dir}))
	rendered := coldStartLatest(t, dir, "report-*.md")
	assert.Contains(t, rendered, "attempt was reserved but no result was committed")
}

func TestReportColdStartFoldsTheReviewReadback(t *testing.T) {
	dir, record := coldStartEvidence(t)
	session := ColdStartSession{
		ID: "empty-export-section-claude-one", Cell: "empty-export-section-claude", Stage: coldStartStageOne,
		Host: record.Manifest.Hosts[0], Task: "empty-export-section",
	}
	writeColdStartAttempt(t, dir, coldStartPhaseSmoke, coldStartAttempt{
		Schema: coldStartSchema, Session: session, Phase: coldStartPhaseSmoke, Fingerprint: record.Fingerprint,
	}, &coldStartAttemptResult{
		Status: "completed", StoreBefore: coldStartStore(map[string]int{}, 0, 0),
		StoreAfter: coldStartStore(map[string]int{"propose": 3}, 3, 0),
		Transcript: ColdStartTranscript{Status: "completed", Calls: []ColdStartCall{}},
	})
	require.NoError(t, writePairedJSON(filepath.Join(dir, "review-20260921T000000.000000000Z.json"), ColdStartReview{
		Schema: coldStartSchema, Study: record.Manifest.Study,
		Cells: []ColdStartReviewCell{{
			Cell: "empty-export-section-claude", Host: "claude", Confirmed: 2, Discarded: 1,
			Candidates: []ColdStartOperation{{ID: "5"}},
		}},
	}))

	require.NoError(t, reportColdStart(context.Background(), ColdStartOptions{Dir: dir}))
	rendered := coldStartLatest(t, dir, "report-*.md")
	assert.Contains(t, rendered, "| empty-export-section-claude | yes | 2 | 1 | 1 |")
}

func TestReportColdStartRefusesAnAttemptFromAnotherStudy(t *testing.T) {
	dir, record := coldStartEvidence(t)
	writeColdStartAttempt(t, dir, coldStartPhaseSmoke, coldStartAttempt{
		Schema: coldStartSchema, Fingerprint: "another-study",
		Session: ColdStartSession{ID: "release-note-claude-one", Cell: "release-note-claude", Stage: coldStartStageOne, Host: record.Manifest.Hosts[0]},
	}, nil)
	err := reportColdStart(context.Background(), ColdStartOptions{Dir: dir})
	require.ErrorContains(t, err, "fingerprint differs")
}

func TestReportColdStartWithoutAStudy(t *testing.T) {
	err := reportColdStart(context.Background(), ColdStartOptions{Dir: t.TempDir()})
	require.ErrorContains(t, err, "no drill has been recorded here")
}

func TestColdStartGrowthCountsOneSession(t *testing.T) {
	growth := coldStartGrowth(
		coldStartStore(map[string]int{"observe": 2, "propose": 1}, 3, 0),
		coldStartStore(map[string]int{"observe": 3, "propose": 1, "confirm": 2}, 4, 0),
	)
	assert.Equal(t, map[string]int{"observe": 1, "confirm": 2}, growth,
		"a second session's row counts what that session added")
}

func TestColdStartCheckVerdict(t *testing.T) {
	assert.Empty(t, coldStartCheckVerdict(nil))
	assert.Equal(t, "did not run", coldStartCheckVerdict(&ColdStartCheck{Error: "exit 4"}))
	assert.Equal(t, "passed, score 100", coldStartCheckVerdict(&ColdStartCheck{Ran: true, Pass: true, Score: 100}))
	assert.Equal(t, "failed, score 80", coldStartCheckVerdict(&ColdStartCheck{Ran: true, Score: 80}))
	assert.Equal(t, "none", coldStartCheckDetail(&ColdStartCheck{Ran: true}))
}

func coldStartLatestPath(t *testing.T, dir, pattern string) string {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(dir, pattern))
	require.NoError(t, err)
	require.NotEmpty(t, matches, "expected a %s under %s", pattern, dir)
	return matches[len(matches)-1]
}

func coldStartLatest(t *testing.T, dir, pattern string) string {
	t.Helper()
	body, err := os.ReadFile(coldStartLatestPath(t, dir, pattern))
	require.NoError(t, err)
	return string(body)
}
