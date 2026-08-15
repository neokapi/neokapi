package server

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/neokapi/neokapi/bowrain/billing"
	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	"github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestServer(t *testing.T) (*Server, string) {
	t.Helper()
	cfg := DefaultConfig()

	cfg.JWTSecret = "test-secret"
	s := shutdownOnCleanup(t, NewServer(cfg))
	initTestStores(t, s)

	require.NotNil(t, s.Services, "services should be initialized")
	require.NotNil(t, s.AuthStore, "auth store should be initialized")

	ctx := t.Context()
	user := &platauth.User{ID: "test-user", Email: "test@example.com", Name: "Test"}
	require.NoError(t, s.AuthStore.CreateUser(ctx, user))
	ws := &platauth.Workspace{ID: "test-ws", Name: "Test", Slug: "test", Type: platauth.WorkspaceTypePersonal}
	require.NoError(t, s.AuthStore.CreateWorkspace(ctx, ws))
	require.NoError(t, s.AuthStore.AddMember(ctx, ws.ID, user.ID, platauth.RoleOwner))

	token, err := platauth.GenerateToken(user, cfg.JWTSecret, 24*time.Hour)
	require.NoError(t, err)
	return s, token
}

func TestProjectCRUDEndpoints(t *testing.T) {
	srv, token := newTestServer(t)
	e := srv.GetEcho()
	authHeader := "Bearer " + token

	// Create project (workspace-scoped: /api/v1/:ws/projects).
	body := `{"name":"Test","default_source_language":"en","target_languages":["fr","de"]}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/test/projects", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", authHeader)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusCreated, rec.Code)

	var project store.Project
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &project))
	assert.Equal(t, "Test", project.Name)
	assert.NotEmpty(t, project.ID)
	projectID := project.ID

	// Get project (workspace-scoped: /api/v1/:ws/:pid).
	req = httptest.NewRequest(http.MethodGet, "/api/v1/test/"+projectID, nil)
	req.Header.Set("Authorization", authHeader)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)

	// List projects (workspace-scoped: /api/v1/:ws/projects).
	req = httptest.NewRequest(http.MethodGet, "/api/v1/test/projects", nil)
	req.Header.Set("Authorization", authHeader)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)

	var projects []*store.Project
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &projects))
	assert.Len(t, projects, 1)

	// Update project (workspace-scoped: /api/v1/:ws/:pid).
	body = `{"name":"Updated","default_source_language":"en","target_languages":["fr"]}`
	req = httptest.NewRequest(http.MethodPut, "/api/v1/test/"+projectID, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", authHeader)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)

	// Delete project (workspace-scoped: /api/v1/:ws/:pid).
	req = httptest.NewRequest(http.MethodDelete, "/api/v1/test/"+projectID, nil)
	req.Header.Set("Authorization", authHeader)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusNoContent, rec.Code)
}

// TestPersonalWorkspaceBlocksPublicVisibility ensures that personal workspaces
// cannot be exposed publicly — neither at the workspace level nor at the
// project level.
func TestPersonalWorkspaceBlocksPublicVisibility(t *testing.T) {
	srv, token := newTestServer(t)
	e := srv.GetEcho()
	authHeader := "Bearer " + token

	// Create a project in the (personal) test workspace.
	body := `{"name":"Test","default_source_language":"en","target_languages":["fr"]}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/test/projects", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", authHeader)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	require.Equal(t, http.StatusCreated, rec.Code)
	var project store.Project
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &project))

	// Workspace-level: public visibility must be rejected.
	req = httptest.NewRequest(http.MethodPatch, "/api/v1/test", strings.NewReader(`{"dashboard_visibility":"public"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", authHeader)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusForbidden, rec.Code)

	// Workspace-level: unlisted visibility must also be rejected.
	req = httptest.NewRequest(http.MethodPatch, "/api/v1/test", strings.NewReader(`{"dashboard_visibility":"unlisted"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", authHeader)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusForbidden, rec.Code)

	// Workspace-level: private is still allowed.
	req = httptest.NewRequest(http.MethodPatch, "/api/v1/test", strings.NewReader(`{"dashboard_visibility":"private"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", authHeader)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)

	// Project-level: public visibility must be rejected.
	req = httptest.NewRequest(http.MethodPut, "/api/v1/test/"+project.ID, strings.NewReader(`{"dashboard_visibility":"public"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", authHeader)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusForbidden, rec.Code)

	// Project-level: private is still allowed.
	req = httptest.NewRequest(http.MethodPut, "/api/v1/test/"+project.ID, strings.NewReader(`{"dashboard_visibility":"private"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", authHeader)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
}

// policyWorkspace is a team workspace whose members cover every workspace role,
// a second plain member, and a user who is not a member of it at all. Its role
// templates are seeded, as `CreateWorkspaceWithOwner` seeds them in production —
// without them a project creator gets no project membership and the mutation
// tests would pass for a reason that does not hold for a real workspace.
type policyWorkspace struct {
	srv         *Server
	slug        string
	byRole      map[platauth.Role]string // role → JWT
	otherMember string                   // JWT of a second plain member
	outsider    string                   // JWT of a non-member
	ownerID     string
	wsID        string
}

func newPolicyWorkspace(t *testing.T) *policyWorkspace {
	t.Helper()

	cfg := DefaultConfig()
	cfg.JWTSecret = "test-secret"
	srv := shutdownOnCleanup(t, NewServer(cfg))
	initTestStores(t, srv)
	require.NotNil(t, srv.AuthStore)

	ctx := t.Context()
	// The Team plan, because these tests create several projects in one
	// workspace and the plan limit is checked before the policy is: on Free the
	// second create is refused with project_limit_reached, which is a 403 for
	// the wrong reason and would read as the policy denying a member.
	ws := &platauth.Workspace{
		ID: "policy-ws", Name: "Policy", Slug: "policy",
		Type: platauth.WorkspaceTypeTeam, Plan: string(billing.PlanTeam),
	}
	require.NoError(t, srv.AuthStore.CreateWorkspace(ctx, ws))
	require.NoError(t, srv.AuthStore.SeedDefaultRoleTemplates(ctx, ws.ID))

	pw := &policyWorkspace{
		srv:    srv,
		slug:   ws.Slug,
		wsID:   ws.ID,
		byRole: map[platauth.Role]string{},
	}

	newUser := func(id string, role platauth.Role) string {
		u := &platauth.User{ID: id, Email: id + "@example.com", Name: id}
		require.NoError(t, srv.AuthStore.CreateUser(ctx, u))
		if role != "" {
			require.NoError(t, srv.AuthStore.AddMember(ctx, ws.ID, u.ID, role))
		}
		tok, err := platauth.GenerateToken(u, cfg.JWTSecret, time.Hour)
		require.NoError(t, err)
		return tok
	}

	for _, role := range []platauth.Role{platauth.RoleOwner, platauth.RoleAdmin, platauth.RoleMember} {
		pw.byRole[role] = newUser("user-"+string(role), role)
		if role == platauth.RoleOwner {
			pw.ownerID = "user-" + string(role)
		}
	}
	pw.otherMember = newUser("user-other-member", platauth.RoleMember)
	pw.outsider = newUser("user-outsider", "")

	return pw
}

// do issues a request against the workspace, authenticated with the given
// bearer credential (a JWT or a bwt_ API token).
func (pw *policyWorkspace) do(t *testing.T, method, path, bearer, body string) *httptest.ResponseRecorder {
	t.Helper()
	var req *http.Request
	if body == "" {
		req = httptest.NewRequest(method, path, nil)
	} else {
		req = httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Authorization", "Bearer "+bearer)
	rec := httptest.NewRecorder()
	pw.srv.GetEcho().ServeHTTP(rec, req)
	return rec
}

// mintToken creates a real API token on the workspace, owned by the workspace
// owner, carrying the given scopes.
func (pw *policyWorkspace) mintToken(t *testing.T, name, scopesJSON string) string {
	t.Helper()
	seed := sha256.Sum256([]byte(name))
	plaintext := "bwt_" + hex.EncodeToString(seed[:])
	hash := sha256.Sum256([]byte(plaintext))
	require.NoError(t, pw.srv.AuthStore.CreateAPIToken(t.Context(), &platauth.APIToken{
		UserID:      pw.ownerID,
		WorkspaceID: pw.wsID,
		Name:        name,
		TokenPrefix: plaintext[:8],
		Scopes:      scopesJSON,
	}, hex.EncodeToString(hash[:])))
	return plaintext
}

const policyProjectBody = `{"name":"Policy","default_source_language":"en","target_languages":["nb"]}`

// assertCreateOutcome checks the status and, for a refusal, that the reason is
// the policy rather than something else that also answers 403 — the plan's
// project limit is checked in the same handler and would otherwise let a
// billing refusal read as a permission one.
func assertCreateOutcome(t *testing.T, rec *httptest.ResponseRecorder, want int) {
	t.Helper()
	assert.Equal(t, want, rec.Code, rec.Body.String())
	if want == http.StatusForbidden {
		assert.NotContains(t, rec.Body.String(), "project_limit_reached",
			"refused by the plan limit, not by the policy under test")
	}
}

// TestCreateWorkspaceProjectIsMemberLevel pins the deliberate asymmetry: any
// workspace member may create a project, and a non-member may not.
func TestCreateWorkspaceProjectIsMemberLevel(t *testing.T) {
	pw := newPolicyWorkspace(t)

	tests := []struct {
		name   string
		bearer string
		want   int
	}{
		{"owner creates", pw.byRole[platauth.RoleOwner], http.StatusCreated},
		{"admin creates", pw.byRole[platauth.RoleAdmin], http.StatusCreated},
		{"member creates", pw.byRole[platauth.RoleMember], http.StatusCreated},
		{"non-member is refused", pw.outsider, http.StatusForbidden},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := pw.do(t, http.MethodPost, "/api/v1/"+pw.slug+"/projects", tt.bearer, policyProjectBody)
			assertCreateOutcome(t, rec, tt.want)
		})
	}
}

// TestCreateWorkspaceProjectHonorsTokenScopes shows the second half of the
// policy: a token authenticates as its owner, so a token that may only read or
// translate cannot create projects even though its owner owns the workspace.
func TestCreateWorkspaceProjectHonorsTokenScopes(t *testing.T) {
	pw := newPolicyWorkspace(t)

	tests := []struct {
		name   string
		scopes string
		want   int
	}{
		{"read-only token is refused", `["read"]`, http.StatusForbidden},
		{"translate token is refused", `["translate"]`, http.StatusForbidden},
		{"review token is refused", `["review"]`, http.StatusForbidden},
		{"contribute token creates", `["contribute"]`, http.StatusCreated},
		{"full-access token creates", `["*"]`, http.StatusCreated},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			token := pw.mintToken(t, tt.name, tt.scopes)
			rec := pw.do(t, http.MethodPost, "/api/v1/"+pw.slug+"/projects", token, policyProjectBody)
			assertCreateOutcome(t, rec, tt.want)
		})
	}
}

// TestProjectMutationStaysManagerOnly is the other side of the asymmetry.
// Creating is member-level; changing a project is manage_project, which a
// member holds only where they hold it — on the project they created, where the
// creator enrollment makes them a project admin. A second member of the same
// workspace, holding nothing on that project, is refused.
func TestProjectMutationStaysManagerOnly(t *testing.T) {
	pw := newPolicyWorkspace(t)

	rec := pw.do(t, http.MethodPost, "/api/v1/"+pw.slug+"/projects", pw.byRole[platauth.RoleMember], policyProjectBody)
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	var created store.Project
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &created))

	base := "/api/v1/" + pw.slug + "/" + created.ID

	rec = pw.do(t, http.MethodPut, base, pw.otherMember, `{"name":"Renamed"}`)
	assert.Equal(t, http.StatusForbidden, rec.Code, "a member may not rename another member's project")

	rec = pw.do(t, http.MethodDelete, base, pw.otherMember, "")
	assert.Equal(t, http.StatusForbidden, rec.Code, "a member may not archive another member's project")

	rec = pw.do(t, http.MethodPut, base, pw.byRole[platauth.RoleMember], `{"name":"Renamed"}`)
	assert.Equal(t, http.StatusOK, rec.Code, "the creator governs what it created: %s", rec.Body.String())

	rec = pw.do(t, http.MethodPut, base, pw.byRole[platauth.RoleAdmin], `{"name":"Renamed by an admin"}`)
	assert.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	rec = pw.do(t, http.MethodDelete, base, pw.byRole[platauth.RoleAdmin], "")
	assert.Equal(t, http.StatusNoContent, rec.Code, rec.Body.String())
}

// TestPersonalWorkspaceBlocksRawShipFeedProperty covers the disclosure guard on
// the path that writes project properties directly: a personal workspace cannot
// publish a ship feed by writing the opt-in as a raw property.
func TestPersonalWorkspaceBlocksRawShipFeedProperty(t *testing.T) {
	srv, token := newTestServer(t)
	e := srv.GetEcho()
	authHeader := "Bearer " + token

	req := httptest.NewRequest(http.MethodPost, "/api/v1/test/projects", strings.NewReader(policyProjectBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", authHeader)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	var project store.Project
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &project))

	req = httptest.NewRequest(http.MethodPut, "/api/v1/test/"+project.ID,
		strings.NewReader(`{"properties":{"`+ShipFeedProperty+`":"true"}}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", authHeader)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusForbidden, rec.Code)

	// An unrelated property still writes.
	req = httptest.NewRequest(http.MethodPut, "/api/v1/test/"+project.ID,
		strings.NewReader(`{"properties":{"colour":"blue"}}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", authHeader)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
}

// TestRetiredProjectHandlersAreOffTheRouter asserts the pre-ABAC CRUD set is
// gone from the router surface, so no route can reach a handler that predates
// the permission model.
func TestRetiredProjectHandlersAreOffTheRouter(t *testing.T) {
	srv, _ := newTestServer(t)

	retired := map[string]bool{
		"HandleCreateProject": true,
		"HandleGetProject":    true,
		"HandleListProjects":  true,
		"HandleUpdateProject": true,
		"HandleDeleteProject": true,
	}

	for _, r := range srv.GetEcho().Routes() {
		name := strings.TrimSuffix(r.Name, "-fm")
		if i := strings.LastIndex(name, "."); i >= 0 {
			name = name[i+1:]
		}
		assert.False(t, retired[name], "%s %s still routes to %s", r.Method, r.Path, r.Name)
	}
}
