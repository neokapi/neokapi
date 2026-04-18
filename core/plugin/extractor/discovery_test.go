package extractor

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writePkg drops a minimal package.json under <root>/<relName> so
// Discover() can find it. relName can be `foo` or `@scope/foo`.
func writePkg(t *testing.T, root, relName, pkgJSON string) {
	t.Helper()
	dir := filepath.Join(root, relName)
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "package.json"), []byte(pkgJSON), 0o644))
}

func TestDiscoverFindsPluginInNodeModules(t *testing.T) {
	root := t.TempDir()
	writePkg(t, filepath.Join(root, "node_modules"), "@neokapi/kapi-react", `{
      "name": "@neokapi/kapi-react",
      "kapi-plugin": {
        "extensions": [".tsx", ".jsx"],
        "extract": { "exec": ["kapi-react", "extract", "--blocks-stream"] }
      }
    }`)

	found, err := Discover(root)
	require.NoError(t, err)
	require.Len(t, found, 1)
	assert.Equal(t, "@neokapi/kapi-react", found[0].PackageName)
	assert.Equal(t, []string{".tsx", ".jsx"}, found[0].Descriptor.Extensions)
	require.NotNil(t, found[0].Descriptor.Extract)
	assert.Equal(t, []string{"kapi-react", "extract", "--blocks-stream"},
		found[0].Descriptor.Extract.Exec)
}

func TestDiscoverSkipsPackagesWithoutKapiPluginField(t *testing.T) {
	root := t.TempDir()
	writePkg(t, filepath.Join(root, "node_modules"), "react", `{"name":"react"}`)
	writePkg(t, filepath.Join(root, "node_modules"), "has-plugin", `{
      "name": "has-plugin",
      "kapi-plugin": {"extensions": [".foo"]}
    }`)
	found, err := Discover(root)
	require.NoError(t, err)
	require.Len(t, found, 1)
	assert.Equal(t, "has-plugin", found[0].PackageName)
}

func TestDiscoverWalksAncestorNodeModules(t *testing.T) {
	// Simulate a workspace hoist: plugin at /repo/node_modules/,
	// project root at /repo/apps/frontend.
	root := t.TempDir()
	projectRoot := filepath.Join(root, "apps", "frontend")
	require.NoError(t, os.MkdirAll(projectRoot, 0o755))
	writePkg(t, filepath.Join(root, "node_modules"), "workspace-plugin", `{
      "name": "workspace-plugin",
      "kapi-plugin": {"extensions": [".md"]}
    }`)

	found, err := Discover(projectRoot)
	require.NoError(t, err)
	require.Len(t, found, 1)
	assert.Equal(t, "workspace-plugin", found[0].PackageName)
}

func TestDiscoverInnerWinsOverAncestor(t *testing.T) {
	root := t.TempDir()
	projectRoot := filepath.Join(root, "apps", "frontend")
	require.NoError(t, os.MkdirAll(projectRoot, 0o755))

	// Hoisted version — older.
	writePkg(t, filepath.Join(root, "node_modules"), "dup-plugin", `{
      "name": "dup-plugin",
      "kapi-plugin": {"extensions": [".old"]}
    }`)
	// Inner version — newer, should win.
	writePkg(t, filepath.Join(projectRoot, "node_modules"), "dup-plugin", `{
      "name": "dup-plugin",
      "kapi-plugin": {"extensions": [".new"]}
    }`)

	found, err := Discover(projectRoot)
	require.NoError(t, err)
	require.Len(t, found, 1)
	assert.Equal(t, []string{".new"}, found[0].Descriptor.Extensions,
		"inner node_modules copy wins")
}

func TestByExtensionBuildsLookupMap(t *testing.T) {
	extractors := []DiscoveredExtractor{
		{PackageName: "kapi-react", Descriptor: PluginDescriptor{
			Extensions: []string{".tsx", ".jsx"},
		}},
		{PackageName: "kapi-markdown", Descriptor: PluginDescriptor{
			Extensions: []string{".md", ".MDX"},
		}},
	}
	m := ByExtension(extractors)
	assert.Equal(t, "kapi-react", m[".tsx"].PackageName)
	assert.Equal(t, "kapi-react", m[".jsx"].PackageName)
	assert.Equal(t, "kapi-markdown", m[".md"].PackageName)
	assert.Equal(t, "kapi-markdown", m[".mdx"].PackageName, "extensions are normalised to lowercase")
}

func TestByExtensionFirstWinsOnCollision(t *testing.T) {
	extractors := []DiscoveredExtractor{
		{PackageName: "first", Descriptor: PluginDescriptor{Extensions: []string{".tsx"}}},
		{PackageName: "second", Descriptor: PluginDescriptor{Extensions: []string{".tsx"}}},
	}
	m := ByExtension(extractors)
	assert.Equal(t, "first", m[".tsx"].PackageName)
}
