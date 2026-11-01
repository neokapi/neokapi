package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	"github.com/neokapi/neokapi/core/workspace"
	"github.com/neokapi/neokapi/host/screens"
)

// The members a question can be assigned to are those whose permissions on
// the project govern a rule, resolved as a request of theirs would be. A
// workspace viewer holding a project role that decides is one. A workspace
// admin holding the project's observer role is not.
func TestRuleAssignees_FollowTheProjectsResolvedPermissions(t *testing.T) {
	srv, jwt, wsSlug, wsID, pid := newProjectMembersTestServer(t)
	ctx := t.Context()
	e := srv.GetEcho()

	call := func(method, path string, body, out any) int {
		t.Helper()
		var reader *strings.Reader
		if body != nil {
			data, err := json.Marshal(body)
			require.NoError(t, err)
			reader = strings.NewReader(string(data))
		} else {
			reader = strings.NewReader("")
		}
		req := httptest.NewRequest(method, "/api/v1/"+wsSlug+"/"+pid+path, reader)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+jwt)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		if out != nil && rec.Code < 300 {
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), out), rec.Body.String())
		}
		if out != nil && rec.Code >= 300 {
			_ = json.Unmarshal(rec.Body.Bytes(), out)
		}
		return rec.Code
	}
	users := map[string]*platauth.User{}
	member := func(name string, role platauth.Role) string {
		u := &platauth.User{Email: name + "@example.com", Name: name}
		require.NoError(t, srv.AuthStore.CreateUser(ctx, u))
		require.NoError(t, srv.AuthStore.AddMember(ctx, wsID, u.ID, role))
		users[u.ID] = u
		return u.ID
	}
	grant := func(userID, role string) {
		body := map[string]any{"user_id": userID, "role_id": roleIDNamed(t, srv, wsID, role)}
		require.Equal(t, http.StatusCreated, call(http.MethodPost, "/members", body, nil))
	}

	decider := member("Vera Viewer", platauth.RoleViewer)
	grant(decider, "project-admin")
	observer := member("Otto Observer", platauth.RoleAdmin)
	grant(observer, "observer")
	plain := member("Mona Member", platauth.RoleMember)

	segment, err := os.ReadFile(filepath.Join(decideParityDir, "log.jsonl"))
	require.NoError(t, err)
	ops, err := workspace.DecodeSegment(segment, "prj_decideparityaaaaaaaa")
	require.NoError(t, err)
	r := &routeRemote{h: e, base: "/api/v1/" + wsSlug + "/" + pid + "/context/objects", bearer: jwt}
	require.NoError(t, r.Put(ctx, workspace.Object{Name: workspace.SegmentName("desktop", ops[0].ID), Data: segment}))

	var assignees []ruleAssignee
	require.Equal(t, http.StatusOK, call(http.MethodGet, "/rules/main/assignees", nil, &assignees))
	ids := map[string]ruleAssignee{}
	for _, a := range assignees {
		ids[a.ID] = a
	}
	assert.Contains(t, ids, decider, "a workspace viewer with a deciding project role decides")
	assert.Equal(t, "Vera Viewer", ids[decider].Name)
	assert.NotContains(t, ids, observer, "a project observer decides nothing")
	assert.NotContains(t, ids, plain, "a member's workspace role governs no rule")
	var me []ruleAssignee
	for _, a := range assignees {
		if a.Me {
			me = append(me, a)
		}
	}
	require.Len(t, me, 1, "the caller is marked once")
	assert.Equal(t, "PM Admin", me[0].Name)

	var view screens.RulesDecideViewDTO
	require.Equal(t, http.StatusOK, call(http.MethodGet, "/rules/main/decide", nil, &view))
	require.NotEmpty(t, view.Decisions)
	question := view.Decisions[0].ID
	if question == "" {
		question = view.Decisions[0].Options[0].ID
	}

	var assigned ruleAssignment
	require.Equal(t, http.StatusOK, call(http.MethodPost, "/rules/main/assign",
		ruleAssignRequest{ID: question, UserID: &decider}, &assigned))
	assert.Equal(t, decider, assigned.AssigneeID)

	var refusal ErrorResponse
	require.Equal(t, http.StatusUnprocessableEntity, call(http.MethodPost, "/rules/main/assign",
		ruleAssignRequest{ID: question, UserID: &observer}, &refusal))
	assert.Equal(t, errCannotDecide, refusal.Error)
	require.Equal(t, http.StatusOK, call(http.MethodGet, "/rules/main/decide", nil, &view))
	assert.Equal(t, decider, view.Decisions[0].AssigneeID, "a refused assignment leaves the one held")

	// A project observer may not read who decides.
	token, err := platauth.GenerateToken(users[observer], "test-pm-secret", time.Hour)
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/"+wsSlug+"/"+pid+"/rules/main/assignees", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusForbidden, rec.Code, rec.Body.String())
}
