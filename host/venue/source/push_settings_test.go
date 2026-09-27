package source

import (
	"encoding/json"
	"maps"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/ref"
	"github.com/neokapi/neokapi/core/venue"
	bowrainconn "github.com/neokapi/neokapi/core/venue/connector"
	apiclient "github.com/neokapi/neokapi/host/venue/client"
)

// settingsServer is a venue that holds recipe-owned project settings, reports
// them at push init and applies the ones a commit carries, counting both. It
// serves no tree, so the producer diffs against its own sync cache and an
// unchanged checkout has no content to send.
type settingsServer struct {
	*httptest.Server

	mu      sync.Mutex
	held    venue.ProjectSettings
	inits   int
	commits []apiclient.PushCommitRequest
}

func newSettingsServer(t *testing.T, projectID string, held venue.ProjectSettings) *settingsServer {
	t.Helper()
	ss := &settingsServer{held: held}
	// The venue already holds the (empty) declared context the pushes carry,
	// so settings are the only thing that can move.
	published := ref.Ref{Content: 1, Context: venue.ContextHashOf(nil)}
	mux := http.NewServeMux()
	base := "/api/v1/projects/" + projectID
	mux.HandleFunc(base, func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(apiclient.ProjectMetadata{ID: projectID, DefaultSourceLanguage: "en", TargetLanguages: []string{"fr"}})
	})
	mux.HandleFunc(base+"/sync/main/ref", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(published)
	})
	mux.HandleFunc(base+"/sync/main/status", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"push_id": "p1", "status": "completed", "total": 1, "completed": 1})
	})
	mux.HandleFunc(base+"/sync/main/tree", func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "no tree", http.StatusNotFound)
	})
	mux.HandleFunc(base+"/sync/main/push/chunks/", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc(base+"/sync/main/push/init", func(w http.ResponseWriter, _ *http.Request) {
		ss.mu.Lock()
		ss.inits++
		held := maps.Clone(ss.held)
		ss.mu.Unlock()
		current := published
		_ = json.NewEncoder(w).Encode(apiclient.PushInitResponse{
			UploadID: "u1", Status: "diff_computed", Ref: &current, Settings: held,
		})
	})
	mux.HandleFunc(base+"/sync/main/push/commit", func(w http.ResponseWriter, r *http.Request) {
		var manifest apiclient.PushCommitRequest
		_ = json.NewDecoder(r.Body).Decode(&manifest)
		ss.mu.Lock()
		ss.commits = append(ss.commits, manifest)
		maps.Copy(ss.held, manifest.Settings)
		ss.mu.Unlock()
		w.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(w).Encode(map[string]any{"push_id": "p1", "status": "queued"})
	})
	ss.Server = httptest.NewServer(mux)
	t.Cleanup(ss.Close)
	return ss
}

func (ss *settingsServer) counts() (inits, commits int) {
	ss.mu.Lock()
	defer ss.mu.Unlock()
	return ss.inits, len(ss.commits)
}

func (ss *settingsServer) lastCommit() apiclient.PushCommitRequest {
	ss.mu.Lock()
	defer ss.mu.Unlock()
	return ss.commits[len(ss.commits)-1]
}

// TestPushCarriesRecipeSettings follows the recipe's project settings across
// three pushes: the first puts a differing setting in force, the second, with
// nothing changed, costs no round trip, and the third carries a setting the
// recipe changed even though no content moved.
func TestPushCarriesRecipeSettings(t *testing.T) {
	const projectID = "p-settings"
	srv := newSettingsServer(t, projectID, venue.ProjectSettings{
		venue.SettingConvergePolicy: "on-push",
		venue.SettingTranslateAfter: "written",
	})
	conn := scopeConnector(t, srv.Server, projectID)

	pushWith := func(converge, translateAfter string) {
		t.Helper()
		pc := apiclient.NewPushContext(nil)
		pc.Settings = venue.ProjectSettings{
			venue.SettingConvergePolicy: converge,
			venue.SettingTranslateAfter: translateAfter,
		}
		conn.SetPushContext(pc)
		_, err := conn.Push(t.Context(), bowrainconn.PushOptions{})
		require.NoError(t, err)
	}

	pushWith("manual", "written")
	inits, commits := srv.counts()
	require.Equal(t, 1, inits)
	require.Equal(t, 1, commits)
	assert.Equal(t, venue.ProjectSettings{venue.SettingConvergePolicy: "manual"}, srv.lastCommit().Settings,
		"only the setting the venue did not hold is carried")

	pushWith("manual", "written")
	inits, commits = srv.counts()
	assert.Equal(t, 1, inits, "settings the venue confirmed are not asked about again")
	assert.Equal(t, 1, commits)

	pushWith("manual", "established")
	inits, commits = srv.counts()
	assert.Equal(t, 2, inits, "a changed setting reaches the venue with no content change")
	require.Equal(t, 2, commits)
	assert.Equal(t, venue.ProjectSettings{venue.SettingTranslateAfter: "established"}, srv.lastCommit().Settings)
}
