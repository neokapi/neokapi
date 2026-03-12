package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// DeviceAuthResponse is returned when starting a device authorization flow (RFC 8628).
type DeviceAuthResponse struct {
	DeviceCode      string `json:"device_code"`
	UserCode        string `json:"user_code"`
	VerificationURI string `json:"verification_uri"`
	ExpiresIn       int    `json:"expires_in"`
	Interval        int    `json:"interval"`
}

// TokenResponse is the successful result of polling the token endpoint.
type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	RefreshToken string `json:"refresh_token,omitempty"`
	ExpiresIn    int    `json:"expires_in"`
	IDToken      string `json:"id_token,omitempty"`
}

// DeviceFlowClient handles the OAuth 2.0 device authorization grant (RFC 8628).
type DeviceFlowClient struct {
	// DeviceAuthURL is the device authorization endpoint (RFC 8628).
	DeviceAuthURL string
	// TokenURL is the token endpoint.
	TokenURL string
	// ClientID is the OAuth client ID.
	ClientID string
	// ClientSecret is the OAuth client secret (optional for public clients).
	ClientSecret string
	// HTTPClient allows overriding the default HTTP client (useful for testing).
	HTTPClient *http.Client
}

func (c *DeviceFlowClient) httpClient() *http.Client {
	if c.HTTPClient != nil {
		return c.HTTPClient
	}
	return http.DefaultClient
}

// StartDeviceAuth initiates the device flow by calling the device authorization endpoint.
func (c *DeviceFlowClient) StartDeviceAuth(ctx context.Context) (*DeviceAuthResponse, error) {
	data := url.Values{
		"client_id": {c.ClientID},
		"scope":     {"openid profile email"},
	}
	if c.ClientSecret != "" {
		data.Set("client_secret", c.ClientSecret)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.DeviceAuthURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.httpClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("device auth request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("device auth failed (status %d): %s", resp.StatusCode, body)
	}

	var result DeviceAuthResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode device auth response: %w", err)
	}
	if result.Interval == 0 {
		result.Interval = 5 // default polling interval
	}
	return &result, nil
}

// PollForToken polls the token endpoint until the user authorizes the device
// or the context is cancelled. It respects the polling interval from the
// device auth response.
func (c *DeviceFlowClient) PollForToken(ctx context.Context, deviceCode string, interval int) (*TokenResponse, error) {
	if interval <= 0 {
		interval = 5
	}
	ticker := time.NewTicker(time.Duration(interval) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
			token, pending, err := c.TryToken(ctx, deviceCode)
			if err != nil {
				return nil, err
			}
			if !pending {
				return token, nil
			}
		}
	}
}

// TryToken makes a single token poll attempt. Returns (token, pending, err).
// If pending is true, the caller should retry after the interval.
func (c *DeviceFlowClient) TryToken(ctx context.Context, deviceCode string) (*TokenResponse, bool, error) {
	data := url.Values{
		"client_id":   {c.ClientID},
		"device_code": {deviceCode},
		"grant_type":  {"urn:ietf:params:oauth:grant-type:device_code"},
	}
	if c.ClientSecret != "" {
		data.Set("client_secret", c.ClientSecret)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.TokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, false, fmt.Errorf("create token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.httpClient().Do(req)
	if err != nil {
		return nil, false, fmt.Errorf("token request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, false, fmt.Errorf("read token response: %w", err)
	}

	// Check for slow_down or authorization_pending errors.
	if resp.StatusCode != http.StatusOK {
		var errResp struct {
			Error string `json:"error"`
		}
		if json.Unmarshal(body, &errResp) == nil {
			switch errResp.Error {
			case "authorization_pending", "slow_down":
				return nil, true, nil // keep polling
			case "expired_token":
				return nil, false, fmt.Errorf("device code expired — please restart the login flow")
			case "access_denied":
				return nil, false, fmt.Errorf("authorization denied by user")
			}
		}
		return nil, false, fmt.Errorf("token request failed (status %d): %s", resp.StatusCode, body)
	}

	var token TokenResponse
	if err := json.Unmarshal(body, &token); err != nil {
		return nil, false, fmt.Errorf("decode token response: %w", err)
	}
	return &token, false, nil
}
