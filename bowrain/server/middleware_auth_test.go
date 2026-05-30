package server

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/neokapi/neokapi/bowrain/auth"
	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	"github.com/neokapi/neokapi/bowrain/testutil/pgtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testSecret = "test-jwt-secret-32-bytes-long!!!"

func TestAuthMiddleware(t *testing.T) {
	user := &platauth.User{ID: "user-1", Email: "test@example.com", Name: "Test"}
	token, err := platauth.GenerateToken(user, testSecret, 1*time.Hour)
	require.NoError(t, err)

	mw := AuthMiddleware(testSecret, nil)

	t.Run("valid token", func(t *testing.T) {
		e := echo.New()
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		var userID string
		handler := mw(func(c echo.Context) error {
			userID = c.Get("user_id").(string)
			return c.NoContent(http.StatusOK)
		})
		err := handler(c)
		require.NoError(t, err)
		assert.Equal(t, "user-1", userID)
	})

	t.Run("missing header", func(t *testing.T) {
		e := echo.New()
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		handler := mw(func(c echo.Context) error {
			return c.NoContent(http.StatusOK)
		})
		_ = handler(c)
		assert.Equal(t, http.StatusUnauthorized, rec.Code)
	})

	t.Run("invalid token", func(t *testing.T) {
		e := echo.New()
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Authorization", "Bearer invalid-token")
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		handler := mw(func(c echo.Context) error {
			return c.NoContent(http.StatusOK)
		})
		_ = handler(c)
		assert.Equal(t, http.StatusUnauthorized, rec.Code)
	})

	t.Run("wrong format", func(t *testing.T) {
		e := echo.New()
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Authorization", "Basic dXNlcjpwYXNz")
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		handler := mw(func(c echo.Context) error {
			return c.NoContent(http.StatusOK)
		})
		_ = handler(c)
		assert.Equal(t, http.StatusUnauthorized, rec.Code)
	})
}

func TestAuthMiddleware_APIToken(t *testing.T) {
	db := pgtest.NewTestDB(t)
	store, err := auth.NewAuthStoreFromDB(db)
	require.NoError(t, err)
	defer store.Close()

	// Create a user and workspace.
	user := &platauth.User{Email: "apiuser@example.com", Name: "API User"}
	require.NoError(t, store.CreateUser(t.Context(), user))
	ws := &platauth.Workspace{Name: "Test WS", Slug: "test-ws"}
	require.NoError(t, store.CreateWorkspace(t.Context(), ws))

	// Create an API token.
	plaintext := "bwt_0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	hash := sha256.Sum256([]byte(plaintext))
	tokenHash := hex.EncodeToString(hash[:])

	apiToken := &platauth.APIToken{
		UserID:      user.ID,
		WorkspaceID: ws.ID,
		Name:        "CI Token",
		TokenPrefix: plaintext[:8],
		Scopes:      `["*"]`,
	}
	require.NoError(t, store.CreateAPIToken(t.Context(), apiToken, tokenHash))

	mw := AuthMiddleware(testSecret, store)

	t.Run("valid api token", func(t *testing.T) {
		e := echo.New()
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Authorization", "Bearer "+plaintext)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		var userID, tokenID string
		handler := mw(func(c echo.Context) error {
			userID = c.Get("user_id").(string)
			tokenID = c.Get("api_token_id").(string)
			return c.NoContent(http.StatusOK)
		})
		err := handler(c)
		require.NoError(t, err)
		assert.Equal(t, user.ID, userID)
		assert.Equal(t, apiToken.ID, tokenID)
	})

	t.Run("invalid api token", func(t *testing.T) {
		e := echo.New()
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Authorization", "Bearer bwt_invalid")
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		handler := mw(func(c echo.Context) error {
			return c.NoContent(http.StatusOK)
		})
		_ = handler(c)
		assert.Equal(t, http.StatusUnauthorized, rec.Code)
	})

	t.Run("expired api token", func(t *testing.T) {
		// Create an expired token.
		expiredPlaintext := "bwt_expired456789abcdef0123456789abcdef0123456789abcdef0123456789"
		expHash := sha256.Sum256([]byte(expiredPlaintext))
		expTokenHash := hex.EncodeToString(expHash[:])

		past := time.Now().Add(-1 * time.Hour)
		expToken := &platauth.APIToken{
			UserID:      user.ID,
			WorkspaceID: ws.ID,
			Name:        "Expired Token",
			TokenPrefix: expiredPlaintext[:8],
			Scopes:      `["*"]`,
			ExpiresAt:   &past,
		}
		require.NoError(t, store.CreateAPIToken(t.Context(), expToken, expTokenHash))

		e := echo.New()
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Authorization", "Bearer "+expiredPlaintext)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		handler := mw(func(c echo.Context) error {
			return c.NoContent(http.StatusOK)
		})
		_ = handler(c)
		assert.Equal(t, http.StatusUnauthorized, rec.Code)
	})
}

func TestProjectAccessMiddleware(t *testing.T) {
	db := pgtest.NewTestDB(t)
	store, err := auth.NewAuthStoreFromDB(db)
	require.NoError(t, err)
	defer store.Close()

	ctx := t.Context()

	// Create user, workspace, seed role templates.
	user := &platauth.User{Email: "proj@example.com", Name: "Proj User"}
	require.NoError(t, store.CreateUser(ctx, user))

	ws := &platauth.Workspace{Name: "Test WS", Slug: "test-ws"}
	require.NoError(t, store.CreateWorkspace(ctx, ws))
	require.NoError(t, store.SeedDefaultRoleTemplates(ctx, ws.ID))

	// Find the "translator" role template (has PermViewContent | PermTranslate).
	templates, err := store.ListRoleTemplates(ctx, ws.ID)
	require.NoError(t, err)
	var translatorRoleID string
	for _, rt := range templates {
		if rt.Name == "translator" {
			translatorRoleID = rt.ID
			break
		}
	}
	require.NotEmpty(t, translatorRoleID, "translator role template not found")

	// Add user as project member with translator role and language scope.
	projectID := "proj-1"
	pm := &platauth.ProjectMembership{
		ProjectID:   projectID,
		UserID:      user.ID,
		RoleID:      translatorRoleID,
		WorkspaceID: ws.ID,
		Languages:   []string{"fr", "de"},
	}
	require.NoError(t, store.AddProjectMember(ctx, pm))

	srv := &Server{AuthStore: store}
	mw := srv.ProjectAccessMiddleware()

	t.Run("member with explicit project role", func(t *testing.T) {
		e := echo.New()
		req := httptest.NewRequest(http.MethodGet, "/projects/"+projectID, nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("id")
		c.SetParamValues(projectID)
		c.Set("user_id", user.ID)
		c.Set("workspace_role", platauth.RoleMember)

		var perms platauth.Permission
		var langs []string
		handler := mw(func(c echo.Context) error {
			perms = c.Get("project_permissions").(platauth.Permission)
			langs = c.Get("project_languages").([]string)
			return c.NoContent(http.StatusOK)
		})
		err := handler(c)
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, rec.Code)
		// Translator template: PermViewContent | PermTranslate
		assert.True(t, perms.Has(platauth.PermViewContent))
		assert.True(t, perms.Has(platauth.PermTranslate))
		assert.False(t, perms.Has(platauth.PermManageProject))
		assert.Equal(t, []string{"fr", "de"}, langs)
	})

	t.Run("workspace owner gets implicit full access", func(t *testing.T) {
		e := echo.New()
		otherProjectID := "proj-no-membership"
		req := httptest.NewRequest(http.MethodGet, "/projects/"+otherProjectID, nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("id")
		c.SetParamValues(otherProjectID)
		c.Set("user_id", user.ID)
		c.Set("workspace_role", platauth.RoleOwner)

		var perms platauth.Permission
		handler := mw(func(c echo.Context) error {
			perms = c.Get("project_permissions").(platauth.Permission)
			return c.NoContent(http.StatusOK)
		})
		err := handler(c)
		require.NoError(t, err)
		assert.Equal(t, platauth.PermAll, perms)
	})

	t.Run("no project membership falls back to workspace role", func(t *testing.T) {
		e := echo.New()
		otherProjectID := "proj-no-membership-2"
		req := httptest.NewRequest(http.MethodGet, "/projects/"+otherProjectID, nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("id")
		c.SetParamValues(otherProjectID)
		c.Set("user_id", user.ID)
		c.Set("workspace_role", platauth.RoleMember)

		var perms platauth.Permission
		handler := mw(func(c echo.Context) error {
			perms = c.Get("project_permissions").(platauth.Permission)
			return c.NoContent(http.StatusOK)
		})
		err := handler(c)
		require.NoError(t, err)
		// Member fallback: PermViewContent | PermTranslate | PermManageFiles | PermRunFlows
		expected := platauth.DefaultPermissionsForRole(platauth.RoleMember)
		assert.Equal(t, expected.Permissions, perms)
	})

	t.Run("workspace-level resource resolves from workspace role", func(t *testing.T) {
		// No project ID in the path (e.g. /:ws/translation-memory): permissions
		// resolve from the user's workspace role so requirePermission enforces.
		e := echo.New()
		req := httptest.NewRequest(http.MethodGet, "/other", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.Set("user_id", user.ID)
		c.Set("workspace_role", platauth.RoleMember)

		var perms platauth.Permission
		handler := mw(func(c echo.Context) error {
			perms = c.Get("project_permissions").(platauth.Permission)
			return c.NoContent(http.StatusOK)
		})
		err := handler(c)
		require.NoError(t, err)
		expected := platauth.DefaultPermissionsForRole(platauth.RoleMember)
		assert.Equal(t, expected.Permissions, perms)
	})
}

func TestRequirePermission(t *testing.T) {
	s := &Server{}

	t.Run("has permission", func(t *testing.T) {
		e := echo.New()
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.Set("project_permissions", platauth.PermViewContent|platauth.PermTranslate)

		err := s.requirePermission(c, platauth.PermTranslate)
		assert.NoError(t, err)
	})

	t.Run("missing permission", func(t *testing.T) {
		e := echo.New()
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.Set("project_permissions", platauth.PermViewContent)

		err := s.requirePermission(c, platauth.PermManageProject)
		require.NoError(t, err) // echo writes to response, returns nil
		assert.Equal(t, http.StatusForbidden, rec.Code)
	})

	t.Run("no permissions on context denies (fail-closed)", func(t *testing.T) {
		e := echo.New()
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		// Fail-closed: when no permission context was resolved on the request,
		// deny rather than silently allow.
		err := s.requirePermission(c, platauth.PermViewContent)
		require.NoError(t, err) // echo writes to response, returns nil
		assert.Equal(t, http.StatusForbidden, rec.Code)
	})
}

func TestRequireLanguagePermission(t *testing.T) {
	s := &Server{}

	t.Run("allowed language", func(t *testing.T) {
		e := echo.New()
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.Set("project_permissions", platauth.PermViewContent|platauth.PermTranslate)
		c.Set("project_languages", []string{"fr", "de"})

		err := s.requireLanguagePermission(c, platauth.PermTranslate, "fr")
		assert.NoError(t, err)
	})

	t.Run("disallowed language", func(t *testing.T) {
		e := echo.New()
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.Set("project_permissions", platauth.PermViewContent|platauth.PermTranslate)
		c.Set("project_languages", []string{"fr", "de"})

		err := s.requireLanguagePermission(c, platauth.PermTranslate, "ja")
		require.NoError(t, err)
		assert.Equal(t, http.StatusForbidden, rec.Code)
	})

	t.Run("empty languages allows all", func(t *testing.T) {
		e := echo.New()
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.Set("project_permissions", platauth.PermViewContent|platauth.PermTranslate)
		c.Set("project_languages", []string{})

		err := s.requireLanguagePermission(c, platauth.PermTranslate, "ja")
		assert.NoError(t, err)
	})
}

func TestScopeRestrictionMiddleware(t *testing.T) {
	mw := ScopeRestrictionMiddleware()

	t.Run("no api token skips", func(t *testing.T) {
		e := echo.New()
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.Set("project_permissions", platauth.PermAll)

		called := false
		handler := mw(func(c echo.Context) error {
			called = true
			return nil
		})
		_ = handler(c)
		assert.True(t, called)
		assert.Equal(t, platauth.PermAll, c.Get("project_permissions"))
	})

	t.Run("full access scope passes through", func(t *testing.T) {
		e := echo.New()
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.Set("project_permissions", platauth.PermAll)
		c.Set("api_token_scopes", `["*"]`)

		called := false
		handler := mw(func(c echo.Context) error {
			called = true
			return nil
		})
		_ = handler(c)
		assert.True(t, called)
		assert.Equal(t, platauth.PermAll, c.Get("project_permissions"))
	})

	t.Run("read scope restricts to view_content", func(t *testing.T) {
		e := echo.New()
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.Set("project_permissions", platauth.PermAll)
		c.Set("api_token_scopes", `["read"]`)

		handler := mw(func(c echo.Context) error { return nil })
		_ = handler(c)
		assert.Equal(t, platauth.PermViewContent, c.Get("project_permissions"))
	})

	t.Run("translate scope with languages restricts", func(t *testing.T) {
		e := echo.New()
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.Set("project_permissions", platauth.PermAll)
		c.Set("api_token_scopes", `["translate:fr,de"]`)

		handler := mw(func(c echo.Context) error { return nil })
		_ = handler(c)

		perms := c.Get("project_permissions").(platauth.Permission)
		assert.True(t, perms.Has(platauth.PermViewContent))
		assert.True(t, perms.Has(platauth.PermTranslate))
		assert.False(t, perms.Has(platauth.PermManageFiles))

		langs := c.Get("project_languages").([]string)
		assert.ElementsMatch(t, []string{"fr", "de"}, langs)
	})
}

func TestSessionGrantMiddleware(t *testing.T) {
	store := NewMemorySessionStore()

	t.Run("no session ID skips", func(t *testing.T) {
		mw := SessionGrantMiddleware(store)
		e := echo.New()
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.Set("project_permissions", platauth.PermAll)

		called := false
		handler := mw(func(c echo.Context) error {
			called = true
			return nil
		})
		_ = handler(c)
		assert.True(t, called)
		assert.Equal(t, platauth.PermAll, c.Get("project_permissions"))
	})

	t.Run("ask mode restricts to view_content", func(t *testing.T) {
		grant := CreateSessionGrantForMode("conv-1", "user-1", platauth.AgentModeAsk, platauth.PermAll, nil)
		require.NoError(t, SetSessionGrant(t.Context(), store, grant))

		mw := SessionGrantMiddleware(store)
		e := echo.New()
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.Set("project_permissions", platauth.PermAll)
		c.Set("bravo_session_id", "conv-1")

		handler := mw(func(c echo.Context) error { return nil })
		_ = handler(c)

		perms := c.Get("project_permissions").(platauth.Permission)
		assert.Equal(t, platauth.PermViewContent, perms)
		assert.Equal(t, "ask", c.Get("bravo_mode"))
	})

	t.Run("coworker mode preserves all permissions", func(t *testing.T) {
		grant := CreateSessionGrantForMode("conv-2", "user-1", platauth.AgentModeCoworker, platauth.PermAll, nil)
		require.NoError(t, SetSessionGrant(t.Context(), store, grant))

		mw := SessionGrantMiddleware(store)
		e := echo.New()
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.Set("project_permissions", platauth.PermAll)
		c.Set("bravo_session_id", "conv-2")

		handler := mw(func(c echo.Context) error { return nil })
		_ = handler(c)

		perms := c.Get("project_permissions").(platauth.Permission)
		assert.Equal(t, platauth.PermAll, perms)
	})

	t.Run("voice mode restricts to view+brand+review", func(t *testing.T) {
		grant := CreateSessionGrantForMode("conv-3", "user-1", platauth.AgentModeVoice, platauth.PermAll, nil)
		require.NoError(t, SetSessionGrant(t.Context(), store, grant))

		mw := SessionGrantMiddleware(store)
		e := echo.New()
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.Set("project_permissions", platauth.PermAll)
		c.Set("bravo_session_id", "conv-3")

		handler := mw(func(c echo.Context) error { return nil })
		_ = handler(c)

		perms := c.Get("project_permissions").(platauth.Permission)
		assert.True(t, perms.Has(platauth.PermViewContent))
		assert.True(t, perms.Has(platauth.PermManageBrand))
		assert.True(t, perms.Has(platauth.PermReview))
		assert.False(t, perms.Has(platauth.PermTranslate))
		assert.False(t, perms.Has(platauth.PermManageFiles))
	})
}
