package project

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/formats"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/registry"
	apiclient "github.com/neokapi/neokapi/bowrain/core/client"
	"github.com/neokapi/neokapi/bowrain/core/connector"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeClaimToken writes a claim token to the sync cache file in the given bowrain dir.
func writeClaimToken(t *testing.T, bowrainDir, token string) {
	t.Helper()
	cache := &SyncCache{
		ClaimToken: token,
		Files:      map[string]*FileCache{},
	}
	require.NoError(t, cache.Save(bowrainDir))
}

// setupTestProject creates a temporary bowrain project with a JSON source file and
// a mock Bowrain server. Returns the project, format registry, and cleanup func.
func setupTestProject(t *testing.T, handler http.Handler) (*Project, *registry.FormatRegistry) {
	t.Helper()

	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	// Create project structure.
	root := t.TempDir()
	bowrainDir := filepath.Join(root, ".bowrain")
	require.NoError(t, os.MkdirAll(bowrainDir, 0755))

	// Write a JSON source file.
	srcDir := filepath.Join(root, "src", "locales")
	require.NoError(t, os.MkdirAll(srcDir, 0755))
	require.NoError(t, os.WriteFile(
		filepath.Join(srcDir, "en.json"),
		[]byte(`{"greeting":"Hello","farewell":"Goodbye"}`),
		0644,
	))

	cfg := &Config{
		URL: FormatProjectURL(srv.URL, "", "proj-123"),
		Defaults: Defaults{
			SourceLanguage: "en",
		},
		Content: []ContentEntry{
			{Path: "src/locales/*.json", Format: "json"},
		},
	}

	proj := &Project{
		Root:      root,
		ConfigDir: bowrainDir,
		Config:    cfg,
	}

	// Write claim token to sync cache (gitignored, not in config).
	writeClaimToken(t, bowrainDir, "test-token")

	formatReg := registry.NewFormatRegistry()
	formats.RegisterAll(formatReg)

	return proj, formatReg
}

// mockSyncHandler is a simple mock that handles the push flow (init → diff → chunk → commit)
// and records push requests for assertions.
type mockSyncHandler struct {
	pushCalls    int
	chunkUploads int      // number of chunk uploads received
	initItems    []string // item names sent to init
	pullCursor   int64
	pullBlocks   []apiclient.SyncBlock            // Blocks to return from pull
	blocksByItem map[string][]apiclient.SyncBlock // item_name → blocks for /sync/blocks
	lastUploadID string
}

func (m *mockSyncHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.Method == http.MethodPost && contains(r.URL.Path, "/sync/push/init"):
		// Respond with diff_computed — all items are new.
		var req struct {
			ItemHashes map[string]string `json:"item_hashes"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		m.lastUploadID = "mock-upload-id"
		var newItems []string
		for k := range req.ItemHashes {
			newItems = append(newItems, k)
			m.initItems = append(m.initItems, k)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"upload_id":            m.lastUploadID,
			"status":               "diff_computed",
			"changed_items":        []string{},
			"new_items":            newItems,
			"deleted_items":        []string{},
			"unchanged_item_count": 0,
		})

	case r.Method == http.MethodPost && contains(r.URL.Path, "/sync/push/diff"):
		// All blocks are needed.
		var req struct {
			BlockHashes map[string]string `json:"block_hashes"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		var needed []string
		for k := range req.BlockHashes {
			needed = append(needed, k)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"needed":    needed,
			"deleted":   []string{},
			"conflicts": []string{},
			"transport": "proxy",
		})

	case r.Method == http.MethodPut && contains(r.URL.Path, "/sync/push/chunks/"):
		// Accept chunk upload.
		_, _ = io.ReadAll(r.Body) // consume body
		m.chunkUploads++
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})

	case r.Method == http.MethodPost && contains(r.URL.Path, "/sync/push/commit"):
		// Record the push and return accepted.
		m.pushCalls++
		m.pullCursor++
		w.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(w).Encode(apiclient.SyncPushResponse{
			PushID:    "mock-push-id",
			NewCursor: m.pullCursor,
		})

	case r.Method == http.MethodGet && contains(r.URL.Path, "/sync/blocks"):
		itemName := r.URL.Query().Get("item_name")
		var blocks []apiclient.SyncBlock
		if m.blocksByItem != nil {
			blocks = m.blocksByItem[itemName]
		}
		if blocks == nil {
			blocks = []apiclient.SyncBlock{}
		}
		_ = json.NewEncoder(w).Encode(blocks)

	case r.Method == http.MethodGet && contains(r.URL.Path, "/sync/pull"):
		blocks := m.pullBlocks
		if blocks == nil {
			blocks = []apiclient.SyncBlock{}
		}
		_ = json.NewEncoder(w).Encode(apiclient.RichPullResponse{
			Blocks:  blocks,
			Cursor:  m.pullCursor,
			HasMore: false,
		})

	default:
		http.Error(w, "not found: "+r.Method+" "+r.URL.Path, http.StatusNotFound)
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

// mockSyncBlock creates a SyncBlock from simple text values for testing.
func mockSyncBlock(id, name, itemName, source string, targets map[string]string) apiclient.SyncBlock {
	sb := apiclient.SyncBlock{
		ID:         id,
		ItemName:   itemName,
		Name:       name,
		SourceText: source,
		Source:     []apiclient.SyncSegment{{ID: "s1", Text: source}},
	}
	if len(targets) > 0 {
		sb.Targets = make(map[string][]apiclient.SyncSegment)
		for loc, text := range targets {
			sb.Targets[loc] = []apiclient.SyncSegment{{ID: "s1", Text: text}}
		}
	}
	return sb
}

func TestNewSourceConnector_RequiresServer(t *testing.T) {
	proj := &Project{
		Root:      t.TempDir(),
		ConfigDir: filepath.Join(t.TempDir(), ".bowrain"),
		Config:    &Config{},
	}

	_, err := NewSourceConnector(proj, registry.NewFormatRegistry())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no server configuration")
}

func TestNewSourceConnector_RequiresURL(t *testing.T) {
	proj := &Project{
		Root:      t.TempDir(),
		ConfigDir: filepath.Join(t.TempDir(), ".bowrain"),
		Config: &Config{
			URL: FormatProjectURL("", "", "p1"),
		},
	}

	_, err := NewSourceConnector(proj, registry.NewFormatRegistry())
	require.Error(t, err)
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
	assert.Greater(t, mock.chunkUploads, 0, "should upload at least one chunk")
	// The item name should have been sent to init.
	assert.Contains(t, mock.initItems, "src/locales/en.json")

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
	assert.Equal(t, "bowrain-source", status.ConnectorID)
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

	// Verify items have distinct names (sent via init).
	itemNames := map[string]bool{}
	for _, name := range mock.initItems {
		itemNames[name] = true
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
		pullBlocks: []apiclient.SyncBlock{
			mockSyncBlock("tu1", "farewell", "src/locales/en.json", "Goodbye", map[string]string{"fr": "Au revoir"}),
			mockSyncBlock("tu2", "greeting", "src/locales/en.json", "Hello", map[string]string{"fr": "Bonjour"}),
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
		pullBlocks: []apiclient.SyncBlock{
			mockSyncBlock("tu2", "greeting", "src/locales/en.json", "Hello", map[string]string{"fr": "Bonjour"}),
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
		pullBlocks: []apiclient.SyncBlock{
			mockSyncBlock("tu2", "greeting", "src/locales/en.json", "Hello", map[string]string{"fr": "Bonjour"}),
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
		pullBlocks: []apiclient.SyncBlock{
			mockSyncBlock("tu2", "greeting", "src/locales/en.json", "Hello", map[string]string{"fr": "Bonjour"}),
		},
	}
	proj, formatReg := setupTestProject(t, mock)

	// Set a dest pattern on the content entry.
	proj.Config.Content[0].Dest = "src/locales/{locale}.json"

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
			Defaults: Defaults{
				SourceLanguage: "en",
			},
			Content: []ContentEntry{
				{Path: "src/locales/*.json", Format: "json"},
			},
		},
	}

	conn := &BowrainSourceConnector{project: proj}

	// Default behavior: replace source locale in path.
	assert.Equal(t, "src/locales/fr.json", conn.resolveTargetPath("src/locales/en.json", "fr"))
	assert.Equal(t, "src/locales/de.json", conn.resolveTargetPath("src/locales/en.json", "de"))

	// With dest template using {locale} placeholder.
	proj.Config.Content[0].Dest = "output/{locale}.json"
	assert.Equal(t, "output/fr.json", conn.resolveTargetPath("src/locales/en.json", "fr"))

	// With {path} and {filename} placeholders.
	proj.Config.Content[0].Dest = "i18n/{locale}/{path}/{filename}"
	assert.Equal(t, "i18n/fr/en.json", conn.resolveTargetPath("src/locales/en.json", "fr"))

	// Docusaurus-style: deeper glob with {path}/{filename}.
	proj.Config.Content = []ContentEntry{
		{Path: "docs/**/*.md", Format: "markdown", Dest: "i18n/{locale}/docusaurus-plugin-content-docs/current/{path}/{filename}"},
	}
	assert.Equal(t, "i18n/nb/docusaurus-plugin-content-docs/current/intro.md", conn.resolveTargetPath("docs/intro.md", "nb"))
	assert.Equal(t, "i18n/nb/docusaurus-plugin-content-docs/current/features/overview.md", conn.resolveTargetPath("docs/features/overview.md", "nb"))

	// JSON with nested path.
	proj.Config.Content = []ContentEntry{
		{Path: "i18n/en/**/*.json", Format: "json", Dest: "i18n/{locale}/{path}/{filename}"},
	}
	assert.Equal(t, "i18n/nb/code.json", conn.resolveTargetPath("i18n/en/code.json", "nb"))
	assert.Equal(t, "i18n/nb/docusaurus-plugin-content-docs/current.json", conn.resolveTargetPath("i18n/en/docusaurus-plugin-content-docs/current.json", "nb"))

	// Fallback when source locale not in path.
	proj.Config.Content[0].Dest = ""
	proj.Config.Defaults.SourceLanguage = "en-US"
	assert.Equal(t, "src/messages.fr.json", conn.resolveTargetPath("src/messages.json", "fr"))
}

func TestGlobFixedPrefix(t *testing.T) {
	tests := []struct {
		pattern  string
		expected string
	}{
		{"docs/**/*.md", "docs/"},
		{"src/locales/*.json", "src/locales/"},
		{"i18n/en/**/*.json", "i18n/en/"},
		{"*.json", ""},
		{"blog/**/*.md", "blog/"},
	}
	for _, tt := range tests {
		t.Run(tt.pattern, func(t *testing.T) {
			assert.Equal(t, tt.expected, globFixedPrefix(tt.pattern))
		})
	}
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

	// Change content entry to a recursive glob, add an exclude.
	proj.Config.Content = []ContentEntry{
		{Path: "src/locales/**/*.json", Format: "json"},
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

	// Verify no items came from the excluded file.
	for _, name := range mock.initItems {
		assert.NotContains(t, name, "legacy", "excluded file should not produce blocks")
	}
}

func TestSourceConnector_PerEntryLanguageOverride(t *testing.T) {
	mock := &mockSyncHandler{}
	srv := httptest.NewServer(mock)
	t.Cleanup(srv.Close)

	root := t.TempDir()
	bowrainDir := filepath.Join(root, ".bowrain")
	require.NoError(t, os.MkdirAll(bowrainDir, 0755))

	// Create files in two source languages.
	require.NoError(t, os.MkdirAll(filepath.Join(root, "src", "en"), 0755))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "src", "es"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "src", "en", "ui.json"), []byte(`{"hello":"Hello"}`), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "src", "es", "ui.json"), []byte(`{"hola":"Hola"}`), 0644))

	cfg := &Config{
		URL: FormatProjectURL(srv.URL, "", "proj-multi"),
		Defaults: Defaults{
			SourceLanguage: "en",
		},
		Content: []ContentEntry{
			{Path: "src/en/**/*.json", Format: "json"},
			{Path: "src/es/**/*.json", Format: "json", Language: "es"},
		},
	}

	proj := &Project{Root: root, ConfigDir: bowrainDir, Config: cfg}
	writeClaimToken(t, bowrainDir, "tok")
	formatReg := registry.NewFormatRegistry()
	formats.RegisterAll(formatReg)

	conn, err := NewSourceConnector(proj, formatReg)
	require.NoError(t, err)
	defer conn.Close()

	// Push should pick up files from both content entries.
	result, err := conn.Push(context.Background(), connector.PushOptions{})
	require.NoError(t, err)
	assert.Equal(t, 2, result.FilesScanned, "should scan files from both source languages")
	assert.Equal(t, 2, result.BlocksPushed, "should push blocks from both files")

	// Verify both item names are present (sent via init).
	itemNames := map[string]bool{}
	for _, name := range mock.initItems {
		itemNames[name] = true
	}
	assert.True(t, itemNames["src/en/ui.json"], "should include English file")
	assert.True(t, itemNames["src/es/ui.json"], "should include Spanish file")
}

func TestSourceConnector_CollectionInPush(t *testing.T) {
	mock := &mockSyncHandler{}
	srv := httptest.NewServer(mock)
	t.Cleanup(srv.Close)

	root := t.TempDir()
	bowrainDir := filepath.Join(root, ".bowrain")
	require.NoError(t, os.MkdirAll(bowrainDir, 0755))

	require.NoError(t, os.MkdirAll(filepath.Join(root, "src", "locales"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "src", "locales", "en.json"), []byte(`{"hello":"Hello"}`), 0644))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "docs"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "docs", "intro.json"), []byte(`{"title":"Welcome"}`), 0644))

	cfg := &Config{
		URL: FormatProjectURL(srv.URL, "", "proj-col"),
		Defaults: Defaults{
			SourceLanguage: "en",
			Collection:     "default-col",
		},
		Content: []ContentEntry{
			{Path: "src/locales/*.json", Format: "json", Collection: "ui"},
			{Path: "docs/*.json", Format: "json"}, // inherits default-col
		},
	}

	proj := &Project{Root: root, ConfigDir: bowrainDir, Config: cfg}
	writeClaimToken(t, bowrainDir, "tok")
	formatReg := registry.NewFormatRegistry()
	formats.RegisterAll(formatReg)

	conn, err := NewSourceConnector(proj, formatReg)
	require.NoError(t, err)
	defer conn.Close()

	_, err = conn.Push(context.Background(), connector.PushOptions{})
	require.NoError(t, err)

	// Verify that items from both collections were pushed.
	itemNames := map[string]bool{}
	for _, name := range mock.initItems {
		itemNames[name] = true
	}
	assert.True(t, itemNames["src/locales/en.json"], "ui file should be pushed")
	assert.True(t, itemNames["docs/intro.json"], "docs file should be pushed")
}

func TestSourceConnector_ServerTargetLanguagesFallback(t *testing.T) {
	// Mock a server that returns project metadata AND handles sync endpoints.
	mock := &mockSyncHandler{
		pullBlocks: []apiclient.SyncBlock{
			mockSyncBlock("tu1", "hello", "src/locales/en.json", "Hello", map[string]string{"fr": "Bonjour"}),
		},
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Metadata endpoint: GET on project prefix without /sync/
		if r.Method == http.MethodGet && !contains(r.URL.Path, "/sync/") && contains(r.URL.Path, "/projects/") {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(apiclient.ProjectMetadata{
				ID:                    "proj-auto",
				Name:                  "Test",
				DefaultSourceLanguage: "en",
				TargetLanguages:       []string{"fr", "de"},
			})
			return
		}
		mock.ServeHTTP(w, r)
	})

	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	root := t.TempDir()
	bowrainDir := filepath.Join(root, ".bowrain")
	require.NoError(t, os.MkdirAll(bowrainDir, 0755))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "src", "locales"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "src", "locales", "en.json"), []byte(`{"hello":"Hello"}`), 0644))

	// Config has NO target_languages — should fall back to server metadata.
	cfg := &Config{
		URL: FormatProjectURL(srv.URL, "", "proj-auto"),
		Defaults: Defaults{
			SourceLanguage: "en",
			// No TargetLanguages — force fallback to server.
		},
		Content: []ContentEntry{
			{Path: "src/locales/*.json", Format: "json"},
		},
	}

	proj := &Project{Root: root, ConfigDir: bowrainDir, Config: cfg}
	writeClaimToken(t, bowrainDir, "test-token")
	formatReg := registry.NewFormatRegistry()
	formats.RegisterAll(formatReg)

	conn, err := NewSourceConnector(proj, formatReg)
	require.NoError(t, err)
	defer conn.Close()

	// Pull should resolve target locales from server metadata.
	result, err := conn.Pull(context.Background(), connector.PullOptions{})
	require.NoError(t, err)
	assert.Equal(t, 2, result.LocalesCount, "should use server's fr+de target locales")

	// Verify metadata was cached.
	require.NotNil(t, conn.cache.ServerMeta)
	assert.Equal(t, []string{"fr", "de"}, conn.cache.ServerMeta.TargetLanguages)
}

func TestContentEntry_EffectiveLanguage(t *testing.T) {
	ce := ContentEntry{Path: "src/*.json"}
	assert.Equal(t, "en", ce.EffectiveLanguage("en"))

	ce.Language = "es"
	assert.Equal(t, "es", ce.EffectiveLanguage("en"))
}

func TestContentEntry_EffectiveTargetLanguages(t *testing.T) {
	defaults := []model.LocaleID{"fr", "de"}

	ce := ContentEntry{Path: "src/*.json"}
	assert.Equal(t, defaults, ce.EffectiveTargetLanguages(defaults))

	ce.TargetLanguages = []model.LocaleID{"ja", "ko"}
	assert.Equal(t, []model.LocaleID{"ja", "ko"}, ce.EffectiveTargetLanguages(defaults))
}

func TestSourceConnector_InterfaceCompliance(t *testing.T) {
	// Compile-time check that BowrainSourceConnector implements connector.SourceConnector.
	var _ connector.SourceConnector = (*BowrainSourceConnector)(nil)
}
