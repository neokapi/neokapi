package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	"github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/venue"
)

// These responses are built from maps. Each is asked for repeatedly and must
// come back byte for byte the same, in its documented order: an answer that
// reorders itself defeats response caching and makes two identical answers
// diff as changed.

func TestInfoResponseIsStable(t *testing.T) {
	e := newPulseTestServer(t).GetEcho()

	var first string
	for i := range 20 {
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/info", nil))
		require.Equal(t, http.StatusOK, rec.Code)
		if i == 0 {
			first = rec.Body.String()
			var info InfoResponse
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &info))
			require.Greater(t, len(info.ConnectorTypes), 1, "the server registers more than one connector type")
			for j := 1; j < len(info.ConnectorTypes); j++ {
				assert.Less(t, info.ConnectorTypes[j-1].Name, info.ConnectorTypes[j].Name, "connector types are sorted by name")
			}
			continue
		}
		assert.Equal(t, first, rec.Body.String())
	}
}

// The Pulse pages rank languages by translated percentage, then by locale. A
// project with no content yet gives every target language 0%, so the locale
// decides the whole order.
func TestPulseLanguageListsAreStable(t *testing.T) {
	srv := newPulseTestServer(t)
	ws := &platauth.Workspace{ID: "ws-1", Name: "Test", Slug: "test"}
	require.NoError(t, srv.ContentStore.CreateProject(t.Context(), &store.Project{
		ID:                    "p-pub",
		Name:                  "Public",
		WorkspaceID:           ws.ID,
		DashboardVisibility:   "public",
		DefaultSourceLanguage: "en",
		TargetLanguages:       []model.LocaleID{"sv", "de", "nb", "fr", "ja", "es"},
	}))
	want := []string{"de", "es", "fr", "ja", "nb", "sv"}

	for _, tc := range []struct {
		name    string
		handle  func(srv *Server) func() (*httptest.ResponseRecorder, error)
		locales func(body []byte) []string
	}{
		{
			name: "overview top languages",
			locales: func(body []byte) []string {
				var o store.PulseOverview
				require.NoError(t, json.Unmarshal(body, &o))
				return pulseLocales(o.TopLanguages)
			},
			handle: func(srv *Server) func() (*httptest.ResponseRecorder, error) {
				return func() (*httptest.ResponseRecorder, error) {
					c, rec := pulseContext(srv.GetEcho(), http.MethodGet, "/api/v1/pulse/test", ws)
					return rec, srv.HandlePulseOverview(c)
				}
			},
		},
		{
			name: "leaderboard languages",
			locales: func(body []byte) []string {
				var lb store.PulseLeaderboard
				require.NoError(t, json.Unmarshal(body, &lb))
				return pulseLocales(lb.Languages)
			},
			handle: func(srv *Server) func() (*httptest.ResponseRecorder, error) {
				return func() (*httptest.ResponseRecorder, error) {
					c, rec := pulseContext(srv.GetEcho(), http.MethodGet, "/api/v1/pulse/test/leaderboard", ws)
					return rec, srv.HandlePulseLeaderboard(c)
				}
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			serve := tc.handle(srv)
			var first string
			for i := range 20 {
				// The responses are cached per workspace; each call computes anew.
				srv.pulseCache.InvalidateWorkspace(ws.ID)
				rec, err := serve()
				require.NoError(t, err)
				require.Equal(t, http.StatusOK, rec.Code)
				if i == 0 {
					first = rec.Body.String()
					assert.Equal(t, want, tc.locales(rec.Body.Bytes()))
					continue
				}
				assert.Equal(t, first, rec.Body.String())
			}
		})
	}
}

func pulseLocales(ranks []store.PulseLanguageRank) []string {
	out := make([]string, 0, len(ranks))
	for _, r := range ranks {
		out = append(out, r.Locale)
	}
	return out
}

// A higher percentage ranks first; equal percentages go by locale.
func TestRankPulseLanguages(t *testing.T) {
	langs := []store.PulseLanguageRank{
		{Locale: "sv", Percentage: 10}, {Locale: "de", Percentage: 50},
		{Locale: "nb", Percentage: 10}, {Locale: "fr", Percentage: 90},
	}
	rankPulseLanguages(langs)
	assert.Equal(t, []string{"fr", "de", "nb", "sv"}, pulseLocales(langs))
}

// A pull lists the media of the items it returns by item name, the same on
// every call.
func TestSyncPullMediaIsStable(t *testing.T) {
	srv, token := newTestServer(t)
	e := srv.GetEcho()
	pid := createProject(t, srv, token)
	items := []string{"f.json", "b.json", "e.json", "a.json", "d.json", "c.json"}
	var blocks []pushBlockItem
	for i, item := range items {
		blocks = append(blocks, pushBlockItem{ID: "b" + item, Text: "Text " + strconv.Itoa(i), ItemName: item})
	}
	pushBlocks(t, srv, e, "Bearer "+token, pid, blocks)
	for _, item := range items {
		require.NoError(t, srv.ContentStore.StoreAsset(t.Context(), pid, "main", &venue.Asset{
			ID: "asset-" + item, ProjectID: pid, ItemName: item, SourceID: "img", BlobKey: "blob-" + item, MimeType: "image/png", Filename: "img.png",
		}))
	}

	var first string
	for i := range 20 {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/projects/"+pid+"/sync/main/pull?cursor=0", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		if i == 0 {
			first = rec.Body.String()
			var resp struct {
				Media []struct {
					ItemName string `json:"item_name"`
				} `json:"media"`
			}
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
			var got []string
			for _, m := range resp.Media {
				got = append(got, m.ItemName)
			}
			assert.Equal(t, []string{"a.json", "b.json", "c.json", "d.json", "e.json", "f.json"}, got)
			continue
		}
		assert.Equal(t, first, rec.Body.String())
	}
}

// The platform-config audit record names the changed keys in sorted order.
func TestJoinKeysIsSorted(t *testing.T) {
	m := map[string]json.RawMessage{"zeta": nil, "alpha": nil, "mu": nil, "beta": nil, "omega": nil}
	for range 20 {
		assert.Equal(t, "alpha,beta,mu,omega,zeta", joinKeys(m))
	}
}
