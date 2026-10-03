package server

import (
	"context"
	"encoding/json"
	"errors"
	"maps"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	bauth "github.com/neokapi/neokapi/bowrain/auth"
	"github.com/neokapi/neokapi/bowrain/billing"
	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
)

// The server MCP holds an agent to what the project access middleware resolves
// for the user it acts for, on the project's workspace: project membership and
// its language scope, the workspace role, deny rules, and the custody a plan
// with no custodian seats suspends.
func TestUserSender_HoldsWhatTheMiddlewareResolves(t *testing.T) {
	srv, jwt, wsSlug, wsID, pid := newProjectMembersTestServer(t)
	ctx := t.Context()
	e := srv.GetEcho()

	setPlan(t, srv, wsID, billing.PlanPro)
	member := func(email string, role platauth.Role) string {
		u := &platauth.User{Email: email, Name: email}
		require.NoError(t, srv.AuthStore.CreateUser(ctx, u))
		require.NoError(t, srv.AuthStore.AddMember(ctx, wsID, u.ID, role))
		return u.ID
	}
	grant := func(userID, roleID string, extra map[string]any) {
		payload := map[string]any{"user_id": userID, "role_id": roleID}
		maps.Copy(payload, extra)
		body, err := json.Marshal(payload)
		require.NoError(t, err)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/"+wsSlug+"/"+pid+"/members", strings.NewReader(string(body)))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+jwt)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	}

	viewer := member("viewer@example.com", platauth.RoleViewer)
	translator := member("translator@example.com", platauth.RoleMember)
	grant(translator, roleIDNamed(t, srv, wsID, "translator"), map[string]any{"languages": []string{"de"}})
	denied := member("denied@example.com", platauth.RoleMember)
	require.NoError(t, srv.AuthStore.CreateDenyRule(ctx, &platauth.DenyRule{
		WorkspaceID: wsID, SubjectType: platauth.DenySubjectUser, SubjectID: denied,
		DeniedPerms: platauth.PermTranslate, Reason: "on leave",
	}))
	custodian := member("custodian@example.com", platauth.RoleMember)
	grant(custodian, roleIDNamed(t, srv, wsID, "project-admin"), map[string]any{"coordinates": map[string]string{"brand": "acme"}})
	// The trial lapses: the custodian's bounded custody stops resolving.
	setPlan(t, srv, wsID, billing.PlanFree)

	w, err := srv.AuthStore.GetWorkspace(ctx, wsID)
	require.NoError(t, err)
	proj, err := srv.ContentStore.GetProject(ctx, pid)
	require.NoError(t, err)

	// resolved is what the middleware sets on a request to the project in its
	// workspace.
	resolved := func(userID string) (platauth.Permission, []string) {
		m, err := srv.AuthStore.GetMembership(ctx, wsID, userID)
		require.NoError(t, err)
		c := echo.New().NewContext(httptest.NewRequest(http.MethodGet, "/", nil), httptest.NewRecorder())
		c.SetParamNames("ws", "id")
		c.SetParamValues(wsSlug, pid)
		c.Set("user_id", userID)
		c.Set("workspace_id", wsID)
		c.Set("workspace_role", m.Role)
		c.Set("workspace_plan", w.Plan)
		var (
			perms platauth.Permission
			langs []string
		)
		require.NoError(t, srv.ProjectAccessMiddleware()(func(c echo.Context) error {
			perms, _ = c.Get("project_permissions").(platauth.Permission)
			langs, _ = c.Get("project_languages").([]string)
			return nil
		})(c))
		return perms, langs
	}

	for _, tc := range []struct {
		name string
		user string
	}{
		{"a viewer", viewer},
		{"a translator scoped to one language", translator},
		{"a member a deny rule withholds translate from", denied},
		{"a custodian whose plan lapsed", custodian},
	} {
		t.Run(tc.name, func(t *testing.T) {
			perms, langs := resolved(tc.user)
			sender, err := srv.userSender(ctx, tc.user, proj)
			require.NoError(t, err)
			for perm := platauth.Permission(1); perm&platauth.PermAll != 0; perm <<= 1 {
				assert.Equal(t, perms.Has(perm), sender.allows(perm, ""), "%s", perm)
			}
			for _, locale := range []string{"fr", "de"} {
				want := perms.Has(platauth.PermTranslate) && (len(langs) == 0 || slices.Contains(langs, locale))
				assert.Equal(t, want, sender.allows(platauth.PermTranslate, locale), "translate into %s", locale)
			}
		})
	}

	t.Run("the cases differ", func(t *testing.T) {
		vp, _ := resolved(viewer)
		assert.False(t, vp.Has(platauth.PermTranslate), "a viewer translates nothing")
		dp, _ := resolved(denied)
		assert.False(t, dp.Has(platauth.PermTranslate), "the deny rule holds")
		cp, _ := resolved(custodian)
		assert.False(t, cp.Has(platauth.PermManageTerms), "the lapsed plan suspends the custody")
		sender, err := srv.userSender(ctx, translator, proj)
		require.NoError(t, err)
		assert.True(t, sender.allows(platauth.PermTranslate, "de"))
		assert.False(t, sender.allows(platauth.PermTranslate, "fr"))
	})

	// The plan decides whether the custodian's authority stands, so a
	// workspace that cannot be read leaves the agent nothing, as the
	// middleware answers such a request with 503.
	t.Run("a workspace that cannot be read", func(t *testing.T) {
		real := srv.AuthStore
		srv.AuthStore = unreadableWorkspace{AuthStore: real}
		t.Cleanup(func() { srv.AuthStore = real })

		_, err := srv.userSender(ctx, custodian, proj)
		require.ErrorIs(t, err, errWorkspaceUnreadable)
		svc, landed, err := srv.mcpChangeService(ctx, custodian, pid, "main")
		require.ErrorIs(t, err, errWorkspaceUnreadable)
		assert.Nil(t, svc)
		assert.Nil(t, landed)
	})
}

// errWorkspaceUnreadable is the failure unreadableWorkspace answers with.
var errWorkspaceUnreadable = errors.New("connection reset by peer")

// unreadableWorkspace is the auth store with its workspace read failing, as a
// database under contention fails it.
type unreadableWorkspace struct{ bauth.AuthStore }

func (unreadableWorkspace) GetWorkspace(context.Context, string) (*platauth.Workspace, error) {
	return nil, errWorkspaceUnreadable
}
