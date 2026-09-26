package server

import (
	"context"
	"testing"
	"time"

	platev "github.com/neokapi/neokapi/bowrain/core/event"
	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/bowrain/knowledge"
	"github.com/neokapi/neokapi/core/id"
	"github.com/neokapi/neokapi/core/model"
	coreprofile "github.com/neokapi/neokapi/core/profile"
	"github.com/neokapi/neokapi/terms"
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

// TestBlockTermCompliance_Directions unit-tests the shared predicate directly:
// both violation directions (forbidden/competitor PRESENCE from the terms store and
// from the brand vocabulary; mandated-rendering ABSENCE), the compliant case, a
// governed target with no text, which is unchecked, and the two ways terminology
// governs nothing: no terms and no profile, and a profile with no block rule.
func TestBlockTermCompliance_Directions(t *testing.T) {
	ctx := context.Background()
	tb := terms.NewInMemoryStore()
	require.NoError(t, tb.AddConcept(ctx, terms.Concept{
		ID:     "c-use",
		Source: terms.TermSourceTerminology,
		Terms: []terms.Term{
			{Text: "utiliser", Locale: "fr", Status: model.TermForbidden},
			{Text: "employer", Locale: "fr", Status: model.TermPreferred},
		},
	}))
	require.NoError(t, tb.AddConcept(ctx, terms.Concept{
		ID:     "c-app",
		Source: terms.TermSourceTerminology,
		Terms: []terms.Term{
			{Text: "app", Locale: "en", Status: model.TermApproved},
			{Text: "application", Locale: "fr", Status: model.TermPreferred},
		},
	}))

	violation := platstore.TermComplianceViolation
	compliant := platstore.TermComplianceCompliant
	unchecked := platstore.TermComplianceUnchecked
	notGoverned := platstore.TermComplianceNotGoverned

	// PRESENCE (terms): the target uses the forbidden "utiliser".
	assert.Equal(t, violation, blockTermCompliance(ctx, mkFrBlock("Use it", "Il faut utiliser ceci"), "en", "fr", tb, nil),
		"a forbidden terms term in the target is a violation")
	// ABSENCE (terms): the source uses "app" but the target omits "application".
	assert.Equal(t, violation, blockTermCompliance(ctx, mkFrBlock("Open the app", "Ouvrir le truc"), "en", "fr", tb, nil),
		"a missing mandated rendering is a violation")
	// Compliant: uses the mandated rendering, no forbidden term.
	assert.Equal(t, compliant, blockTermCompliance(ctx, mkFrBlock("Close the app", "Fermer l'application"), "en", "fr", tb, nil),
		"a target that uses the mandated rendering and no forbidden term is compliant")
	// An untranslated target holds nothing to check.
	assert.Equal(t, unchecked, blockTermCompliance(ctx, mkFrBlock("Open the app", ""), "en", "fr", tb, nil),
		"an empty target is not checked, so it is not compliant either")
	// No terms and no profile: terminology does not govern the locale, even over
	// a would-be violation.
	assert.Equal(t, notGoverned, blockTermCompliance(ctx, mkFrBlock("Use it", "Il faut utiliser ceci"), "en", "fr", nil, nil),
		"with no terms and no profile the locale is not governed")
	// A profile holding only a document-scope rule has nothing to hold a block to.
	docOnly := &coreprofile.VoiceProfile{ID: "d", Style: coreprofile.StyleRules{
		RequiredPatterns: []coreprofile.Pattern{{Regex: `©`}},
	}}
	assert.Equal(t, notGoverned, blockTermCompliance(ctx, mkFrBlock("Affordable", "cheap stuff"), "en", "fr", nil, docOnly),
		"a profile with no block rule governs nothing")

	// PRESENCE (brand vocabulary): a forbidden brand rule matched in the target.
	profile := (&coreprofile.VoiceProfile{ID: "p"}).Carry("voice file", []coreprofile.TermRule{{Term: "cheap"}})
	assert.Equal(t, violation, blockTermCompliance(ctx, mkFrBlock("Affordable", "cheap stuff"), "en", "fr", nil, profile),
		"a forbidden brand-vocabulary term in the target is a violation")
	assert.Equal(t, compliant, blockTermCompliance(ctx, mkFrBlock("Affordable", "budget-friendly"), "en", "fr", nil, profile),
		"a target checked against the profile with no forbidden brand term is compliant")
}

// seedTermUnificationConcepts adds the two concepts the unification tests share:
// c-use marks fr "utiliser" forbidden (PRESENCE) and c-app mandates fr
// "application" for source "app" (ABSENCE).
func seedTermUnificationConcepts(t *testing.T, tb terms.Store) (useID, appID string) {
	t.Helper()
	ctx := context.Background()
	useID, appID = id.New(), id.New()
	require.NoError(t, tb.AddConcept(ctx, terms.Concept{
		ID:     useID,
		Source: terms.TermSourceTerminology,
		Terms: []terms.Term{
			{Text: "utiliser", Locale: "fr", Status: model.TermForbidden},
			{Text: "employer", Locale: "fr", Status: model.TermPreferred},
		},
	}))
	require.NoError(t, tb.AddConcept(ctx, terms.Concept{
		ID:     appID,
		Source: terms.TermSourceTerminology,
		Terms: []terms.Term{
			{Text: "app", Locale: "en", Status: model.TermApproved},
			{Text: "application", Locale: "fr", Status: model.TermPreferred},
		},
	}))
	return useID, appID
}

// TestTermAwareShipPredicateUnification is the unification-agreement proof: over
// ONE set of governed (reviewed) fr targets — one using a forbidden term
// (PRESENCE), one missing a mandated rendering (ABSENCE), one clean — the shared
// term predicate, the dashboard ship-state pass, the compliant-rate derivation,
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

	tb, err := s.wsStores.getTerms("rc")
	require.NoError(t, err)
	useID, appID := seedTermUnificationConcepts(t, tb)

	proj, err := s.ContentStore.GetProject(ctx, projID)
	require.NoError(t, err)
	gate := s.resolveTermGate(ctx, proj, "main", wsID)
	require.NotNil(t, gate, "a workspace terms with concepts yields a live gate")

	badID, missID, okID := ids["Use the app"], ids["Open the app"], ids["Close the app"]
	badBlock := storedBlockByID(t, s, projID, badID)
	missBlock := storedBlockByID(t, s, projID, missID)
	okBlock := storedBlockByID(t, s, projID, okID)

	// (1) Shared predicate.
	assert.Equal(t, platstore.TermComplianceViolation, gate.compliance(ctx, badBlock, "fr"),
		"predicate: forbidden-term target is a violation")
	assert.Equal(t, platstore.TermComplianceViolation, gate.compliance(ctx, missBlock, "fr"),
		"predicate: missing-mandated-term target is a violation")
	assert.Equal(t, platstore.TermComplianceCompliant, gate.compliance(ctx, okBlock, "fr"),
		"predicate: clean target is compliant")

	// (2) Dashboard ship-state + compliant pass agrees: both violators fail the
	// ship gate (pending, not governed/ai_shippable) and count against compliant.
	stats, err := editorGetDashboardStats(ctx, s.ContentStore, proj, "main")
	require.NoError(t, err)
	require.NoError(t, applyShipStates(ctx, s.ContentStore, s.VoiceStore, projID, "main", gate, stats))
	fr := localeByCode(t, stats.LocaleStats, "fr")
	assert.Equal(t, platstore.ShipStatePending, fr.ShipState, "ship: term violations block the ship gate")
	assert.Equal(t, 2, fr.FailingChecks, "ship: exactly the two term-violating blocks fail")
	assertCompliant(t, fr, 1, 0, 0, 1.0/3.0, platstore.ComplianceBasisChecksTerms)

	// (3) Bulk approve-passing predicate agrees.
	assert.False(t, blockCompliantAndPassing(ctx, badBlock, "fr", nil, gate), "bulk: forbidden-term block excluded")
	assert.False(t, blockCompliantAndPassing(ctx, missBlock, "fr", nil, gate), "bulk: missing-mandated block excluded")
	assert.True(t, blockCompliantAndPassing(ctx, okBlock, "fr", nil, gate), "bulk: clean block kept")

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
		"RV-E leaves the clean target alone — the same block ship/compliant/bulk kept")
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

	tb, err := s.wsStores.getTerms("rc")
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

// TestResolveTermGateNoTerms_NotGoverned: a project with no terms concepts and
// no bound voice profile has nothing governing its language beyond the checks.
// Its gate reports every target not governed, the locale counts its clean block
// as not governed and derives no rate, and deriving with that gate matches
// deriving with a nil gate.
func TestResolveTermGateNoTerms_NotGoverned(t *testing.T) {
	s, wsID, _ := newRecheckHarness(t)
	ctx := context.Background()

	b := reviewedBlock("b", "Hello", "Bonjour")
	projID, _ := seedGovernedProject(t, s, wsID, []*model.Block{b})

	proj, err := s.ContentStore.GetProject(ctx, projID)
	require.NoError(t, err)

	// The gate resolves with no terms concepts and no bound profile, so nothing
	// governs a target beyond the checks.
	gate := s.resolveTermGate(ctx, proj, "main", wsID)
	assert.Equal(t, platstore.TermComplianceNotGoverned, gate.compliance(ctx, b, "fr"),
		"no terms → not governed, never compliant")
	assert.False(t, gate.termsGoverned(ctx, "fr"), "no terms → terminology is not in the basis")
	assert.False(t, gate.voiceGoverned(ctx, "fr"), "no bound profile → voice is not in the basis")

	// Deriving with the gate matches deriving with a nil gate exactly.
	withGate, err := editorGetDashboardStats(ctx, s.ContentStore, proj, "main")
	require.NoError(t, err)
	require.NoError(t, applyShipStates(ctx, s.ContentStore, s.VoiceStore, projID, "main", gate, withGate))

	withoutGate, err := editorGetDashboardStats(ctx, s.ContentStore, proj, "main")
	require.NoError(t, err)
	require.NoError(t, applyShipStates(ctx, s.ContentStore, s.VoiceStore, projID, "main", nil, withoutGate))

	fr := localeByCode(t, withGate.LocaleStats, "fr")
	assert.Equal(t, platstore.ShipStateApproved, fr.ShipState, "a clean approved locale nothing governs is approved, not governed")
	assert.Equal(t, 0, fr.FailingChecks)
	assertNotGoverned(t, fr, 1, platstore.ComplianceBasisChecks)
	assert.Equal(t, localeByCode(t, withoutGate.LocaleStats, "fr"), fr, "gate vs nil-gate derive identically")
}

// TestTermGate_GovernsPerLocale pins terminology governance to the locale. A
// workspace holding terms governs only the languages a concept answers for
// (terms.RuleForConcept), so a target in any other language is not
// governed, however much the workspace holds. A do-not-translate concept
// answers for every language.
func TestTermGate_GovernsPerLocale(t *testing.T) {
	ctx := context.Background()
	tb := terms.NewInMemoryStore()
	require.NoError(t, tb.AddConcept(ctx, terms.Concept{
		ID:     "c-use",
		Source: terms.TermSourceTerminology,
		Terms: []terms.Term{
			{Text: "use", Locale: "en", Status: model.TermApproved},
			{Text: "utiliser", Locale: "ja", Status: model.TermForbidden},
		},
	}))
	require.NoError(t, tb.AddConcept(ctx, terms.Concept{
		ID:     "c-app",
		Source: terms.TermSourceTerminology,
		Terms: []terms.Term{
			{Text: "app", Locale: "en", Status: model.TermApproved},
			{Text: "application", Locale: "fr", Status: model.TermPreferred},
		},
	}))
	gate := newTermGate("en", tb, "per-locale", nil)

	assert.True(t, gate.termsGoverned(ctx, "fr"), "a concept with a fr rendering governs fr")
	assert.False(t, gate.termsGoverned(ctx, "ja"), "a forbidden term alone gives ja no term to use")
	assert.Equal(t, platstore.TermComplianceViolation,
		gate.compliance(ctx, mkFrBlock("Open the app", "Ouvrir le truc"), "fr"),
		"the fr target omits the mandated rendering")
	jaBlock := &model.Block{Translatable: true}
	jaBlock.SetSourceText("Use it")
	jaBlock.SetTargetText("ja", "utiliser")
	assert.Equal(t, platstore.TermComplianceNotGoverned,
		gate.compliance(ctx, jaBlock, "ja"),
		"a language no concept answers for is not governed, even over a term marked forbidden in it")

	require.NoError(t, tb.AddConcept(ctx, terms.Concept{
		ID:             "c-kapi",
		Source:         terms.TermSourceTerminology,
		DoNotTranslate: true,
		Terms:          []terms.Term{{Text: "kapi", Locale: "en", Status: model.TermPreferred}},
	}))
	gate = newTermGate("en", tb, "per-locale-dnt", nil)
	assert.True(t, gate.termsGoverned(ctx, "ja"), "a do-not-translate concept governs every language")

	kept := &model.Block{Translatable: true}
	kept.SetSourceText("Open kapi")
	kept.SetTargetText("ja", "kapi を開く")
	assert.Equal(t, platstore.TermComplianceCompliant, gate.compliance(ctx, kept, "ja"),
		"a target that keeps the do-not-translate term verbatim is compliant")
	translated := &model.Block{Translatable: true}
	translated.SetSourceText("Open kapi")
	translated.SetTargetText("ja", "カピを開く")
	assert.Equal(t, platstore.TermComplianceViolation, gate.compliance(ctx, translated, "ja"),
		"a target that translated the do-not-translate term is a violation")
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

// useInMemoryTerms gives a test server in-memory workspace terms stores, one per
// slug, for a harness with no database behind its workspace stores.
func useInMemoryTerms(s *Server) {
	s.wsStores.termsFactory = func() terms.Store {
		return &testTermStore{terms.NewInMemoryStore()}
	}
}

// governingConcept governs fr and de and no other language, with renderings no
// fixture's source uses, so a target it governs is checked and found compliant.
func governingConcept() terms.Concept {
	return terms.Concept{
		ID:     "c-synergy",
		Source: terms.TermSourceTerminology,
		Terms: []terms.Term{
			{Text: "synergy", Locale: "en", Status: model.TermApproved},
			{Text: "synergie", Locale: "fr", Status: model.TermPreferred},
			{Text: "Synergie", Locale: "de", Status: model.TermPreferred},
		},
	}
}

// seedCheckedTerminology binds terms governing fr to a workspace, so every
// pending fr target's terminology is checked and found compliant. A test about
// another bar calls it, so that bar is the one its result turns on.
func seedCheckedTerminology(t *testing.T, s *Server, slug string) {
	t.Helper()
	tb, err := s.wsStores.getTerms(slug)
	require.NoError(t, err)
	require.NoError(t, tb.AddConcept(context.Background(), governingConcept()))
}

// checkedTermsGate is a term gate over a snapshot of governingConcept, so
// terminology governs fr and de and no target violates it. profile, when given,
// is the voice profile every locale resolves to.
func checkedTermsGate(t *testing.T, profile *coreprofile.VoiceProfile) *termGate {
	t.Helper()
	tb := terms.NewInMemoryStore()
	require.NoError(t, tb.AddConcept(context.Background(), governingConcept()))
	var resolve func(context.Context, model.LocaleID) *coreprofile.VoiceProfile
	if profile != nil {
		resolve = func(context.Context, model.LocaleID) *coreprofile.VoiceProfile { return profile }
	}
	return newTermGate("en", tb, "governing-concept", resolve)
}
