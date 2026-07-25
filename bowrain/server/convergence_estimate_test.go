package server

import (
	"context"
	"testing"

	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// balanceBillingStore is a mockBillingStore with a fixed spendable balance, so
// the estimate's credit math can be exercised without Postgres.
type balanceBillingStore struct {
	*mockBillingStore
	balance int64
}

func (b balanceBillingStore) CheckCredits(context.Context, string) (int64, error) {
	return b.balance, nil
}

// TestConvergenceEstimate_SourceHeldVsReady proves the pre-flight estimate
// splits source blocks by the gate — held (below the gate) never counts toward
// the translation estimate — and that the per-locale pending work is computed
// over the READY source only. No AI provider is configured on the harness, so
// the estimate is provably provider-free.
func TestConvergenceEstimate_SourceHeldVsReady(t *testing.T) {
	s, cs, _ := sourceFirstHarness(t)
	mkProject(t, cs, "p", nil) // default gate = checked
	// Two ready (checked) blocks and one held (authored) block, all untranslated.
	storeSourceItem(t, cs, "p", "a.json",
		srcBlk{"ready1", "A well-formed sentence.", model.SourceStatusChecked},
		srcBlk{"ready2", "Another ready sentence.", model.SourceStatusChecked},
		srcBlk{"held1", "Not yet settled.", model.SourceStatusAuthored})

	proj, err := cs.GetProject(t.Context(), "p")
	require.NoError(t, err)

	view, err := s.convergence.buildConvergenceEstimate(t.Context(), proj)
	require.NoError(t, err)

	// Source readiness: 3 total, 2 ready, 1 held on the default `checked` gate.
	assert.Equal(t, model.SourceGateChecked, view.Source.Gate)
	assert.Equal(t, 3, view.Source.Total)
	assert.Equal(t, 2, view.Source.Ready)
	assert.Equal(t, 1, view.Source.Held)

	// Translation work is over the READY source only (fr is the single target).
	require.Len(t, view.Locales, 1)
	fr := view.Locales[0]
	assert.Equal(t, string(model.LocaleFrench), fr.Locale)
	assert.Equal(t, 2, fr.Pending, "only the 2 ready blocks are pending; the held block is excluded")
	assert.Equal(t, 2, fr.ViaAI, "no content memory → all pending is AI work")
	assert.Equal(t, 0, fr.ViaMemory)
	assert.Greater(t, fr.TokenEstimate, 0, "the AI remainder has a non-zero token estimate")

	// Totals roll up the single locale.
	assert.Equal(t, 2, view.Totals.Pending)
	assert.Equal(t, 2, view.Totals.ViaAI)

	// No billing store configured → no credit line (self-hosted path).
	assert.Nil(t, view.Credits)
}

// TestConvergenceEstimate_GateNone_NoHold: the opt-out admits every block, so
// nothing is held and the estimate covers the whole corpus.
func TestConvergenceEstimate_GateNone_NoHold(t *testing.T) {
	s, cs, _ := sourceFirstHarness(t)
	mkProject(t, cs, "p", map[string]string{"source_gate": "none"})
	storeSourceItem(t, cs, "p", "a.json",
		srcBlk{"b1", "One.", model.SourceStatusNew},
		srcBlk{"b2", "Two.", model.SourceStatusNew})

	proj, err := cs.GetProject(t.Context(), "p")
	require.NoError(t, err)

	view, err := s.convergence.buildConvergenceEstimate(t.Context(), proj)
	require.NoError(t, err)

	assert.Equal(t, model.SourceGateNone, view.Source.Gate)
	assert.Equal(t, 2, view.Source.Total)
	assert.Equal(t, 2, view.Source.Ready)
	assert.Equal(t, 0, view.Source.Held, "gate none holds nothing")
	require.Len(t, view.Locales, 1)
	assert.Equal(t, 2, view.Locales[0].Pending)
}

// TestConvergenceEstimate_AllHeld_NoTranslationWork: when every source block is
// below the gate, the estimate reports the hold and no per-locale work — the
// "settle your source first" case.
func TestConvergenceEstimate_AllHeld_NoTranslationWork(t *testing.T) {
	s, cs, _ := sourceFirstHarness(t)
	mkProject(t, cs, "p", nil) // default checked
	storeSourceItem(t, cs, "p", "a.json",
		srcBlk{"h1", "Held.", model.SourceStatusAuthored},
		srcBlk{"h2", "Also held.", model.SourceStatusAuthored})

	proj, err := cs.GetProject(t.Context(), "p")
	require.NoError(t, err)

	view, err := s.convergence.buildConvergenceEstimate(t.Context(), proj)
	require.NoError(t, err)

	assert.Equal(t, 2, view.Source.Held)
	assert.Equal(t, 0, view.Source.Ready)
	assert.Empty(t, view.Locales, "no ready source → no translation work to estimate")
	assert.Equal(t, 0, view.Totals.Pending)
}

// TestConvergenceEstimate_ExcludesTranslatedReady: a ready block that already
// has a target for the locale is not pending — the estimate counts only work
// that remains.
func TestConvergenceEstimate_ExcludesTranslatedReady(t *testing.T) {
	s, cs, _ := sourceFirstHarness(t)
	mkProject(t, cs, "p", nil)

	// One ready block already translated to fr, one ready block pending.
	done := &model.Block{ID: "done", Translatable: true, SourceStatus: model.SourceStatusChecked}
	done.SetSourceText("Already translated.")
	done.SetTargetText(model.LocaleFrench, "Déjà traduit.")
	pending := &model.Block{ID: "pending", Translatable: true, SourceStatus: model.SourceStatusChecked}
	pending.SetSourceText("Still pending.")
	require.NoError(t, cs.StoreItem(t.Context(), "p", "main", &platstore.Item{ProjectID: "p", Name: "a.json", Format: "json"}))
	require.NoError(t, cs.StoreBlocksForItem(t.Context(), "p", "main", "a.json", []*model.Block{done, pending}))

	proj, err := cs.GetProject(t.Context(), "p")
	require.NoError(t, err)

	view, err := s.convergence.buildConvergenceEstimate(t.Context(), proj)
	require.NoError(t, err)

	assert.Equal(t, 2, view.Source.Ready)
	require.Len(t, view.Locales, 1)
	assert.Equal(t, 1, view.Locales[0].Pending, "the already-translated block is not pending")
}

// TestConvergenceEstimate_Credits: with a billing store and a workspace, the
// estimate prices the AI remainder (1 token ≈ 1 credit) and reports whether the
// balance covers it. A balance below the cost yields covers_all_ai=false and a
// partial coverage count.
func TestConvergenceEstimate_Credits(t *testing.T) {
	s, cs, _ := sourceFirstHarness(t)
	require.NoError(t, cs.CreateProject(t.Context(), &platstore.Project{
		ID: "p", Name: "p", DefaultSourceLanguage: model.LocaleEnglish,
		TargetLanguages: []model.LocaleID{model.LocaleFrench},
		WorkspaceID:     "ws-1",
	}))
	storeSourceItem(t, cs, "p", "a.json",
		srcBlk{"r1", "A ready sentence to translate.", model.SourceStatusChecked},
		srcBlk{"r2", "Another ready sentence to translate.", model.SourceStatusChecked})

	proj, err := cs.GetProject(t.Context(), "p")
	require.NoError(t, err)

	// Balance below the estimated cost → partial coverage.
	s.BillingStore = balanceBillingStore{mockBillingStore: &mockBillingStore{}, balance: 1}
	view, err := s.convergence.buildConvergenceEstimate(t.Context(), proj)
	require.NoError(t, err)
	require.NotNil(t, view.Credits, "billing configured → a credit line is returned")
	assert.Equal(t, int64(1), view.Credits.Balance)
	assert.Greater(t, view.Credits.EstimatedCredits, int64(1))
	assert.False(t, view.Credits.CoversAllAI, "a balance below the cost does not cover all AI work")

	// A generous balance covers everything.
	s.BillingStore = balanceBillingStore{mockBillingStore: &mockBillingStore{}, balance: 1_000_000}
	view, err = s.convergence.buildConvergenceEstimate(t.Context(), proj)
	require.NoError(t, err)
	require.NotNil(t, view.Credits)
	assert.True(t, view.Credits.CoversAllAI)
}
