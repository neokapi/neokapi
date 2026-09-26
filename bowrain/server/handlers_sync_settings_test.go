package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/venue"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// commitWithSettings posts a push commit carrying recipe-owned project settings
// and nothing else, as a caller holding perms.
func commitWithSettings(t *testing.T, s *Server, projectID string, perms platauth.Permission,
	settings venue.ProjectSettings,
) *httptest.ResponseRecorder {
	t.Helper()
	payload, err := json.Marshal(map[string]any{"settings": settings})
	require.NoError(t, err)

	r := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(payload))
	r.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := s.GetEcho().NewContext(r, rec)
	c.SetParamNames("ws", "id", "ref")
	c.SetParamValues("test", projectID, "main")
	c.Set("workspace_id", "test-ws")
	c.Set("user_id", "test-user")
	c.Set("project_permissions", perms)
	// A refusal writes its 403 and returns errAccessDenied; the recorder
	// carries the status either way.
	if err := s.HandleSyncPushCommit(c); err != nil {
		require.ErrorIs(t, err, errAccessDenied)
	}
	return rec
}

// initSettings runs a push init and returns the settings the server reports.
func initSettings(t *testing.T, s *Server, authHeader, projectID string) venue.ProjectSettings {
	t.Helper()
	body, _ := json.Marshal(map[string]any{
		"item_hashes":         map[string]string{},
		"content_model_epoch": venue.ContentModelEpoch,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/projects/"+projectID+"/sync/main/push/init", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", authHeader)
	rec := httptest.NewRecorder()
	s.GetEcho().ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var resp struct {
		Settings venue.ProjectSettings `json:"settings"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	return resp.Settings
}

// TestSyncPush_RecipeSettings: the push negotiation reports the recipe-owned
// settings the project holds, and a commit carrying settings puts them in force
// for anyone who may push, with no project-management permission.
func TestSyncPush_RecipeSettings(t *testing.T) {
	srv, token := newTestServer(t)
	pid := createProject(t, srv, token)
	ctx := t.Context()

	assert.Equal(t, venue.ProjectSettings{
		venue.SettingConvergePolicy: platstore.ConvergePolicyOnPush,
		venue.SettingTranslateAfter: string(model.TranslateAfterWritten),
	}, initSettings(t, srv, "Bearer "+token, pid), "a new project reports the defaults")

	contribute, err := platauth.ParseScope("contribute")
	require.NoError(t, err)
	pushers := map[string]platauth.Permission{
		"a workspace member": platauth.DefaultPermissionsForRole(platauth.RoleMember).Permissions,
		"a contribute token": contribute.Permissions,
	}
	for who, perms := range pushers {
		require.False(t, perms.Has(platauth.PermManageProject), "%s cannot manage the project", who)
	}

	t.Run("a pusher puts the recipe's settings in force", func(t *testing.T) {
		rec := commitWithSettings(t, srv, pid, pushers["a contribute token"], venue.ProjectSettings{
			venue.SettingConvergePolicy: platstore.ConvergePolicyManual,
			venue.SettingTranslateAfter: string(model.TranslateAfterEstablished),
		})
		require.Equal(t, http.StatusAccepted, rec.Code, rec.Body.String())

		proj, err := srv.ContentStore.GetProject(ctx, pid)
		require.NoError(t, err)
		assert.Equal(t, platstore.ConvergePolicyManual, proj.ConvergePolicy)
		assert.Equal(t, model.TranslateAfterEstablished, platstore.TranslateAfterFor(proj),
			"the translate_after hold the server applies is the recipe's")

		assert.Equal(t, venue.ProjectSettings{
			venue.SettingConvergePolicy: platstore.ConvergePolicyManual,
			venue.SettingTranslateAfter: string(model.TranslateAfterEstablished),
		}, initSettings(t, srv, "Bearer "+token, pid), "the next negotiation reads what was written")
	})

	t.Run("a member returns the settings to their defaults", func(t *testing.T) {
		rec := commitWithSettings(t, srv, pid, pushers["a workspace member"], venue.ProjectSettings{
			venue.SettingConvergePolicy: platstore.ConvergePolicyOnPush,
			venue.SettingTranslateAfter: string(model.TranslateAfterWritten),
		})
		require.Equal(t, http.StatusAccepted, rec.Code, rec.Body.String())

		proj, err := srv.ContentStore.GetProject(ctx, pid)
		require.NoError(t, err)
		assert.Equal(t, platstore.ConvergePolicyOnPush, proj.ConvergePolicy)
		assert.Equal(t, model.TranslateAfterWritten, platstore.TranslateAfterFor(proj))
	})

	t.Run("a value outside the recipe schema is refused before anything is written", func(t *testing.T) {
		rec := commitWithSettings(t, srv, pid, pushers["a workspace member"], venue.ProjectSettings{
			venue.SettingConvergePolicy: platstore.ConvergePolicyManual,
			venue.SettingTranslateAfter: "approved",
		})
		require.Equal(t, http.StatusBadRequest, rec.Code)
		assert.Contains(t, rec.Body.String(), "translate_after")

		proj, err := srv.ContentStore.GetProject(ctx, pid)
		require.NoError(t, err)
		assert.Equal(t, platstore.ConvergePolicyOnPush, proj.ConvergePolicy,
			"a refused commit leaves every setting as it was")
	})

	t.Run("a caller who may not push sets nothing", func(t *testing.T) {
		rec := commitWithSettings(t, srv, pid, platauth.PermViewContent, venue.ProjectSettings{
			venue.SettingConvergePolicy: platstore.ConvergePolicyManual,
		})
		require.Equal(t, http.StatusForbidden, rec.Code)

		proj, err := srv.ContentStore.GetProject(ctx, pid)
		require.NoError(t, err)
		assert.Equal(t, platstore.ConvergePolicyOnPush, proj.ConvergePolicy)
	})
}
