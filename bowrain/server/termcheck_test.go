package server

import (
	"context"
	"testing"
	"time"

	platev "github.com/neokapi/neokapi/bowrain/core/event"
	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/bowrain/knowledge"
	corebrand "github.com/neokapi/neokapi/core/brand"
	"github.com/neokapi/neokapi/core/id"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/termbase"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mkFrBlock builds a translatable block with an en source and an fr target.
func mkFrBlock(source, frTarget string) *model.Block {
	b := &model.Block{Translatable: true}
	b.SetSourceText(source)
	if frTarget != "" {
		b.SetTargetText("fr", frTarget)
	}
	return b
}

// TestBlockTermCompliant_Directions unit-tests the shared predicate directly:
// both violation directions (forbidden/competitor PRESENCE from the termbase and
// from the brand vocabulary; mandated-rendering ABSENCE), the compliant case, an
// untranslated target, and the no-termbase/no-profile no-op.
func TestBlockTermCompliant_Directions(t *testing.T) {
	ctx := context.Background()
	tb := termbase.NewInMemoryTermBase()
	require.NoError(t, tb.AddConcept(ctx, termbase.Concept{
		ID:     "c-use",
		Source: termbase.TermSourceTerminology,
		Terms: []termbase.Term{
			{Text: "utiliser", Locale: "fr", Status: model.TermForbidden},
			{Text: "employer", Locale: "fr", Status: model.TermPreferred},
		},
	}))
	require.NoError(t, tb.AddConcept(ctx, termbase.Concept{
		ID:     "c-app",
		Source: termbase.TermSourceTerminology,
		Terms: []termbase.Term{
			{Text: "app", Locale: "en", Status: model.TermApproved},
			{Text: "application", Locale: "fr", Status: model.TermPreferred},
		},
	}))

	// PRESENCE (termbase): the target uses the forbidden "utiliser".
	assert.False(t, blockTermCompliant(ctx, mkFrBlock("Use it", "Il faut utiliser ceci"), "en", "fr", tb, nil),
		"a forbidden termbase term in the target is non-compliant")
	// ABSENCE (termbase): the source uses "app" but the target omits "application".
	assert.False(t, blockTermCompliant(ctx, mkFrBlock("Open the app", "Ouvrir le truc"), "en", "fr", tb, nil),
		"a missing mandated rendering is non-compliant")
	// Compliant: uses the mandated rendering, no forbidden term.
	assert.True(t, blockTermCompliant(ctx, mkFrBlock("Close the app", "Fermer l'application"), "en", "fr", tb, nil),
		"a target that uses the mandated rendering and no forbidden term is compliant")
	// An untranslated target is vacuously compliant (nothing to check).
	assert.True(t, blockTermCompliant(ctx, mkFrBlock("Open the app", ""), "en", "fr", tb, nil),
		"an empty target is compliant")
	// No termbase and no profile → pure no-op: even a would-be violation is compliant.
	assert.True(t, blockTermCompliant(ctx, mkFrBlock("Use it", "Il faut utiliser ceci"), "en", "fr", nil, nil),
		"with no termbase and no profile the predicate is a no-op")

	// PRESENCE (brand vocabulary): a forbidden brand rule matched in the target.
	profile := &corebrand.VoiceProfile{
		ID: "p",
		Vocabulary: corebrand.VocabularyRules{
			ForbiddenTerms: []corebrand.TermRule{{Term: "cheap"}},
		},
	}
	assert.False(t, blockTermCompliant(ctx, mkFrBlock("Affordable", "cheap stuff"), "en", "fr", nil, profile),
		"a forbidden brand-vocabulary term in the target is non-compliant")
	assert.True(t, blockTermCompliant(ctx, mkFrBlock("Affordable", "budget-friendly"), "en", "fr", nil, profile),
		"a target with no forbidden brand term is compliant")
}

// seedTermUnificationConcepts adds the two concepts the unification tests share:
// c-use marks fr "utiliser" forbidden (PRESENCE) and c-app mandates fr
// "application" for source "app" (ABSENCE).
func seedTermUnificationConcepts(t *testing.T, tb termbase.TBStore) (useID, appID string) {
	t.Helper()
	ctx := context.Background()
	useID, appID = id.New(), id.New()
	require.NoError(t, tb.AddConcept(ctx, termbase.Concept{
		ID:     useID,
		Source: termbase.TermSourceTerminology,
		Terms: []termbase.Term{
			{Text: "utiliser", Locale: "fr", Status: model.TermForbidden},
			{Text: "employer", Locale: "fr", Status: model.TermPreferred},
		},
	}))
	require.NoError(t, tb.AddConcept(ctx, termbase.Concept{
		ID:     appID,
		Source: termbase.TermSourceTerminology,
		Terms: []termbase.Term{
			{Text: "app", Locale: "en", Status: model.TermApproved},
			{Text: "application", Locale: "fr", Status: model.TermPreferred},
		},
	}))
	return useID, appID
}

// TestTermAwareShipPredicateUnification is the unification-agreement proof: over
// ONE set of governed (reviewed) fr targets — one using a forbidden term
// (PRESENCE), one missing a mandated rendering (ABSENCE), one clean — the shared
// term predicate, the dashboard ship-state pass, the on-brand-rate derivation,
// the bulk approve-passing predicate, and the RV-E concept re-check oracle all
// AGREE on which blocks are non-compliant. They can no longer disagree because
// they all call blockTermCompliant.
func TestTermAwareShipPredicateUnification(t *testing.T) {
	s, wsID, _ := newRecheckHarness(t)
	ctx := context.Background()

	bBad := reviewedBlock("bad", "Use the app", "Il faut utiliser l'application") // forbidden "utiliser"
	bMiss := reviewedBlock("miss", "Open the app", "Ouvrir le truc")              // missing mandated "application"
	bOK := reviewedBlock("ok", "Close the app", "Fermer l'application")           // clean
	projID, ids := seedGovernedProject(t, s, wsID, []*model.Block{bBad, bMiss, bOK})

	tb, err := s.wsStores.getTB("rc")
	require.NoError(t, err)
	useID, appID := seedTermUnificationConcepts(t, tb)

	proj, err := s.ContentStore.GetProject(ctx, projID)
	require.NoError(t, err)
	gate := s.resolveTermGate(ctx, proj, "main", wsID)
	require.NotNil(t, gate, "a workspace termbase with concepts yields a live gate")

	badID, missID, okID := ids["Use the app"], ids["Open the app"], ids["Close the app"]
	badBlock := storedBlockByID(t, s, projID, badID)
	missBlock := storedBlockByID(t, s, projID, missID)
	okBlock := storedBlockByID(t, s, projID, okID)

	// (1) Shared predicate.
	assert.False(t, gate.compliant(ctx, badBlock, "fr"), "predicate: forbidden-term target is non-compliant")
	assert.False(t, gate.compliant(ctx, missBlock, "fr"), "predicate: missing-mandated-term target is non-compliant")
	assert.True(t, gate.compliant(ctx, okBlock, "fr"), "predicate: clean target is compliant")

	// (2) Dashboard ship-state + on-brand pass agrees: both violators fail the
	// ship gate (pending, not governed/ai_shippable) and count against on-brand.
	stats, err := editorGetDashboardStats(ctx, s.ContentStore, proj, "main")
	require.NoError(t, err)
	require.NoError(t, applyShipStates(ctx, s.ContentStore, s.BrandStore, projID, "main", gate, stats))
	fr := localeByCode(t, stats.LocaleStats, "fr")
	assert.Equal(t, platstore.ShipStatePending, fr.ShipState, "ship: term violations block the ship gate")
	assert.Equal(t, 2, fr.FailingChecks, "ship: exactly the two term-violating blocks fail")
	assertOnBrand(t, fr, 1, 1.0/3.0, platstore.OnBrandBasisChecksTerms)

	// (3) Bulk approve-passing predicate agrees.
	assert.False(t, blockOnBrandAndPassing(ctx, badBlock, "fr", nil, gate), "bulk: forbidden-term block excluded")
	assert.False(t, blockOnBrandAndPassing(ctx, missBlock, "fr", nil, gate), "bulk: missing-mandated block excluded")
	assert.True(t, blockOnBrandAndPassing(ctx, okBlock, "fr", nil, gate), "bulk: clean block kept")

	// (4) RV-E oracle agrees: the concept events demote exactly the same two
	// blocks (PRESENCE via c-use, ABSENCE via c-app) and leave the clean one.
	publishConcept := func(cid string) {
		s.EventBus.Publish(platev.Event{
			ID:          id.New(),
			Type:        knowledge.EventConceptTermStatusChanged,
			Source:      "knowledge",
			WorkspaceID: wsID,
			Data:        map[string]string{"concept_id": cid},
			Timestamp:   time.Now().UTC(),
		})
	}
	publishConcept(useID)
	require.Eventually(t, func() bool {
		return frStatus(t, s, projID, badID) == model.TargetStatusDraft
	}, 20*time.Second, 50*time.Millisecond, "RV-E demotes the forbidden-term target")
	publishConcept(appID)
	require.Eventually(t, func() bool {
		return frStatus(t, s, projID, missID) == model.TargetStatusDraft
	}, 20*time.Second, 50*time.Millisecond, "RV-E demotes the missing-mandated target")

	assert.Equal(t, model.TargetStatusReviewed, frStatus(t, s, projID, okID),
		"RV-E leaves the clean target alone — the same block ship/on-brand/bulk kept")
}

// TestApprovePassingExcludesTermViolations drives the real bulk approve-passing
// endpoint: over three pending fr drafts — one forbidden-term, one
// missing-mandated, one clean — it approves ONLY the term-compliant draft and
// leaves both violators pending for a person.
func TestApprovePassingExcludesTermViolations(t *testing.T) {
	s, wsID, ownerID := newRecheckHarness(t)

	bBad := translatedFrBlock("bad", "Use the app", "Il faut utiliser l'application")
	bMiss := translatedFrBlock("miss", "Open the app", "Ouvrir le truc")
	bOK := translatedFrBlock("ok", "Close the app", "Fermer l'application")
	projID, ids := seedGovernedProject(t, s, wsID, []*model.Block{bBad, bMiss, bOK})

	tb, err := s.wsStores.getTB("rc")
	require.NoError(t, err)
	seedTermUnificationConcepts(t, tb)

	_, resp := callApprovePassing(t, s, wsID, projID, ownerID, `{"locales":["fr"]}`)
	assert.Equal(t, 1, resp.Approved, "only the term-compliant draft is auto-approved")
	assert.Equal(t, 2, resp.Skipped, "both term-violating drafts are excluded")
	assert.Equal(t, 2, resp.RemainingPending, "the two excluded drafts stay pending")

	assert.Equal(t, model.TargetStatusReviewed, frStatus(t, s, projID, ids["Close the app"]),
		"the clean draft is promoted to reviewed")
	assert.Less(t, frStatus(t, s, projID, ids["Use the app"]).Rank(), model.TargetStatusReviewed.Rank(),
		"the forbidden-term draft stays below reviewed")
	assert.Less(t, frStatus(t, s, projID, ids["Open the app"]).Rank(), model.TargetStatusReviewed.Rank(),
		"the missing-mandated draft stays below reviewed")
}

// TestResolveTermGateNoTermbaseNoOp proves the byte-stable no-op: a project with
// no termbase concepts and no bound brand profile derives ship/on-brand numbers
// identical to the pre-term behavior (checks-only basis, no spurious failures).
func TestResolveTermGateNoTermbaseNoOp(t *testing.T) {
	s, wsID, _ := newRecheckHarness(t)
	ctx := context.Background()

	b := reviewedBlock("b", "Hello", "Bonjour")
	projID, _ := seedGovernedProject(t, s, wsID, []*model.Block{b})

	proj, err := s.ContentStore.GetProject(ctx, projID)
	require.NoError(t, err)

	// The gate resolves with no termbase concepts and no bound profile → its term
	// half is a no-op (compliant everywhere, no active term governance).
	gate := s.resolveTermGate(ctx, proj, "main", wsID)
	assert.True(t, gate.compliant(ctx, b, "fr"), "no governance → compliant")
	assert.False(t, gate.active(ctx, "fr"), "no governance → terms do not inform the basis")

	// Deriving with the gate matches deriving with a nil gate exactly.
	withGate, err := editorGetDashboardStats(ctx, s.ContentStore, proj, "main")
	require.NoError(t, err)
	require.NoError(t, applyShipStates(ctx, s.ContentStore, s.BrandStore, projID, "main", gate, withGate))

	withoutGate, err := editorGetDashboardStats(ctx, s.ContentStore, proj, "main")
	require.NoError(t, err)
	require.NoError(t, applyShipStates(ctx, s.ContentStore, s.BrandStore, projID, "main", nil, withoutGate))

	fr := localeByCode(t, withGate.LocaleStats, "fr")
	assert.Equal(t, platstore.ShipStateGoverned, fr.ShipState, "a clean approved locale stays governed")
	assert.Equal(t, 0, fr.FailingChecks)
	assertOnBrand(t, fr, 1, 1.0, platstore.OnBrandBasisChecks)
	assert.Equal(t, localeByCode(t, withoutGate.LocaleStats, "fr"), fr, "gate vs nil-gate derive identically")
}

// storedBlockByID reads one stored block back from the content store.
func storedBlockByID(t *testing.T, s *Server, projID, blockID string) *model.Block {
	t.Helper()
	sb, err := s.ContentStore.GetBlock(context.Background(), projID, "main", blockID)
	require.NoError(t, err)
	return sb.Block
}

// translatedFrBlock builds a translatable block whose fr target is at translated
// status — a pending-review draft the bulk approve-passing pass considers. Each
// block carries a distinct id so the three fixtures do not collide on store.
func translatedFrBlock(id, source, frTarget string) *model.Block {
	b := &model.Block{ID: id, Translatable: true}
	b.SetSourceText(source)
	b.SetTargetText("fr", frTarget)
	b.Target("fr").Status = model.TargetStatusTranslated
	return b
}
