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
