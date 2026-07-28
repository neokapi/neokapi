package commands

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	apiclient "github.com/neokapi/neokapi/bowrain/core/client"
	coreprofile "github.com/neokapi/neokapi/core/profile"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// brandUpsertRecorder captures the upsert requests a brand push sends.
type brandUpsertRecorder struct {
	mu     sync.Mutex
	bodies []map[string]any
}

// brandUpsertServer serves POST /api/v1/{ws}/brand-profiles/upsert, recording
// every request body and answering with status + the given response.
func brandUpsertServer(t *testing.T, status int, response any) (*httptest.Server, *brandUpsertRecorder) {
	t.Helper()
	rec := &brandUpsertRecorder{}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/{ws}/brand-profiles/upsert", func(w http.ResponseWriter, r *http.Request) {
		body := decodeBody(r)
		rec.mu.Lock()
		rec.bodies = append(rec.bodies, body)
		rec.mu.Unlock()
		w.WriteHeader(status)
		if response != nil {
			_ = json.NewEncoder(w).Encode(response)
		}
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, rec
}

func testVoiceProfile() *coreprofile.VoiceProfile {
	return &coreprofile.VoiceProfile{
		Name:        "Acme Voice",
		Description: "How Acme sounds.",
		Tone:        coreprofile.ToneProfile{Formality: "neutral", Personality: []string{"clear", "direct"}},
		Vocabulary: coreprofile.VocabularyRules{
			ForbiddenTerms: []coreprofile.TermRule{{Term: "utilize", Replacement: "use"}},
		},
	}
}

func TestPushBrandProfileCreates(t *testing.T) {
	srv, rec := brandUpsertServer(t, http.StatusCreated, apiclient.BrandProfileUpsertResult{
		Action:  "created",
		Profile: &coreprofile.VoiceProfile{ID: "bp-1", Name: "Acme Voice", Version: 1},
	})
	client := apiclient.NewWorkspaceBowrainClient(srv.URL, "acme", "proj1", "tok")

	res, err := PushBrandProfile(context.Background(), client, testVoiceProfile())
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Equal(t, "Acme Voice", res.Name)
	assert.Equal(t, "created", res.Action)
	assert.Equal(t, 1, res.Version)

	// The request carried the authored surface — name and vocabulary — and no
	// server-managed metadata.
	rec.mu.Lock()
	defer rec.mu.Unlock()
	require.Len(t, rec.bodies, 1)
	body := rec.bodies[0]
	assert.Equal(t, "Acme Voice", body["name"])
	vocab, ok := body["vocabulary"].(map[string]any)
	require.True(t, ok)
	forbidden, ok := vocab["forbidden_terms"].([]any)
	require.True(t, ok)
	require.Len(t, forbidden, 1)
	assert.Equal(t, "utilize", forbidden[0].(map[string]any)["term"])
	assert.NotContains(t, body, "id")
	assert.NotContains(t, body, "workspace_id")
	assert.NotContains(t, body, "version")
}

func TestPushBrandProfileUpdatedReportsNewVersion(t *testing.T) {
	srv, _ := brandUpsertServer(t, http.StatusOK, apiclient.BrandProfileUpsertResult{
		Action:  "updated",
		Profile: &coreprofile.VoiceProfile{ID: "bp-1", Name: "Acme Voice", Version: 4},
	})
	client := apiclient.NewWorkspaceBowrainClient(srv.URL, "acme", "proj1", "tok")

	res, err := PushBrandProfile(context.Background(), client, testVoiceProfile())
	require.NoError(t, err)
	assert.Equal(t, "updated", res.Action)
	assert.Equal(t, 4, res.Version)
}

func TestPushBrandProfileUnchangedNoOps(t *testing.T) {
	srv, rec := brandUpsertServer(t, http.StatusOK, apiclient.BrandProfileUpsertResult{
		Action:  "unchanged",
		Profile: &coreprofile.VoiceProfile{ID: "bp-1", Name: "Acme Voice", Version: 2},
	})
	client := apiclient.NewWorkspaceBowrainClient(srv.URL, "acme", "proj1", "tok")

	res, err := PushBrandProfile(context.Background(), client, testVoiceProfile())
	require.NoError(t, err)
	assert.Equal(t, "unchanged", res.Action)
	assert.Equal(t, 2, res.Version)

	rec.mu.Lock()
	defer rec.mu.Unlock()
	assert.Len(t, rec.bodies, 1, "an idempotent re-push is a single upsert call")
}

func TestPushBrandProfilePermissionDeniedSkips(t *testing.T) {
	srv, _ := brandUpsertServer(t, http.StatusForbidden, map[string]string{"error": "insufficient project permissions"})
	client := apiclient.NewWorkspaceBowrainClient(srv.URL, "acme", "proj1", "tok")

	res, err := PushBrandProfile(context.Background(), client, testVoiceProfile())
	require.NoError(t, err, "a 403 must degrade to a skip, not fail the push")
	require.NotNil(t, res)
	assert.Equal(t, "skipped", res.Action)
	assert.Equal(t, "requires the manage-brand permission", res.Reason)
}

func TestPushBrandProfileServerErrorSurfaces(t *testing.T) {
	srv, _ := brandUpsertServer(t, http.StatusInternalServerError, map[string]string{"error": "boom"})
	client := apiclient.NewWorkspaceBowrainClient(srv.URL, "acme", "proj1", "tok")

	res, err := PushBrandProfile(context.Background(), client, testVoiceProfile())
	require.Error(t, err)
	assert.Nil(t, res)
	assert.Contains(t, err.Error(), "HTTP 500")
}
