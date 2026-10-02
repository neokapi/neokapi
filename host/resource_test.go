package host

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/terms"
)

// Named stores are listed through the storage driver, which knows a database
// wherever it keeps one: natively a file, in the browser a database in
// SQLite's memory that os.ReadDir never sees. Only databases are listed.
func TestListNamedResources_ListsTheStoresTheDriverHolds(t *testing.T) {
	t.Setenv("KAPI_CONFIG_DIR", t.TempDir())

	path, err := resolveNamedResource("terms", "acme")
	require.NoError(t, err)
	tb, err := terms.NewSQLiteStore(path)
	require.NoError(t, err)
	require.NoError(t, tb.Close())
	require.NoError(t, os.WriteFile(filepath.Join(filepath.Dir(path), "notes.db"), []byte("not a database"), 0o644))

	resources, err := ListNamedResources("terms")
	require.NoError(t, err)
	require.Len(t, resources, 1)
	assert.Equal(t, "acme", resources[0].Name)
	assert.Equal(t, path, resources[0].Path)
	assert.Positive(t, resources[0].Size)

	none, err := ListNamedResources("memory")
	require.NoError(t, err)
	assert.Empty(t, none, "a kind with no directory holds no stores")
}
