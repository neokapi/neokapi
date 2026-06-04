package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// CreateTokenRequest is the request body for creating an API token.
type CreateTokenRequest struct {
	Name       string `json:"name"`
	ExpireDays int    `json:"expire_days,omitempty"`
}

// CreateTokenResponse is returned after creating a token.
type CreateTokenResponse struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	TokenPrefix string     `json:"token_prefix"`
	Token       string     `json:"token"`
	Scopes      string     `json:"scopes"`
	ExpiresAt   *time.Time `json:"expires_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
}

// TokenInfo represents an API token in list results (no plaintext token).
type TokenInfo struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	TokenPrefix string     `json:"token_prefix"`
	Scopes      string     `json:"scopes"`
	LastUsedAt  *time.Time `json:"last_used_at"`
	ExpiresAt   *time.Time `json:"expires_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
}

// CreateToken creates a new API token in the given workspace.
func CreateToken(serverURL, token, workspace, name string, expireDays int) (*CreateTokenResponse, error) {
	body, err := json.Marshal(CreateTokenRequest{Name: name, ExpireDays: expireDays})
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	u := fmt.Sprintf("%s/api/v1/%s/tokens", strings.TrimRight(serverURL, "/"), workspace)
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, u, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := defaultHTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("create token: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("create token failed (HTTP %d): %s", resp.StatusCode, string(respBody))
	}

	var result CreateTokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	return &result, nil
}

// ListTokens returns all API tokens for the given workspace.
func ListTokens(serverURL, token, workspace string) ([]TokenInfo, error) {
	u := fmt.Sprintf("%s/api/v1/%s/tokens", strings.TrimRight(serverURL, "/"), workspace)
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, u, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := defaultHTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("list tokens: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("list tokens failed (HTTP %d): %s", resp.StatusCode, string(respBody))
	}

	var result []TokenInfo
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	return result, nil
}

// DeleteToken deletes an API token by ID.
func DeleteToken(serverURL, token, workspace, tokenID string) error {
	u := fmt.Sprintf("%s/api/v1/%s/tokens/%s", strings.TrimRight(serverURL, "/"), workspace, tokenID)
	req, err := http.NewRequestWithContext(context.Background(), http.MethodDelete, u, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := defaultHTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("delete token: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("delete token failed (HTTP %d): %s", resp.StatusCode, string(respBody))
	}
	return nil
}
