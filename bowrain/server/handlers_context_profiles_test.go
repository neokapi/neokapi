package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/bowrain/knowledge"
	coreprofile "github.com/neokapi/neokapi/core/profile"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProfilePointSlug(t *testing.T) {
	tests := []struct {
		name   string
		coords map[string]string
		want   string
	}{
		{"no coordinates is the default point", nil, "default"},
		{"an empty map is the default point", map[string]string{}, "default"},
		{"one axis", map[string]string{"product": "bowrain"}, "product~bowrain"},
		{
			"axes sort alphabetically, not in authored order",
			map[string]string{"product": "bowrain", "channel": "docs"},
			"channel~docs.product~bowrain",
		},
		{
			"a hyphen in a value cannot collide with an axis name",
			map[string]string{"a": "b-c"},
			"a~b-c",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, profilePointSlug(tt.coords))
		})
	}
}

// The point's name follows core/project.ProfileBinding.ConventionalName, so a
// project's `.kapi/profiles/<name>/` directory and this label read the same.
func TestProfilePointName(t *testing.T) {
	assert.Equal(t, "", profilePointName(nil))
	assert.Equal(t, "bowrain", profilePointName(map[string]string{"product": "bowrain"}))
	assert.Equal(t, "de-bowrain", profilePointName(map[string]string{"product": "bowrain", "market": "de"}))
	assert.Equal(t, "de-bowrain", profilePointName(map[string]string{"market": "de", "product": "bowrain"}))
}

func TestProfilePointsGroupsCollectionsByPoint(t *testing.T) {
	points := newProfilePoints()
	project := &store.Project{ID: "p1", Name: "Web"}

	points.add(project, &store.Collection{
		ID: "c1", Name: "marketing", ItemLabel: "page", Owner: "recipe",
		Context: map[string]string{"product": "bowrain", "channel": "app"},
		ConnectorConfig: map[string]string{
			coreprofile.PropertyProfileID: "v-app",
			coreprofile.PropertyChannel:   "app",
		},
	})
	points.add(project, &store.Collection{
		ID: "c2", Name: "help", ItemLabel: "article",
		Context:         map[string]string{"product": "bowrain", "channel": "app"},
		ConnectorConfig: map[string]string{coreprofile.PropertyProfileID: "v-app"},
	})
	points.add(project, &store.Collection{ID: "c3", Name: "uploads", ItemLabel: "item"})

	voices := map[string]*ContextProfileVoice{"v-app": {ID: "v-app", Name: "App voice", Version: 2}}
	profiles := points.profiles(voices)

	require.Len(t, profiles, 2, "two collections at one point are one profile, plus the default")
	assert.True(t, profiles[0].IsDefault, "the default profile sorts first")
	assert.Equal(t, "Brand", profiles[0].Label)
	assert.Len(t, profiles[0].Collections, 1)
	assert.True(t, profiles[0].Declared, "a collection with no coordinates declares the default point")

	point := profiles[1]
	assert.Equal(t, "channel~app.product~bowrain", point.Slug)
	assert.Equal(t, "app-bowrain", point.Name)
	assert.Equal(t, "app", point.Channel)
	assert.Len(t, point.Collections, 2)
	require.NotNil(t, point.Voice)
	assert.Equal(t, "App voice", point.Voice.Name)
	assert.Equal(t, 2, point.Voice.Version)
	assert.Equal(t, []string{"channel", "product"}, points.axes())

	// Owner is normalized, so a row that never carried one reads "workspace"
	// rather than empty.
	assert.Equal(t, "recipe", point.Collections[1].Owner, "marketing sorts after help")
	assert.Equal(t, "workspace", point.Collections[0].Owner)
}

// A workspace that has never been pushed to still has its own point.
func TestProfilePointsAlwaysHasTheDefault(t *testing.T) {
	profiles := newProfilePoints().profiles(nil)
	require.Len(t, profiles, 1)
	assert.Equal(t, "default", profiles[0].Slug)
	assert.True(t, profiles[0].IsDefault)
	assert.False(t, profiles[0].Declared, "nothing has been pushed")
	assert.Empty(t, profiles[0].Collections)
}

func TestUnboundVoiceProfiles(t *testing.T) {
	voices := map[string]*ContextProfileVoice{
		"v1": {ID: "v1", Name: "Support voice"},
		"v2": {ID: "v2", Name: "Core voice"},
	}
	out := unboundVoiceProfiles(voices, map[string]bool{"v2": true})

	require.Len(t, out, 1, "a bound voice is reported on its point, not here")
	assert.Equal(t, "voice~v1", out[0].Slug)
	assert.Equal(t, "Support voice", out[0].Label)
	assert.False(t, out[0].Declared)
	assert.Empty(t, out[0].Coordinates)
}

func TestVoiceProfileIDsInOps(t *testing.T) {
	payload := func(v any) json.RawMessage {
		raw, err := json.Marshal(v)
		require.NoError(t, err)
		return raw
	}
	ops := []*knowledge.ChangeSetOp{
		{Op: knowledge.OpVoiceRuleAdd, Payload: payload(map[string]string{"profile_id": "v1"})},
		{Op: knowledge.OpVoiceRuleRemove, Payload: payload(map[string]string{"profile_id": "v1"})},
		{Op: knowledge.OpVoiceRuleAdd, Payload: payload(map[string]string{"profile_id": "v2"})},
		// A concept edit is workspace-wide: it belongs to no point.
		{Op: knowledge.OpTermStatus, Payload: payload(map[string]string{"concept_id": "term:1"})},
		{Op: knowledge.OpVoiceRuleAdd, Payload: json.RawMessage(`not json`)},
		nil,
	}

	got := voiceProfileIDsInOps(ops)
	assert.Equal(t, map[string]bool{"v1": true, "v2": true}, got)
}

// The endpoint is a read over the real stores, so it runs against the
// container-backed database like the rest of the package (skipped in -short).
func TestListContextProfilesEndpoint(t *testing.T) {
	srv, token := newTestServer(t)
	e := srv.GetEcho()
	ctx := t.Context()

	project := &store.Project{
		ID: "proj-profiles", Name: "Web", WorkspaceID: "test-ws",
		DefaultSourceLanguage: "en",
	}
	require.NoError(t, srv.ContentStore.CreateProject(ctx, project))
	require.NoError(t, srv.ContentStore.CreateCollection(ctx, &store.Collection{
		ID: "col-docs", ProjectID: project.ID, Name: "docs", Stream: "main",
		Context: map[string]string{"product": "bowrain", "channel": "docs"},
		Owner:   "recipe",
	}))
	require.NoError(t, srv.ContentStore.CreateCollection(ctx, &store.Collection{
		ID: "col-uploads", ProjectID: project.ID, Name: "uploads", Stream: "main",
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/test/profiles", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	var resp ContextProfilesResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

	assert.Equal(t, []string{"channel", "product"}, resp.Axes)
	assert.Equal(t, "workspace", resp.ScanScope, "a scan carries no coordinates")
	require.Len(t, resp.Profiles, 2)
	assert.True(t, resp.Profiles[0].IsDefault)
	assert.Equal(t, "docs-bowrain", resp.Profiles[1].Name)
	assert.Equal(t, map[string]string{"product": "bowrain", "channel": "docs"}, resp.Profiles[1].Coordinates)
}
