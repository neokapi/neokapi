package backend

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/neokapi/neokapi/core/project"
	apiclient "github.com/neokapi/neokapi/host/venue/client"
	"github.com/neokapi/neokapi/host/venue/config"
	"github.com/neokapi/neokapi/host/venue/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zalando/go-keyring"
	"gopkg.in/yaml.v3"
)

// previewFixture is one of the screen fixtures host/screens writes, the JSON
// a Bowrain server answers each screen route with.
func previewFixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "..", "packages", "project-screens", "fixtures", name))
	require.NoError(t, err)
	return data
}

// previewServer answers the four screen routes of project acme/p1 with the
// recorded JSON, to the bearer token "tok" alone, and refuses the project
// acme/locked with the server's 403 body.
func previewServer(t *testing.T) *httptest.Server {
	t.Helper()
	routes := map[string][]byte{
		"/api/v1/acme/p1/map/main":          previewFixture(t, "map-summary.json"),
		"/api/v1/acme/p1/reader/main":       previewFixture(t, "reader-file.json"),
		"/api/v1/acme/p1/rules/main/decide": previewFixture(t, "rules-decide-view.json"),
		"/api/v1/acme/p1/rules/main/kept":   previewFixture(t, "kept-rules.json"),
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Header.Get("Authorization") != "Bearer tok" {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":"invalid token"}`))
			return
		}
		if r.URL.Path == "/api/v1/acme/locked/map/main" {
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"error":"insufficient project permissions"}`))
			return
		}
		if r.URL.Path == "/api/v1/acme/p1/reader/main" && r.URL.Query().Get("item") != "docs/billing.md" {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":"no such item"}`))
			return
		}
		body, ok := routes[r.URL.Path]
		if !ok || r.Method != http.MethodGet {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":"project not found"}`))
			return
		}
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// previewApp is an app with one tab whose recipe connects to url.
func previewApp(t *testing.T, url string) *App {
	t.Helper()
	return venueApp(&project.KapiProject{
		Extras: map[string]yaml.Node{schema.VenueKey: serverExtra(t, "url: "+url+"\n")},
	})
}

// signIn stores a sign-in for server with token, in a throwaway config
// directory and a keychain held in memory.
func signIn(t *testing.T, server, token string) {
	t.Helper()
	keyring.MockInit()
	t.Setenv("BOWRAIN_CONFIG_DIR", t.TempDir())
	t.Setenv("BOWRAIN_AUTH_TOKEN", "")
	require.NoError(t, config.SaveAuth(config.StoredAuth{
		ServerURL: server, AccessToken: token, User: config.StoredUser{Name: "Mira Holt"},
	}))
}

func TestBowrainPreviewReadsEachPart(t *testing.T) {
	srv := previewServer(t)
	signIn(t, srv.URL, "tok")
	a := previewApp(t, srv.URL+"/acme/p1")

	got, err := a.BowrainPreview("t", PreviewMap, "")
	require.NoError(t, err)
	require.Nil(t, got.Refusal)
	require.NotNil(t, got.Map)
	assert.Equal(t, "acme/docs", got.Map.ProjectKey)
	assert.Equal(t, 3, got.Map.Files)

	got, err = a.BowrainPreview("t", PreviewReader, "docs/billing.md")
	require.NoError(t, err)
	require.Nil(t, got.Refusal)
	require.NotNil(t, got.Reader)
	assert.Equal(t, "docs/billing.md", got.Reader.Path)

	got, err = a.BowrainPreview("t", PreviewDecide, "")
	require.NoError(t, err)
	require.Nil(t, got.Refusal)
	require.NotNil(t, got.Decide)
	assert.NotEmpty(t, got.Decide.Decisions)

	got, err = a.BowrainPreview("t", PreviewKept, "")
	require.NoError(t, err)
	require.Nil(t, got.Refusal)
	require.Len(t, got.Kept, 3)
	assert.Equal(t, "ada", got.Kept[1].KeptBy.Name, "the server's member names come through")
}

func TestBowrainPreviewRefusals(t *testing.T) {
	srv := previewServer(t)

	t.Run("the server does not take the sign-in", func(t *testing.T) {
		signIn(t, srv.URL, "expired")
		got, err := previewApp(t, srv.URL+"/acme/p1").BowrainPreview("t", PreviewMap, "")
		require.NoError(t, err)
		assert.Equal(t, &PreviewRefusal{Status: 401, Message: "invalid token", Action: PreviewActionSignIn}, got.Refusal)
		assert.Nil(t, got.Map)
	})
	t.Run("no permission on the project", func(t *testing.T) {
		signIn(t, srv.URL, "tok")
		got, err := previewApp(t, srv.URL+"/acme/locked").BowrainPreview("t", PreviewMap, "")
		require.NoError(t, err)
		assert.Equal(t, &PreviewRefusal{Status: 403, Message: "insufficient project permissions", Action: PreviewActionOpen}, got.Refusal)
	})
	t.Run("no such project", func(t *testing.T) {
		signIn(t, srv.URL, "tok")
		got, err := previewApp(t, srv.URL+"/acme/gone").BowrainPreview("t", PreviewKept, "")
		require.NoError(t, err)
		assert.Equal(t, &PreviewRefusal{Status: 404, Message: "project not found", Action: PreviewActionOpen}, got.Refusal)
	})
	t.Run("no sign-in on this Mac", func(t *testing.T) {
		keyring.MockInit()
		t.Setenv("BOWRAIN_CONFIG_DIR", t.TempDir())
		t.Setenv("BOWRAIN_AUTH_TOKEN", "")
		got, err := previewApp(t, srv.URL+"/acme/p1").BowrainPreview("t", PreviewMap, "")
		require.NoError(t, err)
		assert.Equal(t, &PreviewRefusal{Action: PreviewActionSignIn}, got.Refusal)
	})
	t.Run("the sign-in names another server, which is sent nothing", func(t *testing.T) {
		signIn(t, "https://other.example.com", "tok")
		got, err := previewApp(t, srv.URL+"/acme/p1").BowrainPreview("t", PreviewMap, "")
		require.NoError(t, err)
		assert.Equal(t, &PreviewRefusal{Action: PreviewActionSignIn}, got.Refusal)
	})
}

func TestBowrainPreviewRefusesWhatItCannotRead(t *testing.T) {
	signIn(t, "https://bowrain.example.com", "tok")
	a := previewApp(t, "https://bowrain.example.com/acme/p1")

	_, err := a.BowrainPreview("t", "settings", "")
	require.Error(t, err)
	_, err = a.BowrainPreview("t", PreviewReader, "")
	require.Error(t, err, "the reader part names a file")
	_, err = a.BowrainPreview("missing", PreviewMap, "")
	require.Error(t, err)

	_, err = previewApp(t, "http://bowrain.example.com/acme/p1").BowrainPreview("t", PreviewMap, "")
	require.ErrorContains(t, err, "unencrypted")
	_, err = previewApp(t, "https://bowrain.example.com/projects/p1").BowrainPreview("t", PreviewMap, "")
	require.ErrorContains(t, err, "no project in a workspace")
	_, err = venueApp(&project.KapiProject{}).BowrainPreview("t", PreviewMap, "")
	require.ErrorContains(t, err, "no Bowrain server")
}

func TestSignedInTo(t *testing.T) {
	signIn(t, "https://bowrain.example.com", "tok")
	ok, who := signedInTo("https://bowrain.example.com/acme/p1")
	assert.True(t, ok)
	assert.Equal(t, "Mira Holt", who)
	ok, _ = signedInTo("https://other.example.com/acme/p1")
	assert.False(t, ok)
}

// rotatingServer refreshes an access token the way the Bowrain server does:
// each refresh token works once and is replaced, and presenting a replaced
// one revokes the sign-in. Requests carrying a stale access token wait until
// `stale` of them have arrived, so they meet the expired token together.
type rotatingServer struct {
	*httptest.Server

	mu        sync.Mutex
	access    string
	refresh   string
	used      map[string]bool
	refreshes int
	revoked   bool

	stale   int
	arrived atomic.Int32
	gate    chan struct{}
}

func newRotatingServer(t *testing.T, stale int) *rotatingServer {
	t.Helper()
	rs := &rotatingServer{access: "access-1", refresh: "refresh-1", used: map[string]bool{}, stale: stale, gate: make(chan struct{})}
	kept := previewFixture(t, "kept-rules.json")
	mapJSON := previewFixture(t, "map-summary.json")
	rs.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/v1/auth/refresh" {
			rs.serveRefresh(w, r)
			return
		}
		rs.mu.Lock()
		current := rs.access
		rs.mu.Unlock()
		if r.Header.Get("Authorization") != "Bearer "+current {
			if int(rs.arrived.Add(1)) == rs.stale {
				close(rs.gate)
			}
			select {
			case <-rs.gate:
			case <-time.After(2 * time.Second):
			}
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":"invalid token"}`))
			return
		}
		switch r.URL.Path {
		case "/api/v1/acme/p1/map/main":
			_, _ = w.Write(mapJSON)
		case "/api/v1/acme/p1/rules/main/kept":
			_, _ = w.Write(kept)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(rs.Close)
	return rs
}

func (rs *rotatingServer) serveRefresh(w http.ResponseWriter, r *http.Request) {
	var body struct {
		RefreshToken string `json:"refresh_token"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	rs.mu.Lock()
	defer rs.mu.Unlock()
	if rs.used[body.RefreshToken] {
		rs.revoked = true
	}
	if rs.revoked || body.RefreshToken != rs.refresh {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"invalid refresh token"}`))
		return
	}
	rs.used[body.RefreshToken] = true
	rs.refreshes++
	rs.access = fmt.Sprintf("access-%d", rs.refreshes+1)
	rs.refresh = fmt.Sprintf("refresh-%d", rs.refreshes+1)
	_ = json.NewEncoder(w).Encode(map[string]string{"access_token": rs.access, "refresh_token": rs.refresh})
}

// signInWithRefresh stores an expired access token beside a refresh token.
func signInWithRefresh(t *testing.T, server, refresh string) {
	t.Helper()
	keyring.MockInit()
	t.Setenv("BOWRAIN_CONFIG_DIR", t.TempDir())
	t.Setenv("BOWRAIN_AUTH_TOKEN", "")
	require.NoError(t, config.SaveAuth(config.StoredAuth{
		ServerURL: server, AccessToken: "expired", RefreshToken: refresh,
	}))
}

// Two parts read at once meet an expired access token together: one of them
// refreshes it, the other retries with the new token, and the sign-in stays.
func TestBowrainPreviewRefreshesOnceForReadsAtOnce(t *testing.T) {
	rs := newRotatingServer(t, 2)
	signInWithRefresh(t, rs.URL, "refresh-1")
	a := previewApp(t, rs.URL+"/acme/p1")

	var wg sync.WaitGroup
	answers := make([]*BowrainPreviewAnswer, 2)
	errs := make([]error, 2)
	for i, part := range []string{PreviewMap, PreviewKept} {
		wg.Go(func() { answers[i], errs[i] = a.BowrainPreview("t", part, "") })
	}
	wg.Wait()

	for i := range answers {
		require.NoError(t, errs[i])
		assert.Nil(t, answers[i].Refusal)
	}
	assert.NotNil(t, answers[0].Map)
	assert.Len(t, answers[1].Kept, 3)
	rs.mu.Lock()
	defer rs.mu.Unlock()
	assert.False(t, rs.revoked, "a refresh token is presented once")
	assert.Equal(t, 1, rs.refreshes)

	stored, err := config.LoadAuth()
	require.NoError(t, err)
	assert.Equal(t, rs.refresh, stored.RefreshToken, "the rotated sign-in is stored")
}

// A refresh the server refuses asks for a sign-in, and the next read takes
// the sign-in stored after it.
func TestBowrainPreviewRefreshRefusedAsksForSignIn(t *testing.T) {
	rs := newRotatingServer(t, 1)
	signInWithRefresh(t, rs.URL, "refresh-unknown")
	a := previewApp(t, rs.URL+"/acme/p1")

	got, err := a.BowrainPreview("t", PreviewMap, "")
	require.NoError(t, err)
	assert.Equal(t, &PreviewRefusal{Status: http.StatusUnauthorized, Action: PreviewActionSignIn}, got.Refusal)

	require.NoError(t, config.SaveAuth(config.StoredAuth{ServerURL: rs.URL, AccessToken: "access-1"}))
	got, err = a.BowrainPreview("t", PreviewMap, "")
	require.NoError(t, err)
	assert.Nil(t, got.Refusal)
	assert.NotNil(t, got.Map)
}

// The token goes only to the server the sign-in it is read with names.
func TestBowrainPreviewChecksTheServerOfTheTokenItSends(t *testing.T) {
	srv := previewServer(t)
	keyring.MockInit()
	t.Setenv("BOWRAIN_CONFIG_DIR", t.TempDir())
	// BOWRAIN_AUTH_TOKEN with no server names none, which matches no recipe.
	t.Setenv("BOWRAIN_AUTH_TOKEN", "tok")
	t.Setenv("BOWRAIN_SERVER_URL", "")
	got, err := previewApp(t, srv.URL+"/acme/p1").BowrainPreview("t", PreviewMap, "")
	require.NoError(t, err)
	assert.Equal(t, &PreviewRefusal{Action: PreviewActionSignIn}, got.Refusal)
}

func TestPreviewRefusalKeepsMarkupOut(t *testing.T) {
	got := previewRefusal(apiclient.NewStatusError("map", http.StatusBadGateway, []byte("<html><body><h1>502 Bad Gateway</h1></body></html>")))
	assert.Equal(t, &PreviewRefusal{Status: http.StatusBadGateway, Message: "Bad Gateway"}, got)
	got = previewRefusal(apiclient.NewStatusError("map", http.StatusBadGateway, []byte("upstream timed out")))
	assert.Equal(t, "upstream timed out", got.Message)
}

// A refresh that reached no server names no action, and the sign-in stays.
func TestPreviewRefusalOfAnUnreachedRefresh(t *testing.T) {
	got := previewRefusal(&apiclient.RefreshError{Err: errors.New("dial tcp: connection refused")})
	assert.Equal(t, &PreviewRefusal{Message: "token refresh failed: dial tcp: connection refused"}, got)
	got = previewRefusal(&apiclient.RefreshError{StatusCode: http.StatusUnauthorized, Err: errors.New("refresh failed with HTTP 401")})
	assert.Equal(t, &PreviewRefusal{Status: http.StatusUnauthorized, Action: PreviewActionSignIn}, got)
}

// kapi in another process refreshes the shared sign-in while the desktop
// holds the pair it read first. The desktop's next refresh starts from the
// pair the keychain holds then, so no consumed refresh token is presented.
func TestBowrainPreviewRefreshesFromTheKeychainAfterAnotherProcessRefreshed(t *testing.T) {
	rs := newRotatingServer(t, 1)
	keyring.MockInit()
	t.Setenv("BOWRAIN_CONFIG_DIR", t.TempDir())
	t.Setenv("BOWRAIN_AUTH_TOKEN", "")
	require.NoError(t, config.SaveAuth(config.StoredAuth{ServerURL: rs.URL, AccessToken: "access-1", RefreshToken: "refresh-1"}))
	a := previewApp(t, rs.URL+"/acme/p1")

	got, err := a.BowrainPreview("t", PreviewMap, "")
	require.NoError(t, err)
	require.Nil(t, got.Refusal, "the first read caches access-1 and refresh-1")

	// The CLI refreshes: refresh-1 is consumed, and access-1 no longer works.
	resp, err := http.Post(rs.URL+"/api/v1/auth/refresh", "application/json", strings.NewReader(`{"refresh_token":"refresh-1"}`))
	require.NoError(t, err)
	var rotated struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&rotated))
	require.NoError(t, resp.Body.Close())
	require.NoError(t, config.SaveAuth(config.StoredAuth{ServerURL: rs.URL, AccessToken: rotated.AccessToken, RefreshToken: rotated.RefreshToken}))
	rs.mu.Lock()
	rs.access = "access-expired"
	rs.mu.Unlock()

	got, err = a.BowrainPreview("t", PreviewKept, "")
	require.NoError(t, err)
	assert.Nil(t, got.Refusal)
	assert.Len(t, got.Kept, 3)
	rs.mu.Lock()
	defer rs.mu.Unlock()
	assert.False(t, rs.revoked, "the consumed refresh-1 is never presented again")
	assert.Equal(t, 2, rs.refreshes)
}

// A sign-in removed while the sheet is open ends the cached one at the next
// read.
func TestBowrainPreviewStopsAfterSignOut(t *testing.T) {
	srv := previewServer(t)
	signIn(t, srv.URL, "tok")
	a := previewApp(t, srv.URL+"/acme/p1")

	got, err := a.BowrainPreview("t", PreviewMap, "")
	require.NoError(t, err)
	require.Nil(t, got.Refusal)

	require.NoError(t, config.DeleteAuth(srv.URL))
	got, err = a.BowrainPreview("t", PreviewMap, "")
	require.NoError(t, err)
	assert.Equal(t, &PreviewRefusal{Action: PreviewActionSignIn}, got.Refusal)
	assert.Nil(t, got.Map)
}
