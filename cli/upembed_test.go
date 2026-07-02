package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/gate"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRunUp_EmbeddedConverge: the exported RunUp drives the same engine as
// `kapi up` — auto-extract on drift, loop to the gate, materialized targets —
// and returns the structured result with per-pass events instead of printing.
func TestRunUp_EmbeddedConverge(t *testing.T) {
	a := processOnlyApp(t)
	recipe, root := convergeFixture(t, []model.LocaleID{"nb-NO"}, gate.Gate{"translated": {Pct: 100}})

	var passes []ConvergePassEvent
	var log bytes.Buffer
	out, err := a.RunUp(context.Background(), recipe, "", UpOptions{
		UntilGate: true,
		OnPass:    func(ev ConvergePassEvent) { passes = append(passes, ev) },
		LogWriter: &log,
	})
	require.NoError(t, err)
	require.NotNil(t, out)

	// Converged: the deterministic pseudo flow materializes both targets in
	// one pass and the presence baseline meets translated:100.
	assert.True(t, out.Converged)
	assert.Equal(t, 1, out.Passes)
	require.Len(t, out.Locales, 1)
	assert.Equal(t, "nb-NO", out.Locales[0].Locale)
	assert.True(t, out.Locales[0].Shippable)
	assert.Empty(t, out.ParkedScopes)
	for _, f := range []string{"a.json", "b.json"} {
		_, rerr := os.Stat(filepath.Join(root, "src/locales/nb-NO", f))
		require.NoError(t, rerr, "RunUp must materialize %s", f)
	}

	// One structured pass event: the pre-pass auto-extract populated the
	// missing store, and the pass produced both units.
	require.Len(t, passes, 1)
	assert.Equal(t, 1, passes[0].Pass)
	assert.Positive(t, passes[0].ExtractedBlocks, "missing store triggers auto-extract")
	assert.Equal(t, 2, passes[0].Produced)
	assert.Equal(t, 2, passes[0].ProducedDelta)
	assert.Empty(t, passes[0].PendingLocales)
	assert.Contains(t, log.String(), "Extracted", "auto-extract note lands in the log writer")
}

// TestRunUp_ParksUnreachableGate: a gate the flow cannot satisfy parks with
// per-scope detail — and is a result, never an error.
func TestRunUp_ParksUnreachableGate(t *testing.T) {
	a := processOnlyApp(t)
	recipe, _ := convergeFixture(t, []model.LocaleID{"nb-NO"}, gate.Gate{"reviewed": {Pct: 100}})

	out, err := a.RunUp(context.Background(), recipe, "", UpOptions{UntilGate: true, MaxPasses: 2})
	require.NoError(t, err, "parked work is a result, not an error")
	require.NotNil(t, out)
	assert.False(t, out.Converged)
	require.Len(t, out.Locales, 1)
	assert.True(t, out.Locales[0].Parked, "reviewed:100 needs a human — the locale parks")
	require.NotEmpty(t, out.ParkedScopes)
	assert.Equal(t, "nb-NO", out.ParkedScopes[0].Locale)
}

// TestUpPlan_Embedded: the exported UpPlan derives the same dry-run work plan
// as `kapi up --plan`, self-contained from the recipe path.
func TestUpPlan_Embedded(t *testing.T) {
	a := processOnlyApp(t)
	recipe, _ := convergeFixture(t, []model.LocaleID{"nb-NO", "de-DE"}, gate.Gate{"translated": {Pct: 100}})

	plan, err := a.UpPlan(context.Background(), recipe, "")
	require.NoError(t, err)
	require.NotNil(t, plan)

	// Two locales × two source files with one unit each → 2 missing per locale.
	assert.Equal(t, 4, plan.Totals.MissingTarget)
	assert.Equal(t, 4, plan.Totals.AIRemaining, "no TM → everything is AI work")
	assert.Positive(t, plan.Totals.TokenEstimate)
	assert.NotEmpty(t, plan.Note, "the estimation heuristic is disclosed")
	require.Len(t, plan.Scopes, 2)
}
