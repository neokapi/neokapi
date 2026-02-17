package project

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/gokapi/gokapi/core/formats"
	"github.com/gokapi/gokapi/core/registry"
	apiclient "github.com/gokapi/gokapi/platform/client"
	"github.com/gokapi/gokapi/platform/connector"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupTestProject creates a temporary kapi project with a JSON source file and
// a mock Bowrain server. Returns the project, format registry, and cleanup func.
func setupTestProject(t *testing.T, handler http.Handler) (*Project, *registry.FormatRegistry) {
	t.Helper()

	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	// Create project structure.
	root := t.TempDir()
	kapiDir := filepath.Join(root, ".kapi")
	require.NoError(t, os.MkdirAll(kapiDir, 0755))

	// Write a JSON source file.
	srcDir := filepath.Join(root, "src", "locales")
	require.NoError(t, os.MkdirAll(srcDir, 0755))
	require.NoError(t, os.WriteFile(
		filepath.Join(srcDir, "en.json"),
		[]byte(`{"greeting":"Hello","farewell":"Goodbye"}`),
		0644,
	))

	cfg := &Config{
		Project: ProjectMeta{
			Name:         "test-project",
			SourceLocale: "en",
		},
		Server: &ServerConfig{
			URL:       srv.URL,
			ProjectID: "proj-123",
		},
		Mappings: []Mapping{
			{Local: "src/locales/*.json", Format: "json"},
		},
	}

	proj := &Project{
		Root:    root,
		KapiDir: kapiDir,
		Config:  cfg,
	}

	formatReg := registry.NewFormatRegistry()
	formats.RegisterAll(formatReg)

	return proj, formatReg
}

// mockSyncHandler is a simple mock that records push requests and returns pull responses.
type mockSyncHandler struct {
	pushCalls  int
	pushBlocks []apiclient.BlockInput
	pullCursor int64
}

func (m *mockSyncHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.Method == http.MethodPost && contains(r.URL.Path, "/sync/push"):
		var req apiclient.SyncPushRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		m.pushCalls++
		m.pushBlocks = append(m.pushBlocks, req.Blocks...)
		m.pullCursor += int64(len(req.Blocks))
		_ = json.NewEncoder(w).Encode(apiclient.SyncPushResponse{
			Stored:    len(req.Blocks),
			NewCursor: m.pullCursor,
		})

	case r.Method == http.MethodGet && contains(r.URL.Path, "/sync/pull"):
		_ = json.NewEncoder(w).Encode(apiclient.SyncPullResponse{
			Changes:   nil,
			NewCursor: m.pullCursor,
			HasMore:   false,
		})

	default:
		http.Error(w, "not found", http.StatusNotFound)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstr(s, substr))
}

func containsSubstr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

func TestNewSourceConnector_RequiresServer(t *testing.T) {
	proj := &Project{
		Root:    t.TempDir(),
		KapiDir: filepath.Join(t.TempDir(), ".kapi"),
		Config:  &Config{},
	}

	_, err := NewSourceConnector(proj, registry.NewFormatRegistry())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no server configuration")
}

func TestNewSourceConnector_RequiresURL(t *testing.T) {
	proj := &Project{
		Root:    t.TempDir(),
		KapiDir: filepath.Join(t.TempDir(), ".kapi"),
		Config:  &Config{Server: &ServerConfig{ProjectID: "p1"}},
	}

	_, err := NewSourceConnector(proj, registry.NewFormatRegistry())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "server URL not configured")
}

func TestSourceConnector_Push(t *testing.T) {
	mock := &mockSyncHandler{}
	proj, formatReg := setupTestProject(t, mock)

	conn, err := NewSourceConnector(proj, formatReg)
	require.NoError(t, err)
	defer conn.Close()

	ctx := context.Background()

	// First push should send all blocks.
	result, err := conn.Push(ctx, connector.PushOptions{})
	require.NoError(t, err)
	assert.Equal(t, 2, result.BlocksPushed)
	assert.Equal(t, 1, mock.pushCalls)
	assert.Len(t, mock.pushBlocks, 2)

	// Second push (no local changes) should send nothing.
	result, err = conn.Push(ctx, connector.PushOptions{})
	require.NoError(t, err)
	assert.Equal(t, 0, result.BlocksPushed)
}

func TestSourceConnector_Push_DryRun(t *testing.T) {
	mock := &mockSyncHandler{}
	proj, formatReg := setupTestProject(t, mock)

	conn, err := NewSourceConnector(proj, formatReg)
	require.NoError(t, err)
	defer conn.Close()

	ctx := context.Background()

	// Dry run should not actually push.
	result, err := conn.Push(ctx, connector.PushOptions{DryRun: true})
	require.NoError(t, err)
	assert.Equal(t, 2, result.BlocksPushed)
	assert.Equal(t, 0, mock.pushCalls, "dry run should not make any server calls")
}

func TestSourceConnector_Push_Force(t *testing.T) {
	mock := &mockSyncHandler{}
	proj, formatReg := setupTestProject(t, mock)

	conn, err := NewSourceConnector(proj, formatReg)
	require.NoError(t, err)
	defer conn.Close()

	ctx := context.Background()

	// Push all blocks first.
	_, err = conn.Push(ctx, connector.PushOptions{})
	require.NoError(t, err)
	assert.Equal(t, 1, mock.pushCalls)

	// Force push should resend all blocks.
	result, err := conn.Push(ctx, connector.PushOptions{Force: true})
	require.NoError(t, err)
	assert.Equal(t, 2, result.BlocksPushed)
	assert.Equal(t, 2, mock.pushCalls)
}

func TestSourceConnector_Pull(t *testing.T) {
	mock := &mockSyncHandler{}
	proj, formatReg := setupTestProject(t, mock)

	conn, err := NewSourceConnector(proj, formatReg)
	require.NoError(t, err)
	defer conn.Close()

	ctx := context.Background()

	result, err := conn.Pull(ctx, connector.PullOptions{})
	require.NoError(t, err)
	assert.Equal(t, 0, result.BlocksPulled, "no remote changes")
}

func TestSourceConnector_Status(t *testing.T) {
	mock := &mockSyncHandler{}
	proj, formatReg := setupTestProject(t, mock)

	conn, err := NewSourceConnector(proj, formatReg)
	require.NoError(t, err)
	defer conn.Close()

	ctx := context.Background()

	// Before any sync: all local blocks should be pending push.
	status, err := conn.Status(ctx)
	require.NoError(t, err)
	assert.Equal(t, "kapi-source", status.ConnectorID)
	assert.Equal(t, 2, status.ItemCount)
	assert.Equal(t, 2, status.PendingPush)

	// After push: nothing pending.
	_, err = conn.Push(ctx, connector.PushOptions{})
	require.NoError(t, err)

	status, err = conn.Status(ctx)
	require.NoError(t, err)
	assert.Equal(t, 0, status.PendingPush)
}

func TestSourceConnector_SyncCachePersistence(t *testing.T) {
	mock := &mockSyncHandler{}
	proj, formatReg := setupTestProject(t, mock)

	// Push with first connector instance.
	conn, err := NewSourceConnector(proj, formatReg)
	require.NoError(t, err)
	_, err = conn.Push(context.Background(), connector.PushOptions{})
	require.NoError(t, err)
	require.NoError(t, conn.Close())

	// Create new connector (simulates new CLI invocation).
	conn2, err := NewSourceConnector(proj, formatReg)
	require.NoError(t, err)
	defer conn2.Close()

	// Should not re-push since cache persisted.
	result, err := conn2.Push(context.Background(), connector.PushOptions{})
	require.NoError(t, err)
	assert.Equal(t, 0, result.BlocksPushed, "blocks already cached from previous push")
	assert.Equal(t, 1, mock.pushCalls, "only the original push call")
}

func TestSourceConnector_InterfaceCompliance(t *testing.T) {
	// Compile-time check that KapiSourceConnector implements SourceConnector.
	var _ connector.SourceConnector = (*KapiSourceConnector)(nil)
}
