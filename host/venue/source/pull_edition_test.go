package source

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/formats"
	"github.com/neokapi/neokapi/core/model"
	coreproj "github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/ref/refcache"
	"github.com/neokapi/neokapi/core/registry"
	bowrainconn "github.com/neokapi/neokapi/core/venue/connector"
	apiclient "github.com/neokapi/neokapi/host/venue/client"
	"github.com/neokapi/neokapi/host/venue/config"
	bproject "github.com/neokapi/neokapi/host/venue/project"
)

// TestPull_AnInlineCodeKeepsItsCode pins that a pull lands the runs the
// server holds, inline codes included: the translation of a Markdown
// paragraph with bold text keeps the bold, and the translation's file is
// written from the source's skeleton through the change service.
func TestPull_AnInlineCodeKeepsItsCode(t *testing.T) {
	const source = "# Title\n\nHello **world** today.\n"
	root := t.TempDir()
	reg := registry.NewFormatRegistry()
	formats.RegisterAll(reg)

	srcPath := filepath.Join(root, "docs", "en", "guide.md")
	require.NoError(t, os.MkdirAll(filepath.Dir(srcPath), 0o755))
	require.NoError(t, os.WriteFile(srcPath, []byte(source), 0o644))

	var pulled []apiclient.SyncBlock
	mux := http.NewServeMux()
	metaPath := "/api/v1/projects/proj123"
	mux.HandleFunc(metaPath, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(apiclient.ProjectMetadata{ID: "proj123", DefaultSourceLanguage: "en", TargetLanguages: []string{"fr"}})
	})
	mux.HandleFunc(metaPath+"/sync/main/pull", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(apiclient.RichPullResponse{Cursor: 42, Blocks: pulled})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	proj, err := bproject.InitProject(root, &bproject.Recipe{
		Defaults: coreproj.Defaults{SourceLanguage: "en", TargetLanguages: []model.LocaleID{"fr"}},
		Collections: []coreproj.Collection{
			{Path: "docs/en/guide.md", Target: "docs/{lang}/guide.md", Format: &coreproj.FormatSpec{Name: "markdown"}},
		},
		Server: &bproject.ServerSpec{URL: srv.URL + "/projects/proj123", Stream: "main"},
	})
	require.NoError(t, err)
	serverURL := config.NormalizeServerURL(srv.URL)
	client := apiclient.NewProjectBearerClient(srv.URL, "proj123", "test-token")
	client.SetStream("main")
	conn := &BowrainSourceConnector{
		app: testApp(t), project: proj, client: client, formatReg: reg,
		cache: bproject.LoadSyncCache(proj.Layout), refs: refcache.Load(proj.Layout, serverURL, "proj123"),
		stream: "main", maxBatch: 1000,
	}
	defer conn.Close()

	// The server's translation is the source's runs with French text, so it
	// carries the source's bold code as a reader of the file finds it.
	french := map[string]string{"Title": "Titre", "Hello ": "Bonjour ", "world": "le monde", " today.": " aujourd'hui."}
	local, err := conn.readBlocks(context.Background(), srcPath, "markdown")
	require.NoError(t, err)
	for _, b := range local {
		if !b.Translatable {
			continue
		}
		var runs []model.Run
		for _, r := range b.Source {
			if r.Text != nil {
				fr, ok := french[r.Text.Text]
				require.True(t, ok, "no French for %q", r.Text.Text)
				r = model.Run{Text: &model.TextRun{Text: fr}}
			}
			runs = append(runs, r)
		}
		b.SetTargetRuns("fr", runs)
		pulled = append(pulled, apiclient.BlockToSyncBlock(b, "docs/en/guide.md"))
	}
	require.Len(t, pulled, 2)

	res, err := conn.Pull(context.Background(), bowrainconn.PullOptions{})
	require.NoError(t, err)
	assert.Equal(t, 1, res.FilesWritten)
	got, err := os.ReadFile(filepath.Join(root, "docs", "fr", "guide.md"))
	require.NoError(t, err)
	assert.Equal(t, "# Titre\n\nBonjour **le monde** aujourd'hui.\n", string(got))
}
