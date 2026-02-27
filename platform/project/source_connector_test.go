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
	"github.com/gokapi/gokapi/core/model"
	"github.com/gokapi/gokapi/core/registry"
	apiclient "github.com/gokapi/gokapi/platform/client"
	"github.com/gokapi/gokapi/platform/connector"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupTestProject creates a temporary brain project with a JSON source file and
// a mock Bowrain server. Returns the project, format registry, and cleanup func.
func setupTestProject(t *testing.T, handler http.Handler) (*Project, *registry.FormatRegistry) {
	t.Helper()

	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	// Create project structure.
	root := t.TempDir()
	brainDir := filepath.Join(root, ".brain")
	require.NoError(t, os.MkdirAll(brainDir, 0755))

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
			URL:        srv.URL,
			ProjectID:  "proj-123",
			ClaimToken: "test-token",
		},
		Mappings: []Mapping{
			{Local: "src/locales/*.json", Format: "json"},
		},
	}

	proj := &Project{
		Root:    root,
		ConfigDir: brainDir,
		Config:  cfg,
	}

	formatReg := registry.NewFormatRegistry()
	formats.RegisterAll(formatReg)

	return proj, formatReg
}

// mockSyncHandler is a simple mock that records push requests and returns pull responses.
type mockSyncHandler struct {
	pushCalls    int
	pushBlocks   []apiclient.BlockInput
	pullCursor   int64
	pullChanges  []apiclient.ChangeEntry             // Changes to return from pull
	blocksByItem map[string][]apiclient.BlockContent // item_name → blocks for /sync/blocks
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

	case r.Method == http.MethodGet && contains(r.URL.Path, "/sync/blocks"):
		itemName := r.URL.Query().Get("item_name")
		var blocks []apiclient.BlockContent
		if m.blocksByItem != nil {
			blocks = m.blocksByItem[itemName]
		}
		if blocks == nil {
			blocks = []apiclient.BlockContent{}
		}
		_ = json.NewEncoder(w).Encode(blocks)

	case r.Method == http.MethodGet && contains(r.URL.Path, "/sync/pull"):
		changes := m.pullChanges
		if changes == nil {
			changes = []apiclient.ChangeEntry{}
		}
		_ = json.NewEncoder(w).Encode(apiclient.SyncPullResponse{
			Changes:   changes,
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
		ConfigDir: filepath.Join(t.TempDir(), ".brain"),
		Config:  &Config{},
	}

	_, err := NewSourceConnector(proj, registry.NewFormatRegistry())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no server configuration")
}

func TestNewSourceConnector_RequiresURL(t *testing.T) {
	proj := &Project{
		Root:    t.TempDir(),
		ConfigDir: filepath.Join(t.TempDir(), ".brain"),
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

	// All blocks should have the item name set to the relative file path.
	for _, bi := range mock.pushBlocks {
		assert.Equal(t, "src/locales/en.json", bi.ItemName, "block %s should have item_name set", bi.ID)
	}

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
	assert.Equal(t, "brain-source", status.ConnectorID)
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

func TestSourceConnector_Push_MultipleFiles(t *testing.T) {
	mock := &mockSyncHandler{}
	proj, formatReg := setupTestProject(t, mock)

	// Add a second JSON file with overlapping block IDs.
	srcDir := filepath.Join(proj.Root, "src", "locales")
	require.NoError(t, os.WriteFile(
		filepath.Join(srcDir, "fr.json"),
		[]byte(`{"greeting":"Bonjour","farewell":"Au revoir"}`),
		0644,
	))

	conn, err := NewSourceConnector(proj, formatReg)
	require.NoError(t, err)
	defer conn.Close()

	ctx := context.Background()

	result, err := conn.Push(ctx, connector.PushOptions{})
	require.NoError(t, err)
	assert.Equal(t, 4, result.BlocksPushed, "should push blocks from both files")
	assert.Equal(t, 2, result.FilesScanned)

	// Verify blocks have distinct item names.
	itemNames := map[string]bool{}
	for _, bi := range mock.pushBlocks {
		itemNames[bi.ItemName] = true
		assert.NotEmpty(t, bi.ItemName, "every block must have an item name")
	}
	assert.Len(t, itemNames, 2, "blocks should come from 2 different files")

	// Cache should store per-file entries, not "_blocks".
	_, hasLegacy := conn.cache.Files["_blocks"]
	assert.False(t, hasLegacy, "cache should not use legacy _blocks key")
	assert.Len(t, conn.cache.Files, 2, "cache should have one entry per file")
}

func TestSourceConnector_Pull_WriteBack(t *testing.T) {
	// The JSON reader generates block IDs as tu1, tu2, etc. in sorted key order.
	// For {"farewell":"Goodbye","greeting":"Hello"}, the reader produces:
	//   tu1 (farewell) and tu2 (greeting)
	mock := &mockSyncHandler{
		pullChanges: []apiclient.ChangeEntry{
			{Seq: 1, BlockID: "tu1", ChangeType: "target_added", Locale: "fr"},
			{Seq: 2, BlockID: "tu2", ChangeType: "target_added", Locale: "fr"},
		},
		blocksByItem: map[string][]apiclient.BlockContent{
			"src/locales/en.json": {
				{ID: "tu1", Name: "farewell", ItemName: "src/locales/en.json", Source: "Goodbye", Targets: map[string]string{"fr": "Au revoir"}},
				{ID: "tu2", Name: "greeting", ItemName: "src/locales/en.json", Source: "Hello", Targets: map[string]string{"fr": "Bonjour"}},
			},
		},
	}
	proj, formatReg := setupTestProject(t, mock)

	conn, err := NewSourceConnector(proj, formatReg)
	require.NoError(t, err)
	defer conn.Close()

	ctx := context.Background()

	result, err := conn.Pull(ctx, connector.PullOptions{
		Locales: []model.LocaleID{"fr"},
	})
	require.NoError(t, err)
	assert.Equal(t, 2, result.BlocksPulled)
	assert.Equal(t, 1, result.LocalesCount)
	assert.Equal(t, 1, result.FilesWritten, "should write one translated file for fr locale")

	// Verify the translated file was created.
	targetPath := filepath.Join(proj.Root, "src", "locales", "fr.json")
	_, err = os.Stat(targetPath)
	assert.NoError(t, err, "translated file should exist at %s", targetPath)

	// Read and verify the translated file contains French text.
	if err == nil {
		data, err := os.ReadFile(targetPath)
		require.NoError(t, err)
		content := string(data)
		assert.Contains(t, content, "Bonjour")
		assert.Contains(t, content, "Au revoir")
	}
}

func TestSourceConnector_Pull_WriteBack_DryRun(t *testing.T) {
	mock := &mockSyncHandler{
		pullChanges: []apiclient.ChangeEntry{
			{Seq: 1, BlockID: "tu2", ChangeType: "target_added", Locale: "fr"},
		},
		blocksByItem: map[string][]apiclient.BlockContent{
			"src/locales/en.json": {
				{ID: "tu2", Name: "greeting", ItemName: "src/locales/en.json", Source: "Hello", Targets: map[string]string{"fr": "Bonjour"}},
			},
		},
	}
	proj, formatReg := setupTestProject(t, mock)

	conn, err := NewSourceConnector(proj, formatReg)
	require.NoError(t, err)
	defer conn.Close()

	ctx := context.Background()

	result, err := conn.Pull(ctx, connector.PullOptions{
		Locales: []model.LocaleID{"fr"},
		DryRun:  true,
	})
	require.NoError(t, err)
	assert.Equal(t, 1, result.BlocksPulled)
	assert.Equal(t, 0, result.FilesWritten, "dry run should not write files")

	// Verify no translated file was created.
	targetPath := filepath.Join(proj.Root, "src", "locales", "fr.json")
	_, err = os.Stat(targetPath)
	assert.True(t, os.IsNotExist(err), "no file should be written in dry-run mode")
}

func TestSourceConnector_Pull_NoLocales(t *testing.T) {
	mock := &mockSyncHandler{
		pullChanges: []apiclient.ChangeEntry{
			{Seq: 1, BlockID: "tu2", ChangeType: "target_added", Locale: "fr"},
		},
	}
	proj, formatReg := setupTestProject(t, mock)

	conn, err := NewSourceConnector(proj, formatReg)
	require.NoError(t, err)
	defer conn.Close()

	ctx := context.Background()

	// Pull without specifying locales should not write files.
	result, err := conn.Pull(ctx, connector.PullOptions{})
	require.NoError(t, err)
	assert.Equal(t, 1, result.BlocksPulled)
	assert.Equal(t, 0, result.FilesWritten, "no files written without locale specification")
}

func TestSourceConnector_Pull_TargetPathTemplate(t *testing.T) {
	mock := &mockSyncHandler{
		pullChanges: []apiclient.ChangeEntry{
			{Seq: 1, BlockID: "tu2", ChangeType: "target_added", Locale: "fr"},
		},
		blocksByItem: map[string][]apiclient.BlockContent{
			"src/locales/en.json": {
				{ID: "tu2", Name: "greeting", ItemName: "src/locales/en.json", Source: "Hello", Targets: map[string]string{"fr": "Bonjour"}},
			},
		},
	}
	proj, formatReg := setupTestProject(t, mock)

	// Set a target_path template on the mapping.
	proj.Config.Mappings[0].TargetPath = "src/locales/{locale}.json"

	conn, err := NewSourceConnector(proj, formatReg)
	require.NoError(t, err)
	defer conn.Close()

	ctx := context.Background()

	result, err := conn.Pull(ctx, connector.PullOptions{
		Locales: []model.LocaleID{"fr"},
	})
	require.NoError(t, err)
	assert.Equal(t, 1, result.FilesWritten)

	// Verify file was written to the template path.
	targetPath := filepath.Join(proj.Root, "src", "locales", "fr.json")
	_, err = os.Stat(targetPath)
	assert.NoError(t, err, "translated file should exist at template path")
}

func TestSourceConnector_ResolveTargetPath(t *testing.T) {
	proj := &Project{
		Root: "/project",
		Config: &Config{
			Project: ProjectMeta{
				SourceLocale: "en",
			},
			Mappings: []Mapping{
				{Local: "src/locales/*.json", Format: "json"},
			},
		},
	}

	conn := &BrainSourceConnector{project: proj}

	// Default behavior: replace source locale in path.
	assert.Equal(t, "src/locales/fr.json", conn.resolveTargetPath("src/locales/en.json", "fr"))
	assert.Equal(t, "src/locales/de.json", conn.resolveTargetPath("src/locales/en.json", "de"))

	// With target_path template.
	proj.Config.Mappings[0].TargetPath = "output/{locale}.json"
	assert.Equal(t, "output/fr.json", conn.resolveTargetPath("src/locales/en.json", "fr"))

	// Fallback when source locale not in path.
	proj.Config.Mappings[0].TargetPath = ""
	proj.Config.Project.SourceLocale = "en-US"
	assert.Equal(t, "src/messages.fr.json", conn.resolveTargetPath("src/messages.json", "fr"))
}

func TestSourceConnector_ScanRespectsExcludes(t *testing.T) {
	mock := &mockSyncHandler{}
	proj, formatReg := setupTestProject(t, mock)

	// Add a second JSON file in a "legacy" subdirectory.
	legacyDir := filepath.Join(proj.Root, "src", "locales", "legacy")
	require.NoError(t, os.MkdirAll(legacyDir, 0755))
	require.NoError(t, os.WriteFile(
		filepath.Join(legacyDir, "old.json"),
		[]byte(`{"obsolete":"Remove me"}`),
		0644,
	))

	// Change mapping to a recursive glob, add an exclude.
	proj.Config.Mappings = []Mapping{
		{Local: "src/locales/**/*.json", Format: "json"},
	}
	proj.Config.Exclude = []string{"src/locales/legacy/*.json"}

	conn, err := NewSourceConnector(proj, formatReg)
	require.NoError(t, err)
	defer conn.Close()

	ctx := context.Background()

	// Push should only see the non-excluded file.
	result, err := conn.Push(ctx, connector.PushOptions{})
	require.NoError(t, err)
	assert.Equal(t, 1, result.FilesScanned, "excluded legacy file should not be scanned")
	assert.Equal(t, 2, result.BlocksPushed, "only blocks from en.json")

	// Verify no blocks came from the excluded file.
	for _, bi := range mock.pushBlocks {
		assert.NotContains(t, bi.ItemName, "legacy", "excluded file should not produce blocks")
	}
}

func TestSourceConnector_InterfaceCompliance(t *testing.T) {
	// Compile-time check that BrainSourceConnector implements connector.SourceConnector.
	var _ connector.SourceConnector = (*BrainSourceConnector)(nil)
}
