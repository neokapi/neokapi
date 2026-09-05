package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/core/check"
	"github.com/neokapi/neokapi/core/model"
	coreprofile "github.com/neokapi/neokapi/core/profile"
	"github.com/neokapi/neokapi/core/review"
	"github.com/neokapi/neokapi/core/state"
	"github.com/neokapi/neokapi/core/venue"
	"github.com/neokapi/neokapi/memory"
)

// These cases pin what the endpoint hands a surface, layer by layer, and what
// it hands a unit that has none of it, because an empty state a surface invents
// is how "no matches" becomes a blank panel. The shape is the review model
// every client reads (core/review); the last case holds this assembler to the
// host's over one unit.

// getReviewContext calls the handler as the router would.
func getReviewContext(t *testing.T, s *Server, wsID, projID, blockID, locale string) (*httptest.ResponseRecorder, reviewContextResponse) {
	t.Helper()
	e := s.GetEcho()
	r := httptest.NewRequest(http.MethodGet, "/?target_locale="+locale, nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(r, rec)
	c.SetParamNames("ws", "id", "ref", "bid")
	c.SetParamValues("rc", projID, "main", blockID)
	c.Set("workspace_id", wsID)
	c.Set("project_permissions", platauth.PermAll)
	require.NoError(t, s.HandleGetReviewContext(c))
	// A refusal decodes into a zero-valued response, which reads exactly like a
	// unit with nothing resolved, so the status is asserted here rather than
	// left to surface as a nil three assertions later.
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	return rec, decodeJSON[reviewContextResponse](t, rec)
}

// TestReviewContext_GathersEveryLayer walks one governed unit and asserts each
// of the five layers arrives: where it sits and what governs it, what surrounds
// it, what the ledger already said about it, what the scoring pass found in it,
// and how its target was produced.
func TestReviewContext_GathersEveryLayer(t *testing.T) {
	s, wsID, _ := newRecheckHarness(t)
	ctx := context.Background()

	// Three pending drafts, each stamped with how it was produced: provenance
	// rides on the target, which the blocks payload never carried.
	seeded := []*model.Block{
		pendingFrBlock("a", "Open the app", "Ouvrir l'application"),
		pendingFrBlock("b", "Use the app", "Il faut utiliser l'application"),
		pendingFrBlock("c", "Close the app", "Fermer l'application"),
	}
	for _, b := range seeded {
		b.Target("fr").Origin = model.Origin{
			Kind: "ai", Engine: "claude", Tool: "translate", Timestamp: "2026-09-01T10:00:00Z",
		}
	}
	projID, _ := seedGovernedProject(t, s, wsID, seeded)

	// The store mints its own ids and the neighbourhood cursor orders by them,
	// so the unit with a neighbour on each side is the middle of the STORED
	// order, not of the order they were written in.
	stored, err := s.ContentStore.GetBlocks(ctx, platstore.BlockQuery{
		ProjectID: projID, Stream: "main", ItemName: "greetings.txt",
	})
	require.NoError(t, err)
	require.Len(t, stored, 3)
	middleID := stored[1].Block.ID

	tb, terr := s.wsStores.getTerms("rc")
	require.NoError(t, terr)
	seedTermUnificationConcepts(t, tb)

	// The profile bound at this point, with a raised bar and a vocabulary rule.
	profile := &coreprofile.VoiceProfile{
		ID: "p-ctx", Scope: wsID, Name: "Bowrain Voice", MinScore: 90,
		Tone: coreprofile.ToneProfile{Formality: "neutral", Guidelines: "Say what the product does"},
		Vocabulary: coreprofile.VocabularyRules{
			ForbiddenTerms: []coreprofile.TermRule{{Term: "leverage", Replacement: "use", Severity: "major"}},
		},
	}
	require.NoError(t, s.VoiceStore.CreateProfile(ctx, profile))
	proj, err := s.ContentStore.GetProject(ctx, projID)
	require.NoError(t, err)
	proj.Properties = map[string]string{"source_gate": "none", "voice_profile_id": profile.ID}
	require.NoError(t, s.ContentStore.UpdateProject(ctx, proj))

	// The findings the scoring pass persists and no surface read.
	require.NoError(t, s.VoiceStore.StoreScore(ctx, &coreprofile.StoredScore{
		ProjectID: projID, Stream: "main", BlockID: middleID, ProfileID: profile.ID,
		Locale: "fr", Score: 62, CheckedAt: time.Now().UTC(),
		Findings: []coreprofile.VoiceFinding{{
			Category: "compliance", Severity: check.SeverityMajor,
			Message: "Uses a forbidden term", OriginalText: "utiliser", Suggestion: "employer",
		}},
	}))

	// A previous decision, and a note beside it.
	sb := stored[1]
	ds, ok := s.ContentStore.(platstore.DecisionStore)
	require.True(t, ok)
	_, derr := ds.UpsertUnitDecisions(ctx, projID, "main", []venue.UnitDecision{{
		ItemName: sb.ItemName, Unit: sb.SourceID, Variant: "fr", Status: "draft",
		ReviewState: "rejected", DecidedBy: "owner@rc.test", DecidedAt: "2026-09-01T11:00:00Z",
		Note: "Reads as machine output", Updated: "2026-09-01T11:00:00Z",
	}})
	require.NoError(t, derr)
	require.NoError(t, s.ContentStore.AddBlockNote(ctx, projID, "main", middleID, model.BlockNote{
		ID: "n1", BlockID: middleID, Author: "owner@rc.test", Text: "Check with legal",
		CreatedAt: time.Now().UTC(),
	}))

	_, got := getReviewContext(t, s, wsID, projID, middleID, "fr")

	// The unit's own address.
	assert.Equal(t, middleID, got.BlockID)
	assert.Equal(t, "greetings.txt", got.ItemName)
	assert.Equal(t, "fr", got.Locale)

	// Point.
	assert.Equal(t, "greetings.txt", got.Point.Path, "the item is the file the unit sits in")
	assert.Equal(t, "fr", got.Point.Language)
	assert.False(t, got.Point.IsSource)
	require.NotNil(t, got.Point.Voice, "the profile bound at this point")
	assert.Equal(t, "Bowrain Voice", got.Point.Voice.Name)
	assert.Equal(t, "store:p-ctx", got.Point.Voice.Source, "loaded from the voice store, named the way the host names it")
	assert.Contains(t, got.Point.Voice.Guide, "Say what the product does")
	require.Len(t, got.Point.TermRules, 1)
	assert.Equal(t, "leverage", got.Point.TermRules[0].Term)
	assert.Equal(t, 1, got.Point.TermsTotal)
	assert.NotEmpty(t, got.Terms, "the terms the source matches")

	// Neighbourhood: the units either side, as run sequences.
	assert.Equal(t, middleID, got.Neighbourhood.Key)
	assert.Equal(t, review.DefaultWindow, got.Neighbourhood.Window, "the window the translate prompt reads")
	require.Len(t, got.Neighbourhood.Before, 1)
	require.Len(t, got.Neighbourhood.After, 1)
	previous, next := got.Neighbourhood.Before[0], got.Neighbourhood.After[0]
	assert.Equal(t, stored[0].Block.ID, previous.Key, "the nearest predecessor")
	assert.Equal(t, stored[2].Block.ID, next.Key, "the nearest successor")
	assert.NotEmpty(t, previous.Source, "the neighbour travels as runs, not as text")
	// A reviewer reads a neighbour for the wording that was settled on around
	// this unit, so both sides travel. The expectations come from the stored
	// order for the same reason the ids above do.
	assert.Equal(t, stored[0].Block.SourceText(), model.RunsText(previous.Source))
	assert.Equal(t, stored[0].Block.TargetText("fr"), model.RunsText(previous.Target))
	assert.NotEmpty(t, previous.Target, "the neighbour is translated, so it reads as one")
	assert.Equal(t, stored[2].Block.TargetText("fr"), model.RunsText(next.Target))
	assert.Equal(t, "draft", next.Status)

	// History: nothing in the memory yet, so nothing is invented.
	assert.Nil(t, got.History.Match)
	assert.Nil(t, got.History.Prior)
	require.Len(t, got.Notes, 1)
	assert.Equal(t, "Check with legal", got.Notes[0].Text)

	// Judgement: the findings behind the score, not only the number.
	require.Len(t, got.Judgement.Findings, 1)
	assert.Equal(t, "Uses a forbidden term", got.Judgement.Findings[0].Message)
	assert.Equal(t, "employer", got.Judgement.Findings[0].Suggestion)
	assert.Equal(t, "utiliser", got.Judgement.Findings[0].OriginalText)
	require.NotNil(t, got.VoiceScore)
	assert.Equal(t, 62, *got.VoiceScore)
	require.NotNil(t, got.VoiceBar)
	assert.Equal(t, 90, *got.VoiceBar)

	// Provenance: the decision in force, and how the target was produced.
	assert.Equal(t, "rejected", got.Provenance.ReviewState)
	assert.Equal(t, "draft", got.Provenance.Status)
	assert.Equal(t, "owner@rc.test", got.Provenance.By)
	assert.Equal(t, "2026-09-01T11:00:00Z", got.Provenance.At)
	assert.Equal(t, "Reads as machine output", got.Provenance.Note)
	assert.False(t, got.Provenance.Stale)
	require.NotNil(t, got.Provenance.Origin)
	assert.Equal(t, "ai", got.Provenance.Origin.Kind)
	assert.Equal(t, "claude", got.Provenance.Origin.Engine)
}

// TestReviewContext_EmptyLayersStayEmpty pins the honest answer for a unit with
// no governance, no history and no score: 200 with empty lists and absent
// pointers. A surface that renders this must draw an empty state, and a payload
// that invented a zero-valued profile or a blank decision would defeat it.
func TestReviewContext_EmptyLayersStayEmpty(t *testing.T) {
	s, wsID, _ := newRecheckHarness(t)

	only := pendingFrBlock("only", "Hello", "Bonjour")
	projID, ids := seedGovernedProject(t, s, wsID, []*model.Block{only})

	rec, got := getReviewContext(t, s, wsID, projID, ids["Hello"], "fr")

	assert.Nil(t, got.Point.Voice, "no profile is bound")
	assert.Empty(t, got.Point.TermRules)
	assert.Empty(t, got.Terms)
	assert.NotNil(t, got.Terms, "an empty list, never null")
	assert.Empty(t, got.Neighbourhood.Before, "the only unit of an item has no neighbours")
	assert.Empty(t, got.Neighbourhood.After)
	assert.Equal(t, review.DefaultWindow, got.Neighbourhood.Window, "a short list means the document ended")
	assert.Nil(t, got.History.Match)
	assert.Nil(t, got.History.Prior)
	assert.Empty(t, got.Provenance.ReviewState)
	assert.Empty(t, got.Provenance.By)
	assert.Empty(t, got.Notes)
	assert.NotNil(t, got.Notes)
	assert.Empty(t, got.Judgement.Findings)
	assert.Nil(t, got.VoiceScore)
	assert.Nil(t, got.VoiceBar)

	// The wire spells the five layers the way every client reads them.
	var layers map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &layers))
	for _, layer := range []string{"point", "neighbourhood", "history", "judgement", "provenance"} {
		assert.Contains(t, layers, layer)
	}
}

// TestReviewContext_UntranslatedNeighbourTravelsBare: a neighbour nobody has
// translated yet carries its source and no target at all, as the host's
// neighbour does, so the surface draws a blank target cell rather than deciding
// for itself what an empty list means.
func TestReviewContext_UntranslatedNeighbourTravelsBare(t *testing.T) {
	s, wsID, _ := newRecheckHarness(t)

	untranslated := &model.Block{ID: "u", Translatable: true}
	untranslated.SetSourceText("Sign in")
	projID, ids := seedGovernedProject(t, s, wsID, []*model.Block{
		untranslated,
		pendingFrBlock("d", "Sign out", "Se deconnecter"),
	})

	rec, got := getReviewContext(t, s, wsID, projID, ids["Sign out"], "fr")

	// The store mints the ids the neighbourhood cursor orders by, so which side
	// the untranslated unit lands on is the store's business, not the test's.
	var bare *review.Neighbour
	for _, side := range [][]review.Neighbour{got.Neighbourhood.Before, got.Neighbourhood.After} {
		for i := range side {
			if side[i].Key == ids["Sign in"] {
				bare = &side[i]
			}
		}
	}
	require.NotNil(t, bare, "the source-only unit is the neighbour")
	assert.Equal(t, "Sign in", model.RunsText(bare.Source))
	assert.Empty(t, bare.Target, "nothing has been written on the target side")
	assert.Empty(t, bare.Status, "a unit with no target sits on no rung")

	var wire struct {
		Neighbourhood struct {
			Before []map[string]json.RawMessage `json:"before"`
			After  []map[string]json.RawMessage `json:"after"`
		} `json:"neighbourhood"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &wire))
	for _, n := range append(wire.Neighbourhood.Before, wire.Neighbourhood.After...) {
		if string(n["key"]) == `"`+ids["Sign in"]+`"` {
			assert.NotContains(t, n, "target", "the wire leaves the target out, as the host does")
			assert.NotContains(t, n, "status")
		}
	}
}

// TestReviewContext_NeedsATargetLocale: the context is per (block, locale), so
// a request naming no locale is answered as the bad request it is rather than
// silently gathered for the wrong side.
func TestReviewContext_NeedsATargetLocale(t *testing.T) {
	s, wsID, _ := newRecheckHarness(t)
	only := pendingFrBlock("only", "Hello", "Bonjour")
	projID, ids := seedGovernedProject(t, s, wsID, []*model.Block{only})

	e := s.GetEcho()
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(r, rec)
	c.SetParamNames("ws", "id", "ref", "bid")
	c.SetParamValues("rc", projID, "main", ids["Hello"])
	c.Set("workspace_id", wsID)
	c.Set("project_permissions", platauth.PermAll)
	require.NoError(t, s.HandleGetReviewContext(c))
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

// TestReviewContext_RequiresViewContent: the payload carries source and target
// wording, so it is gated exactly as the rest of the content surfaces are.
func TestReviewContext_RequiresViewContent(t *testing.T) {
	s, wsID, _ := newRecheckHarness(t)
	only := pendingFrBlock("only", "Hello", "Bonjour")
	projID, ids := seedGovernedProject(t, s, wsID, []*model.Block{only})

	e := s.GetEcho()
	r := httptest.NewRequest(http.MethodGet, "/?target_locale=fr", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(r, rec)
	c.SetParamNames("ws", "id", "ref", "bid")
	c.SetParamValues("rc", projID, "main", ids["Hello"])
	c.Set("workspace_id", wsID)
	c.Set("project_permissions", platauth.Permission(0))
	require.Error(t, s.HandleGetReviewContext(c))
	assert.Equal(t, http.StatusForbidden, rec.Code)
}

// TestReviewContext_HandEditReadsAsHuman: a reviewer who rewrites an AI draft
// produced the wording in front of them, and the provenance card is where they
// read that. Both target-writing paths stamp it, because the editor writes
// plain text through one and runs through the other.
func TestReviewContext_HandEditReadsAsHuman(t *testing.T) {
	s, wsID, _ := newRecheckHarness(t)
	ctx := context.Background()

	drafted := pendingFrBlock("h", "Open the app", "Ouvrir l'application")
	drafted.Target("fr").Origin = model.Origin{
		Kind: "ai", Engine: "claude", Tool: "translate", Timestamp: "2026-09-01T10:00:00Z",
	}
	projID, ids := seedGovernedProject(t, s, wsID, []*model.Block{drafted})
	bid := ids["Open the app"]

	_, asDrafted := getReviewContext(t, s, wsID, projID, bid, "fr")
	require.NotNil(t, asDrafted.Provenance.Origin)
	require.Equal(t, "ai", asDrafted.Provenance.Origin.Kind, "the fixture starts as an AI draft")

	require.NoError(t, editorUpdateBlockTarget(ctx, s.ContentStore, projID, "main", bid,
		UpdateBlockTargetRequest{TargetLocale: "fr", Text: "Ouvre l'application"}))

	_, edited := getReviewContext(t, s, wsID, projID, bid, "fr")
	require.NotNil(t, edited.Provenance.Origin)
	assert.Equal(t, "human", edited.Provenance.Origin.Kind)
	assert.Empty(t, edited.Provenance.Origin.Engine, "the model that drafted it wrote none of this wording")
	assert.Empty(t, edited.Provenance.Origin.Tool)
	assert.NotEmpty(t, edited.Provenance.Origin.Timestamp)

	// Back to an AI draft, then rewritten through the run-native path.
	sb, err := s.ContentStore.GetBlock(ctx, projID, "main", bid)
	require.NoError(t, err)
	sb.Block.Target("fr").Origin = model.Origin{Kind: "ai", Engine: "claude"}
	require.NoError(t, s.ContentStore.StoreBlocks(ctx, projID, "main", []*model.Block{sb.Block}))

	require.NoError(t, editorUpdateBlockTargetRuns(ctx, s.ContentStore, projID, "main", bid,
		UpdateBlockTargetRunsRequest{
			TargetLocale: "fr",
			Runs:         []model.Run{{Text: &model.TextRun{Text: "Ouvrez l'application"}}},
		}))

	_, rewritten := getReviewContext(t, s, wsID, projID, bid, "fr")
	require.NotNil(t, rewritten.Provenance.Origin)
	assert.Equal(t, "human", rewritten.Provenance.Origin.Kind)
	assert.Empty(t, rewritten.Provenance.Origin.Engine)
}

// TestReviewContext_ReadsTheSameFactsAsTheHost seeds one unit here and holds
// this assembler to the functions in core/review the host's assembler composes
// (host.AssembleReviewContext, held to them by its own test), over the same
// blocks, the same memory entry and the same decision: the neighbourhood in
// document order with each neighbour's rung, the prior version and whether its
// context still governs, the memory match on one scale, and the decision in
// force with the target's origin. The point is each venue's own (a recipe on
// the host, a workspace here) and the checks each venue runs are its own, so
// those two are pinned by the cases above rather than compared.
func TestReviewContext_ReadsTheSameFactsAsTheHost(t *testing.T) {
	s, wsID, _ := newRecheckHarness(t)
	ctx := context.Background()

	authored := []*model.Block{
		pendingFrBlock("a", "Open the app", "Ouvrir l'application"),
		pendingFrBlock("b", "Reset your password", "Réinitialisez votre mot de passe"),
		pendingFrBlock("c", "Close the app", "Fermer l'application"),
	}
	for _, b := range authored {
		// The chain identity the version chain is keyed on, and the stamp the
		// producer leaves on what it produced.
		b.Unit = "auth." + b.ID
		b.Target("fr").Origin = model.Origin{Kind: "ai", Engine: "claude", ContextFingerprint: "fp-1"}
	}
	authored[0].Target("fr").Status = model.TargetStatusReviewed
	projID, _ := seedGovernedProject(t, s, wsID, authored)

	// The host reads the file in document order; the server orders by the ids
	// it minted. Put the host's blocks in the server's order, so both
	// assemblers are asked about the same middle unit with the same sides.
	stored, err := s.ContentStore.GetBlocks(ctx, platstore.BlockQuery{
		ProjectID: projID, Stream: "main", ItemName: "greetings.txt",
	})
	require.NoError(t, err)
	require.Len(t, stored, 3)
	bySource := map[string]*model.Block{}
	for _, b := range authored {
		bySource[b.SourceText()] = b
	}
	ordered := make([]*model.Block, 0, len(stored))
	for _, sb := range stored {
		ordered = append(ordered, bySource[sb.Block.SourceText()])
	}
	middle := stored[1]

	unit := bySource[middle.Block.SourceText()]

	// The same approved answer in both memories: the previous wording of the
	// middle unit, produced under the context still in force.
	at := time.Date(2026, 8, 1, 9, 0, 0, 0, time.UTC)
	entry := memory.Entry{
		ID: "m-prior", Unit: unit.Unit, HintSrcLang: "en",
		Variants: map[model.LocaleID][]model.Run{
			"en": {model.TextR(unit.SourceText())},
			"fr": {model.TextR(unit.TargetText("fr"))},
		},
		Origins:   []memory.Origin{{Source: "tool", AddedAt: at, ContextFingerprint: "fp-1"}},
		CreatedAt: at, UpdatedAt: at,
	}
	// The same store implementation on both sides, so the two lookups classify
	// a match the same way and the comparison is about the assemblers.
	s.wsStores.memoryFactory = func() memory.Store { return memory.NewInMemoryStore() }
	serverMemory, merr := s.wsStores.getMemory("rc")
	require.NoError(t, merr)
	require.NoError(t, serverMemory.Add(ctx, entry))
	localMemory := memory.NewInMemoryStore()
	require.NoError(t, localMemory.Add(ctx, entry))

	// The same decision on both sides: the server's ledger row, and the state
	// record the host reads.
	ds, ok := s.ContentStore.(platstore.DecisionStore)
	require.True(t, ok)
	_, derr := ds.UpsertUnitDecisions(ctx, projID, "main", []venue.UnitDecision{{
		ItemName: middle.ItemName, Unit: middle.SourceID, Variant: "fr", Status: "reviewed",
		ReviewState: "approved", DecidedBy: "owner@rc.test", DecidedAt: "2026-09-01T11:00:00Z",
		Note: "Matches the approved wording", ContentHash: middle.ContentHash,
		Updated: "2026-09-01T11:00:00Z",
	}})
	require.NoError(t, derr)
	record := &state.UnitState{
		Status: model.TargetStatusReviewed,
		Decision: state.Decision{
			ReviewState: "approved", By: "owner@rc.test", At: "2026-09-01T11:00:00Z",
			Note: "Matches the approved wording",
		},
		ContentHash: state.SourceHash(middle.Block.SourceText()),
	}

	_, got := getReviewContext(t, s, wsID, projID, middle.Block.ID, "fr")

	// Neighbourhood: the same blocks, in the same order, on the same rungs.
	wantHood := review.NeighbourhoodOf(ordered, 1, review.DefaultWindow, "fr")
	assert.Equal(t, wantHood.Window, got.Neighbourhood.Window)
	assert.Equal(t, neighbourFacts(wantHood.Before), neighbourFacts(got.Neighbourhood.Before))
	assert.Equal(t, neighbourFacts(wantHood.After), neighbourFacts(got.Neighbourhood.After))
	assert.Equal(t, stored[0].Block.ID, got.Neighbourhood.Before[0].Key, "keyed by the block the platform addresses")

	// History: one prior version, one score, one scale.
	fingerprint := review.GoverningFingerprint(unit, "fr", record.Origin)
	wantPrior := review.PriorVersionOf(ctx, localMemory, unit, "en", "fr", fingerprint)
	require.NotNil(t, wantPrior)
	assert.Equal(t, wantPrior, got.History.Prior)
	assert.True(t, got.History.Prior.Governed, "produced under the context still in force")
	lookup := &model.Block{ID: "review-lookup", Translatable: true, Source: unit.SourceRuns()}
	matches, lerr := localMemory.Lookup(ctx, lookup, "en", "fr", memory.LookupOptions{MinScore: 0.5, MaxResults: 1})
	require.NoError(t, lerr)
	require.NotEmpty(t, matches)
	wantMatch := review.MatchOf(matches[0], "en", "fr")
	require.NotNil(t, wantMatch)
	require.NotNil(t, got.History.Match)
	assert.Equal(t, wantMatch.Score, got.History.Match.Score)
	assert.Equal(t, wantMatch.Target, got.History.Match.Target)
	assert.Equal(t, wantMatch.Source, got.History.Match.Source)

	// Provenance: the decision in force, and the target's origin.
	assert.Equal(t, review.ProvenanceOf(unit, "fr", record), got.Provenance)
	assert.Equal(t, "reviewed", got.Provenance.Status)
	assert.False(t, got.Provenance.Stale)

	// Point: each venue's own, but the language it answers for is the same.
	assert.Equal(t, "fr", got.Point.Language)
	assert.False(t, got.Point.IsSource)
}

// neighbourFacts reduces a neighbour list to what both assemblers must agree
// on: the wording on both sides and the rung, in order. The keys differ by
// design: the host addresses a block by its stable unit key, the platform by
// the id it minted.
func neighbourFacts(ns []review.Neighbour) [][3]string {
	out := make([][3]string, 0, len(ns))
	for _, n := range ns {
		out = append(out, [3]string{model.RunsText(n.Source), model.RunsText(n.Target), n.Status})
	}
	return out
}
