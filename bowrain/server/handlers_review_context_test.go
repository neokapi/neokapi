package server

import (
	"context"
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
	"github.com/neokapi/neokapi/core/venue"
)

// These cases pin what the endpoint hands a surface, layer by layer, and what
// it hands a unit that has none of it, because an empty state a surface invents
// is how "no matches" becomes a blank panel.

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
	// unit with nothing resolved — so the status is asserted here rather than
	// left to surface as a nil three assertions later.
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	return rec, decodeJSON[reviewContextResponse](t, rec)
}

// TestReviewContext_GathersEveryLayer walks one governed unit and asserts each
// of the five layers arrives: where it sits and what governs it, what surrounds
// it, what the corpus and the ledger already said about it, what the scoring
// pass found in it, and how its target was produced.
func TestReviewContext_GathersEveryLayer(t *testing.T) {
	s, wsID, _ := newRecheckHarness(t)
	ctx := context.Background()

	// Three pending drafts, each stamped with how it was produced — provenance
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

	// Point.
	require.NotNil(t, got.VoiceProfile, "the profile bound at this point")
	assert.Equal(t, "Bowrain Voice", got.VoiceProfile.Name)
	assert.Equal(t, 90, got.VoiceProfile.ComplianceBar)
	assert.Contains(t, got.VoiceProfile.Guidance, "Say what the product does")
	require.Len(t, got.VoiceProfile.TermRules, 1)
	assert.Equal(t, "leverage", got.VoiceProfile.TermRules[0].Term)
	assert.NotEmpty(t, got.Terms, "the terms the source matches")

	// Neighbourhood: the units either side, as run sequences.
	require.NotNil(t, got.Previous)
	require.NotNil(t, got.Next)
	assert.Equal(t, stored[0].Block.ID, got.Previous.BlockID, "the nearest predecessor")
	assert.Equal(t, stored[2].Block.ID, got.Next.BlockID, "the nearest successor")
	assert.NotEmpty(t, got.Previous.SourceRuns, "the neighbour travels as runs, not as text")

	// History.
	require.NotNil(t, got.Decision)
	assert.Equal(t, "rejected", got.Decision.State)
	assert.Equal(t, "owner@rc.test", got.Decision.By)
	assert.Equal(t, "2026-09-01T11:00:00Z", got.Decision.At)
	assert.Equal(t, "Reads as machine output", got.Decision.Note)
	require.Len(t, got.Notes, 1)
	assert.Equal(t, "Check with legal", got.Notes[0].Text)

	// Judgement: the findings, not only the number.
	require.Len(t, got.VoiceFindings, 1)
	assert.Equal(t, "Uses a forbidden term", got.VoiceFindings[0].Message)
	assert.Equal(t, "employer", got.VoiceFindings[0].Suggestion)
	assert.Equal(t, "utiliser", got.VoiceFindings[0].OriginalText)
	require.NotNil(t, got.VoiceScore)
	assert.Equal(t, 62, *got.VoiceScore)
	require.NotNil(t, got.VoiceBar)
	assert.Equal(t, 90, *got.VoiceBar)

	// Provenance.
	require.NotNil(t, got.Origin)
	assert.Equal(t, "ai", got.Origin.Kind)
	assert.Equal(t, "claude", got.Origin.Engine)
}

// TestReviewContext_EmptyLayersStayEmpty pins the honest answer for a unit with
// no governance, no history and no score: 200 with empty lists and absent
// pointers. A surface that renders this must draw an empty state, and a payload
// that invented a zero-valued profile or a blank decision would defeat it.
func TestReviewContext_EmptyLayersStayEmpty(t *testing.T) {
	s, wsID, _ := newRecheckHarness(t)

	only := pendingFrBlock("only", "Hello", "Bonjour")
	projID, ids := seedGovernedProject(t, s, wsID, []*model.Block{only})

	_, got := getReviewContext(t, s, wsID, projID, ids["Hello"], "fr")

	assert.Nil(t, got.VoiceProfile, "no profile is bound")
	assert.Empty(t, got.Terms)
	assert.NotNil(t, got.Terms, "an empty list, never null")
	assert.Nil(t, got.Previous, "the only unit of an item has no neighbours")
	assert.Nil(t, got.Next)
	assert.Nil(t, got.MemoryMatch)
	assert.Nil(t, got.Decision)
	assert.Empty(t, got.Notes)
	assert.NotNil(t, got.Notes)
	assert.Empty(t, got.VoiceFindings)
	assert.NotNil(t, got.VoiceFindings)
	assert.Nil(t, got.VoiceScore)
	assert.Nil(t, got.VoiceBar)
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
	require.NotNil(t, asDrafted.Origin)
	require.Equal(t, "ai", asDrafted.Origin.Kind, "the fixture starts as an AI draft")

	require.NoError(t, editorUpdateBlockTarget(ctx, s.ContentStore, projID, "main", bid,
		UpdateBlockTargetRequest{TargetLocale: "fr", Text: "Ouvre l'application"}))

	_, edited := getReviewContext(t, s, wsID, projID, bid, "fr")
	require.NotNil(t, edited.Origin)
	assert.Equal(t, "human", edited.Origin.Kind)
	assert.Empty(t, edited.Origin.Engine, "the model that drafted it wrote none of this wording")
	assert.Empty(t, edited.Origin.Tool)
	assert.NotEmpty(t, edited.Origin.Timestamp)

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
	require.NotNil(t, rewritten.Origin)
	assert.Equal(t, "human", rewritten.Origin.Kind)
	assert.Empty(t, rewritten.Origin.Engine)
}
