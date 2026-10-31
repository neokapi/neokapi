package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// defaultClientTimeout is the read/connect timeout applied to all sync operations.
const defaultClientTimeout = 120 * time.Second

// Transport is the HTTP plumbing shared by every client of the Bowrain REST
// API: the server base URL, the credentials, and a request path that refreshes
// an expired access token and replays the request once.
//
// It is a separate type because the surfaces built on it are separate: the sync
// client is bound to one project, while the editor client browses many
// workspaces and holds only a token. Both need the same 401-refresh-retry
// behaviour, and neither should own a second copy of it.
//
// A transport is safe for concurrent use. Requests that meet an expired
// access token at the same time refresh it once: the server rotates the
// refresh token on each use and revokes the sign-in when a rotated token is
// presented again, so a second refresh with the same token would sign the
// person out.
type Transport struct {
	baseURL    string
	claimToken string // ClaimToken for unclaimed projects
	httpClient *http.Client

	mu             sync.Mutex                         // guards authToken and refreshToken
	authToken      string                             // JWT bearer token
	refreshToken   string                             // opaque refresh token for auto-refresh
	onTokenRefresh func(newAccess, newRefresh string) // callback after successful refresh

	refreshMu sync.Mutex // serializes refreshes
}

// RefreshError is a refresh of the access token that failed. StatusCode is
// the refresh endpoint's HTTP status, 0 when no response came back. The
// stored sign-in no longer works, so the person signs in again.
type RefreshError struct {
	StatusCode int
	Err        error
}

func (e *RefreshError) Error() string { return "token refresh failed: " + e.Err.Error() }

func (e *RefreshError) Unwrap() error { return e.Err }

// NewTransport creates a bearer-authenticated transport against serverURL.
func NewTransport(serverURL, authToken string) *Transport {
	return &Transport{
		baseURL:    strings.TrimRight(serverURL, "/"),
		authToken:  authToken,
		httpClient: &http.Client{Timeout: defaultClientTimeout},
	}
}

// BaseURL returns the server base URL with any trailing slash removed.
func (t *Transport) BaseURL() string { return t.baseURL }

// SetAuthToken updates the bearer token used for authentication. It is used
// after a token refresh or re-login.
func (t *Transport) SetAuthToken(token string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.authToken = token
}

// tokens answers the access and refresh tokens the transport holds now.
func (t *Transport) tokens() (access, refresh string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.authToken, t.refreshToken
}

// SetRefreshToken configures auto-refresh with the given refresh token.
// The onRefresh callback is called after a successful token refresh so the
// caller can persist the new tokens.
func (t *Transport) SetRefreshToken(token string, onRefresh func(newAccess, newRefresh string)) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.refreshToken = token
	t.onTokenRefresh = onRefresh
}

// RefuseRedirectsOffServer makes the transport refuse a redirect to any scheme
// or host other than the server's own. Go drops the Authorization header on a
// redirect to another host, and keeps it on one to another port of the same
// host or to a subdomain, so a caller holding a sign-in for one server pins
// its requests to that server.
func (t *Transport) RefuseRedirectsOffServer() {
	base, err := url.Parse(t.baseURL)
	t.httpClient.CheckRedirect = func(req *http.Request, _ []*http.Request) error {
		if err != nil || req.URL.Scheme != base.Scheme || req.URL.Host != base.Host {
			return fmt.Errorf("refused a redirect to %s://%s: requests go only to %s", req.URL.Scheme, req.URL.Host, t.baseURL)
		}
		return nil
	}
}

// ApplyAuth adds authorization headers. Callers that build their own request
// and send it on their own http.Client — a long-lived SSE stream, say — use
// this to authenticate it.
func (t *Transport) ApplyAuth(req *http.Request) {
	t.applyAuth(req)
}

// applyAuth adds the authorization header and answers the access token it
// carries, empty for a claim token or none.
func (t *Transport) applyAuth(req *http.Request) string {
	access, _ := t.tokens()
	if access != "" {
		req.Header.Set("Authorization", "Bearer "+access)
		return access
	}
	if t.claimToken != "" {
		req.Header.Set("Authorization", "ClaimToken "+t.claimToken)
	}
	return ""
}

// Do executes an HTTP request and automatically retries once with a refreshed
// access token when the server returns 401 Unauthorized. For requests with a
// body (POST/PUT), the body is buffered so it can be replayed on retry after a
// token refresh.
func (t *Transport) Do(req *http.Request) (*http.Response, error) {
	// Buffer the request body so we can replay it after a token refresh.
	var bodyBytes []byte
	if req.Body != nil && req.Body != http.NoBody {
		var err error
		bodyBytes, err = io.ReadAll(req.Body)
		if err != nil {
			return nil, fmt.Errorf("buffer request body: %w", err)
		}
		req.Body = io.NopCloser(bytes.NewReader(bodyBytes))
	}

	sent := t.applyAuth(req)
	resp, err := t.httpClient.Do(req)
	if err != nil {
		return nil, err
	}

	// Auto-refresh on 401 if we have a refresh token.
	if _, refresh := t.tokens(); resp.StatusCode == http.StatusUnauthorized && refresh != "" && sent != "" {
		resp.Body.Close()

		if refreshErr := t.refreshAfter(req.Context(), sent); refreshErr != nil {
			return nil, refreshErr
		}

		// Retry the original request with the new token and replayed body.
		var body io.Reader
		if bodyBytes != nil {
			body = bytes.NewReader(bodyBytes)
		}
		retryReq, err := http.NewRequestWithContext(req.Context(), req.Method, req.URL.String(), body)
		if err != nil {
			return nil, err
		}
		maps.Copy(retryReq.Header, req.Header)
		t.ApplyAuth(retryReq)
		return t.httpClient.Do(retryReq)
	}

	return resp, nil
}

// DoJSON issues a request against an absolute path under the server base URL,
// optionally sending a JSON body and decoding a JSON response. It accepts any
// 2xx status. A nil out skips decoding (for 204 responses).
func (t *Transport) DoJSON(ctx context.Context, method, path string, query url.Values, body, out any) error {
	u := t.baseURL + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}

	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshal request: %w", err)
		}
		reader = bytes.NewReader(b)
	}

	req, err := http.NewRequestWithContext(ctx, method, u, reader)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := t.Do(req)
	if err != nil {
		return fmt.Errorf("request %s: %w", strings.TrimPrefix(path, "/"), err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return NewStatusError(strings.TrimPrefix(path, "/"), resp.StatusCode, respBody)
	}
	if out != nil && resp.StatusCode != http.StatusNoContent {
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			return fmt.Errorf("decode %s response: %w", strings.TrimPrefix(path, "/"), err)
		}
	}
	return nil
}

// refreshAfter refreshes the access token a request was refused with. A
// refresh another request finished while this one waited has already
// replaced that token, and the request then retries with the new one.
func (t *Transport) refreshAfter(ctx context.Context, refused string) error {
	t.refreshMu.Lock()
	defer t.refreshMu.Unlock()
	if access, _ := t.tokens(); access != refused {
		return nil
	}
	return t.doRefresh(ctx)
}

// doRefresh calls the /api/v1/auth/refresh endpoint to obtain a new access
// token and a rotated refresh token. A failure is a *RefreshError.
func (t *Transport) doRefresh(ctx context.Context) error {
	_, refresh := t.tokens()
	body, _ := json.Marshal(map[string]string{"refresh_token": refresh})
	refreshURL := t.baseURL + "/api/v1/auth/refresh"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, refreshURL, bytes.NewReader(body))
	if err != nil {
		return &RefreshError{Err: err}
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := t.httpClient.Do(req)
	if err != nil {
		return &RefreshError{Err: err}
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return &RefreshError{StatusCode: resp.StatusCode, Err: fmt.Errorf("refresh failed with HTTP %d", resp.StatusCode)}
	}

	var tokenResp struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return &RefreshError{StatusCode: resp.StatusCode, Err: err}
	}

	t.mu.Lock()
	t.authToken = tokenResp.AccessToken
	t.refreshToken = tokenResp.RefreshToken
	onRefresh := t.onTokenRefresh
	t.mu.Unlock()
	if onRefresh != nil {
		onRefresh(tokenResp.AccessToken, tokenResp.RefreshToken)
	}
	return nil
}
