package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/bowrain/auth"
	"github.com/neokapi/neokapi/bowrain/billing"
	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCustodialAuthoritySuspended(t *testing.T) {
	t.Parallel()

	// Free carries no custodian seats, which is the state a lapsed trial leaves
	// behind.
	assert.True(t, custodialAuthoritySuspended(string(billing.PlanFree)))
	assert.False(t, custodialAuthoritySuspended(string(billing.PlanPro)))
	assert.False(t, custodialAuthoritySuspended(string(billing.PlanTeam)))
	// An unknown or unset plan does not suspend anything: a workspace with no
	// plan recorded is not a workspace whose trial ran out.
	assert.False(t, custodialAuthoritySuspended(""))
	assert.False(t, custodialAuthoritySuspended("nonsense"))
}

// setPlan puts the workspace on a plan, the way every subscription writer does.
func setPlan(t *testing.T, srv *Server, wsID string, plan billing.Plan) {
	t.Helper()
	require.NoError(t, auth.NewPlanSyncer(srv.AuthStore).SyncWorkspacePlan(
		t.Context(), wsID, string(plan), ""))
}

// roleIDNamed resolves a seeded role template by name.
func roleIDNamed(t *testing.T, srv *Server, wsID, name string) string {
	t.Helper()
	templates, err := srv.AuthStore.ListRoleTemplates(t.Context(), wsID)
	require.NoError(t, err)
	for _, rt := range templates {
		if rt.Name == name {
			return rt.ID
		}
	}
	t.Fatalf("role template %q not found", name)
	return ""
}

func TestCustodianSeatGuard(t *testing.T) {
	srv, jwt, wsSlug, wsID, pid := newProjectMembersTestServer(t)
	e := srv.GetEcho()
	ctx := t.Context()

	// Pro carries exactly one custodian.
	setPlan(t, srv, wsID, billing.PlanPro)
	admin := roleIDNamed(t, srv, wsID, "project-admin")
	reviewer := roleIDNamed(t, srv, wsID, "reviewer")

	post := func(userID, roleID string, coords map[string]string) *httptest.ResponseRecorder {
		payload := map[string]any{"user_id": userID, "role_id": roleID}
		if coords != nil {
			payload["coordinates"] = coords
		}
		body, err := json.Marshal(payload)
		require.NoError(t, err)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/"+wsSlug+"/"+pid+"/members",
			strings.NewReader(string(body)))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+jwt)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		return rec
	}

	mustUser := func(email string) string {
		u := &platauth.User{Email: email, Name: email}
		require.NoError(t, srv.AuthStore.CreateUser(ctx, u))
		require.NoError(t, srv.AuthStore.AddMember(ctx, wsID, u.ID, platauth.RoleMember))
		return u.ID
	}

	first := mustUser("custodian-one@example.com")
	second := mustUser("custodian-two@example.com")
	contributor := mustUser("contributor@example.com")
	unbounded := mustUser("unbounded@example.com")

	t.Run("the first custodian fits the allowance", func(t *testing.T) {
		rec := post(first, admin, map[string]string{"brand": "acme"})
		require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	})

	t.Run("the second is refused with the count and the limit", func(t *testing.T) {
		rec := post(second, admin, map[string]string{"brand": "other"})
		require.Equal(t, http.StatusForbidden, rec.Code, rec.Body.String())
		assert.Contains(t, rec.Body.String(), "custodian_limit_reached")
		assert.Contains(t, rec.Body.String(), "\"limit\":1")
	})

	t.Run("members are free and uncapped", func(t *testing.T) {
		// A reviewer bounded to a region is a contributor with a narrow beat,
		// not a custodian, so the allowance does not reach them.
		rec := post(contributor, reviewer, map[string]string{"brand": "other"})
		require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	})

	t.Run("blanket authority is not a custodian seat", func(t *testing.T) {
		// Someone with no region governs everywhere by role, which is the
		// workspace-owner shape rather than custody of a place.
		rec := post(unbounded, admin, nil)
		require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	})

	t.Run("re-saving a custodian does not count their own seat twice", func(t *testing.T) {
		payload := map[string]any{
			"role_id":     admin,
			"coordinates": map[string]string{"brand": "acme", "channel": "support"},
		}
		body, err := json.Marshal(payload)
		require.NoError(t, err)
		req := httptest.NewRequest(http.MethodPut,
			"/api/v1/"+wsSlug+"/"+pid+"/members/"+first, strings.NewReader(string(body)))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+jwt)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	})
}

func TestFreeCarriesNoCustodian(t *testing.T) {
	srv, jwt, wsSlug, wsID, pid := newProjectMembersTestServer(t)
	e := srv.GetEcho()
	ctx := t.Context()

	// A workspace is created on Free, which carries no custodian at all. This is
	// the state a lapsed trial leaves behind, and the reason the trial exists.
	setPlan(t, srv, wsID, billing.PlanFree)
	admin := roleIDNamed(t, srv, wsID, "project-admin")

	u := &platauth.User{Email: "would-be@example.com", Name: "would-be"}
	require.NoError(t, srv.AuthStore.CreateUser(ctx, u))
	require.NoError(t, srv.AuthStore.AddMember(ctx, wsID, u.ID, platauth.RoleMember))

	post := func(coords map[string]string) *httptest.ResponseRecorder {
		payload := map[string]any{"user_id": u.ID, "role_id": admin}
		if coords != nil {
			payload["coordinates"] = coords
		}
		body, err := json.Marshal(payload)
		require.NoError(t, err)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/"+wsSlug+"/"+pid+"/members",
			strings.NewReader(string(body)))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+jwt)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		return rec
	}

	rec := post(map[string]string{"brand": "acme"})
	require.Equal(t, http.StatusForbidden, rec.Code, rec.Body.String())
	assert.Contains(t, rec.Body.String(), "custodian_limit_reached")

	// And the refusal must actually refuse: no membership may exist behind the
	// 403. apiErr returns nil on success, so a guard that returns it rather than
	// a sentinel writes the error and grants the membership anyway.
	members, err := srv.AuthStore.ListProjectMembers(ctx, pid)
	require.NoError(t, err)
	for _, m := range members {
		assert.NotEqual(t, u.ID, m.UserID, "a refused grant must not be written")
	}

	// The same person with no region is not custody, and joins freely.
	rec = post(nil)
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
}

func TestCustodialAuthorityLapsesWithoutDeletingAnything(t *testing.T) {
	srv, jwt, wsSlug, wsID, pid := newProjectMembersTestServer(t)
	e := srv.GetEcho()
	ctx := t.Context()

	// Grant custody while the workspace is on a plan that carries a seat — the
	// trial's shape.
	setPlan(t, srv, wsID, billing.PlanPro)
	admin := roleIDNamed(t, srv, wsID, "project-admin")
	u := &platauth.User{Email: "custodian@example.com", Name: "Custodian"}
	require.NoError(t, srv.AuthStore.CreateUser(ctx, u))
	require.NoError(t, srv.AuthStore.AddMember(ctx, wsID, u.ID, platauth.RoleMember))

	body, err := json.Marshal(map[string]any{
		"user_id": u.ID, "role_id": admin,
		"coordinates": map[string]string{"brand": "acme"},
	})
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/"+wsSlug+"/"+pid+"/members",
		strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+jwt)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())

	resolvedWhilePaid, err := srv.AuthStore.ResolveProjectPermissions(ctx, pid, u.ID)
	require.NoError(t, err)
	require.True(t, platauth.IsCustodian(resolvedWhilePaid.Permissions, resolvedWhilePaid.Coordinates))

	// The trial lapses.
	setPlan(t, srv, wsID, billing.PlanFree)

	t.Run("the authority stops resolving", func(t *testing.T) {
		assert.True(t, custodialAuthoritySuspended(string(billing.PlanFree)))
	})

	t.Run("the governance itself is untouched", func(t *testing.T) {
		// The membership, its region and the role behind it all stay exactly as
		// they were. Suspension and deletion are different code paths, and only
		// one of them is reachable from a plan change.
		after, err := srv.AuthStore.GetProjectMembership(ctx, pid, u.ID)
		require.NoError(t, err)
		assert.Equal(t, platauth.CoordinateFilter{"brand": "acme"}, after.Coordinates)
		assert.Equal(t, admin, after.RoleID)

		resolved, err := srv.AuthStore.ResolveProjectPermissions(ctx, pid, u.ID)
		require.NoError(t, err)
		assert.Equal(t, resolvedWhilePaid.Permissions, resolved.Permissions,
			"the grant is unchanged; only what the request layer does with it changes")
	})

	t.Run("the authority returns with a plan", func(t *testing.T) {
		setPlan(t, srv, wsID, billing.PlanPro)
		assert.False(t, custodialAuthoritySuspended(string(billing.PlanPro)))
	})
}

func TestGrantCannotExceedTheGrantorsCustody(t *testing.T) {
	t.Parallel()

	// Delegation cannot exceed the grantor. Reaches() gives exactly subset
	// semantics here: a filter is inside the caller's reach when every axis the
	// caller names is satisfied by the grant.
	acme := platauth.CoordinateReach{platauth.CoordinateFilter{"brand": "acme"}}

	t.Run("a narrower grant inside the region is allowed", func(t *testing.T) {
		assert.True(t, acme.Reaches(platauth.CoordinateFilter{"brand": "acme", "channel": "support"}))
	})
	t.Run("the same region is allowed", func(t *testing.T) {
		assert.True(t, acme.Reaches(platauth.CoordinateFilter{"brand": "acme"}))
	})
	t.Run("a different region is refused", func(t *testing.T) {
		assert.False(t, acme.Reaches(platauth.CoordinateFilter{"brand": "other"}))
	})
	t.Run("a broader grant is refused", func(t *testing.T) {
		// Granting channel=support with no brand would let acme's custodian
		// hand out support content across every brand.
		assert.False(t, acme.Reaches(platauth.CoordinateFilter{"channel": "support"}))
	})
	t.Run("an unbounded caller grants anywhere", func(t *testing.T) {
		var everywhere platauth.CoordinateReach
		assert.True(t, everywhere.Reaches(platauth.CoordinateFilter{"brand": "other"}))
	})
}

func TestNoSeatOrProjectCap(t *testing.T) {
	srv, jwt, wsSlug, wsID, _ := newProjectMembersTestServer(t)
	e := srv.GetEcho()
	ctx := t.Context()

	// Free used to allow one project and one seat. Neither is metered now.
	setPlan(t, srv, wsID, billing.PlanFree)

	for i := range 3 {
		body := `{"name":"Project ` + string(rune('A'+i)) + `","default_source_language":"en","target_languages":["fr"]}`
		req := httptest.NewRequest(http.MethodPost, "/api/v1/"+wsSlug+"/projects", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+jwt)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		require.Equal(t, http.StatusCreated, rec.Code, "project %d: %s", i, rec.Body.String())
	}

	for _, email := range []string{"m1@example.com", "m2@example.com", "m3@example.com"} {
		u := &platauth.User{Email: email, Name: email}
		require.NoError(t, srv.AuthStore.CreateUser(ctx, u))
		body, err := json.Marshal(map[string]any{"user_id": u.ID, "role": "member"})
		require.NoError(t, err)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/"+wsSlug+"/members",
			strings.NewReader(string(body)))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+jwt)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		require.Less(t, rec.Code, http.StatusBadRequest, "member %s: %s", email, rec.Body.String())
	}
}
