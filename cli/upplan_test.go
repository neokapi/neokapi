package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/neokapi/neokapi/core/gate"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/sievepen"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// seedPlanTM writes a project TM with one exact entry for the fixture's
// "Hello, world." source (a.json), leaving b.json's "Goodbye." uncovered.
func seedPlanTM(t *testing.T, root string) {
	t.Helper()
	tmPath := filepath.Join(root, ".kapi", "tm.db")
	tm, err := sievepen.NewSQLiteTM(tmPath)
	require.NoError(t, err)
	defer tm.Close()
	now := time.Now().UTC()
	require.NoError(t, tm.Add(t.Context(), sievepen.TMEntry{
		ID: "seed-1",
		Variants: map[model.LocaleID][]model.Run{
			"en-US": {{Text: &model.TextRun{Text: "Hello, world."}}},
			"nb-NO": {{Text: &model.TextRun{Text: "Hei, verden."}}},
		},
		HintSrcLang: "en-US",
		CreatedAt:   now,
		UpdatedAt:   now,
	}))
}

// TestUpPlan_TMLeverageAndTokenEstimate: --plan reports, per locale, the units
// missing a target split into exact TM hits and remaining AI work with a
// chars/4 token estimate — and writes nothing.
func TestUpPlan_TMLeverageAndTokenEstimate(t *testing.T) {
	a := processOnlyApp(t)
	recipe, root := convergeFixture(t, []model.LocaleID{"nb-NO"}, gate.Gate{"translated": gate.Threshold{Pct: 100}})
	seedPlanTM(t, root)

	out, err := runUp(t, a, recipe, "--plan", "--json")
	require.NoError(t, err, out)

	var plan UpPlanOutput
	require.NoError(t, json.Unmarshal([]byte(out), &plan), "plan --json must be valid JSON: %s", out)
	require.Len(t, plan.Scopes, 1)
	s := plan.Scopes[0]
	assert.Equal(t, "nb-NO", s.Locale)
	assert.Equal(t, 2, s.MissingTarget, "both fixture units lack targets")
	assert.Equal(t, 1, s.TMExact, `"Hello, world." has an exact TM hit`)
	assert.Equal(t, 1, s.AIRemaining, `"Goodbye." remains for AI`)
	// "Goodbye." is 8 chars → ceil(8/4) = 2 estimated tokens.
	assert.Equal(t, 2, s.TokenEstimate)
	assert.Equal(t, plan.Totals.MissingTarget, s.MissingTarget)
	assert.NotEmpty(t, plan.Note, "the estimation method is disclosed")

	// Dry run: no targets written, no block store created.
	_, statErr := os.Stat(filepath.Join(root, "src/locales/nb-NO", "a.json"))
	assert.True(t, os.IsNotExist(statErr), "--plan must not write targets")
	_, statErr = os.Stat(filepath.Join(root, ".kapi", "cache", "blocks.db"))
	assert.True(t, os.IsNotExist(statErr), "--plan must not create the block store")
}

// TestUpPlan_TextTable: the human rendering is a table with a totals row and
// the estimation note.
func TestUpPlan_TextTable(t *testing.T) {
	a := processOnlyApp(t)
	recipe, _ := convergeFixture(t, []model.LocaleID{"nb-NO"}, gate.Gate{"translated": gate.Threshold{Pct: 100}})

	out, err := runUp(t, a, recipe, "--plan")
	require.NoError(t, err, out)
	assert.Contains(t, out, "dry run")
	assert.Contains(t, out, "TM exact")
	assert.Contains(t, out, "total")
	assert.Contains(t, out, "chars / 4", "the token heuristic is disclosed")
}

// TestUpPlan_NoWritesToExistingStore: with an existing block store, --plan
// leaves it byte-for-byte alone (mtime unchanged).
func TestUpPlan_NoWritesToExistingStore(t *testing.T) {
	a := processOnlyApp(t)
	recipe, root := convergeFixture(t, []model.LocaleID{"nb-NO"}, gate.Gate{"translated": gate.Threshold{Pct: 100}})

	// Materialize the store via a real up first.
	out, err := runUp(t, a, recipe)
	require.NoError(t, err, out)
	storePath := filepath.Join(root, ".kapi", "cache", "blocks.db")
	before, err := os.Stat(storePath)
	require.NoError(t, err)

	a2 := processOnlyApp(t)
	out2, err := runUp(t, a2, recipe, "--plan")
	require.NoError(t, err, out2)

	after, err := os.Stat(storePath)
	require.NoError(t, err)
	assert.Equal(t, before.ModTime(), after.ModTime(), "--plan must not touch the block store")
	assert.Equal(t, before.Size(), after.Size())
}

// TestUpPlan_ConvergedProjectHasNoWork: after a converged up, --plan reports
// nothing to do.
func TestUpPlan_ConvergedProjectHasNoWork(t *testing.T) {
	a := processOnlyApp(t)
	recipe, _ := convergeFixture(t, []model.LocaleID{"nb-NO"}, gate.Gate{"translated": gate.Threshold{Pct: 100}})
	out, err := runUp(t, a, recipe)
	require.NoError(t, err, out)

	a2 := processOnlyApp(t)
	out2, err := runUp(t, a2, recipe, "--plan")
	require.NoError(t, err, out2)
	assert.Contains(t, out2, "Nothing to do")
}

// TestEstimateTokens: the chars/4 heuristic rounds up and zeroes on empty.
func TestEstimateTokens(t *testing.T) {
	assert.Equal(t, 0, estimateTokens(""))
	assert.Equal(t, 1, estimateTokens("abc"))
	assert.Equal(t, 1, estimateTokens("abcd"))
	assert.Equal(t, 2, estimateTokens("abcde"))
}
