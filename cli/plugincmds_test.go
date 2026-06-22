package cli

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/cli/pluginhost"
	pluginreg "github.com/neokapi/neokapi/cli/pluginhost/registry"
	"github.com/neokapi/neokapi/core/plugin/manifest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// pluginFixture builds a tiny gzipped tarball containing a single
// manifest.json. The returned bytes can be served by an httptest
// server so InstallFromRegistry can complete end-to-end.
func buildPluginFixture(t *testing.T, name, version string) []byte {
	t.Helper()
	manifest := map[string]any{
		"manifest_version": "1",
		"plugin":           name,
		"version":          version,
		"binary":           "kapi-" + name,
		"capabilities":     map[string]any{},
	}
	body, err := json.Marshal(manifest)
	require.NoError(t, err)

	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	hdr := &tar.Header{
		Name:     name + "/",
		Mode:     0o755,
		Typeflag: tar.TypeDir,
	}
	require.NoError(t, tw.WriteHeader(hdr))
	hdr = &tar.Header{
		Name:     name + "/manifest.json",
		Mode:     0o644,
		Size:     int64(len(body)),
		Typeflag: tar.TypeReg,
	}
	require.NoError(t, tw.WriteHeader(hdr))
	_, err = tw.Write(body)
	require.NoError(t, err)
	require.NoError(t, tw.Close())
	require.NoError(t, gz.Close())
	return buf.Bytes()
}

func sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// servePluginRegistry returns an httptest.Server hosting:
//   - /plugins.json    → registry index
//   - /demo-VERSION.tar.gz → individual plugin tarballs
//
// versions is map[version]channel; "" channel falls back to "stable".
func servePluginRegistry(t *testing.T, name string, versions map[string]string) (*httptest.Server, map[string][]byte) {
	t.Helper()
	tarballs := make(map[string][]byte, len(versions))
	for v := range versions {
		tarballs[v] = buildPluginFixture(t, name, v)
	}

	mux := http.NewServeMux()
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	idx := pluginreg.IndexV2{
		Schema: "v2",
		Plugins: map[string]pluginreg.PluginEntry{
			name: {
				Description: "Test plugin",
				License:     "Apache-2.0",
				Author:      "tests",
				Versions:    map[string]pluginreg.VersionEntry{},
			},
		},
	}
	platKey := runtime.GOOS + "/" + runtime.GOARCH
	for v, ch := range versions {
		channel := ch
		if channel == "" {
			channel = "stable"
		}
		fileURL := fmt.Sprintf("%s/%s-%s.tar.gz", server.URL, name, v)
		idx.Plugins[name].Versions[v] = pluginreg.VersionEntry{
			Channel: channel,
			Platforms: map[string]pluginreg.PlatformEntry{
				platKey: {
					URL:    fileURL,
					SHA256: sha256Hex(tarballs[v]),
				},
			},
		}
	}

	mux.HandleFunc("/plugins.json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(idx)
	})
	for v, body := range tarballs {
		mux.HandleFunc(fmt.Sprintf("/%s-%s.tar.gz", name, v), func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/gzip")
			_, _ = w.Write(body)
		})
	}
	return server, tarballs
}

// withIsolatedXDG redirects InstallTarget() and CacheLocation() to a
// per-test temp dir by tweaking $XDG_DATA_HOME / $XDG_CACHE_HOME and
// $KAPI_REGISTRY_CACHE.
func withIsolatedXDG(t *testing.T) string {
	t.Helper()
	dataHome := t.TempDir()
	cacheHome := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dataHome)
	t.Setenv("XDG_CACHE_HOME", cacheHome)
	// Ensure registry cache uses the per-test home.
	t.Setenv("KAPI_REGISTRY_CACHE", filepath.Join(cacheHome, "registry-index.json"))
	t.Setenv("KAPI_PLUGIN_CACHE", filepath.Join(cacheHome, "plugins-cache.json"))
	t.Setenv("KAPI_PLUGINS_DIR", "")
	return dataHome
}

// runUpdate drives `kapi plugin update <args...>` through the parent
// plugin command (which is how cobra's Args validation gets exercised
// in production). Returns the captured stdout/stderr + the RunE error.
func runUpdate(t *testing.T, app *App, args ...string) (stdout, stderr bytes.Buffer, err error) {
	t.Helper()
	cmd := app.NewPluginCmd()
	cmd.SetContext(context.Background())
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs(append([]string{"update"}, args...))
	err = cmd.Execute()
	return stdout, stderr, err
}

func TestPluginUpdate_SecondInstallReplacesFirst(t *testing.T) {
	withIsolatedXDG(t)

	server, _ := servePluginRegistry(t, "demo", map[string]string{
		"1.0.0": "stable",
		"1.1.0": "stable",
	})

	// Step 1: install 1.0.0 explicitly so installed.json records that
	// version. We pin the constraint to the exact version to avoid
	// the registry returning the higher 1.1.0 build.
	first, err := pluginhost.InstallFromRegistry(context.Background(), pluginhost.InstallOptions{
		IndexURL:   server.URL + "/plugins.json",
		PluginName: "demo",
		Constraint: "1.0.0",
		Channel:    "stable",
		Unsafe:     true, // local httptest registry has no signatures
	})
	require.NoError(t, err)
	assert.Equal(t, "1.0.0", first.Version)

	meta, err := pluginhost.ReadInstalledMetadata(filepath.Join(pluginhost.InstallTarget(), "demo"))
	require.NoError(t, err)
	assert.Equal(t, "stable", meta.Channel)
	assert.Equal(t, "1.0.0", meta.Constraint)
	assert.Equal(t, "1.0.0", meta.Version)

	// Step 2: run `plugin update demo --constraint ^1.0.0` so the
	// resolver rolls forward to 1.1.0.
	app := &App{}
	stdout, stderr, err := runUpdate(t, app, "demo",
		"--index", server.URL+"/plugins.json",
		"--constraint", "^1.0.0",
		"--unsafe",
	)
	require.NoErrorf(t, err, "update failed; stderr=%s", stderr.String())
	assert.Contains(t, stdout.String(), "Updated demo 1.0.0 → 1.1.0")

	meta2, err := pluginhost.ReadInstalledMetadata(filepath.Join(pluginhost.InstallTarget(), "demo"))
	require.NoError(t, err)
	assert.Equal(t, "1.1.0", meta2.Version)
}

func TestPluginUpdate_AlreadyUpToDate(t *testing.T) {
	withIsolatedXDG(t)

	server, _ := servePluginRegistry(t, "demo", map[string]string{
		"1.0.0": "stable",
	})

	first, err := pluginhost.InstallFromRegistry(context.Background(), pluginhost.InstallOptions{
		IndexURL:   server.URL + "/plugins.json",
		PluginName: "demo",
		Constraint: "^1.0.0",
		Channel:    "stable",
		Unsafe:     true, // local httptest registry has no signatures
	})
	require.NoError(t, err)
	require.Equal(t, "1.0.0", first.Version)

	app := &App{}
	stdout, stderr, err := runUpdate(t, app, "demo", "--index", server.URL+"/plugins.json", "--unsafe")
	require.NoErrorf(t, err, "update failed; stderr=%s", stderr.String())
	assert.Contains(t, stdout.String(), "demo is already up to date")
}

func TestPluginUpdate_NotInstalled_ReturnsError(t *testing.T) {
	withIsolatedXDG(t)

	app := &App{}
	_, _, err := runUpdate(t, app, "missing-plugin")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "is not installed")
}

// Sanity: the registry index built by servePluginRegistry contains
// what we expect — guards against silent test-fixture drift.
func TestPluginRegistryFixture_IsValid(t *testing.T) {
	server, _ := servePluginRegistry(t, "demo", map[string]string{
		"1.0.0": "stable",
		"1.1.0": "stable",
	})
	// Bypass the cache: hit the wire directly.
	idx, err := pluginreg.FetchIndex(context.Background(), server.URL+"/plugins.json")
	require.NoError(t, err)
	require.Contains(t, idx.Plugins, "demo")
	require.Len(t, idx.Plugins["demo"].Versions, 2)

	// Resolve uses runtime.GOOS/GOARCH.
	v, _, err := idx.Resolve("demo", "^1.0.0", "stable", "")
	require.NoError(t, err)
	assert.Equal(t, "1.1.0", v)
}

// _ keeps the registry import live for tests that may add direct
// queries against the cached index.
var _ = pluginreg.FetchOrCached

// runPlugins drives `kapi plugins <args...>` through the parent command,
// non-interactively, capturing output and the RunE error.
func runPlugins(t *testing.T, app *App, args ...string) (stdout, stderr bytes.Buffer, err error) {
	t.Helper()
	cmd := app.NewPluginCmd()
	cmd.SetContext(context.Background())
	cmd.SetIn(strings.NewReader(""))
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs(args)
	err = cmd.Execute()
	return stdout, stderr, err
}

// A retired plugin cannot be (re)installed — the compiled-in tombstone refuses
// it before any network access, pointing at the replacement.
func TestPluginInstall_RetiredRefused(t *testing.T) {
	_, _, err := runPlugins(t, &App{}, "install", "llm")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "retired")
	assert.Contains(t, err.Error(), "Ollama")
}

// `kapi plugins prune` lists retired installs and removes user-installed ones
// after confirmation (or --yes), leaving everything else alone.
func TestPluginPrune_RemovesRetiredUserInstall(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "llm")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "manifest.json"), []byte("{}"), 0o644))
	p := &pluginhost.Plugin{
		Dir:      dir,
		Source:   pluginhost.Source{Order: 1, Label: dir, Path: dir},
		Manifest: &manifest.Manifest{Plugin: "llm", Version: "0.1.0", Binary: "kapi-llm"},
	}
	app := &App{PluginHost: pluginhost.NewHost([]*pluginhost.Plugin{p}, nil)}

	// dry-run lists but keeps it.
	stdout, _, err := runPlugins(t, app, "prune", "--dry-run")
	require.NoError(t, err)
	assert.Contains(t, stdout.String(), "llm")
	_, statErr := os.Stat(dir)
	require.NoError(t, statErr, "dry-run must not remove the install")

	// --yes removes it.
	stdout, _, err = runPlugins(t, app, "prune", "--yes")
	require.NoError(t, err)
	assert.Contains(t, stdout.String(), "removed llm")
	_, statErr = os.Stat(dir)
	assert.True(t, os.IsNotExist(statErr), "prune --yes should delete the install dir")
}

// Pruning with no retired plugins installed is a clean no-op.
func TestPluginPrune_NothingToPrune(t *testing.T) {
	p := &pluginhost.Plugin{
		Source:   pluginhost.Source{Order: 1, Label: "user"},
		Manifest: &manifest.Manifest{Plugin: "sat", Version: "1.0.0", Binary: "kapi-sat"},
	}
	app := &App{PluginHost: pluginhost.NewHost([]*pluginhost.Plugin{p}, nil)}
	stdout, _, err := runPlugins(t, app, "prune", "--yes")
	require.NoError(t, err)
	assert.Contains(t, stdout.String(), "Nothing to prune")
}
