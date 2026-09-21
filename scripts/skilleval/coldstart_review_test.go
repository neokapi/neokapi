package main

import (
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func coldStartReviewCell() ColdStartReviewCell {
	root := filepath.Join("/tmp", "kapi-coldstart-demo", "release-note-codex")
	return ColdStartReviewCell{
		Cell: "release-note-codex", Host: "codex", Task: "release-note",
		Repo: filepath.Join(root, coldStartRepoName), DataDir: filepath.Join(root, "kapi-data"),
		Recorded: 3, WithEvidence: 2, AgentRecorded: 2, PersonRecorded: 1,
		Candidates: []ColdStartOperation{
			{ID: "op_1", Kind: "observe", Subject: "sign in", Actor: coldStartActorAgent,
				AgentName: "codex", Session: "019a2c", Evidence: []string{"docs/getting-started.md sign in"}},
			{ID: "op_2", Kind: "propose", Subject: `term "Fernwell", use "Fernwell Ledger"`, Actor: coldStartActorPerson},
		},
	}
}

// A person reading the sheet decides on each candidate, and who recorded it is
// part of the decision: an entry an agent recorded is a candidate, and one
// attributed to a person carries a person's standing.
func TestColdStartReviewSheetNamesWhoRecordedEachCandidate(t *testing.T) {
	sheet := renderColdStartReview(ColdStartReview{
		Schema: coldStartSchema, Study: "kapi-cold-start", Cells: []ColdStartReviewCell{coldStartReviewCell()},
	})

	assert.Contains(t, sheet, "Recorded by an agent: 2; by a person: 1.")
	assert.Contains(t, sheet, "| op_1 | observe | sign in | docs/getting-started.md sign in | agent codex, session 019a2c |")
	assert.Contains(t, sheet, "| op_2 | propose")
	assert.Contains(t, sheet, "| person |")
}

// macOS `open` hands the application the login session's environment, so a
// prefix in front of it would reach the shell and never the app.
func TestColdStartDesktopCommandCarriesTheCellRoots(t *testing.T) {
	cell := coldStartReviewCell()

	command := coldStartDesktopCommand(cell, "")

	assert.Contains(t, command, cell.DataDir)
	assert.Contains(t, command, "KAPI_PLUGINS_DIR_ONLY=1")
	if runtime.GOOS != "darwin" {
		assert.True(t, strings.HasSuffix(command, pairedShellQuote(coldStartDesktopApp)), "rendered %s", command)
		return
	}
	assert.True(t, strings.HasPrefix(command, "open -n --env "), "rendered %s", command)
	assert.Contains(t, command, "--env KAPI_DATA_DIR="+pairedShellQuote(cell.DataDir))
	assert.True(t, strings.HasSuffix(command, "-a "+pairedShellQuote(coldStartDesktopApp)), "rendered %s", command)
	assert.Contains(t, coldStartDesktopCommand(cell, "/opt/kapi/Kapi.app"), "-a "+pairedShellQuote("/opt/kapi/Kapi.app"))
}

func TestColdStartActorLabel(t *testing.T) {
	assert.Equal(t, "agent codex, session 019a2c",
		coldStartActorLabel(ColdStartOperation{Actor: coldStartActorAgent, AgentName: "codex", Session: "019a2c"}))
	assert.Equal(t, "person", coldStartActorLabel(ColdStartOperation{Actor: coldStartActorPerson}))
	assert.Equal(t, "unstated", coldStartActorLabel(ColdStartOperation{}))
}

func TestColdStartRecordedSinceMatchesByID(t *testing.T) {
	before := ColdStartStore{Operations: []ColdStartOperation{{ID: "op_1", Kind: "observe"}}}
	after := ColdStartStore{Operations: []ColdStartOperation{
		{ID: "op_1", Kind: "observe"},
		{ID: "op_2", Kind: "propose", Actor: coldStartActorAgent},
	}}

	added := coldStartRecordedSince(before, after)

	assert.Len(t, added, 1)
	assert.Equal(t, "op_2", added[0].ID)
}
