package sample

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestManifestRoundtrip(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".kapi"), 0o755))

	// Not a sample yet.
	_, ok := ReadManifest(dir)
	assert.False(t, ok)

	require.NoError(t, writeManifest("kapimart", dir))
	m, ok := ReadManifest(dir)
	require.True(t, ok)
	assert.Equal(t, "kapimart", m.Sample)
	assert.Equal(t, CurrentRevision("kapimart"), m.Revision)
	assert.NotEmpty(t, m.ScaffoldedAt)

	// Acknowledge an older copy → revision rewritten, sample preserved.
	require.NoError(t, SetManifestRevision(dir, 1))
	m, ok = ReadManifest(dir)
	require.True(t, ok)
	assert.Equal(t, 1, m.Revision)
	assert.Equal(t, "kapimart", m.Sample)
}
