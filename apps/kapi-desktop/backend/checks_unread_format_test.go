package backend

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/check"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/registry"
)

// The Checks panel runs the check `kapi check` runs, over the content the
// recipe declares. A collection whose format a plugin supplies cannot be read
// on a machine without that plugin. The panel checks the rest and names the
// files it could not read in the result it renders, as the CLI does.

// unreadCheckProject writes and opens a project with a source-only collection in
// `sourcecode` and, when layout is set, a translated collection in `okf_idml`.
// Both are formats a plugin supplies. When readable is non-empty it adds a JSON
// collection with a French target. It returns the tab ID.
func unreadCheckProject(t *testing.T, app *App, readable string, layout bool) string {
	t.Helper()
	dir := t.TempDir()
	write := func(rel, content string) {
		t.Helper()
		path := filepath.Join(dir, filepath.FromSlash(rel))
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	}
	write("voice.yaml", `id: house
name: House Style
vocabulary:
  forbidden_terms:
    - term: utilize
      replacement: use
      severity: major
`)
	write("cask/kapi.rb", "cask \"kapi\" do\n  desc \"Please utilize the content engine\"\nend\n")

	proj := &project.KapiProject{
		Version:  project.CurrentVersion,
		Defaults: project.Defaults{SourceLanguage: "en"},
	}
	if readable != "" {
		write("locales/en.json", readable)
		write("locales/fr.json", `{"greeting":"Bonjour le monde"}`)
		proj.Collections = append(proj.Collections, project.Collection{
			Name:    "app",
			Content: []project.ContentItem{{Path: "locales/en.json", Target: "locales/{lang}.json"}},
		})
	}
	proj.Collections = append(proj.Collections, project.Collection{
		Name:    "cask",
		Content: []project.ContentItem{{Path: "cask/*.rb", Format: &project.FormatSpec{Name: "sourcecode"}}},
	})
	if layout {
		write("pkg/doc.idml", "<doc>Hello</doc>\n")
		write("pkg/doc.fr.idml", "<doc>Bonjour</doc>\n")
		proj.Collections = append(proj.Collections, project.Collection{
			Name: "layout",
			Content: []project.ContentItem{{
				Path: "pkg/doc.idml", Target: "pkg/doc.{lang}.idml",
				Format: &project.FormatSpec{Name: "okf_idml"},
			}},
		})
	}
	projPath := filepath.Join(dir, "proj.kapi")
	require.NoError(t, project.Save(projPath, proj))

	tab, err := app.OpenProject(projPath)
	require.NoError(t, err)
	t.Cleanup(func() { app.CloseProject(tab.ID) })
	return tab.ID
}

// isolateCheckPlugins keeps the app from discovering plugins installed on the
// machine, so a plugin's format is absent however the machine is set up.
func isolateCheckPlugins(t *testing.T) {
	t.Helper()
	isolateConfig(t)
	dir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", filepath.Join(dir, "data"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(dir, "cache"))
	t.Setenv("KAPI_PLUGINS_DIR_ONLY", "1")
	t.Setenv("KAPI_PLUGINS_DIR", filepath.Join(dir, "plugins"))
}

// requireNoReaderWarning holds a result to naming the file it could not read,
// the format and the plugin to install, as data the panel renders.
func requireNoReaderWarning(t *testing.T, res *CheckRunResult, file, format string) {
	t.Helper()
	var found *check.Warning
	for i := range res.Warnings {
		if res.Warnings[i].Code == "format.no_reader" && res.Warnings[i].Source == file {
			found = &res.Warnings[i]
		}
	}
	require.NotNil(t, found, "a format.no_reader warning names %s: %+v", file, res.Warnings)
	assert.Contains(t, found.Message, `"`+format+`"`)
	assert.Contains(t, found.Message, "kapi plugins install "+format)
}

func checkedPaths(res *CheckRunResult) []string {
	var paths []string
	for _, f := range res.Files {
		paths = append(paths, filepath.ToSlash(f.Path))
	}
	return paths
}

// The lead's first repro: a source-only collection in `sourcecode` beside
// content the panel reads.
func TestRunChecksSkipsDeclaredContentWithNoReader(t *testing.T) {
	isolateCheckPlugins(t)
	app := NewApp()
	tabID := unreadCheckProject(t, app, `{"greeting":"Hello world"}`, false)

	res, err := app.RunChecks(tabID, ProjectFilter{Languages: []string{"fr"}})
	require.NoError(t, err, "a missing plugin format leaves the rest of the project checkable")
	// A passed verdict is decided over checked blocks: a run that checked none
	// did not run.
	assert.Equal(t, "passed", res.Verdict)
	assert.True(t, res.Pass)
	requireNoReaderWarning(t, res, filepath.Join("cask", "kapi.rb"), "sourcecode")
	paths := checkedPaths(res)
	assert.True(t, slicesContainSuffix(paths, "locales/en.json"), "the readable file was checked: %v", paths)
	assert.False(t, slicesContainSuffix(paths, "cask/kapi.rb"), "an unread file is never listed as checked: %v", paths)
}

// The lead's second repro: a translated collection in `okf_idml`. Its source and
// its target are skipped together.
func TestRunChecksSkipsTranslatedContentWithNoReader(t *testing.T) {
	isolateCheckPlugins(t)
	app := NewApp()
	tabID := unreadCheckProject(t, app, `{"greeting":"Hello world"}`, true)

	res, err := app.RunChecks(tabID, ProjectFilter{Languages: []string{"fr"}})
	require.NoError(t, err)
	assert.Equal(t, "passed", res.Verdict)
	requireNoReaderWarning(t, res, filepath.Join("pkg", "doc.idml"), "okf_idml")
	requireNoReaderWarning(t, res, filepath.Join("cask", "kapi.rb"), "sourcecode")
	paths := checkedPaths(res)
	assert.True(t, slicesContainSuffix(paths, "locales/en.json"), "%v", paths)
	assert.False(t, slicesContainSuffix(paths, "pkg/doc.idml"), "%v", paths)
}

// A project whose only content has no reader gave the panel nothing to check,
// and content in its scope went unchecked. The panel never shows that as a pass.
func TestRunChecksOfOnlyUnreadableContentDidNotRun(t *testing.T) {
	isolateCheckPlugins(t)
	app := NewApp()
	tabID := unreadCheckProject(t, app, "", true)

	res, err := app.RunChecks(tabID, ProjectFilter{Languages: []string{"fr"}})
	require.NoError(t, err)
	assert.False(t, res.Pass)
	assert.Equal(t, "did_not_run", res.Verdict)
	assert.Equal(t, check.CauseContentNotChecked, res.DidNotRunCause)
	reasons := strings.Join(res.DidNotRun, "; ")
	assert.Contains(t, reasons, filepath.Join("cask", "kapi.rb"))
	assert.Contains(t, reasons, filepath.Join("pkg", "doc.idml"))
	requireNoReaderWarning(t, res, filepath.Join("cask", "kapi.rb"), "sourcecode")
}

// Only a missing reader is skipped. A JSON file that does not parse was opened
// and is broken, and the run still fails on it.
func TestRunChecksStillFailsOnABrokenFileInAKnownFormat(t *testing.T) {
	isolateCheckPlugins(t)
	app := NewApp()
	tabID := unreadCheckProject(t, app, `{"greeting": "Hello`, false)

	_, err := app.RunChecks(tabID, ProjectFilter{Languages: []string{"fr"}})
	require.Error(t, err)
	require.NotErrorIs(t, err, registry.ErrUnknownFormat)
}

func slicesContainSuffix(paths []string, suffix string) bool {
	for _, p := range paths {
		if strings.HasSuffix(p, suffix) {
			return true
		}
	}
	return false
}
