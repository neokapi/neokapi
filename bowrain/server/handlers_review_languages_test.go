package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	bstore "github.com/neokapi/neokapi/bowrain/store"
	"github.com/neokapi/neokapi/core/model"
)

// languagesCall runs one of the review-languages handlers as user, with the
// permissions and the language grant a project route resolves.
func languagesCall(t *testing.T, s *Server, h echo.HandlerFunc, method, wsID, projID, user string, perms platauth.Permission, langs []string, body string) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(method, "/", strings.NewReader(body))
	r.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(r, rec)
	c.SetParamNames("ws", "id")
	c.SetParamValues("rc", projID)
	c.Set("workspace_id", wsID)
	c.Set("user_id", user)
	c.Set("project_permissions", perms)
	if langs != nil {
		c.Set("project_languages", langs)
	}
	if err := h(c); err != nil {
		require.ErrorIs(t, err, errAccessDenied)
	}
	return rec
}

// setTargets gives a project the target languages named.
func setTargets(t *testing.T, s *Server, projID string, locales ...model.LocaleID) {
	t.Helper()
	ctx := context.Background()
	p, err := s.ContentStore.GetProject(ctx, projID)
	require.NoError(t, err)
	p.TargetLanguages = locales
	require.NoError(t, s.ContentStore.UpdateProject(ctx, p))
}

func TestReviewLanguages_FollowsEveryAllowedLanguageUntilSaved(t *testing.T) {
	s, wsID, _ := newRecheckHarness(t)
	projID, _ := seedGovernedProject(t, s, wsID, nil)
	setTargets(t, s, projID, "fr", "de", "nb")

	read := func(langs []string) reviewLanguagesResponse {
		rec := languagesCall(t, s, s.HandleGetReviewLanguages, http.MethodGet, wsID, projID, "u-ada", testReviewer, langs, "")
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		return decodeJSON[reviewLanguagesResponse](t, rec)
	}

	got := read(nil)
	assert.False(t, got.Saved)
	assert.Equal(t, []string{"fr", "de", "nb"}, got.Allowed)
	assert.Equal(t, got.Allowed, got.Followed, "no list saved follows every allowed language")
	assert.Equal(t, []string{"fr", "de", "nb"}, got.Project)
	assert.Equal(t, "en", got.Source)
	assert.False(t, got.CanManage, "a reviewer does not manage the project")

	narrowed := read([]string{"de", "nb"})
	assert.Equal(t, []string{"de", "nb"}, narrowed.Allowed, "a membership that names languages narrows what is allowed")

	// A language outside the grant is refused, and nothing is saved.
	rec := languagesCall(t, s, s.HandlePutReviewLanguages, http.MethodPut, wsID, projID, "u-ada", testReviewer, []string{"de", "nb"}, `{"followed":["fr"]}`)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.False(t, read([]string{"de", "nb"}).Saved)

	rec = languagesCall(t, s, s.HandlePutReviewLanguages, http.MethodPut, wsID, projID, "u-ada", testReviewer, []string{"de", "nb"}, `{"followed":["nb","nb"]}`)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	saved := decodeJSON[reviewLanguagesResponse](t, rec)
	assert.True(t, saved.Saved)
	assert.Equal(t, []string{"nb"}, saved.Followed)

	// Another person's list is their own.
	other := languagesCall(t, s, s.HandleGetReviewLanguages, http.MethodGet, wsID, projID, "u-ben", testReviewer, nil, "")
	assert.False(t, decodeJSON[reviewLanguagesResponse](t, other).Saved)

	// An empty list is a saved choice to follow none.
	rec = languagesCall(t, s, s.HandlePutReviewLanguages, http.MethodPut, wsID, projID, "u-ada", testReviewer, nil, `{"followed":[]}`)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	none := decodeJSON[reviewLanguagesResponse](t, rec)
	assert.True(t, none.Saved)
	assert.Empty(t, none.Followed)
}

func TestReviewPlan_CountsOnlyFollowedLanguages(t *testing.T) {
	s, wsID, _ := newRecheckHarness(t)
	projID, _ := containerProject(t, s, wsID)
	require.Len(t, readPlan(t, s, wsID, projID).Languages, 1)

	rec := languagesCall(t, s, s.HandlePutReviewLanguages, http.MethodPut, wsID, projID, "u-reviewer", testReviewer, nil, `{"followed":[]}`)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Empty(t, readPlan(t, s, wsID, projID).Languages, "a person who follows no language is counted for none")
}

func TestSetProjectLanguage_FilesARecipeChange(t *testing.T) {
	s, wsID, _ := newRecheckHarness(t)
	cs, ok := s.ContentStore.(*bstore.PostgresStore)
	require.True(t, ok)
	s.RecipeChangeStore = cs
	projID, _ := seedGovernedProject(t, s, wsID, nil)

	rec := languagesCall(t, s, s.HandleSetProjectLanguage, http.MethodPost, wsID, projID, "u-rev", testReviewer, nil, `{"locale":"nb","enabled":true}`)
	assert.NotEqual(t, http.StatusOK, rec.Code, "turning a language on needs the project settings permission")

	rec = languagesCall(t, s, s.HandleSetProjectLanguage, http.MethodPost, wsID, projID, "u-owner", platauth.PermAll, nil, `{"locale":"not a locale!","enabled":true}`)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	rec = languagesCall(t, s, s.HandleSetProjectLanguage, http.MethodPost, wsID, projID, "u-owner", platauth.PermAll, nil, `{"locale":"en","enabled":true}`)
	assert.Equal(t, http.StatusBadRequest, rec.Code, "the source language is not a target")

	rec = languagesCall(t, s, s.HandleSetProjectLanguage, http.MethodPost, wsID, projID, "u-owner", platauth.PermAll, nil, `{"locale":"nb","enabled":true}`)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	got := decodeJSON[reviewLanguagesResponse](t, rec)
	assert.True(t, got.CanManage)
	assert.Equal(t, []string{"fr"}, got.Project, "the project keeps its languages until the recipe changes")
	assert.Equal(t, []string{"fr", "nb"}, got.PendingProject)

	// A second change before a pull composes with the first.
	rec = languagesCall(t, s, s.HandleSetProjectLanguage, http.MethodPost, wsID, projID, "u-owner", platauth.PermAll, nil, `{"locale":"fr","enabled":false}`)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Equal(t, []string{"nb"}, decodeJSON[reviewLanguagesResponse](t, rec).PendingProject)

	pending, err := cs.PendingRecipeChanges(t.Context(), projID)
	require.NoError(t, err)
	require.Len(t, pending, 1, "one recipe line waits, holding the last decision")
	assert.Equal(t, recipePathTargetLanguages, pending[0].Path)
	var value []string
	require.NoError(t, json.Unmarshal(pending[0].Value, &value))
	assert.Equal(t, []string{"nb"}, value)
	assert.Equal(t, "u-owner", pending[0].CreatedBy)
}

func TestReviewInbox_HonoursFollowedLanguagesAndSummarizesThePlan(t *testing.T) {
	s, wsID, owner := newRecheckHarness(t)
	projID, _ := seedGovernedProject(t, s, wsID, []*model.Block{pendingFrBlock("a1", "One", "Un")})

	inbox := readInbox(t, s, owner)
	require.Len(t, inbox.Languages, 1)
	require.Len(t, inbox.Projects, 1)
	p := inbox.Projects[0]
	assert.Equal(t, projID, p.Project)
	assert.False(t, p.Saved)
	assert.Equal(t, []string{"fr"}, p.Followed)
	assert.True(t, p.CanManage, "the workspace owner manages the project")
	require.NotNil(t, p.Plan, "a project with work waiting carries its plan in brief")
	levels := []string{}
	for _, st := range p.Plan.Steps {
		levels = append(levels, st.Level)
	}
	assert.Equal(t, []string{planCoordinates, planKeyLanguage, planExceptions, planInterfaces, planPolicy}, levels)
	assert.Equal(t, planCoordinates, p.Plan.Next)
	assert.False(t, p.Plan.Steps[0].Done, "the coordinates are not confirmed")
	assert.Equal(t, 1, p.Plan.Steps[0].Count)

	// Following no language takes the language out of the inbox, and keeps
	// the project listed so the choice can be changed.
	rec := languagesCall(t, s, s.HandlePutReviewLanguages, http.MethodPut, wsID, projID, owner, platauth.PermAll, nil, `{"followed":[]}`)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	inbox = readInbox(t, s, owner)
	assert.Empty(t, inbox.Languages)
	require.Len(t, inbox.Projects, 1)
	assert.True(t, inbox.Projects[0].Saved)
	assert.Empty(t, inbox.Projects[0].Followed)
	assert.Nil(t, inbox.Projects[0].Plan, "nothing waits, so no plan is computed")
}

func TestSummarizePlan_CountsWithinFollowedLanguages(t *testing.T) {
	findings := 0
	p := reviewPlanResponse{
		Involvement: involvementContext,
		Next:        planKeyLanguage,
		Coordinates: planCoordinatesDTO{Confirmed: true},
		KeyLanguage: planKeyLanguageDTO{Waiting: 2, Concepts: 10, Untranslated: map[string]int{"nb": 4, "de": 9}},
		Exceptions: planExceptionsDTO{SourceFindings: &findings, ByRule: planRulesDTO{
			Targets: map[string][]planRuleDTO{"nb": {{Key: "a"}, {Key: "b"}}, "de": {{Key: "c"}}},
		}},
		Languages: []planLanguageDTO{{Locale: "nb", deckWaiting: deckWaiting{Waiting: 10}, Failing: 5, Pages: 3, Settleable: 4}},
		Policy:    planPolicyDTO{Settle: settleHeld},
	}
	got := summarizePlan(p)
	byLevel := map[string]planStepDTO{}
	for _, st := range got.Steps {
		byLevel[st.Level] = st
	}
	assert.Equal(t, planStepDTO{Level: planCoordinates, Progress: 100, Done: true}, byLevel[planCoordinates])
	key := byLevel[planKeyLanguage]
	assert.Equal(t, 6, key.Count, "two cards and four concepts without a Norwegian term; German is not followed")
	assert.Equal(t, 16*100/22, key.Progress)
	assert.Equal(t, 2, byLevel[planExceptions].Count, "the rules in followed languages")
	assert.Equal(t, 50, byLevel[planExceptions].Progress)
	assert.Equal(t, 3, byLevel[planInterfaces].Count)
	assert.Equal(t, planStepDTO{Level: planPolicy, Count: 4, Settle: settleHeld}, byLevel[planPolicy])
}
