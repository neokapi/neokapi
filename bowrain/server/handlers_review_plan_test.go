package server

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/terms"
)

func readPlan(t *testing.T, s *Server, wsID, projID string) reviewPlanResponse {
	t.Helper()
	rec := deckCall(t, s, s.HandleGetReviewPlan, wsID, projID, "", "u-reviewer", testReviewer, nil, url.Values{})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	return decodeJSON[reviewPlanResponse](t, rec)
}

func planWrite(t *testing.T, s *Server, h echo.HandlerFunc, wsID, projID, body string, perms platauth.Permission) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(http.MethodPut, "/", strings.NewReader(body))
	r.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(r, rec)
	c.SetParamNames("ws", "id")
	c.SetParamValues("rc", projID)
	c.Set("workspace_id", wsID)
	c.Set("user_id", "u-owner")
	c.Set("project_permissions", perms)
	if err := h(c); err != nil {
		require.ErrorIs(t, err, errAccessDenied)
	}
	return rec
}

func TestReviewPlan_StartsWithTheCoordinates(t *testing.T) {
	s, wsID, _ := newRecheckHarness(t)
	projID, _ := containerProject(t, s, wsID)

	plan := readPlan(t, s, wsID, projID)
	assert.Equal(t, involvementContext, plan.Involvement, "context is the default")
	assert.Equal(t, "en", plan.Coordinates.SourceLanguage)
	assert.Equal(t, []string{"fr"}, plan.Coordinates.TargetLanguages)
	assert.False(t, plan.Coordinates.Confirmed)
	assert.Equal(t, planCoordinates, plan.Next, "nothing comes before the coordinates")

	rec := planWrite(t, s, s.HandleConfirmReviewCoordinates, wsID, projID, "", platauth.PermAll)
	require.Equal(t, http.StatusNoContent, rec.Code, rec.Body.String())
	plan = readPlan(t, s, wsID, projID)
	assert.True(t, plan.Coordinates.Confirmed)
	assert.Equal(t, "u-owner", plan.Coordinates.ConfirmedBy)
	assert.NotEqual(t, planCoordinates, plan.Next)
}

func TestReviewPlan_CountsTheTermsEachLanguageLacks(t *testing.T) {
	s, wsID, _ := newRecheckHarness(t)
	projID, _ := containerProject(t, s, wsID)
	ctx := t.Context()
	tb, err := s.wsStores.getTerms(ctx, "rc")
	require.NoError(t, err)
	for _, c := range []terms.Concept{
		{ID: "c-server", Terms: []terms.Term{
			{Text: "server", Locale: "en", Status: model.TermPreferred},
			{Text: "serveur", Locale: "fr", Status: model.TermPreferred},
		}},
		{ID: "c-workspace", Terms: []terms.Term{{Text: "workspace", Locale: "en", Status: model.TermPreferred}}},
		{ID: "c-bowrain", DoNotTranslate: true, Terms: []terms.Term{{Text: "Bowrain", Locale: "en"}}},
	} {
		require.NoError(t, tb.AddConcept(ctx, c))
	}
	require.Equal(t, http.StatusNoContent,
		planWrite(t, s, s.HandleConfirmReviewCoordinates, wsID, projID, "", platauth.PermAll).Code)

	plan := readPlan(t, s, wsID, projID)
	assert.Equal(t, 3, plan.KeyLanguage.Concepts)
	assert.Equal(t, map[string]int{"fr": 1}, plan.KeyLanguage.Untranslated, "workspace has no French term; Bowrain is kept as written")
	assert.Equal(t, planKeyLanguage, plan.Next)
}

func TestReviewPlan_LanguagesCountTheDeck(t *testing.T) {
	s, wsID, _ := newRecheckHarness(t)
	projID, _ := containerProject(t, s, wsID)
	plan := readPlan(t, s, wsID, projID)
	require.Len(t, plan.Languages, 1)
	fr := plan.Languages[0]
	assert.Equal(t, "fr", fr.Locale)
	assert.Positive(t, fr.Waiting)
	assert.Positive(t, fr.Pages)
}

func TestReviewPlan_InvolvementIsValidatedAndKept(t *testing.T) {
	s, wsID, _ := newRecheckHarness(t)
	projID, _ := containerProject(t, s, wsID)

	rec := planWrite(t, s, s.HandlePutReviewInvolvement, wsID, projID, `{"involvement":"sometimes"}`, platauth.PermAll)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	rec = planWrite(t, s, s.HandlePutReviewInvolvement, wsID, projID, `{"involvement":"interfaces"}`, testReviewer)
	assert.NotEqual(t, http.StatusNoContent, rec.Code, "setting the involvement needs the project settings permission")

	rec = planWrite(t, s, s.HandlePutReviewInvolvement, wsID, projID, `{"involvement":"every"}`, platauth.PermAll)
	require.Equal(t, http.StatusNoContent, rec.Code, rec.Body.String())
	plan := readPlan(t, s, wsID, projID)
	assert.Equal(t, involvementEvery, plan.Involvement)
	for _, l := range plan.Languages {
		assert.Zero(t, l.Settleable, "nothing settles by policy when every translation is reviewed")
	}
}

func TestInterfaceSample_TakesOneInTenPerCollectionAndEveryFinding(t *testing.T) {
	var pages []platstore.ReviewDeckPage
	for i := range 25 {
		pages = append(pages, platstore.ReviewDeckPage{CollectionID: "docs", ItemName: fmt.Sprintf("d%02d", i)})
	}
	pages[7].Findings = 2
	pages = append(pages, platstore.ReviewDeckPage{CollectionID: "cli", ItemName: "c00"})

	var got []string
	for _, p := range interfaceSample(pages) {
		got = append(got, p.ItemName)
	}
	assert.Equal(t, []string{"d00", "d07", "d10", "d20", "c00"}, got,
		"the first of every ten pages of each collection, and every page with a finding")
}

func TestReviewPlan_SettleTakesOneFirstPassThenWaitsForTheKeyLanguage(t *testing.T) {
	s, wsID, _ := newRecheckHarness(t)
	projID, _ := containerProject(t, s, wsID)
	ctx := t.Context()
	tb, err := s.wsStores.getTerms(ctx, "rc")
	require.NoError(t, err)
	require.NoError(t, tb.AddConcept(ctx, terms.Concept{ID: "c-workspace",
		Terms: []terms.Term{{Text: "workspace", Locale: "en", Status: model.TermPreferred}}}))

	plan := readPlan(t, s, wsID, projID)
	assert.Equal(t, settleFirstPass, plan.Policy.Settle, "the key language is open and no pass has run")
	assert.Positive(t, plan.Policy.KeyLanguageOpen)

	rec := planWrite(t, s, s.HandleSettleReviewPlan, wsID, projID, `{}`, platauth.PermAll)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.True(t, decodeJSON[reviewPlanSettleResponse](t, rec).FirstPass)

	plan = readPlan(t, s, wsID, projID)
	assert.Equal(t, settleHeld, plan.Policy.Settle)
	assert.Equal(t, "u-owner", plan.Policy.FirstPassBy)
	rec = planWrite(t, s, s.HandleSettleReviewPlan, wsID, projID, `{}`, platauth.PermAll)
	assert.Equal(t, http.StatusConflict, rec.Code, "a second pass waits for the key language")

	require.NoError(t, tb.AddConcept(ctx, terms.Concept{ID: "c-workspace", Terms: []terms.Term{
		{Text: "workspace", Locale: "en", Status: model.TermPreferred},
		{Text: "espace de travail", Locale: "fr", Status: model.TermPreferred},
	}}))
	plan = readPlan(t, s, wsID, projID)
	assert.Equal(t, settlePolicy, plan.Policy.Settle, "the key language is decided")
	rec = planWrite(t, s, s.HandleSettleReviewPlan, wsID, projID, `{}`, platauth.PermAll)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.False(t, decodeJSON[reviewPlanSettleResponse](t, rec).FirstPass)
}

func TestReviewPlan_SettleRefusesUnderEvery(t *testing.T) {
	s, wsID, _ := newRecheckHarness(t)
	projID, _ := containerProject(t, s, wsID)
	require.Equal(t, http.StatusNoContent,
		planWrite(t, s, s.HandlePutReviewInvolvement, wsID, projID, `{"involvement":"every"}`, platauth.PermAll).Code)
	assert.Equal(t, settleNone, readPlan(t, s, wsID, projID).Policy.Settle)
	rec := planWrite(t, s, s.HandleSettleReviewPlan, wsID, projID, `{}`, platauth.PermAll)
	assert.Equal(t, http.StatusConflict, rec.Code)
}

// The exceptions are grouped by rule: the source's rules summed across the
// items the last project-wide check counted, and each language's rules over
// the translations the deck holds back, the most findings first. A rule's key
// narrows the deck to the translations that break it.
func TestReviewPlan_ExceptionsByRule(t *testing.T) {
	s, wsID, _ := newRecheckHarness(t)
	ctx := t.Context()
	projID, ids := seedGovernedProject(t, s, wsID, []*model.Block{
		pendingFrBlock("b1", "Open the workspace", "Ouvrez l'atelier"),
		pendingFrBlock("b2", "Close the workspace", "Fermez l'atelier"),
		placeholderBlock("b3", "Hello", false),
		pendingFrBlock("b4", "Fine", "Bien"),
	})
	tb, err := s.wsStores.getTerms(ctx, "rc")
	require.NoError(t, err)
	require.NoError(t, tb.AddConcept(ctx, terms.Concept{ID: "c-ws", Terms: []terms.Term{
		{Text: "workspace", Locale: "en", Status: model.TermPreferred},
		{Text: "espace de travail", Locale: "fr", Status: model.TermPreferred},
		{Text: "atelier", Locale: "fr", Status: model.TermForbidden},
	}}))

	tallies, ok := platstore.ItemCheckTallies(s.ContentStore)
	require.True(t, ok)
	voice := platstore.RuleTally{Key: "voice.style:!", Rule: "voice.style", Subject: "!", Message: "Prohibited pattern: exclamation", Count: 2}
	utilize := platstore.RuleTally{Key: "terms.vocabulary:utilize", Rule: "terms.vocabulary", Subject: "utilize", ConceptID: "c-use", Fails: true, Count: 3}
	require.NoError(t, tallies.PutItemCheckTally(ctx, platstore.ItemCheckTally{ProjectID: projID, ItemID: "item-a", Findings: 5, Rules: []platstore.RuleTally{utilize, voice}}))
	utilize.Count = 2
	require.NoError(t, tallies.PutItemCheckTally(ctx, platstore.ItemCheckTally{ProjectID: projID, ItemID: "item-b", Findings: 2, Rules: []platstore.RuleTally{utilize}}))

	// Reading the deck classifies it; the plan counts what is classified.
	readDeck(t, s, wsID, projID, url.Values{"locale": {"fr"}})
	plan := readPlan(t, s, wsID, projID)

	require.NotNil(t, plan.Exceptions.SourceFindings)
	assert.Equal(t, 7, *plan.Exceptions.SourceFindings)
	require.Len(t, plan.Exceptions.ByRule.Source, 2)
	assert.Equal(t, planRuleDTO{Key: "terms.vocabulary:utilize", Rule: "terms.vocabulary", Subject: "utilize", ConceptID: "c-use", Fails: true, Count: 5},
		plan.Exceptions.ByRule.Source[0], "one rule broken in two items counts once")
	assert.Equal(t, "voice.style:!", plan.Exceptions.ByRule.Source[1].Key)

	fr := plan.Exceptions.ByRule.Targets["fr"]
	require.Len(t, fr, 4, "%+v", fr)
	assert.Equal(t, "terms.vocabulary:atelier", fr[0].Key)
	assert.Equal(t, "atelier", fr[0].Subject)
	assert.Equal(t, "c-ws", fr[0].ConceptID)
	assert.Equal(t, 2, fr[0].Count, "the forbidden term breaks two translations")
	assert.True(t, fr[0].Fails)
	assert.NotEmpty(t, fr[0].Message)
	for _, r := range fr[1:] {
		assert.Equal(t, 1, r.Count, "the lost placeholder breaks the checks of one translation: %s", r.Key)
		assert.Equal(t, "qa", strings.SplitN(r.Rule, ".", 2)[0])
	}
	require.Len(t, plan.Languages, 1)
	assert.Equal(t, 3, plan.Languages[0].Failing)

	deck := readDeck(t, s, wsID, projID, url.Values{"locale": {"fr"}, "rule": {fr[0].Key}})
	got := map[string]bool{}
	for _, c := range deck.Cards {
		got[c.BlockID] = true
	}
	assert.Equal(t, map[string]bool{ids["Open the workspace"]: true, ids["Close the workspace"]: true}, got)
	assert.Equal(t, 2, deck.Counts.Desk)
}
