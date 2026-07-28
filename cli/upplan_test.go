package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/neokapi/neokapi/core/gate"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/host/config"
	"github.com/neokapi/neokapi/memory"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// seedPlanMemory writes a project content memory with one exact entry for the fixture's
// "Hello, world." source (a.json), leaving b.json's "Goodbye." uncovered.
func seedPlanMemory(t *testing.T, root string) {
	t.Helper()
	memoryPath := filepath.Join(root, ".kapi", "memory.db")
	tm, err := memory.NewSQLiteStore(memoryPath)
	require.NoError(t, err)
	defer tm.Close()
	now := time.Now().UTC()
	require.NoError(t, tm.Add(t.Context(), memory.Entry{
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

// TestUpPlan_MemoryLeverageAndTokenEstimate: --plan reports, per locale, the units
// missing a target split into exact content-memory hits and remaining AI work with a
// chars/4 token estimate — and writes nothing.
func TestUpPlan_MemoryLeverageAndTokenEstimate(t *testing.T) {
	a := processOnlyApp(t)
	recipe, root := convergeFixture(t, []model.LocaleID{"nb-NO"}, gate.Gate{"translated": gate.Threshold{Pct: 100}})
	seedPlanMemory(t, root)

	out, err := runUp(t, a, recipe, "--plan", "--json")
	require.NoError(t, err, out)

	var plan UpPlanOutput
	require.NoError(t, json.Unmarshal([]byte(out), &plan), "plan --json must be valid JSON: %s", out)
	require.Len(t, plan.Scopes, 1)
	s := plan.Scopes[0]
	assert.Equal(t, "nb-NO", s.Locale)
	assert.Equal(t, 2, s.MissingTarget, "both fixture units lack targets")
	assert.Equal(t, 1, s.MemoryExact, `"Hello, world." has an exact content-memory hit`)
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
	assert.Contains(t, out, "content memory exact")
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
	assert.Equal(t, 0, EstimateTokens(""))
	assert.Equal(t, 1, EstimateTokens("abc"))
	assert.Equal(t, 1, EstimateTokens("abcd"))
	assert.Equal(t, 2, EstimateTokens("abcde"))
}

// TestUpPlan_SubscriptionProviderNote: with the claude-code default provider,
// the plan swaps the metered framing for subscription wording — in the text
// table and in the JSON (provider + subscription fields).
func TestUpPlan_SubscriptionProviderNote(t *testing.T) {
	a := processOnlyApp(t)
	a.Config = config.NewAppConfig()
	a.Config.Set(config.KeyAIProvider, "claude-code")
	recipe, _ := convergeFixture(t, []model.LocaleID{"nb-NO"}, gate.Gate{"translated": gate.Threshold{Pct: 100}})

	out, err := runUp(t, a, recipe, "--plan")
	require.NoError(t, err, out)
	assert.Contains(t, out, "runs on your Claude subscription")
	assert.Contains(t, out, "no per-token API cost")

	a2 := processOnlyApp(t)
	a2.Config = config.NewAppConfig()
	a2.Config.Set(config.KeyAIProvider, "claude-code")
	outJSON, err := runUp(t, a2, recipe, "--plan", "--json")
	require.NoError(t, err, outJSON)
	var plan UpPlanOutput
	require.NoError(t, json.Unmarshal([]byte(outJSON), &plan))
	assert.Equal(t, "claude-code", plan.Provider)
	assert.True(t, plan.Subscription)
	assert.Contains(t, plan.Note, "Claude subscription")
}

// TestUpPlan_MeteredProviderKeepsNote: a metered provider keeps the standard
// estimation note with no subscription framing.
func TestUpPlan_MeteredProviderKeepsNote(t *testing.T) {
	a := processOnlyApp(t)
	a.Config = config.NewAppConfig()
	a.Config.Set(config.KeyAIProvider, "anthropic")
	recipe, _ := convergeFixture(t, []model.LocaleID{"nb-NO"}, gate.Gate{"translated": gate.Threshold{Pct: 100}})

	outJSON, err := runUp(t, a, recipe, "--plan", "--json")
	require.NoError(t, err, outJSON)
	var plan UpPlanOutput
	require.NoError(t, json.Unmarshal([]byte(outJSON), &plan))
	assert.Equal(t, "anthropic", plan.Provider)
	assert.False(t, plan.Subscription)
	assert.NotContains(t, plan.Note, "subscription")
}
