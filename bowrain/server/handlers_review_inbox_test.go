package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	"github.com/neokapi/neokapi/core/model"
)

// These tests run the review inbox against the real PostgreSQL content and
// auth stores.

func readInbox(t *testing.T, s *Server, user string) reviewInboxResponse {
	t.Helper()
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(httptest.NewRequest(http.MethodGet, "/", nil), rec)
	c.Set("user_id", user)
	require.NoError(t, s.HandleReviewInbox(c))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	return decodeJSON[reviewInboxResponse](t, rec)
}

func addInboxUser(t *testing.T, s *Server, wsID, id string, role platauth.Role) {
	t.Helper()
	ctx := context.Background()
	require.NoError(t, s.AuthStore.CreateUser(ctx, &platauth.User{ID: id, Email: id + "@rc.test", Name: id}))
	require.NoError(t, s.AuthStore.AddMember(ctx, wsID, id, role))
}

func grantProjectRole(t *testing.T, s *Server, wsID, projID, user, roleName string, languages []string) {
	t.Helper()
	ctx := context.Background()
	templates, err := s.AuthStore.ListRoleTemplates(ctx, wsID)
	require.NoError(t, err)
	roleID := ""
	for _, rt := range templates {
		if rt.Name == roleName {
			roleID = rt.ID
		}
	}
	require.NotEmpty(t, roleID, "role template %q", roleName)
	require.NoError(t, s.AuthStore.AddProjectMember(ctx, &platauth.ProjectMembership{
		ProjectID: projID, UserID: user, RoleID: roleID, WorkspaceID: wsID, Languages: languages,
	}))
}

func TestReviewInbox_ListsReviewableLanguagesMostWaitingFirst(t *testing.T) {
	s, wsID, owner := newRecheckHarness(t)
	big, _ := seedGovernedProject(t, s, wsID, []*model.Block{
		pendingFrBlock("a1", "One", "Un"),
		pendingFrBlock("a2", "Two", "Deux"),
		pendingFrBlock("a3", "Three", "Trois"),
	})
	small, _ := seedGovernedProject(t, s, wsID, []*model.Block{
		pendingFrBlock("b1", "Four", "Quatre"),
	})
	// A project with nothing waiting is left out.
	_, _ = seedGovernedProject(t, s, wsID, nil)

	inbox := readInbox(t, s, owner)
	require.Len(t, inbox.Languages, 2)
	assert.Equal(t, big, inbox.Languages[0].Project)
	assert.Equal(t, 3, inbox.Languages[0].Waiting)
	assert.Equal(t, 3, inbox.Languages[0].Unclassified, "nothing has opened the deck yet")
	assert.Equal(t, small, inbox.Languages[1].Project)
	assert.Equal(t, 1, inbox.Languages[1].Waiting)
	for _, l := range inbox.Languages {
		assert.Equal(t, "rc", l.Workspace)
		assert.Equal(t, "RC", l.WorkspaceName)
		assert.Equal(t, "fr", l.Locale)
		assert.Equal(t, "main", l.Stream)
	}

	// Once the deck has grouped the language, the inbox counts what the deck
	// counts.
	deck := readDeck(t, s, wsID, big, url.Values{"locale": {"fr"}})
	inbox = readInbox(t, s, owner)
	assert.Equal(t, deck.Counts.Total, inbox.Languages[0].Waiting)
	assert.Zero(t, inbox.Languages[0].Unclassified)
	assert.Equal(t, deck.Counts.Fast, inbox.Languages[0].Fast)
	assert.Equal(t, deck.Counts.Closer, inbox.Languages[0].Closer)
	assert.Equal(t, deck.Counts.Desk, inbox.Languages[0].Desk)
}

func TestReviewInbox_FollowsReviewPermissionAndLanguages(t *testing.T) {
	s, wsID, _ := newRecheckHarness(t)
	projA, _ := seedGovernedProject(t, s, wsID, []*model.Block{pendingFrBlock("a1", "One", "Un")})
	projB, _ := seedGovernedProject(t, s, wsID, []*model.Block{pendingFrBlock("b1", "Two", "Deux")})

	// A workspace member holds no review permission of their own.
	addInboxUser(t, s, wsID, "member", platauth.RoleMember)
	assert.Empty(t, readInbox(t, s, "member").Languages)

	// A reviewer on one project sees that project only.
	addInboxUser(t, s, wsID, "reviewer", platauth.RoleMember)
	grantProjectRole(t, s, wsID, projB, "reviewer", "reviewer", nil)
	inbox := readInbox(t, s, "reviewer")
	require.Len(t, inbox.Languages, 1)
	assert.Equal(t, projB, inbox.Languages[0].Project)

	// A reviewer scoped to another language sees nothing waiting.
	addInboxUser(t, s, wsID, "de-reviewer", platauth.RoleMember)
	grantProjectRole(t, s, wsID, projA, "de-reviewer", "reviewer", []string{"de"})
	assert.Empty(t, readInbox(t, s, "de-reviewer").Languages)

	// Someone outside the workspace sees nothing.
	require.NoError(t, s.AuthStore.CreateUser(context.Background(), &platauth.User{ID: "outsider", Email: "o@x.test"}))
	assert.Empty(t, readInbox(t, s, "outsider").Languages)
}

func TestReviewInbox_APITokenScopesNarrow(t *testing.T) {
	s, wsID, owner := newRecheckHarness(t)
	_, _ = seedGovernedProject(t, s, wsID, []*model.Block{pendingFrBlock("a1", "One", "Un")})

	call := func(scopes string) reviewInboxResponse {
		rec := httptest.NewRecorder()
		c := echo.New().NewContext(httptest.NewRequest(http.MethodGet, "/", nil), rec)
		c.Set("user_id", owner)
		c.Set("api_token_scopes", scopes)
		require.NoError(t, s.HandleReviewInbox(c))
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		return decodeJSON[reviewInboxResponse](t, rec)
	}
	assert.Len(t, call(`["*"]`).Languages, 1)
	assert.Empty(t, call(`["read"]`).Languages, "a read token reviews nothing")
}
