package connector

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	bowrainconn "github.com/neokapi/neokapi/bowrain/core/connector"
	bproject "github.com/neokapi/neokapi/bowrain/core/project"
	"github.com/neokapi/neokapi/core/formats"
	"github.com/neokapi/neokapi/core/model"
	coreproj "github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newServerProject scaffolds a project whose recipe carries the given server
// block, with one JSON content entry on disk.
func newServerProject(t *testing.T, server *bproject.ServerSpec, content []coreproj.ContentCollection) (*bproject.Project, *registry.FormatRegistry) {
	t.Helper()
	root := t.TempDir()

	reg := registry.NewFormatRegistry()
	formats.RegisterAll(reg)

	if content == nil {
		content = []coreproj.ContentCollection{
			{Path: "locales/en.json", Format: &coreproj.FormatSpec{Name: "json"}},
		}
	}
	recipe := &bproject.Recipe{
		KapiProject: coreproj.KapiProject{
			Defaults: coreproj.Defaults{
				SourceLanguage:  "en",
				TargetLanguages: []model.LocaleID{"fr"},
			},
			Content: content,
		},
		Server: server,
	}
	proj, err := bproject.InitProject(root, recipe)
	require.NoError(t, err)

	abs := filepath.Join(root, "locales", "en.json")
	require.NoError(t, os.MkdirAll(filepath.Dir(abs), 0o755))
	require.NoError(t, os.WriteFile(abs, []byte(`{"greeting":"Hello","farewell":"Goodbye"}`), 0o644))

	return proj, reg
}

func TestNewSourceConnector_RequiresServerBlock(t *testing.T) {
	proj, reg := newDiffTestProject(t, `{"greeting":"Hello"}`)

	_, err := NewSourceConnector(proj, reg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "server")
}

func TestNewSourceConnector_RequiresProjectID(t *testing.T) {
	proj, reg := newServerProject(t, &bproject.ServerSpec{URL: "https://bowrain.example"}, nil)

	_, err := NewSourceConnector(proj, reg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "project ID")
}

func TestNewSourceConnector_ClaimTokenFromCache(t *testing.T) {
	proj, reg := newServerProject(t, &bproject.ServerSpec{
		URL:    "https://bowrain.example/projects/proj123",
		Stream: "main",
	}, nil)

	// A persisted claim token must be enough to construct the connector —
	// no keychain or auth.json involved.
	cache := bproject.LoadSyncCache(proj.Layout)
	cache.ClaimToken = "clm_test"
	require.NoError(t, cache.Save(proj.Layout))

	conn, err := NewSourceConnector(proj, reg)
	require.NoError(t, err)
	defer conn.Close()

	assert.Equal(t, "bowrain-source", conn.ID())
	assert.NotEmpty(t, conn.Name())
	assert.Equal(t, "main", conn.stream)
}

// TestSyncCache_Persistence covers the round-trip the connector relies on:
// cursors, claim token, and per-file block hashes survive Save + reload.
func TestSyncCache_Persistence(t *testing.T) {
	proj, _ := newDiffTestProject(t, `{"greeting":"Hello"}`)

	cache := bproject.LoadSyncCache(proj.Layout)
	cache.ClaimToken = "clm_persist"
	cache.SetStreamCursor("main", 42)
	cache.Files["locales/en.json"] = &bproject.FileCache{Blocks: map[string]string{"b1": "h1"}}
	require.NoError(t, cache.Save(proj.Layout))

	reloaded := bproject.LoadSyncCache(proj.Layout)
	assert.Equal(t, "clm_persist", reloaded.ClaimToken)
	assert.Equal(t, int64(42), reloaded.GetStreamCursor("main"))
	require.Contains(t, reloaded.Files, "locales/en.json")
	assert.Equal(t, "h1", reloaded.Files["locales/en.json"].Blocks["b1"])
}

// TestListFiles_ContentIteration covers content iteration with the unified
// Recipe type: the connector lists exactly the files the recipe's content
// entries match, with their resolved format.
func TestListFiles_ContentIteration(t *testing.T) {
	proj, reg := newDiffTestProject(t, `{"greeting":"Hello"}`)

	conn := NewLocalConnector(proj, reg)
	defer conn.Close()

	files, err := conn.ListFiles(context.Background(), nil)
	require.NoError(t, err)
	require.Len(t, files, 1)
	assert.Equal(t, "locales/en.json", files[0].Path)
	assert.Equal(t, "json", files[0].Format)
}

func TestResolveTargetPath(t *testing.T) {
	tests := []struct {
		name    string
		content []coreproj.ContentCollection
		item    string
		locale  string
		want    string
	}{
		{
			name: "default replaces source locale segment",
			content: []coreproj.ContentCollection{
				{Path: "locales/en.json", Format: &coreproj.FormatSpec{Name: "json"}},
			},
			item:   "locales/en.json",
			locale: "fr",
			want:   "locales/fr.json",
		},
		{
			name: "lang placeholder pattern",
			content: []coreproj.ContentCollection{
				{Path: "src/{lang}/**/*.json", Target: "src/{lang}/**/*.json", Format: &coreproj.FormatSpec{Name: "json"}},
			},
			item:   "src/en/foo/bar.json",
			locale: "fr",
			want:   "src/fr/foo/bar.json",
		},
		{
			name: "legacy locale path filename placeholders",
			content: []coreproj.ContentCollection{
				{Path: "locales/en.json", Target: "out/{locale}/{path}/{filename}", Format: &coreproj.FormatSpec{Name: "json"}},
			},
			item:   "locales/en.json",
			locale: "fr",
			want:   "out/fr/en.json",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			reg := registry.NewFormatRegistry()
			formats.RegisterAll(reg)

			recipe := &bproject.Recipe{
				KapiProject: coreproj.KapiProject{
					Defaults: coreproj.Defaults{
						SourceLanguage:  "en",
						TargetLanguages: []model.LocaleID{"fr"},
					},
					Content: tt.content,
				},
			}
			proj, err := bproject.InitProject(root, recipe)
			require.NoError(t, err)

			conn := NewLocalConnector(proj, reg)
			defer conn.Close()

			assert.Equal(t, tt.want, conn.resolveTargetPath(tt.item, tt.locale))
		})
	}
}

// TestStatus_CountsPendingPushAndPull covers Status end to end: local blocks
// not in the sync cache count as pending pushes, and a server with rich pull
// blocks past the cursor counts as pending pulls.
func TestStatus_CountsPendingPushAndPull(t *testing.T) {
	srv := pullTestServer(t, "proj123", []string{"fr"}, 7)
	conn := newPullTestConnector(t, srv, []string{"fr"}, 5)
	defer conn.Close()

	status, err := conn.Status(context.Background())
	require.NoError(t, err)

	assert.Equal(t, "bowrain-source", status.ConnectorID)
	assert.Equal(t, 1, status.FileCount)
	assert.Equal(t, 2, status.ItemCount, "two blocks scanned from locales/en.json")
	assert.Equal(t, 2, status.PendingPush, "nothing in the sync cache yet, both blocks pending")
	assert.Equal(t, 2, status.PendingPull, "server reports two blocks past the cursor")
	assert.Positive(t, status.WordCount)
}

// TestStatus_CleanAfterSeededCache is the quiet-state counterpart: with the
// cache seeded from the current scan and no cursor, nothing is pending.
func TestStatus_CleanAfterSeededCache(t *testing.T) {
	srv := pullTestServer(t, "proj123", []string{"fr"}, 7)
	conn := newPullTestConnector(t, srv, []string{"fr"}, 0)
	defer conn.Close()

	seedCacheFromScan(t, conn.project, conn.formatReg, nil)
	conn.cache = bproject.LoadSyncCache(conn.project.Layout)

	status, err := conn.Status(context.Background())
	require.NoError(t, err)
	assert.Zero(t, status.PendingPush)
	assert.Zero(t, status.PendingPull, "cursor 0 means no remote probe")
}

// TestPush_DryRunAndUpToDate covers the push paths that need no Merkle
// endpoints: a dry run reports what would be pushed without contacting the
// server, and a fully cached project (with no cursor) reports up to date.
func TestPush_DryRunAndUpToDate(t *testing.T) {
	srv := pullTestServer(t, "proj123", []string{"fr"}, 7)
	conn := newPullTestConnector(t, srv, []string{"fr"}, 0)
	defer conn.Close()

	dry, err := conn.Push(context.Background(), bowrainconn.PushOptions{DryRun: true})
	require.NoError(t, err)
	assert.Equal(t, 2, dry.BlocksPushed)
	assert.Equal(t, 1, dry.FilesScanned)
	assert.Positive(t, dry.WordCount)

	seedCacheFromScan(t, conn.project, conn.formatReg, nil)
	conn.cache = bproject.LoadSyncCache(conn.project.Layout)

	res, err := conn.Push(context.Background(), bowrainconn.PushOptions{})
	require.NoError(t, err)
	assert.Zero(t, res.BlocksPushed)
	assert.Equal(t, 1, res.FilesScanned)
}
