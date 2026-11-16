package server

import (
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
)

// newPeopleHarness is the onboarding harness with one more account: Sam, who
// set no name and has the personal workspace "sam".
func newPeopleHarness(t *testing.T) *welcomeHarness {
	t.Helper()
	h := newWelcomeHarness(t)
	h.store.users["u-sam"] = &platauth.User{ID: "u-sam", Email: "sam.k@example.test"}
	h.store.addWorkspace(&platauth.Workspace{ID: "w-sam", Slug: "sam", Type: platauth.WorkspaceTypePersonal}, "u-sam")
	return h
}

func callMe(t *testing.T, h *welcomeHarness, method, userID, body string) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	req := httptest.NewRequest(method, "/api/v1/auth/me", strings.NewReader(body))
	req.Header.Set("Content-Type", echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := h.srv.GetEcho().NewContext(req, rec)
	c.Set("user_id", userID)
	handler := h.srv.HandleAuthMe
	if method == http.MethodPatch {
		handler = h.srv.HandleUpdateMe
	}
	require.NoError(t, handler(c))
	var got map[string]any
	if rec.Code == http.StatusOK {
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	}
	return rec, got
}

func TestAuthMe_NoNameShowsTheHandle(t *testing.T) {
	h := newPeopleHarness(t)

	rec, got := callMe(t, h, http.MethodGet, "u-sam", "")
	require.Equal(t, http.StatusOK, rec.Code)
	assert.NotContains(t, got, "name", "an unset name is left out, not sent as an empty string")
	assert.Equal(t, "sam", got["shown_name"])
	assert.Equal(t, "sam", got["handle"])
}

func TestUpdateMe_SetsAndClearsTheName(t *testing.T) {
	h := newPeopleHarness(t)

	rec, got := callMe(t, h, http.MethodPatch, "u-sam", `{"name":"  Sam Kim "}`)
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "Sam Kim", got["name"])
	assert.Equal(t, "Sam Kim", got["shown_name"])
	assert.Equal(t, "Sam Kim", h.store.users["u-sam"].Name)

	rec, got = callMe(t, h, http.MethodPatch, "u-sam", `{}`)
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "Sam Kim", got["name"], "a body without name leaves it as it is")

	rec, got = callMe(t, h, http.MethodPatch, "u-sam", `{"name":""}`)
	require.Equal(t, http.StatusOK, rec.Code)
	assert.NotContains(t, got, "name")
	assert.Equal(t, "sam", got["shown_name"])
}

func TestUpdateMe_RefusesABadName(t *testing.T) {
	h := newPeopleHarness(t)
	for _, body := range []string{
		`{"name":"` + strings.Repeat("x", maxPersonName+1) + `"}`,
		`{"name":"Sam\u0007"}`,
	} {
		rec, _ := callMe(t, h, http.MethodPatch, "u-sam", body)
		assert.Equal(t, http.StatusBadRequest, rec.Code, body)
	}
	assert.Empty(t, h.store.users["u-sam"].Name)
}

func TestOnboarding_DisplayNameBecomesTheUsersName(t *testing.T) {
	h := newWelcomeHarness(t)
	h.store.users["u-new"].Name = ""

	rec := h.onboard(t, "u-new", "dana")
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "Dana", h.store.users["u-new"].Name)
}

func TestFeedEntryOf_NamesAndViewer(t *testing.T) {
	h := newPeopleHarness(t)
	names := h.srv.feedMemberNames(t.Context())

	other := feedEntryOf(&bstore.Activity{ActorID: "u-sam", ActorName: "sam.k@example.test"}, names, "u-new")
	assert.Equal(t, "sam", other.Actor.Name, "the current shown name replaces what the row recorded")
	assert.False(t, other.Actor.Viewer)

	own := feedEntryOf(&bstore.Activity{ActorID: "u-new"}, names, "u-new")
	assert.Equal(t, "Dana", own.Actor.Name)
	assert.True(t, own.Actor.Viewer)

	gone := feedEntryOf(&bstore.Activity{ActorID: "u-gone", ActorName: "Former Member"}, names, "u-new")
	assert.Equal(t, "Former Member", gone.Actor.Name, "an account that no longer exists keeps its recorded name")
}
