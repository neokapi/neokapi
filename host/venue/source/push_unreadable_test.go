package source

import (
	"context"
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
	"github.com/neokapi/neokapi/core/venue"
	bowrainconn "github.com/neokapi/neokapi/core/venue/connector"
	apiclient "github.com/neokapi/neokapi/host/venue/client"
	"github.com/neokapi/neokapi/host/venue/config"
	bproject "github.com/neokapi/neokapi/host/venue/project"
)

// futureBundle is a bundle in a schema major this build does not read, as a
// catalog written by a newer extractor is to an older kapi.
const futureBundle = `{
  "schemaVersion": "3.0",
  "kind": "kapi-bundle",
  "generator": {"id": "test", "version": "0"},
  "project": {"id": "app", "sourceLocale": "en"},
  "documents": []
}`

// fileConnector is a project with the given collections and files, pointed at
// srv.
func fileConnector(t *testing.T, srv *scopeServer, collections []coreproj.Collection, files map[string]string) *BowrainSourceConnector {
	t.Helper()
	root := t.TempDir()
	reg := registry.NewFormatRegistry()
	formats.RegisterAll(reg)
	proj, err := bproject.InitProject(root, &bproject.Recipe{
		Defaults:    coreproj.Defaults{SourceLanguage: "en", TargetLanguages: []model.LocaleID{"fr"}},
		Collections: collections,
		Server:      &bproject.ServerSpec{URL: srv.URL + "/projects/proj1", Stream: "main"},
	})
	require.NoError(t, err)
	for rel, body := range files {
		abs := filepath.Join(root, filepath.FromSlash(rel))
		require.NoError(t, os.MkdirAll(filepath.Dir(abs), 0o755))
		require.NoError(t, os.WriteFile(abs, []byte(body), 0o644))
	}
	client := apiclient.NewProjectBearerClient(srv.URL, "proj1", "test-token")
	client.SetStream("main")
	conn := &BowrainSourceConnector{
		app:       testApp(t),
		project:   proj,
		client:    client,
		formatReg: reg,
		cache:     bproject.LoadSyncCache(proj.Layout),
		refs:      refcache.Load(proj.Layout, config.NormalizeServerURL(srv.URL), "proj1"),
		stream:    "main",
		maxBatch:  1000,
	}
	t.Cleanup(func() { conn.Close() })
	return conn
}

// A bundle the reader refuses is a file the push covers and cannot read. Were
// it skipped, the declared tree would leave it out and the venue would delete
// what it holds there, so the push stops before it sends anything, naming the
// file.
func TestPushStopsWhenAFileItCoversCannotBeRead(t *testing.T) {
	srv := newScopeServer(t, "proj1", []venue.TreeItem{
		{Path: "locales/en/app.json", ID: "item-app", Keys: []string{"greeting"}, Content: []string{"sha-app"}},
		{Path: "i18n/app.kbf.json", ID: "item-catalog", Keys: []string{"d1:b1"}, Content: []string{"sha-catalog"}},
	})
	conn := fileConnector(t, srv, []coreproj.Collection{
		{Name: "app", Content: []coreproj.ContentItem{{Path: "locales/en/*.json", Format: &coreproj.FormatSpec{Name: "json"}}}},
		{Name: "ui", Content: []coreproj.ContentItem{{Path: "i18n/*.kbf.json"}}},
	}, map[string]string{
		"locales/en/app.json": `{"greeting":"Hello"}`,
		"i18n/app.kbf.json":   futureBundle,
	})

	_, err := conn.Push(context.Background(), bowrainconn.PushOptions{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "i18n/app.kbf.json")
	assert.Contains(t, err.Error(), "unsupported major schemaVersion 3")
	assert.Empty(t, srv.commits, "nothing was committed")
	assert.Empty(t, srv.removals, "nothing was removed")

	// The commands that report on the same scan say why too, rather than
	// counting the file as gone.
	_, err = conn.Diff(context.Background(), nil)
	require.Error(t, err)
	_, err = conn.ListFiles(context.Background(), nil)
	require.Error(t, err)
	status, err := conn.Status(context.Background())
	require.NoError(t, err)
	require.Len(t, status.Errors, 1)
	assert.Contains(t, status.Errors[0], "i18n/app.kbf.json")
}

// A file the recipe's pattern matches and no format reads holds nothing to
// push. The push leaves it out of the scope it declares, so the venue keeps
// what it holds under that path, while a file the collection no longer has is
// still removed.
func TestPushLeavesAFileNoFormatReadsOutOfItsScope(t *testing.T) {
	srv := newScopeServer(t, "proj1", []venue.TreeItem{
		{Path: "docs/guide.md", ID: "item-guide", Keys: []string{"p1"}, Content: []string{"sha-guide"}},
		{Path: "docs/diagram.zzz", ID: "item-diagram", Keys: []string{"k"}, Content: []string{"sha-diagram"}},
		{Path: "docs/gone.md", ID: "item-gone", Keys: []string{"p1"}, Content: []string{"sha-gone"}},
	})
	conn := fileConnector(t, srv, []coreproj.Collection{
		{Name: "docs", Content: []coreproj.ContentItem{{Path: "docs/*"}}},
	}, map[string]string{
		"docs/guide.md":    "# Guide\n\nRead this first.\n",
		"docs/diagram.zzz": "\x00\x01\x02 not a document \x03",
	})
	require.Empty(t, conn.detectFormat(filepath.Join(conn.project.Root, "docs", "diagram.zzz")),
		"the fixture needs a file no format reads")

	_, err := conn.Push(context.Background(), bowrainconn.PushOptions{})
	require.NoError(t, err)

	require.Len(t, srv.commits, 1)
	assert.Contains(t, []string(srv.commits[0].Scope), "!docs/diagram.zzz")
	assert.Equal(t, []string{"docs/gone.md"}, srv.removals,
		"the file no format reads stays, and the file the collection no longer has goes")
}

// An excluded path names that file alone, even when it holds a character a
// pattern would read as a glob.
func TestExcludePathsNamesEachFileAlone(t *testing.T) {
	scope := excludePaths(venue.Scope{"docs/*"}, []string{"docs/a[1].json", "docs/b*.json", "docs/plain.json"})
	assert.False(t, scope.Covers("docs/a[1].json"))
	assert.False(t, scope.Covers("docs/b*.json"))
	assert.False(t, scope.Covers("docs/plain.json"))
	assert.True(t, scope.Covers("docs/a1.json"), "the bracket is no character class")
	assert.True(t, scope.Covers("docs/bee.json"), "the star is no wildcard")
	assert.True(t, scope.Covers("docs/other.json"))

	assert.Nil(t, excludePaths(nil, []string{"docs/a.json"}), "no scope stays no scope")
}
