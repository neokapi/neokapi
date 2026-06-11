package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// KeycloakAdminConfig holds the parameters needed to talk to the Keycloak
// Admin API on behalf of a service-account client. The admin client must be
// granted the realm-management `manage-users` role in the target realm.
type KeycloakAdminConfig struct {
	// BaseURL is the Keycloak base URL, e.g. "https://auth.bowrain.cloud".
	BaseURL string
	// Realm is the realm name whose users we manage, e.g. "bowrain".
	Realm string
	// ClientID is the service-account client ID.
	ClientID string
	// ClientSecret is the service-account client secret.
	ClientSecret string
}

// Validate returns nil if the config is fully populated.
func (c KeycloakAdminConfig) Validate() error {
	switch {
	case strings.TrimSpace(c.BaseURL) == "":
		return errors.New("keycloak admin base URL is required")
	case strings.TrimSpace(c.Realm) == "":
		return errors.New("keycloak realm is required")
	case strings.TrimSpace(c.ClientID) == "":
		return errors.New("keycloak admin client ID is required")
	case strings.TrimSpace(c.ClientSecret) == "":
		return errors.New("keycloak admin client secret is required")
	}
	return nil
}

// KeycloakAdminClient calls the Keycloak Admin REST API using a cached
// service-account access token (client_credentials grant). It is safe for
// concurrent use.
type KeycloakAdminClient struct {
	cfg  KeycloakAdminConfig
	http *http.Client

	mu          sync.Mutex
	cachedToken string
	tokenExpiry time.Time
}

// NewKeycloakAdminClient returns a client that lazily fetches and refreshes
// its admin token. NewKeycloakAdminClient does not validate connectivity;
// the first API call surfaces errors.
func NewKeycloakAdminClient(cfg KeycloakAdminConfig) (*KeycloakAdminClient, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return &KeycloakAdminClient{
		cfg:  cfg,
		http: &http.Client{Timeout: 15 * time.Second},
	}, nil
}

// kcUserRep matches the subset of Keycloak's UserRepresentation we need.
type kcUserRep struct {
	ID            string `json:"id"`
	Email         string `json:"email,omitempty"`
	EmailVerified bool   `json:"emailVerified"`
}

// UpdateUserEmail finds the Keycloak user by oidcSub (their `sub` claim ==
// Keycloak user ID) and sets a new email + emailVerified=true. Returns an
// error if the user is not found or the update fails.
func (c *KeycloakAdminClient) UpdateUserEmail(ctx context.Context, oidcSub, newEmail string) error {
	if oidcSub == "" {
		return errors.New("oidc subject is required")
	}
	if newEmail == "" {
		return errors.New("new email is required")
	}
	body := kcUserRep{
		Email:         newEmail,
		EmailVerified: true,
	}
	path := fmt.Sprintf("/admin/realms/%s/users/%s", url.PathEscape(c.cfg.Realm), url.PathEscape(oidcSub))
	req, err := c.newJSONRequest(ctx, http.MethodPut, path, body)
	if err != nil {
		return err
	}
	resp, err := c.do(ctx, req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	msg, _ := io.ReadAll(resp.Body)
	return fmt.Errorf("keycloak update user: status %d: %s", resp.StatusCode, string(msg))
}

// do dispatches a request, attaching a fresh admin token. On 401 it
// invalidates the cached token and retries once.
func (c *KeycloakAdminClient) do(ctx context.Context, req *http.Request) (*http.Response, error) {
	tok, err := c.token(ctx)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("keycloak admin request: %w", err)
	}
	if resp.StatusCode != http.StatusUnauthorized {
		return resp, nil
	}
	resp.Body.Close()
	c.invalidateToken()
	tok, err = c.token(ctx)
	if err != nil {
		return nil, err
	}
	req2 := req.Clone(ctx)
	req2.Header.Set("Authorization", "Bearer "+tok)
	if req.Body != nil {
		// http.Request.Clone shares the body; for safety re-set it from GetBody.
		if req.GetBody != nil {
			b, err := req.GetBody()
			if err != nil {
				return nil, fmt.Errorf("rewind request body: %w", err)
			}
			req2.Body = b
		}
	}
	resp2, err := c.http.Do(req2)
	if err != nil {
		return nil, fmt.Errorf("keycloak admin retry: %w", err)
	}
	return resp2, nil
}

func (c *KeycloakAdminClient) newJSONRequest(ctx context.Context, method, path string, payload any) (*http.Request, error) {
	buf, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal payload: %w", err)
	}
	u := strings.TrimRight(c.cfg.BaseURL, "/") + path
	req, err := http.NewRequestWithContext(ctx, method, u, bytes.NewReader(buf))
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.GetBody = func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(buf)), nil
	}
	return req, nil
}

// token returns a non-expired admin token, fetching a fresh one if needed.
func (c *KeycloakAdminClient) token(ctx context.Context) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.cachedToken != "" && time.Now().Before(c.tokenExpiry) {
		return c.cachedToken, nil
	}
	form := url.Values{
		"grant_type":    {"client_credentials"},
		"client_id":     {c.cfg.ClientID},
		"client_secret": {c.cfg.ClientSecret},
	}
	tokenURL := fmt.Sprintf("%s/realms/%s/protocol/openid-connect/token",
		strings.TrimRight(c.cfg.BaseURL, "/"), url.PathEscape(c.cfg.Realm))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", fmt.Errorf("build token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("token request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		msg, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("keycloak token: status %d: %s", resp.StatusCode, string(msg))
	}
	var tr struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tr); err != nil {
		return "", fmt.Errorf("decode token response: %w", err)
	}
	if tr.AccessToken == "" {
		return "", errors.New("empty access token from keycloak")
	}
	// Refresh 30s before actual expiry to avoid races.
	ttl := max(time.Duration(tr.ExpiresIn)*time.Second, 30*time.Second)
	c.cachedToken = tr.AccessToken
	c.tokenExpiry = time.Now().Add(ttl - 30*time.Second)
	return c.cachedToken, nil
}

func (c *KeycloakAdminClient) invalidateToken() {
	c.mu.Lock()
	c.cachedToken = ""
	c.tokenExpiry = time.Time{}
	c.mu.Unlock()
}
