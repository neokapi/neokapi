package gen

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGenerate_Deterministic asserts that running the generator twice
// against separate output directories produces byte-identical trees.
// The freshness CI gate (`git diff --exit-code core/i18n/builtins/`) is
// only meaningful when this holds.
func TestGenerate_Deterministic(t *testing.T) {
	dirA := t.TempDir()
	dirB := t.TempDir()

	require.NoError(t, Generate(dirA))
	require.NoError(t, Generate(dirB))

	filesA := walk(t, dirA)
	filesB := walk(t, dirB)
	require.Equal(t, sortedKeys(filesA), sortedKeys(filesB),
		"generator produced different file sets on successive runs")
	for name, bytesA := range filesA {
		bytesB := filesB[name]
		assert.Equal(t, string(bytesA), string(bytesB),
			"byte drift in %s — generator is not deterministic", name)
	}
}

func TestGenerate_ProducesMetadataDoc(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, Generate(dir))

	metadataPath := filepath.Join(dir, "metadata.json")
	info, err := os.Stat(metadataPath)
	require.NoError(t, err)
	assert.Greater(t, info.Size(), int64(100),
		"metadata.json suspiciously small — tool registry silently empty?")
}

func TestGenerate_ReplacesStaleArtifacts(t *testing.T) {
	// A stale file from a previous generation (e.g. a renamed tool) must
	// not linger. .gitkeep, if present, IS preserved.
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".gitkeep"), []byte("keep\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "stale.json"), []byte("{}"), 0o644))

	require.NoError(t, Generate(dir))

	_, err := os.Stat(filepath.Join(dir, "stale.json"))
	assert.True(t, os.IsNotExist(err), "stale.json should have been removed")
	_, err = os.Stat(filepath.Join(dir, ".gitkeep"))
	assert.NoError(t, err, ".gitkeep must be preserved")
	_, err = os.Stat(filepath.Join(dir, "metadata.json"))
	assert.NoError(t, err, "metadata.json must exist")
}

// walk reads every regular file under root into a map keyed by the path
// relative to root. Small trees only — tmp dirs stay under 1 MB.
func walk(t *testing.T, root string) map[string][]byte {
	t.Helper()
	result := map[string][]byte{}
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		result[rel] = data
		return nil
	})
	require.NoError(t, err)
	return result
}

func sortedKeys(m map[string][]byte) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	// sort.Strings is fine but we want no extra import; slices.Sort handles it
	// — imported transitively, but simpler to just inline.
	for i := 1; i < len(keys); i++ {
		for j := i; j > 0 && keys[j-1] > keys[j]; j-- {
			keys[j-1], keys[j] = keys[j], keys[j-1]
		}
	}
	return keys
}
