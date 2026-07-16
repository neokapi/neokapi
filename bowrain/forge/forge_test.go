package forge

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestKindForRepo(t *testing.T) {
	assert.Equal(t, KindGitHub, KindForRepo("https://github.com/acme/site.git"))
	assert.Equal(t, KindGitHub, KindForRepo("https://www.github.com/acme/site"))
	assert.Equal(t, KindGitLab, KindForRepo("https://gitlab.com/acme/site.git"))
	assert.Equal(t, KindGitLab, KindForRepo("https://git.example.com/acme/site.git"))
}

func TestParseRepo(t *testing.T) {
	r, err := ParseRepo("https://github.com/acme/site.git")
	require.NoError(t, err)
	assert.Equal(t, Repo{Host: "github.com", Path: "acme/site"}, r)

	r, err = ParseRepo("https://gitlab.example.com/group/sub/site")
	require.NoError(t, err)
	assert.Equal(t, Repo{Host: "gitlab.example.com", Path: "group/sub/site"}, r)

	_, err = ParseRepo("git@github.com:acme/site.git")
	assert.Error(t, err, "ssh URLs cannot carry token auth or API calls")

	_, err = ParseRepo("https://github.com/")
	assert.Error(t, err)
}

func TestGitHubEnsureDeliveryPR_CreatesWhenNoneOpen(t *testing.T) {
	var createdBody map[string]any
	var labeled bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/repos/acme/site/pulls":
			assert.Equal(t, "open", r.URL.Query().Get("state"))
			assert.Equal(t, "acme:bowrain/translations", r.URL.Query().Get("head"))
			assert.Equal(t, "Bearer tok", r.Header.Get("Authorization"))
			_, _ = w.Write([]byte("[]"))
		case r.Method == http.MethodPost && r.URL.Path == "/repos/acme/site/pulls":
			require.NoError(t, json.NewDecoder(r.Body).Decode(&createdBody))
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"number": 7, "html_url": "https://github.com/acme/site/pull/7"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/repos/acme/site/issues/7/labels":
			labeled = true
			_, _ = w.Write([]byte(`[]`))
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL)
		}
	}))
	defer srv.Close()

	c := &githubClient{token: "tok", http: srv.Client(), baseURL: srv.URL}
	pr, err := c.EnsureDeliveryPR(context.Background(), DeliveryRequest{
		Repo:       Repo{Host: "github.com", Path: "acme/site"},
		HeadBranch: "bowrain/translations",
		BaseBranch: "main",
		Title:      "Update translations",
		Body:       "report",
		Labels:     []string{"translations"},
	})
	require.NoError(t, err)
	assert.Equal(t, 7, pr.Number)
	assert.Equal(t, "https://github.com/acme/site/pull/7", pr.URL)
	assert.Equal(t, "bowrain/translations", createdBody["head"])
	assert.Equal(t, "main", createdBody["base"])
	assert.True(t, labeled)
}

func TestGitHubEnsureDeliveryPR_UpdatesExisting(t *testing.T) {
	var patched map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/repos/acme/site/pulls":
			_, _ = w.Write([]byte(`[{"number": 3, "html_url": "https://github.com/acme/site/pull/3"}]`))
		case r.Method == http.MethodPatch && r.URL.Path == "/repos/acme/site/pulls/3":
			require.NoError(t, json.NewDecoder(r.Body).Decode(&patched))
			_, _ = w.Write([]byte(`{}`))
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL)
		}
	}))
	defer srv.Close()

	c := &githubClient{token: "tok", http: srv.Client(), baseURL: srv.URL}
	pr, err := c.EnsureDeliveryPR(context.Background(), DeliveryRequest{
		Repo: Repo{Host: "github.com", Path: "acme/site"}, HeadBranch: "b", BaseBranch: "main",
		Title: "t2", Body: "fresh report",
	})
	require.NoError(t, err)
	assert.Equal(t, 3, pr.Number)
	assert.Equal(t, "fresh report", patched["body"])
}

func TestGitLabEnsureDeliveryPR_CreateAndUpdate(t *testing.T) {
	openMRs := "[]"
	var created, updated map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "tok", r.Header.Get("PRIVATE-TOKEN"))
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/projects/group/sub/site/merge_requests":
			_, _ = w.Write([]byte(openMRs))
		case r.Method == http.MethodPost && r.URL.Path == "/projects/group/sub/site/merge_requests":
			require.NoError(t, json.NewDecoder(r.Body).Decode(&created))
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"iid": 11, "web_url": "https://gitlab.example.com/group/sub/site/-/merge_requests/11"}`))
		case r.Method == http.MethodPut && r.URL.Path == "/projects/group/sub/site/merge_requests/11":
			require.NoError(t, json.NewDecoder(r.Body).Decode(&updated))
			_, _ = w.Write([]byte(`{}`))
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL)
		}
	}))
	defer srv.Close()

	// Note: httptest paths arrive unescaped, so the %2F-escaped project id
	// shows up as the literal nested path in r.URL.Path above.
	c := &gitlabClient{token: "tok", http: srv.Client(), baseURL: srv.URL}
	req := DeliveryRequest{
		Repo: Repo{Host: "gitlab.example.com", Path: "group/sub/site"}, HeadBranch: "b", BaseBranch: "main",
		Title: "t", Body: "d", Labels: []string{"translations", "kapi"},
	}
	pr, err := c.EnsureDeliveryPR(context.Background(), req)
	require.NoError(t, err)
	assert.Equal(t, 11, pr.Number)
	assert.Equal(t, "b", created["source_branch"])
	assert.Equal(t, "translations,kapi", created["labels"])

	openMRs = `[{"iid": 11, "web_url": "https://gitlab.example.com/group/sub/site/-/merge_requests/11"}]`
	req.Body = "second report"
	pr, err = c.EnsureDeliveryPR(context.Background(), req)
	require.NoError(t, err)
	assert.Equal(t, 11, pr.Number)
	assert.Equal(t, "second report", updated["description"])
}

func TestVerifyGitHubSignature(t *testing.T) {
	body := []byte(`{"ref":"refs/heads/main"}`)
	mac := hmac.New(sha256.New, []byte("s3cret"))
	mac.Write(body)
	sig := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	assert.True(t, VerifyGitHubSignature("s3cret", body, sig))
	assert.False(t, VerifyGitHubSignature("wrong", body, sig))
	assert.False(t, VerifyGitHubSignature("s3cret", []byte("tampered"), sig))
	assert.False(t, VerifyGitHubSignature("s3cret", body, "sha256=zz"))
	assert.False(t, VerifyGitHubSignature("s3cret", body, ""))
}

func TestVerifyGitLabToken(t *testing.T) {
	assert.True(t, VerifyGitLabToken("s3cret", "s3cret"))
	assert.False(t, VerifyGitLabToken("s3cret", "nope"))
	assert.False(t, VerifyGitLabToken("", ""))
}

func TestParsePushEvent(t *testing.T) {
	gh := []byte(`{"ref":"refs/heads/main","repository":{"full_name":"acme/site"}}`)
	ev, ok := ParsePushEvent(KindGitHub, gh)
	require.True(t, ok)
	assert.Equal(t, PushEvent{Branch: "main", RepoPath: "acme/site"}, ev)

	_, ok = ParsePushEvent(KindGitHub, []byte(`{"ref":"refs/tags/v1.0.0"}`))
	assert.False(t, ok, "tag pushes are not deliveries")

	gl := []byte(`{"object_kind":"push","ref":"refs/heads/main","project":{"path_with_namespace":"group/site"}}`)
	ev, ok = ParsePushEvent(KindGitLab, gl)
	require.True(t, ok)
	assert.Equal(t, PushEvent{Branch: "main", RepoPath: "group/site"}, ev)

	_, ok = ParsePushEvent(KindGitLab, []byte(`{"object_kind":"merge_request"}`))
	assert.False(t, ok, "non-push GitLab events are ignored")

	_, ok = ParsePushEvent(KindGitHub, []byte(`not json`))
	assert.False(t, ok)
}
