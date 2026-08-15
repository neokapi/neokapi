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
type Transport struct {
	baseURL    string
	authToken  string // JWT bearer token
	claimToken string // ClaimToken for unclaimed projects
	httpClient *http.Client

	refreshToken   string                             // opaque refresh token for auto-refresh
	onTokenRefresh func(newAccess, newRefresh string) // callback after successful refresh
}

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
func (t *Transport) SetAuthToken(token string) { t.authToken = token }

// SetRefreshToken configures auto-refresh with the given refresh token.
// The onRefresh callback is called after a successful token refresh so the
// caller can persist the new tokens.
func (t *Transport) SetRefreshToken(token string, onRefresh func(newAccess, newRefresh string)) {
	t.refreshToken = token
	t.onTokenRefresh = onRefresh
}

// ApplyAuth adds authorization headers. Callers that build their own request
// and send it on their own http.Client — a long-lived SSE stream, say — use
// this to authenticate it.
func (t *Transport) ApplyAuth(req *http.Request) {
	if t.authToken != "" {
		req.Header.Set("Authorization", "Bearer "+t.authToken)
	} else if t.claimToken != "" {
		req.Header.Set("Authorization", "ClaimToken "+t.claimToken)
	}
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

	t.ApplyAuth(req)
	resp, err := t.httpClient.Do(req)
	if err != nil {
		return nil, err
	}

	// Auto-refresh on 401 if we have a refresh token.
	if resp.StatusCode == http.StatusUnauthorized && t.refreshToken != "" && t.authToken != "" {
		resp.Body.Close()

		if refreshErr := t.doRefresh(req.Context()); refreshErr != nil {
			return nil, fmt.Errorf("token refresh failed: %w", refreshErr)
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

// doRefresh calls the /api/v1/auth/refresh endpoint to obtain a new access
// token and a rotated refresh token.
func (t *Transport) doRefresh(ctx context.Context) error {
	body, _ := json.Marshal(map[string]string{"refresh_token": t.refreshToken})
	refreshURL := t.baseURL + "/api/v1/auth/refresh"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, refreshURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := t.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("refresh failed with HTTP %d", resp.StatusCode)
	}

	var tokenResp struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return err
	}

	t.authToken = tokenResp.AccessToken
	t.refreshToken = tokenResp.RefreshToken
	if t.onTokenRefresh != nil {
		t.onTokenRefresh(tokenResp.AccessToken, tokenResp.RefreshToken)
	}
	return nil
}
