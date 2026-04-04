package service

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"time"
)

// DockerRuntime implements ContainerRuntime using the Docker Engine API.
// Designed for local development with Docker Desktop / Docker for Mac.
//
// It communicates with the Docker daemon over its REST API via the Unix
// socket (or TCP endpoint). No external SDK dependency — just net/http
// against the Docker Engine API v1.43+.
type DockerRuntime struct {
	client  *http.Client
	baseURL string // e.g. "http://localhost"
	network string // Docker network to attach containers to (optional)
}

// DockerRuntimeConfig configures the Docker container runtime.
type DockerRuntimeConfig struct {
	// Host is the Docker daemon endpoint.
	// Defaults to "unix:///var/run/docker.sock".
	Host string

	// Network is the Docker network to attach agent containers to.
	// When set, the container joins this network so it can reach
	// bowrain-server by hostname. When empty, the default bridge is used.
	Network string
}

// NewDockerRuntime creates a Docker runtime that talks to the local daemon.
func NewDockerRuntime(cfg DockerRuntimeConfig) *DockerRuntime {
	host := cfg.Host
	if host == "" {
		host = "unix:///var/run/docker.sock"
	}

	var client *http.Client
	var baseURL string

	if len(host) > 7 && host[:7] == "unix://" {
		sockPath := host[7:]
		client = &http.Client{
			Transport: &http.Transport{
				DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
					return (&net.Dialer{Timeout: 5 * time.Second}).DialContext(ctx, "unix", sockPath)
				},
			},
			Timeout: 30 * time.Second,
		}
		baseURL = "http://localhost"
	} else {
		// TCP endpoint (e.g. "tcp://192.168.1.100:2375").
		baseURL = "http" + host[3:] // tcp:// → http://
		client = &http.Client{Timeout: 30 * time.Second}
	}

	return &DockerRuntime{
		client:  client,
		baseURL: baseURL,
		network: cfg.Network,
	}
}

// Spawn creates and starts a new Docker container running the ZeroClaw agent.
func (r *DockerRuntime) Spawn(ctx context.Context, cfg ContainerConfig) (*AgentContainer, error) {
	gatewayPort := cfg.GatewayPort
	if gatewayPort == 0 {
		gatewayPort = 42617
	}

	// Build environment variables for the ZeroClaw config template.
	env := []string{
		"BRAVO_MODEL_PROVIDER=" + cfg.ModelProvider,
		"BRAVO_MODEL_NAME=" + cfg.ModelName,
		"BRAVO_MODEL_API_BASE=" + cfg.ModelAPIBase,
		"BRAVO_MODEL_API_KEY=" + cfg.ModelAPIKey,
		"BRAVO_MCP_ENDPOINT=" + cfg.MCPEndpoint,
		"BRAVO_AGENT_TOKEN=" + cfg.AgentToken,
	}
	if cfg.SystemPrompt != "" {
		env = append(env, "BRAVO_SYSTEM_PROMPT="+cfg.SystemPrompt)
	}
	for k, v := range cfg.Env {
		env = append(env, k+"="+v)
	}

	portStr := strconv.Itoa(gatewayPort)
	containerName := "bravo-" + cfg.ConversationID

	// Docker Engine API: create container.
	// We use an ephemeral port binding (host port 0 = auto-assign).
	createBody := fmt.Sprintf(`{
		"Image": %q,
		"Env": %s,
		"ExposedPorts": {"%s/tcp": {}},
		"HostConfig": {
			"PortBindings": {"%s/tcp": [{"HostPort": "0"}]},
			"Memory": 67108864,
			"NanoCpus": 250000000,
			"AutoRemove": true
		},
		"Labels": {
			"neokapi.agent": "bravo",
			"neokapi.conversation": %q,
			"neokapi.workspace": %q,
			"neokapi.user": %q
		}
	}`, cfg.Image, jsonStringArray(env), portStr, portStr,
		cfg.ConversationID, cfg.WorkspaceID, cfg.UserID)

	createURL := r.baseURL + "/v1.43/containers/create?name=" + containerName
	resp, err := r.doRequest(ctx, http.MethodPost, createURL, createBody)
	if err != nil {
		return nil, fmt.Errorf("docker create: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusConflict {
		// Container name already exists — remove it and retry.
		resp.Body.Close()
		_ = r.removeContainer(ctx, containerName)
		resp, err = r.doRequest(ctx, http.MethodPost, createURL, createBody)
		if err != nil {
			return nil, fmt.Errorf("docker create (retry): %w", err)
		}
		defer resp.Body.Close()
	}

	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("docker create returned %d: %s", resp.StatusCode, string(body))
	}

	var createResp struct {
		ID string `json:"Id"`
	}
	if err := readJSON(resp.Body, &createResp); err != nil {
		return nil, fmt.Errorf("parse create response: %w", err)
	}
	containerID := createResp.ID

	// Connect to network if specified.
	if r.network != "" {
		if err := r.connectNetwork(ctx, containerID); err != nil {
			_ = r.removeContainer(ctx, containerID)
			return nil, fmt.Errorf("docker network connect: %w", err)
		}
	}

	// Start the container.
	startURL := r.baseURL + "/v1.43/containers/" + containerID + "/start"
	startResp, err := r.doRequest(ctx, http.MethodPost, startURL, "")
	if err != nil {
		_ = r.removeContainer(ctx, containerID)
		return nil, fmt.Errorf("docker start: %w", err)
	}
	startResp.Body.Close()
	if startResp.StatusCode != http.StatusNoContent && startResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(startResp.Body, 1024))
		_ = r.removeContainer(ctx, containerID)
		return nil, fmt.Errorf("docker start returned %d: %s", startResp.StatusCode, string(body))
	}

	// Inspect to get the assigned host port.
	gatewayURL, err := r.inspectPort(ctx, containerID, gatewayPort)
	if err != nil {
		_ = r.Stop(ctx, containerID)
		return nil, fmt.Errorf("inspect port: %w", err)
	}

	return &AgentContainer{
		ID:             containerID,
		GatewayURL:     gatewayURL,
		ConversationID: cfg.ConversationID,
		WorkspaceID:    cfg.WorkspaceID,
		UserID:         cfg.UserID,
		CreatedAt:      time.Now(),
	}, nil
}

// Stop stops and removes a container.
func (r *DockerRuntime) Stop(ctx context.Context, containerID string) error {
	// Stop with a 5-second grace period.
	stopURL := r.baseURL + "/v1.43/containers/" + containerID + "/stop?t=5"
	resp, err := r.doRequest(ctx, http.MethodPost, stopURL, "")
	if err != nil {
		return fmt.Errorf("docker stop: %w", err)
	}
	resp.Body.Close()

	// Remove if not auto-removed.
	return r.removeContainer(ctx, containerID)
}

// Health checks if a container is running and responsive.
func (r *DockerRuntime) Health(ctx context.Context, containerID string) (bool, error) {
	inspectURL := r.baseURL + "/v1.43/containers/" + containerID + "/json"
	resp, err := r.doRequest(ctx, http.MethodGet, inspectURL, "")
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return false, nil
	}
	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("docker inspect returned %d", resp.StatusCode)
	}

	var inspectResp struct {
		State struct {
			Running bool `json:"Running"`
		} `json:"State"`
	}
	if err := readJSON(resp.Body, &inspectResp); err != nil {
		return false, err
	}
	return inspectResp.State.Running, nil
}

// inspectPort reads the dynamically assigned host port for the gateway.
func (r *DockerRuntime) inspectPort(ctx context.Context, containerID string, containerPort int) (string, error) {
	inspectURL := r.baseURL + "/v1.43/containers/" + containerID + "/json"
	resp, err := r.doRequest(ctx, http.MethodGet, inspectURL, "")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("docker inspect returned %d", resp.StatusCode)
	}

	var inspectResp struct {
		NetworkSettings struct {
			Ports map[string][]struct {
				HostIP   string `json:"HostIp"`
				HostPort string `json:"HostPort"`
			} `json:"Ports"`
		} `json:"NetworkSettings"`
	}
	if err := readJSON(resp.Body, &inspectResp); err != nil {
		return "", err
	}

	portKey := fmt.Sprintf("%d/tcp", containerPort)
	bindings, ok := inspectResp.NetworkSettings.Ports[portKey]
	if !ok || len(bindings) == 0 {
		return "", fmt.Errorf("no port binding for %s", portKey)
	}

	hostPort := bindings[0].HostPort
	hostIP := bindings[0].HostIP
	if hostIP == "" || hostIP == "0.0.0.0" || hostIP == "::" {
		hostIP = "127.0.0.1"
	}

	return "http://" + hostIP + ":" + hostPort, nil
}

// connectNetwork connects a container to the configured Docker network.
func (r *DockerRuntime) connectNetwork(ctx context.Context, containerID string) error {
	url := r.baseURL + "/v1.43/networks/" + r.network + "/connect"
	body := fmt.Sprintf(`{"Container": %q}`, containerID)
	resp, err := r.doRequest(ctx, http.MethodPost, url, body)
	if err != nil {
		return err
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("network connect returned %d", resp.StatusCode)
	}
	return nil
}

// removeContainer forcibly removes a container, ignoring errors (best-effort).
func (r *DockerRuntime) removeContainer(ctx context.Context, containerID string) error {
	url := r.baseURL + "/v1.43/containers/" + containerID + "?force=true"
	resp, err := r.doRequest(ctx, http.MethodDelete, url, "")
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

// doRequest sends a JSON request to the Docker daemon.
func (r *DockerRuntime) doRequest(ctx context.Context, method, url, body string) (*http.Response, error) {
	var reqBody io.Reader
	if body != "" {
		reqBody = stringReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, url, reqBody)
	if err != nil {
		return nil, err
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	return r.client.Do(req)
}
