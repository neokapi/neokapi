package source

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
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
		for _, r := range b.SourceRuns() {
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

// TestPull_ABlockScalarTranslationKeepsItsEnding pins that a pull lands a
// translation whose ending differs from its source's. The source is a YAML
// literal block scalar, which ends in a line break, and the server holds the
// translation without one. The translation's file already holds the same
// words with the line break. The pull writes the strip indicator, a read of
// the file gives the translation as the server holds it, and a second pull of
// the same translation succeeds and leaves the file as it is.
func TestPull_ABlockScalarTranslationKeepsItsEnding(t *testing.T) {
	const (
		base   = "scripts/gen-refs/nativedocs"
		item   = base + "/tools/script.yaml"
		source = "description: Run a user-supplied ES5 JavaScript script over each part.\n" +
			"parameters:\n" +
			"  script_file:\n" +
			"    description: Path to a JavaScript file.\n" +
			"    help: |\n" +
			"      Path to a `.js` file to load and run, used when `source` is `file`.\n" +
			"      Required in that mode. In a recipe, a relative path is read from the\n" +
			"      project root, the directory that holds `kapi.yaml`.\n" +
			"    values: \"file path\"\n"
		heldHelp = "Sti til en `.js`-fil som skal lastes inn og kjøres når `source` er `file`.\n" +
			"Påkrevd i denne modusen.\n"
		translation = "description: Kjør et brukerspesifisert ES5 JavaScript-skript på hver del.\n" +
			"parameters:\n" +
			"  script_file:\n" +
			"    description: Bane til en JavaScript-fil.\n" +
			"    help: |%s\n" +
			"      Sti til en `.js`-fil som skal lastes inn og kjøres når `source` er `file`.\n" +
			"      Påkrevd i denne modusen.\n" +
			"    values: \"filbane\"\n"
	)
	root := t.TempDir()
	reg := registry.NewFormatRegistry()
	formats.RegisterAll(reg)

	srcPath := filepath.Join(root, filepath.FromSlash(item))
	nbPath := filepath.Join(root, filepath.FromSlash(base), "nb", "tools", "script.yaml")
	for path, body := range map[string]string{srcPath: source, nbPath: fmt.Sprintf(translation, "")} {
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
	}

	var pulled []apiclient.SyncBlock
	mux := http.NewServeMux()
	metaPath := "/api/v1/projects/proj123"
	mux.HandleFunc(metaPath, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(apiclient.ProjectMetadata{ID: "proj123", DefaultSourceLanguage: "en", TargetLanguages: []string{"nb"}})
	})
	mux.HandleFunc(metaPath+"/sync/main/pull", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(apiclient.RichPullResponse{Cursor: 42, Blocks: pulled})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	proj, err := bproject.InitProject(root, &bproject.Recipe{
		Defaults: coreproj.Defaults{SourceLanguage: "en", TargetLanguages: []model.LocaleID{"nb"}},
		Collections: []coreproj.Collection{{
			Name: "reference", Base: base,
			Content: []coreproj.ContentItem{{Path: "*/*.yaml", Target: "{lang}/{path}.yaml", Format: &coreproj.FormatSpec{Name: "yaml"}}},
		}},
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

	// The server holds what the translation's file holds, except that the
	// help text ends without the line break.
	ctx := context.Background()
	const helpKey = "parameters.script_file.help"
	served := strings.TrimSuffix(heldHelp, "\n")
	held, err := conn.readBlocks(ctx, nbPath, "yaml")
	require.NoError(t, err)
	heldText := map[string]string{}
	for _, b := range held {
		heldText[b.Name] = b.SourceText()
	}
	require.Equal(t, heldHelp, heldText[helpKey])
	local, err := conn.readBlocks(ctx, srcPath, "yaml")
	require.NoError(t, err)
	for _, b := range local {
		text, ok := heldText[b.Name]
		if !b.Translatable || !ok {
			continue
		}
		if b.Name == helpKey {
			text = served
		}
		b.SetTargetRuns("nb", []model.Run{{Text: &model.TextRun{Text: text}}})
		pulled = append(pulled, apiclient.BlockToSyncBlock(b, item))
	}
	require.Len(t, pulled, len(heldText))

	_, err = conn.Pull(ctx, bowrainconn.PullOptions{})
	require.NoError(t, err)
	got, err := os.ReadFile(nbPath)
	require.NoError(t, err)
	assert.Equal(t, fmt.Sprintf(translation, "-"), string(got))
	held, err = conn.readBlocks(ctx, nbPath, "yaml")
	require.NoError(t, err)
	for _, b := range held {
		if b.Name == helpKey {
			assert.Equal(t, served, b.SourceText(), "the file holds the translation as the server holds it")
		}
	}

	_, err = conn.Pull(ctx, bowrainconn.PullOptions{Force: true})
	require.NoError(t, err, "a second pull of the same translation changes nothing and succeeds")
	again, err := os.ReadFile(nbPath)
	require.NoError(t, err)
	assert.Equal(t, string(got), string(again))
}
