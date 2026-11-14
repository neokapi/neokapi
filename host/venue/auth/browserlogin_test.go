package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// standInServer answers the native-client sign-in routes the way the Bowrain
// server does: the login route redirects to the client's loopback with a
// one-time code, and the token route redeems it only with the verifier whose
// challenge the login carried, and only for the same redirect.
type standInServer struct {
	*httptest.Server
	mu        sync.Mutex
	challenge string
	redirect  string
	// redirectQuery, when set, replaces the code and state the login
	// redirect carries.
	redirectQuery func(state string) url.Values
}

func newStandInServer(t *testing.T) *standInServer {
	s := &standInServer{}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/auth/desktop/login", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("response_mode") != "code" || q.Get("code_challenge_method") != "S256" || q.Get("state") == "" {
			http.Error(w, "bad login request", http.StatusBadRequest)
			return
		}
		s.mu.Lock()
		s.challenge, s.redirect = q.Get("code_challenge"), q.Get("redirect_uri")
		s.mu.Unlock()
		back := url.Values{"code": {"one-time"}, "state": {q.Get("state")}}
		if s.redirectQuery != nil {
			back = s.redirectQuery(q.Get("state"))
		}
		http.Redirect(w, r, q.Get("redirect_uri")+"?"+back.Encode(), http.StatusFound)
	})
	mux.HandleFunc("/api/v1/auth/desktop/token", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Code         string `json:"code"`
			CodeVerifier string `json:"code_verifier"`
			RedirectURI  string `json:"redirect_uri"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad body", http.StatusBadRequest)
			return
		}
		s.mu.Lock()
		ok := req.Code == "one-time" && ComputeCodeChallenge(req.CodeVerifier) == s.challenge && req.RedirectURI == s.redirect
		s.mu.Unlock()
		if !ok {
			http.Error(w, `{"error":"invalid_grant"}`, http.StatusBadRequest)
			return
		}
		_ = json.NewEncoder(w).Encode(TokenResponse{AccessToken: "access", RefreshToken: "refresh", TokenType: "Bearer", ExpiresIn: 3600})
	})
	s.Server = httptest.NewServer(mux)
	t.Cleanup(s.Close)
	return s
}

// visit stands in for the browser: it follows the redirects to the loopback.
func visit(ctx context.Context, target string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	return resp.Body.Close()
}

func TestBrowserTokens_RedeemsTheCodeWithTheVerifier(t *testing.T) {
	srv := newStandInServer(t)
	var progress bytes.Buffer
	tok, err := browserTokens(t.Context(), srv.URL, LoginOptions{Progress: &progress, OpenBrowser: visit})
	require.NoError(t, err)
	assert.Equal(t, "access", tok.AccessToken)
	assert.Equal(t, "refresh", tok.RefreshToken)
	assert.True(t, strings.HasPrefix(srv.redirect, "http://127.0.0.1:"), srv.redirect)
	assert.Contains(t, progress.String(), "Opening your browser to sign in to "+srv.URL)
}

func TestBrowserTokens_IgnoresARedirectWithAnotherState(t *testing.T) {
	srv := newStandInServer(t)
	strayStatus := 0
	open := func(ctx context.Context, target string) error {
		u, err := url.Parse(target)
		if err != nil {
			return err
		}
		redirect := u.Query().Get("redirect_uri")
		// A request with the wrong state is answered and does not end the
		// wait; the real redirect that follows does.
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, redirect+"?code=forged&state=other", nil)
		if err != nil {
			return err
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return err
		}
		strayStatus = resp.StatusCode
		_ = resp.Body.Close()
		return visit(ctx, target)
	}
	tok, err := browserTokens(t.Context(), srv.URL, LoginOptions{Progress: &bytes.Buffer{}, OpenBrowser: open})
	require.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, strayStatus)
	assert.Equal(t, "access", tok.AccessToken)
}

func TestBrowserTokens_ReportsAnErrorRedirect(t *testing.T) {
	srv := newStandInServer(t)
	srv.redirectQuery = func(state string) url.Values {
		return url.Values{"error": {"access_denied"}, "error_description": {"cancelled"}, "state": {state}}
	}
	_, err := browserTokens(t.Context(), srv.URL, LoginOptions{Progress: &bytes.Buffer{}, OpenBrowser: visit})
	require.ErrorContains(t, err, "the sign-in was not completed: access_denied: cancelled")
}

func TestBrowserTokens_NoBrowserPrintsTheURL(t *testing.T) {
	srv := newStandInServer(t)
	progress := &syncBuffer{b: &bytes.Buffer{}}
	done := make(chan struct{})
	opened := false
	go func() {
		defer close(done)
		// The person opens the printed URL themselves.
		for {
			line := progress.String()
			if i := strings.Index(line, srv.URL+"/api/v1/auth/desktop/login?"); i >= 0 {
				target := strings.Fields(line[i:])[0]
				_ = visit(context.Background(), target)
				return
			}
		}
	}()
	tok, err := browserTokens(t.Context(), srv.URL, LoginOptions{
		Progress:  progress,
		NoBrowser: true,
		OpenBrowser: func(context.Context, string) error {
			opened = true
			return nil
		},
	})
	<-done
	require.NoError(t, err)
	assert.False(t, opened, "--no-browser opens nothing")
	assert.Equal(t, "access", tok.AccessToken)
	assert.Contains(t, progress.String(), "Open this URL in a browser on this machine to sign in:")
}

// syncBuffer lets the test read the progress while the login writes it.
type syncBuffer struct {
	mu sync.Mutex
	b  *bytes.Buffer
}

func (s *syncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func TestBrowserTokens_ARejectedCodeFails(t *testing.T) {
	srv := newStandInServer(t)
	srv.redirectQuery = func(state string) url.Values {
		return url.Values{"code": {"not-the-one"}, "state": {state}}
	}
	_, err := browserTokens(t.Context(), srv.URL, LoginOptions{Progress: &bytes.Buffer{}, OpenBrowser: visit})
	require.ErrorContains(t, err, "HTTP 400")
}

func TestCanOpenBrowser_NotOverSSHOrInCI(t *testing.T) {
	t.Setenv("CI", "")
	t.Setenv("SSH_TTY", "")
	t.Setenv("SSH_CONNECTION", "1.2.3.4 22 5.6.7.8 22")
	assert.False(t, CanOpenBrowser())
	t.Setenv("SSH_CONNECTION", "")
	t.Setenv("CI", "true")
	assert.False(t, CanOpenBrowser())
}
