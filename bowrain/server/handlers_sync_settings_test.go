package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	platev "github.com/neokapi/neokapi/bowrain/core/event"
	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/core/venue"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// settingsCommitResponse is the part of a commit response that reports the
// recipe-owned settings.
type settingsCommitResponse struct {
	Applied venue.ProjectSettings  `json:"settings_applied"`
	Refused []venue.SettingRefusal `json:"settings_refused"`
}

// commitWithSettings posts a push commit to stream carrying recipe-owned project
// settings and nothing else, as a caller holding perms.
func commitWithSettings(t *testing.T, s *Server, projectID, stream string, perms platauth.Permission,
	settings venue.ProjectSettings,
) (*httptest.ResponseRecorder, settingsCommitResponse) {
	t.Helper()
	payload, err := json.Marshal(map[string]any{"settings": settings})
	require.NoError(t, err)

	r := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(payload))
	r.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := s.GetEcho().NewContext(r, rec)
	c.SetParamNames("ws", "id", "ref")
	c.SetParamValues("test", projectID, stream)
	c.Set("workspace_id", "test-ws")
	c.Set("user_id", "test-user")
	c.Set("project_permissions", perms)
	// A refusal writes its 403 and returns errAccessDenied; the recorder
	// carries the status either way.
	if err := s.HandleSyncPushCommit(c); err != nil {
		require.ErrorIs(t, err, errAccessDenied)
	}
	var resp settingsCommitResponse
	if rec.Code == http.StatusAccepted {
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	}
	return rec, resp
}

// initSettings runs a push init declaring requested, and returns the settings
// the server holds and the ones it says the push may not apply.
func initSettings(t *testing.T, s *Server, authHeader, projectID string, requested venue.ProjectSettings) (venue.ProjectSettings, []venue.SettingRefusal) {
	t.Helper()
	body, _ := json.Marshal(map[string]any{
		"item_hashes":         map[string]string{},
		"content_model_epoch": venue.ContentModelEpoch,
		"settings":            requested,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/projects/"+projectID+"/sync/main/push/init", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", authHeader)
	rec := httptest.NewRecorder()
	s.GetEcho().ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var resp struct {
		Settings venue.ProjectSettings  `json:"settings"`
		Refused  []venue.SettingRefusal `json:"settings_refused"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	return resp.Settings, resp.Refused
}

func projectSettings(t *testing.T, s *Server, projectID string) venue.ProjectSettings {
	t.Helper()
	proj, err := s.ContentStore.GetProject(t.Context(), projectID)
	require.NoError(t, err)
	return platstore.RecipeSettingsOf(proj)
}

func settingsOf(converge, translateAfter string) venue.ProjectSettings {
	return venue.ProjectSettings{
		venue.SettingConvergePolicy: converge,
		venue.SettingTranslateAfter: translateAfter,
	}
}

// TestSyncPush_RecipeSettings: a push to the project's default stream applies a
// recipe-owned setting that tightens what the project allows for anyone who may
// push, and one that loosens it only for a pusher who may manage the project.
// Anything else is kept at the server's value and reported.
func TestSyncPush_RecipeSettings(t *testing.T) {
	srv, token := newTestServer(t)
	pid := createProject(t, srv, token)

	var mu sync.Mutex
	var audited []platev.Event
	srv.EventBus.Subscribe(platev.EventProjectSettingChanged, func(ev platev.Event) {
		mu.Lock()
		defer mu.Unlock()
		audited = append(audited, ev)
	})
	auditedCount := func() int {
		mu.Lock()
		defer mu.Unlock()
		return len(audited)
	}

	contribute, err := platauth.ParseScope("contribute")
	require.NoError(t, err)
	member := platauth.DefaultPermissionsForRole(platauth.RoleMember).Permissions
	admin := platauth.DefaultPermissionsForRole(platauth.RoleAdmin).Permissions
	require.False(t, member.Has(platauth.PermManageProject))
	require.False(t, contribute.Permissions.Has(platauth.PermManageProject))
	require.True(t, admin.Has(platauth.PermManageProject))

	held, refused := initSettings(t, srv, "Bearer "+token, pid, nil)
	assert.Equal(t, settingsOf(platstore.ConvergePolicyOnPush, "written"), held, "a new project holds the defaults")
	assert.Empty(t, refused)

	t.Run("a contribute token tightens both settings", func(t *testing.T) {
		rec, resp := commitWithSettings(t, srv, pid, "main", contribute.Permissions,
			settingsOf(platstore.ConvergePolicyManual, "established"))
		require.Equal(t, http.StatusAccepted, rec.Code, rec.Body.String())
		assert.Equal(t, settingsOf(platstore.ConvergePolicyManual, "established"), resp.Applied)
		assert.Empty(t, resp.Refused)
		assert.Equal(t, settingsOf(platstore.ConvergePolicyManual, "established"), projectSettings(t, srv, pid))

		require.Eventually(t, func() bool { return auditedCount() == 2 }, 5*time.Second, 10*time.Millisecond,
			"each applied change is audited")
		mu.Lock()
		byKey := map[string]platev.Event{}
		for _, ev := range audited {
			byKey[ev.ResourceID] = ev
		}
		mu.Unlock()
		ev := byKey[venue.SettingTranslateAfter]
		assert.Equal(t, "test-user", ev.Actor)
		assert.Equal(t, pid, ev.ProjectID)
		assert.Equal(t, map[string]string{venue.SettingTranslateAfter: "written"}, ev.Before)
		assert.Equal(t, map[string]string{venue.SettingTranslateAfter: "established"}, ev.After)
	})

	t.Run("a member cannot loosen, and is told who can", func(t *testing.T) {
		_, predicted := initSettings(t, srv, "Bearer "+token, pid, settingsOf(platstore.ConvergePolicyOnPush, "none"))
		assert.Empty(t, predicted, "the test token's owner may manage the project, so its negotiation refuses nothing")

		before := auditedCount()
		rec, resp := commitWithSettings(t, srv, pid, "main", member,
			settingsOf(platstore.ConvergePolicyOnPush, "none"))
		require.Equal(t, http.StatusAccepted, rec.Code, "a refused setting never refuses the push")
		assert.Empty(t, resp.Applied)
		assert.Equal(t, []venue.SettingRefusal{
			{Setting: venue.SettingConvergePolicy, Requested: platstore.ConvergePolicyOnPush,
				InForce: platstore.ConvergePolicyManual, Reason: venue.SettingLoosens, Requires: "manage_project"},
			{Setting: venue.SettingTranslateAfter, Requested: "none", InForce: "established",
				Reason: venue.SettingLoosens, Requires: "manage_project"},
		}, resp.Refused)
		assert.Equal(t, settingsOf(platstore.ConvergePolicyManual, "established"), projectSettings(t, srv, pid),
			"the server keeps its values")
		assert.Equal(t, before, auditedCount(), "nothing changed, so nothing is audited")
	})

	t.Run("an admin loosens", func(t *testing.T) {
		rec, resp := commitWithSettings(t, srv, pid, "main", admin, settingsOf(platstore.ConvergePolicyOnPush, "written"))
		require.Equal(t, http.StatusAccepted, rec.Code, rec.Body.String())
		assert.Equal(t, settingsOf(platstore.ConvergePolicyOnPush, "written"), resp.Applied)
		assert.Empty(t, resp.Refused)
		assert.Equal(t, settingsOf(platstore.ConvergePolicyOnPush, "written"), projectSettings(t, srv, pid))
	})

	t.Run("a push to another stream applies nothing, whoever pushes", func(t *testing.T) {
		for who, perms := range map[string]platauth.Permission{"member": member, "admin": admin} {
			rec, resp := commitWithSettings(t, srv, pid, "feature-x", perms,
				settingsOf(platstore.ConvergePolicyManual, "established"))
			require.Equal(t, http.StatusAccepted, rec.Code, who)
			assert.Empty(t, resp.Applied, who)
			require.Len(t, resp.Refused, 2, who)
			for _, r := range resp.Refused {
				assert.Equal(t, venue.SettingNotDefaultStream, r.Reason, who)
				assert.Equal(t, "main", r.DefaultStream, who)
			}
		}
		assert.Equal(t, settingsOf(platstore.ConvergePolicyOnPush, "written"), projectSettings(t, srv, pid))
	})

	t.Run("a value outside the recipe schema is refused before anything is written", func(t *testing.T) {
		rec, _ := commitWithSettings(t, srv, pid, "main", admin, venue.ProjectSettings{
			venue.SettingConvergePolicy: platstore.ConvergePolicyManual,
			venue.SettingTranslateAfter: "approved",
		})
		require.Equal(t, http.StatusBadRequest, rec.Code)
		assert.Contains(t, rec.Body.String(), "translate_after")
		assert.Equal(t, settingsOf(platstore.ConvergePolicyOnPush, "written"), projectSettings(t, srv, pid))
	})

	t.Run("a caller who may not push sets nothing", func(t *testing.T) {
		rec, _ := commitWithSettings(t, srv, pid, "main", platauth.PermViewContent,
			settingsOf(platstore.ConvergePolicyManual, "established"))
		require.Equal(t, http.StatusForbidden, rec.Code)
		assert.Equal(t, settingsOf(platstore.ConvergePolicyOnPush, "written"), projectSettings(t, srv, pid))
	})
}

// TestSyncPush_InitPredictsRefusals: the negotiation answers, for the caller
// making it, which of the recipe's settings the commit would keep, so the
// producer leaves them out and reports them without a commit.
func TestSyncPush_InitPredictsRefusals(t *testing.T) {
	srv, token := newTestServer(t)
	pid := createProject(t, srv, token)

	proj, err := srv.ContentStore.GetProject(t.Context(), pid)
	require.NoError(t, err)
	proj.ConvergePolicy = platstore.ConvergePolicyManual
	require.NoError(t, srv.ContentStore.UpdateProject(t.Context(), proj))

	body, _ := json.Marshal(map[string]any{
		"item_hashes":         map[string]string{},
		"content_model_epoch": venue.ContentModelEpoch,
		"settings":            settingsOf(platstore.ConvergePolicyOnPush, "established"),
	})
	r := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := srv.GetEcho().NewContext(r, rec)
	c.SetParamNames("ws", "id", "ref")
	c.SetParamValues("test", pid, "main")
	c.Set("user_id", "test-user")
	c.Set("project_permissions", platauth.DefaultPermissionsForRole(platauth.RoleMember).Permissions)
	require.NoError(t, srv.HandleSyncPushInit(c))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	var resp struct {
		Settings venue.ProjectSettings  `json:"settings"`
		Refused  []venue.SettingRefusal `json:"settings_refused"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, settingsOf(platstore.ConvergePolicyManual, "written"), resp.Settings)
	assert.Equal(t, []venue.SettingRefusal{{
		Setting: venue.SettingConvergePolicy, Requested: platstore.ConvergePolicyOnPush,
		InForce: platstore.ConvergePolicyManual, Reason: venue.SettingLoosens, Requires: "manage_project",
	}}, resp.Refused, "the tightening translate_after is not refused; the loosening policy is")
}
