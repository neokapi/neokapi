package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newDockerTestServer returns an httptest.Server that simulates the Docker Engine API.
func newDockerTestServer(t *testing.T) (*httptest.Server, *dockerTestState) {
	t.Helper()
	state := &dockerTestState{
		containers: make(map[string]*dockerTestContainer),
	}
	mux := http.NewServeMux()

	// POST /v1.43/containers/create
	mux.HandleFunc("/v1.43/containers/create", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var body struct {
			Image string   `json:"Image"`
			Env   []string `json:"Env"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)

		id := fmt.Sprintf("test-container-%d", len(state.containers)+1)
		state.containers[id] = &dockerTestContainer{
			id:      id,
			image:   body.Image,
			env:     body.Env,
			running: false,
		}

		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]string{"Id": id})
	})

	// POST /v1.43/containers/{id}/start
	mux.HandleFunc("/v1.43/containers/", func(w http.ResponseWriter, r *http.Request) {
		parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/v1.43/containers/"), "/")
		if len(parts) < 2 {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		containerID := parts[0]
		action := parts[1]

		switch {
		case action == "start" && r.Method == http.MethodPost:
			c, ok := state.containers[containerID]
			if !ok {
				http.Error(w, "no such container", http.StatusNotFound)
				return
			}
			c.running = true
			w.WriteHeader(http.StatusNoContent)

		case action == "stop" && r.Method == http.MethodPost:
			c, ok := state.containers[containerID]
			if ok {
				c.running = false
			}
			w.WriteHeader(http.StatusNoContent)

		case action == "json" && r.Method == http.MethodGet:
			c, ok := state.containers[containerID]
			if !ok {
				http.Error(w, "no such container", http.StatusNotFound)
				return
			}
			resp := map[string]interface{}{
				"State": map[string]bool{"Running": c.running},
				"NetworkSettings": map[string]interface{}{
					"Ports": map[string]interface{}{
						"42617/tcp": []map[string]string{
							{"HostIp": "0.0.0.0", "HostPort": "54321"},
						},
					},
				},
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(resp)

		default:
			http.Error(w, "not found", http.StatusNotFound)
		}
	})

	// DELETE /v1.43/containers/{id}
	// Handled by the prefix handler above if needed, but let's also handle force delete.
	// The mux above handles paths starting with /v1.43/containers/ which includes delete.

	// POST /v1.43/networks/{name}/connect
	mux.HandleFunc("/v1.43/networks/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	srv := httptest.NewServer(mux)
	return srv, state
}

type dockerTestState struct {
	containers map[string]*dockerTestContainer
}

type dockerTestContainer struct {
	id      string
	image   string
	env     []string
	running bool
}

func TestDockerRuntimeSpawnAndHealth(t *testing.T) {
	srv, state := newDockerTestServer(t)
	defer srv.Close()

	rt := &DockerRuntime{
		client:  srv.Client(),
		baseURL: srv.URL,
	}

	ctx := context.Background()
	container, err := rt.Spawn(ctx, ContainerConfig{
		Image:          "ghcr.io/neokapi/bravo-agent:latest",
		ConversationID: "conv-123",
		WorkspaceID:    "ws-1",
		UserID:         "user-1",
		MCPEndpoint:    "http://localhost:8080/mcp/",
		AgentToken:     "test-token",
		ModelProvider:  "anthropic",
		ModelName:      "claude-sonnet-4-20250514",
		GatewayPort:    42617,
	})
	require.NoError(t, err)
	assert.NotEmpty(t, container.ID)
	assert.Equal(t, "conv-123", container.ConversationID)
	assert.Equal(t, "ws-1", container.WorkspaceID)
	assert.Contains(t, container.GatewayURL, "54321") // assigned port from mock

	// Container should be running in mock state.
	tc := state.containers[container.ID]
	require.NotNil(t, tc)
	assert.True(t, tc.running)
	assert.Equal(t, "ghcr.io/neokapi/bravo-agent:latest", tc.image)

	// Verify env vars were passed.
	envMap := make(map[string]string)
	for _, e := range tc.env {
		parts := strings.SplitN(e, "=", 2)
		if len(parts) == 2 {
			envMap[parts[0]] = parts[1]
		}
	}
	assert.Equal(t, "anthropic", envMap["BRAVO_MODEL_PROVIDER"])
	assert.Equal(t, "claude-sonnet-4-20250514", envMap["BRAVO_MODEL_NAME"])
	assert.Equal(t, "test-token", envMap["BRAVO_AGENT_TOKEN"])

	// Health check should return true.
	healthy, err := rt.Health(ctx, container.ID)
	require.NoError(t, err)
	assert.True(t, healthy)
}

func TestDockerRuntimeStop(t *testing.T) {
	srv, state := newDockerTestServer(t)
	defer srv.Close()

	rt := &DockerRuntime{
		client:  srv.Client(),
		baseURL: srv.URL,
	}

	ctx := context.Background()
	container, err := rt.Spawn(ctx, ContainerConfig{
		Image:          "ghcr.io/neokapi/bravo-agent:latest",
		ConversationID: "conv-stop",
		WorkspaceID:    "ws-1",
		GatewayPort:    42617,
	})
	require.NoError(t, err)

	tc := state.containers[container.ID]
	require.True(t, tc.running)

	err = rt.Stop(ctx, container.ID)
	require.NoError(t, err)
	assert.False(t, tc.running)
}

func TestDockerRuntimeHealthNotFound(t *testing.T) {
	srv, _ := newDockerTestServer(t)
	defer srv.Close()

	rt := &DockerRuntime{
		client:  srv.Client(),
		baseURL: srv.URL,
	}

	healthy, err := rt.Health(context.Background(), "nonexistent")
	require.NoError(t, err)
	assert.False(t, healthy)
}
