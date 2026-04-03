// Package sandbox provides an isolated code execution environment backed by
// Docker containers. It implements the mcp.SandboxExecutor interface.
package sandbox

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	mcpserver "github.com/neokapi/neokapi/bowrain/server/mcp"
)

// Language-to-image mapping.
var languageImages = map[string]string{
	"python": "ghcr.io/neokapi/bravo-sandbox-python:latest",
	"node":   "ghcr.io/neokapi/bravo-sandbox-node:latest",
	"bash":   "alpine:latest",
}

// languageCmd returns the command to execute a script in the given language.
func languageCmd(language, code string) []string {
	switch language {
	case "python":
		return []string{"python3", "-c", code}
	case "node":
		return []string{"node", "-e", code}
	case "bash":
		return []string{"bash", "-c", code}
	default:
		return nil
	}
}

// Config configures the Docker sandbox executor.
type Config struct {
	// DockerHost is the Docker daemon endpoint.
	// Defaults to "unix:///var/run/docker.sock".
	DockerHost string

	// Timeout is the maximum execution time per sandbox run.
	// Defaults to 30s.
	Timeout time.Duration

	// MemoryMB is the memory limit in megabytes.
	// Defaults to 64.
	MemoryMB int64
}

func (c Config) timeout() time.Duration {
	if c.Timeout > 0 {
		return c.Timeout
	}
	return 30 * time.Second
}

func (c Config) memoryBytes() int64 {
	mb := c.MemoryMB
	if mb <= 0 {
		mb = 64
	}
	return mb * 1024 * 1024
}

// DockerSandbox executes code in ephemeral Docker containers.
// It communicates with the Docker daemon over its REST API via Unix socket
// (or TCP), with no external SDK dependency.
type DockerSandbox struct {
	client  *http.Client
	baseURL string
	cfg     Config
}

// Ensure DockerSandbox implements SandboxExecutor at compile time.
var _ mcpserver.SandboxExecutor = (*DockerSandbox)(nil)

// New creates a new Docker-backed sandbox executor.
func New(cfg Config) *DockerSandbox {
	host := cfg.DockerHost
	if host == "" {
		host = "unix:///var/run/docker.sock"
	}

	var client *http.Client
	var baseURL string

	if len(host) > 7 && host[:7] == "unix://" {
		sockPath := host[7:]
		client = &http.Client{
			Transport: &http.Transport{
				DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
					return net.DialTimeout("unix", sockPath, 5*time.Second)
				},
			},
		}
		baseURL = "http://localhost"
	} else {
		// TCP endpoint (e.g. "tcp://192.168.1.100:2375").
		baseURL = "http" + host[3:] // tcp:// -> http://
		client = &http.Client{}
	}

	return &DockerSandbox{
		client:  client,
		baseURL: baseURL,
		cfg:     cfg,
	}
}

// Execute runs code in an isolated Docker container and returns the result.
func (d *DockerSandbox) Execute(ctx context.Context, req mcpserver.SandboxRequest) (*mcpserver.SandboxResult, error) {
	if req.Code == "" {
		return nil, fmt.Errorf("sandbox: code is required")
	}

	image, ok := languageImages[req.Language]
	if !ok {
		return nil, fmt.Errorf("sandbox: unsupported language %q", req.Language)
	}

	cmd := languageCmd(req.Language, req.Code)
	if cmd == nil {
		return nil, fmt.Errorf("sandbox: unsupported language %q", req.Language)
	}

	// Apply execution timeout.
	execCtx, cancel := context.WithTimeout(ctx, d.cfg.timeout())
	defer cancel()

	// 1. Create container.
	containerID, err := d.createContainer(execCtx, image, cmd, req.Env)
	if err != nil {
		return nil, fmt.Errorf("sandbox create: %w", err)
	}

	// Ensure cleanup on any exit path.
	defer d.removeContainer(context.Background(), containerID)

	// 2. Copy input files to /workspace if any.
	if len(req.Files) > 0 {
		if err := d.copyFiles(execCtx, containerID, req.Files); err != nil {
			return nil, fmt.Errorf("sandbox copy files: %w", err)
		}
	}

	// 3. Start container.
	if err := d.startContainer(execCtx, containerID); err != nil {
		return nil, fmt.Errorf("sandbox start: %w", err)
	}

	// 4. Wait for container to finish.
	exitCode, err := d.waitContainer(execCtx, containerID)
	if err != nil {
		return nil, fmt.Errorf("sandbox wait: %w", err)
	}

	// 5. Capture logs.
	stdout, stderr, err := d.getLogs(execCtx, containerID)
	if err != nil {
		return nil, fmt.Errorf("sandbox logs: %w", err)
	}

	return &mcpserver.SandboxResult{
		Stdout:   stdout,
		Stderr:   stderr,
		ExitCode: exitCode,
	}, nil
}

// createContainer creates an ephemeral container with security constraints.
func (d *DockerSandbox) createContainer(ctx context.Context, image string, cmd []string, env map[string]string) (string, error) {
	var envList []string
	for k, v := range env {
		envList = append(envList, k+"="+v)
	}

	body := containerCreateRequest{
		Image:      image,
		Cmd:        cmd,
		Env:        envList,
		WorkingDir: "/workspace",
		HostConfig: hostConfig{
			Memory:         d.cfg.memoryBytes(),
			NanoCPUs:       250000000, // 0.25 CPU
			ReadonlyRootfs: true,
			NetworkMode:    "none",
			AutoRemove:     true,
			Tmpfs:          map[string]string{"/workspace": "rw,noexec,size=32m"},
		},
		NetworkDisabled: true,
	}

	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return "", err
	}

	url := d.baseURL + "/v1.43/containers/create"
	resp, err := d.doRequest(ctx, http.MethodPost, url, bodyBytes)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return "", fmt.Errorf("docker create returned %d: %s", resp.StatusCode, string(msg))
	}

	var createResp struct {
		ID string `json:"Id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&createResp); err != nil {
		return "", fmt.Errorf("parse create response: %w", err)
	}
	return createResp.ID, nil
}

// copyFiles uploads files to the container's /workspace via a tar archive.
func (d *DockerSandbox) copyFiles(ctx context.Context, containerID string, files map[string][]byte) error {
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	for name, data := range files {
		hdr := &tar.Header{
			Name: name,
			Mode: 0644,
			Size: int64(len(data)),
		}
		if err := tw.WriteHeader(hdr); err != nil {
			return err
		}
		if _, err := tw.Write(data); err != nil {
			return err
		}
	}
	if err := tw.Close(); err != nil {
		return err
	}

	url := d.baseURL + "/v1.43/containers/" + containerID + "/archive?path=/workspace"
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, url, &buf)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-tar")

	resp, err := d.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("docker archive upload returned %d: %s", resp.StatusCode, string(msg))
	}
	return nil
}

// startContainer starts the created container.
func (d *DockerSandbox) startContainer(ctx context.Context, containerID string) error {
	url := d.baseURL + "/v1.43/containers/" + containerID + "/start"
	resp, err := d.doRequest(ctx, http.MethodPost, url, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("docker start returned %d: %s", resp.StatusCode, string(msg))
	}
	return nil
}

// waitContainer blocks until the container exits and returns the exit code.
func (d *DockerSandbox) waitContainer(ctx context.Context, containerID string) (int, error) {
	url := d.baseURL + "/v1.43/containers/" + containerID + "/wait"
	resp, err := d.doRequest(ctx, http.MethodPost, url, nil)
	if err != nil {
		return -1, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return -1, fmt.Errorf("docker wait returned %d: %s", resp.StatusCode, string(msg))
	}

	var waitResp struct {
		StatusCode int `json:"StatusCode"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&waitResp); err != nil {
		return -1, fmt.Errorf("parse wait response: %w", err)
	}
	return waitResp.StatusCode, nil
}

// getLogs retrieves stdout and stderr from the container.
// Docker multiplexed stream format: each frame has an 8-byte header
// [stream_type(1), 0, 0, 0, size(4 big-endian)] followed by the payload.
func (d *DockerSandbox) getLogs(ctx context.Context, containerID string) (stdout, stderr string, err error) {
	url := d.baseURL + "/v1.43/containers/" + containerID + "/logs?stdout=1&stderr=1"
	resp, err := d.doRequest(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return "", "", fmt.Errorf("docker logs returned %d: %s", resp.StatusCode, string(msg))
	}

	// Read the multiplexed stream. Stream type 1 = stdout, 2 = stderr.
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 4*1024*1024)) // 4MB limit
	if err != nil {
		return "", "", err
	}

	var stdoutBuf, stderrBuf strings.Builder
	for len(raw) >= 8 {
		streamType := raw[0]
		size := int(raw[4])<<24 | int(raw[5])<<16 | int(raw[6])<<8 | int(raw[7])
		raw = raw[8:]
		if size > len(raw) {
			size = len(raw)
		}
		payload := string(raw[:size])
		raw = raw[size:]

		switch streamType {
		case 1:
			stdoutBuf.WriteString(payload)
		case 2:
			stderrBuf.WriteString(payload)
		}
	}

	return stdoutBuf.String(), stderrBuf.String(), nil
}

// removeContainer forcibly removes a container (best-effort cleanup).
func (d *DockerSandbox) removeContainer(ctx context.Context, containerID string) {
	url := d.baseURL + "/v1.43/containers/" + containerID + "?force=true"
	resp, err := d.doRequest(ctx, http.MethodDelete, url, nil)
	if err != nil {
		return
	}
	resp.Body.Close()
}

// doRequest sends a request to the Docker daemon.
func (d *DockerSandbox) doRequest(ctx context.Context, method, url string, body []byte) (*http.Response, error) {
	var reqBody io.Reader
	if body != nil {
		reqBody = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, url, reqBody)
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return d.client.Do(req)
}

// containerCreateRequest is the JSON body for POST /containers/create.
type containerCreateRequest struct {
	Image           string     `json:"Image"`
	Cmd             []string   `json:"Cmd"`
	Env             []string   `json:"Env,omitempty"`
	WorkingDir      string     `json:"WorkingDir"`
	NetworkDisabled bool       `json:"NetworkDisabled"`
	HostConfig      hostConfig `json:"HostConfig"`
}

type hostConfig struct {
	Memory         int64             `json:"Memory"`
	NanoCPUs       int64             `json:"NanoCpus"`
	ReadonlyRootfs bool              `json:"ReadonlyRootfs"`
	NetworkMode    string            `json:"NetworkMode"`
	AutoRemove     bool              `json:"AutoRemove"`
	Tmpfs          map[string]string `json:"Tmpfs,omitempty"`
}
