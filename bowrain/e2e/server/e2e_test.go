//go:build e2e

package server

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var serverURL = getServerURL()

func getServerURL() string {
	if u := os.Getenv("BOWRAIN_URL"); u != "" {
		return u
	}
	return "http://localhost:8080"
}

// helper to make JSON requests with optional auth token.
func apiRequest(t *testing.T, method, path, token string, body string) *http.Response {
	t.Helper()
	var bodyReader io.Reader
	if body != "" {
		bodyReader = strings.NewReader(body)
	}
	req, err := http.NewRequest(method, serverURL+path, bodyReader)
	require.NoError(t, err)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	return resp
}

func readJSON(t *testing.T, resp *http.Response) map[string]any {
	t.Helper()
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	var result map[string]any
	require.NoError(t, json.Unmarshal(data, &result))
	return result
}

func TestHealth(t *testing.T) {
	resp := apiRequest(t, http.MethodGet, "/api/v1/health", "", "")
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	result := readJSON(t, resp)
	assert.Equal(t, "ok", result["status"])
}

func TestInfo(t *testing.T) {
	resp := apiRequest(t, http.MethodGet, "/api/v1/info", "", "")
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	result := readJSON(t, resp)
	assert.Equal(t, "server", result["mode"])
}

// TestDeviceAuthFlow exercises the full device authorization flow:
// 1. Start device auth → get device_code + user_code
// 2. Verify the user_code (simulating user approval with email/name)
// 3. Poll for token → get access_token
// 4. Use token to call /auth/me
func TestDeviceAuthFlow(t *testing.T) {
	// Step 1: Start device auth.
	form := url.Values{"client_id": {"e2e-test"}}
	resp, err := http.PostForm(serverURL+"/api/v1/auth/device/start", form)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var startResp struct {
		DeviceCode      string `json:"device_code"`
		UserCode        string `json:"user_code"`
		VerificationURI string `json:"verification_uri"`
		ExpiresIn       int    `json:"expires_in"`
		Interval        int    `json:"interval"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&startResp))
	require.NotEmpty(t, startResp.DeviceCode)
	require.NotEmpty(t, startResp.UserCode)

	// Step 2: Verify user_code (simulating browser approval with email/name via form values).
	verifyForm := url.Values{
		"user_code": {startResp.UserCode},
		"email":     {"admin@example.com"},
		"name":      {"Admin User"},
	}
	verifyResp, err := http.PostForm(serverURL+"/api/v1/auth/device/verify", verifyForm)
	require.NoError(t, err)
	defer verifyResp.Body.Close()
	require.Equal(t, http.StatusOK, verifyResp.StatusCode)

	// Step 3: Poll for token.
	var accessToken string
	pollForm := url.Values{
		"device_code": {startResp.DeviceCode},
		"grant_type":  {"urn:ietf:params:oauth:grant-type:device_code"},
	}

	for i := 0; i < 10; i++ {
		pollResp, err := http.PostForm(serverURL+"/api/v1/auth/device/poll", pollForm)
		require.NoError(t, err)

		body, _ := io.ReadAll(pollResp.Body)
		pollResp.Body.Close()

		if pollResp.StatusCode == http.StatusOK {
			var tokenResp struct {
				AccessToken string `json:"access_token"`
				TokenType   string `json:"token_type"`
			}
			require.NoError(t, json.Unmarshal(body, &tokenResp))
			accessToken = tokenResp.AccessToken
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	require.NotEmpty(t, accessToken, "failed to obtain access token")

	// Step 4: Use token to call /auth/me.
	meResp := apiRequest(t, http.MethodGet, "/api/v1/auth/me", accessToken, "")
	defer meResp.Body.Close()
	require.Equal(t, http.StatusOK, meResp.StatusCode)
	meResult := readJSON(t, meResp)
	assert.Equal(t, "admin@example.com", meResult["email"])
}

// TestWorkspaceCRUD tests creating and listing workspaces.
func TestWorkspaceCRUD(t *testing.T) {
	token := getTestToken(t)

	// Create workspace.
	body := `{"name":"E2E Workspace","slug":"e2e-ws"}`
	resp := apiRequest(t, http.MethodPost, "/api/v1/workspaces", token, body)
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	ws := readJSON(t, resp)
	assert.Equal(t, "E2E Workspace", ws["name"])

	// List workspaces.
	listResp := apiRequest(t, http.MethodGet, "/api/v1/workspaces", token, "")
	defer listResp.Body.Close()
	require.Equal(t, http.StatusOK, listResp.StatusCode)

	var workspaces []map[string]any
	data, _ := io.ReadAll(listResp.Body)
	require.NoError(t, json.Unmarshal(data, &workspaces))
	require.NotEmpty(t, workspaces)

	found := false
	for _, w := range workspaces {
		if w["slug"] == "e2e-ws" {
			found = true
			break
		}
	}
	assert.True(t, found, "expected to find e2e-ws workspace")
}

// TestProjectInWorkspace tests creating a project inside a workspace.
func TestProjectInWorkspace(t *testing.T) {
	token := getTestToken(t)

	// Ensure workspace exists.
	body := `{"name":"Project Test WS","slug":"proj-ws"}`
	apiRequest(t, http.MethodPost, "/api/v1/workspaces", token, body)

	// Create project in workspace.
	projBody := `{"name":"Test Project","default_source_language":"en","target_languages":["fr","de"]}`
	resp := apiRequest(t, http.MethodPost, "/api/v1/proj-ws/projects", token, projBody)
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	proj := readJSON(t, resp)
	assert.Equal(t, "Test Project", proj["name"])

	// List projects in workspace.
	listResp := apiRequest(t, http.MethodGet, "/api/v1/proj-ws/projects", token, "")
	defer listResp.Body.Close()
	require.Equal(t, http.StatusOK, listResp.StatusCode)

	var projects []map[string]any
	data, _ := io.ReadAll(listResp.Body)
	require.NoError(t, json.Unmarshal(data, &projects))
	require.NotEmpty(t, projects)
}

// getTestToken performs the device auth flow and returns an access token.
func getTestToken(t *testing.T) string {
	t.Helper()

	// Start device auth.
	form := url.Values{"client_id": {"e2e-test"}}
	resp, err := http.PostForm(serverURL+"/api/v1/auth/device/start", form)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var startResp struct {
		DeviceCode string `json:"device_code"`
		UserCode   string `json:"user_code"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&startResp))

	// Verify with test user.
	verifyForm := url.Values{
		"user_code": {startResp.UserCode},
		"email":     {"admin@example.com"},
		"name":      {"Admin User"},
	}
	verifyResp, err := http.PostForm(serverURL+"/api/v1/auth/device/verify", verifyForm)
	require.NoError(t, err)
	verifyResp.Body.Close()

	// Poll for token.
	pollForm := url.Values{
		"device_code": {startResp.DeviceCode},
		"grant_type":  {"urn:ietf:params:oauth:grant-type:device_code"},
	}
	for i := 0; i < 10; i++ {
		pollResp, err := http.PostForm(serverURL+"/api/v1/auth/device/poll", pollForm)
		require.NoError(t, err)
		body, _ := io.ReadAll(pollResp.Body)
		pollResp.Body.Close()

		if pollResp.StatusCode == http.StatusOK {
			var tokenResp struct {
				AccessToken string `json:"access_token"`
			}
			require.NoError(t, json.Unmarshal(body, &tokenResp))
			return tokenResp.AccessToken
		}
		time.Sleep(500 * time.Millisecond)
	}
	t.Fatal("failed to obtain test token")
	return ""
}

// TestWebUI verifies the embedded web UI serves index.html at /.
func TestWebUI(t *testing.T) {
	resp, err := http.Get(serverURL + "/")
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	// The web UI should serve an HTML page.
	assert.Contains(t, resp.Header.Get("Content-Type"), "text/html")
	assert.Contains(t, string(body), "<html", fmt.Sprintf("expected HTML response, got: %.100s", body))
}
