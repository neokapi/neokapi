package host

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// pinNode reads the shim's installed runtimes and picks the newest by version,
// not by name order; a version without a node binary, a lock file and a
// cache file are passed over.
func TestNewestVPNode(t *testing.T) {
	root := t.TempDir()
	runtime := func(v string, withNode bool) {
		t.Helper()
		bin := filepath.Join(root, v, "bin")
		require.NoError(t, os.MkdirAll(bin, 0o700))
		if withNode {
			require.NoError(t, os.WriteFile(filepath.Join(bin, "node"), nil, 0o700))
		}
	}
	runtime("9.0.0", true)
	runtime("24.22.3", true)
	runtime("26.13.1", true)
	runtime("27.0.0", false)
	require.NoError(t, os.WriteFile(filepath.Join(root, "26.13.1.lock"), nil, 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(root, "index_cache.json"), nil, 0o600))

	assert.Equal(t, filepath.Join(root, "26.13.1", "bin"), newestVPNode(root))
	assert.Empty(t, newestVPNode(filepath.Join(root, "missing")))
}
