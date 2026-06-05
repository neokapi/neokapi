package sandbox

import (
	"encoding/binary"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	mcpserver "github.com/neokapi/neokapi/bowrain/server/mcp"
)

// mockDockerState tracks containers created during a test.
type mockDockerState struct {
	containers map[string]*mockContainer
}

type mockContainer struct {
	id      string
	image   string
	cmd     []string
	env     []string
	started bool
	config  containerCreateRequest
}

// newMockDockerServer creates an httptest.Server that simulates the Docker
// Engine API endpoints needed by DockerSandbox.
func newMockDockerServer(t *testing.T, opts ...mockOpt) (*httptest.Server, *mockDockerState) {
	t.Helper()

	state := &mockDockerState{
		containers: make(map[string]*mockContainer),
	}

	mc := &mockConfig{
		exitCode:  0,
		stdout:    "",
		stderr:    "",
		waitDelay: 0,
	}
	for _, o := range opts {
		o(mc)
	}

	mux := http.NewServeMux()

	// POST /v1.43/containers/create
	mux.HandleFunc("/v1.43/containers/create", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var body containerCreateRequest
		_ = json.NewDecoder(r.Body).Decode(&body)

		id := "sandbox-test-" + strings.ReplaceAll(body.Image, "/", "-")
		state.containers[id] = &mockContainer{
			id:     id,
			image:  body.Image,
			cmd:    body.Cmd,
			env:    body.Env,
			config: body,
		}

		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]string{"Id": id})
	})

	// Container actions: start, wait, logs, archive, json
	mux.HandleFunc("/v1.43/containers/", func(w http.ResponseWriter, r *http.Request) {
		// Parse: /v1.43/containers/{id}/{action}
		path := strings.TrimPrefix(r.URL.Path, "/v1.43/containers/")
		containerID, action, _ := strings.Cut(path, "/")

		switch {
		case action == "start" && r.Method == http.MethodPost:
			c, ok := state.containers[containerID]
			if !ok {
				http.Error(w, "no such container", http.StatusNotFound)
				return
			}
			c.started = true
			w.WriteHeader(http.StatusNoContent)

		case action == "wait" && r.Method == http.MethodPost:
			if mc.waitDelay > 0 {
				select {
				case <-time.After(mc.waitDelay):
				case <-r.Context().Done():
					// Client cancelled (timeout) — don't write response.
					return
				}
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]int{"StatusCode": mc.exitCode})

		case action == "logs" && r.Method == http.MethodGet:
			w.Header().Set("Content-Type", "application/octet-stream")
			w.WriteHeader(http.StatusOK)
			// Write Docker multiplexed stream format.
			if mc.stdout != "" {
				writeDockerLogFrame(w, 1, mc.stdout)
			}
			if mc.stderr != "" {
				writeDockerLogFrame(w, 2, mc.stderr)
			}

		case action == "archive" && r.Method == http.MethodPut:
			// Accept file upload (just consume the body).
			_, _ = io.ReadAll(r.Body)
			w.WriteHeader(http.StatusOK)

		case action == "" && r.Method == http.MethodDelete:
			// Force remove container.
			delete(state.containers, containerID)
			w.WriteHeader(http.StatusNoContent)

		default:
			http.Error(w, "not found", http.StatusNotFound)
		}
	})

	srv := httptest.NewServer(mux)
	return srv, state
}

type mockConfig struct {
	exitCode  int
	stdout    string
	stderr    string
	waitDelay time.Duration
}

type mockOpt func(*mockConfig)

func withStdout(s string) mockOpt {
	return func(c *mockConfig) { c.stdout = s }
}

func withStderr(s string) mockOpt {
	return func(c *mockConfig) { c.stderr = s }
}

func withExitCode(code int) mockOpt {
	return func(c *mockConfig) { c.exitCode = code }
}

func withWaitDelay(d time.Duration) mockOpt {
	return func(c *mockConfig) { c.waitDelay = d }
}

// writeDockerLogFrame writes a single Docker multiplexed log frame.
func writeDockerLogFrame(w io.Writer, streamType byte, payload string) {
	header := make([]byte, 8)
	header[0] = streamType
	binary.BigEndian.PutUint32(header[4:], uint32(len(payload)))
	_, _ = w.Write(header)
	_, _ = w.Write([]byte(payload))
}

// newTestSandbox creates a DockerSandbox pointing at the mock server.
func newTestSandbox(srv *httptest.Server) *DockerSandbox {
	return &DockerSandbox{
		client:  srv.Client(),
		baseURL: srv.URL,
		cfg:     Config{Timeout: 5 * time.Second, MemoryMB: 64},
	}
}

func TestDockerSandboxExecutePython(t *testing.T) {
	srv, state := newMockDockerServer(t,
		withStdout("Hello from Python\n"),
		withExitCode(0),
	)
	defer srv.Close()

	sb := newTestSandbox(srv)
	result, err := sb.Execute(t.Context(), mcpserver.SandboxRequest{
		Language: "python",
		Code:     `print("Hello from Python")`,
		Env:      map[string]string{"MY_VAR": "test_value"},
	})

	require.NoError(t, err)
	assert.Equal(t, "Hello from Python\n", result.Stdout)
	assert.Empty(t, result.Stderr)
	assert.Equal(t, 0, result.ExitCode)

	// Verify container was created with the right image and command.
	require.Empty(t, state.containers) // removed after execution
	// The container was deleted by removeContainer, but we can verify creation
	// happened by checking that the mock handled it (no error returned).
}

func TestDockerSandboxExecutePythonContainerConfig(t *testing.T) {
	// Use a server that doesn't delete on the mock side so we can inspect state.
	state := &mockDockerState{containers: make(map[string]*mockContainer)}

	mux := http.NewServeMux()
	mux.HandleFunc("/v1.43/containers/create", func(w http.ResponseWriter, r *http.Request) {
		var body containerCreateRequest
		_ = json.NewDecoder(r.Body).Decode(&body)

		id := "test-python-1"
		state.containers[id] = &mockContainer{
			id:     id,
			image:  body.Image,
			cmd:    body.Cmd,
			env:    body.Env,
			config: body,
		}
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]string{"Id": id})
	})
	mux.HandleFunc("/v1.43/containers/", func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/v1.43/containers/")
		_, action, _ := strings.Cut(path, "/")
		switch {
		case action == "start":
			w.WriteHeader(http.StatusNoContent)
		case action == "wait":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]int{"StatusCode": 0})
		case action == "logs":
			w.WriteHeader(http.StatusOK)
			writeDockerLogFrame(w, 1, "ok\n")
		case action == "" && r.Method == http.MethodDelete:
			// Don't delete from state so we can inspect.
			w.WriteHeader(http.StatusNoContent)
		default:
			http.Error(w, "not found", http.StatusNotFound)
		}
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	sb := newTestSandbox(srv)
	result, err := sb.Execute(t.Context(), mcpserver.SandboxRequest{
		Language: "python",
		Code:     `print("ok")`,
		Env:      map[string]string{"MY_VAR": "hello"},
	})
	require.NoError(t, err)
	assert.Equal(t, 0, result.ExitCode)

	// Verify container configuration.
	c := state.containers["test-python-1"]
	require.NotNil(t, c)
	assert.Equal(t, "ghcr.io/neokapi/bravo-sandbox-python:latest", c.config.Image)
	assert.Equal(t, []string{"python3", "-c", `print("ok")`}, c.config.Cmd)
	assert.True(t, c.config.NetworkDisabled)
	assert.True(t, c.config.HostConfig.ReadonlyRootfs)
	assert.Equal(t, "none", c.config.HostConfig.NetworkMode)
	assert.True(t, c.config.HostConfig.AutoRemove)
	assert.Equal(t, int64(64*1024*1024), c.config.HostConfig.Memory)
	assert.Equal(t, int64(250000000), c.config.HostConfig.NanoCPUs)
	assert.Contains(t, c.env, "MY_VAR=hello")
}

func TestDockerSandboxExecuteBash(t *testing.T) {
	srv, _ := newMockDockerServer(t,
		withStdout("hello world\n"),
		withExitCode(0),
	)
	defer srv.Close()

	sb := newTestSandbox(srv)
	result, err := sb.Execute(t.Context(), mcpserver.SandboxRequest{
		Language: "bash",
		Code:     `echo "hello world"`,
	})

	require.NoError(t, err)
	assert.Equal(t, "hello world\n", result.Stdout)
	assert.Equal(t, 0, result.ExitCode)
}

func TestDockerSandboxExecuteNode(t *testing.T) {
	srv, _ := newMockDockerServer(t,
		withStdout(`{"result":42}`+"\n"),
		withExitCode(0),
	)
	defer srv.Close()

	sb := newTestSandbox(srv)
	result, err := sb.Execute(t.Context(), mcpserver.SandboxRequest{
		Language: "node",
		Code:     `console.log(JSON.stringify({result: 42}))`,
	})

	require.NoError(t, err)
	assert.Equal(t, `{"result":42}`+"\n", result.Stdout)
	assert.Equal(t, 0, result.ExitCode)
}

func TestDockerSandboxUnsupportedLanguage(t *testing.T) {
	srv, _ := newMockDockerServer(t)
	defer srv.Close()

	sb := newTestSandbox(srv)
	_, err := sb.Execute(t.Context(), mcpserver.SandboxRequest{
		Language: "ruby",
		Code:     `puts "hello"`,
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported language")
	assert.Contains(t, err.Error(), "ruby")
}

func TestDockerSandboxEmptyCode(t *testing.T) {
	srv, _ := newMockDockerServer(t)
	defer srv.Close()

	sb := newTestSandbox(srv)
	_, err := sb.Execute(t.Context(), mcpserver.SandboxRequest{
		Language: "python",
		Code:     "",
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "code is required")
}

func TestDockerSandboxTimeout(t *testing.T) {
	srv, _ := newMockDockerServer(t,
		withWaitDelay(10*time.Second), // longer than our timeout
	)
	defer srv.Close()

	sb := &DockerSandbox{
		client:  srv.Client(),
		baseURL: srv.URL,
		cfg:     Config{Timeout: 200 * time.Millisecond, MemoryMB: 64},
	}

	_, err := sb.Execute(t.Context(), mcpserver.SandboxRequest{
		Language: "python",
		Code:     `import time; time.sleep(60)`,
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "sandbox wait")
}

func TestDockerSandboxWithStderr(t *testing.T) {
	srv, _ := newMockDockerServer(t,
		withStdout("output\n"),
		withStderr("warning: something\n"),
		withExitCode(1),
	)
	defer srv.Close()

	sb := newTestSandbox(srv)
	result, err := sb.Execute(t.Context(), mcpserver.SandboxRequest{
		Language: "bash",
		Code:     `echo output; echo "warning: something" >&2; exit 1`,
	})

	require.NoError(t, err)
	assert.Equal(t, "output\n", result.Stdout)
	assert.Equal(t, "warning: something\n", result.Stderr)
	assert.Equal(t, 1, result.ExitCode)
}

func TestDockerSandboxWithFiles(t *testing.T) {
	// Track whether archive upload was called.
	archiveCalled := false

	mux := http.NewServeMux()
	mux.HandleFunc("/v1.43/containers/create", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]string{"Id": "files-test"})
	})
	mux.HandleFunc("/v1.43/containers/", func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/v1.43/containers/")
		_, action, _ := strings.Cut(path, "/")
		switch {
		case action == "archive" && r.Method == http.MethodPut:
			archiveCalled = true
			// Verify content-type is tar.
			assert.Equal(t, "application/x-tar", r.Header.Get("Content-Type"))
			_, _ = io.ReadAll(r.Body)
			w.WriteHeader(http.StatusOK)
		case action == "start":
			w.WriteHeader(http.StatusNoContent)
		case action == "wait":
			_ = json.NewEncoder(w).Encode(map[string]int{"StatusCode": 0})
		case action == "logs":
			w.WriteHeader(http.StatusOK)
			writeDockerLogFrame(w, 1, "processed\n")
		case action == "" && r.Method == http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		default:
			http.Error(w, "not found", http.StatusNotFound)
		}
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	sb := newTestSandbox(srv)
	result, err := sb.Execute(t.Context(), mcpserver.SandboxRequest{
		Language: "python",
		Code:     `print("processed")`,
		Files: map[string][]byte{
			"input.txt": []byte("some data"),
		},
	})

	require.NoError(t, err)
	assert.True(t, archiveCalled, "archive upload should have been called")
	assert.Equal(t, "processed\n", result.Stdout)
	assert.Equal(t, 0, result.ExitCode)
}
