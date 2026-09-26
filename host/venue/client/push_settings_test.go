package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/venue"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// settingsTestServer answers the push init with the settings it holds and a
// status, and records every commit it receives.
type settingsTestServer struct {
	commits []PushCommitRequest
}

func newSettingsTestServer(t *testing.T, status string, held venue.ProjectSettings) (*settingsTestServer, *BowrainClient) {
	t.Helper()
	rec := &settingsTestServer{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/push/init"):
			_ = json.NewEncoder(w).Encode(PushInitResponse{
				UploadID: "up1",
				Status:   status,
				Settings: held,
			})
		case strings.HasSuffix(r.URL.Path, "/push/commit"):
			var req PushCommitRequest
			_ = json.NewDecoder(r.Body).Decode(&req)
			rec.commits = append(rec.commits, req)
			_ = json.NewEncoder(w).Encode(SyncPushResponse{PushID: "push1"})
		default:
			w.WriteHeader(http.StatusOK)
		}
	}))
	t.Cleanup(srv.Close)
	return rec, NewClaimTokenClient(srv.URL, "proj1", "tok")
}

func recipeSettings(converge, translateAfter string) venue.ProjectSettings {
	return venue.ProjectSettings{
		venue.SettingConvergePolicy: converge,
		venue.SettingTranslateAfter: translateAfter,
	}
}

func settingsPushContext(s venue.ProjectSettings) *PushContext {
	pc := NewPushContext(nil)
	pc.Settings = s
	return pc
}

// TestPushCarriesOnlyDifferingSettings pins the read-before-write: the push
// compares the recipe's settings with what the venue reported at init and the
// commit carries only the one that differs.
func TestPushCarriesOnlyDifferingSettings(t *testing.T) {
	rec, c := newSettingsTestServer(t, "diff_computed", recipeSettings("on-push", "written"))

	resp, err := c.Push(context.Background(), nil, nil,
		settingsPushContext(recipeSettings("on-push", "established")), nil)
	require.NoError(t, err)

	require.Len(t, rec.commits, 1)
	assert.Equal(t, venue.ProjectSettings{venue.SettingTranslateAfter: "established"}, rec.commits[0].Settings)
	assert.True(t, resp.SettingsInForce)
}

// TestPushUnchangedSettingsSendNothing pins that a venue already holding the
// recipe's settings is sent none, and that an otherwise unchanged push still
// takes the no-commit exit.
func TestPushUnchangedSettingsSendNothing(t *testing.T) {
	rec, c := newSettingsTestServer(t, "unchanged", recipeSettings("manual", "none"))

	resp, err := c.Push(context.Background(), nil, nil,
		settingsPushContext(recipeSettings("manual", "none")), nil)
	require.NoError(t, err)

	assert.Empty(t, rec.commits, "nothing differs, so nothing is committed")
	assert.Equal(t, PushUnchanged, resp.PushID)
	assert.True(t, resp.SettingsInForce, "the venue reported holding the recipe's settings")
}

// TestPushSettingsOnlyChangeCommits pins that a recipe whose only change is a
// setting still reaches the venue: an unchanged negotiation proceeds to an
// empty commit carrying the setting.
func TestPushSettingsOnlyChangeCommits(t *testing.T) {
	rec, c := newSettingsTestServer(t, "unchanged", recipeSettings("on-push", "written"))

	resp, err := c.Push(context.Background(), nil, nil,
		settingsPushContext(recipeSettings("manual", "written")), nil)
	require.NoError(t, err)

	require.Len(t, rec.commits, 1)
	assert.Empty(t, rec.commits[0].Chunks)
	assert.Equal(t, venue.ProjectSettings{venue.SettingConvergePolicy: "manual"}, rec.commits[0].Settings)
	assert.True(t, resp.SettingsInForce)
}

// TestPushSendsNoSettingsToAVenueThatReportsNone pins the older-server case: a
// venue that reports no settings at init takes none on a push, so none are
// sent and the push does not record them as in force.
func TestPushSendsNoSettingsToAVenueThatReportsNone(t *testing.T) {
	rec, c := newSettingsTestServer(t, "diff_computed", nil)

	resp, err := c.Push(context.Background(), nil, nil,
		settingsPushContext(recipeSettings("manual", "none")), nil)
	require.NoError(t, err)

	require.Len(t, rec.commits, 1)
	assert.Nil(t, rec.commits[0].Settings)
	assert.False(t, resp.SettingsInForce)
}
