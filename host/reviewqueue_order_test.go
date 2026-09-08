package host

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A queue lists a file the way a reader reads it. Keys are what a format calls
// its units, and ordering by them puts a document's opening line wherever its
// name happens to sort.
//
// It runs under the dogfood isolation contract (CLAUDE.md): every root this run
// could otherwise inherit is pinned to a throwaway dir and project discovery is
// off, so the repo's own recipe can never be found.
func writeUnorderedKeysProject(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	t.Setenv("KAPI_CONFIG_DIR", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("KAPI_PLUGINS_DIR_ONLY", "1")
	t.Setenv("KAPI_PLUGINS_DIR", t.TempDir())
	t.Setenv("KAPI_NO_PROJECT", "1")

	recipe := `version: v1
name: rev-order
defaults:
  source_language: en
  target_languages: [nb]
  source_gate: approved
collections:
  - path: en.json
    target: "{lang}.json"
`
	require.NoError(t, os.WriteFile(filepath.Join(root, "kapi.yaml"), []byte(recipe), 0o644))
	// Keys deliberately out of alphabetical order: the file opens on `title`.
	require.NoError(t, os.WriteFile(filepath.Join(root, "en.json"),
		[]byte(`{"title":"About Acme","mission":"Our Mission","about":"Our History"}`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "nb.json"),
		[]byte(`{"title":"Om Acme","mission":"Vart oppdrag","about":"Var historie"}`), 0o644))
	return root
}

func TestReviewQueue_ListsAFileInDocumentOrder(t *testing.T) {
	root := writeUnorderedKeysProject(t)
	queue, err := (&App{}).ReviewQueue(t.Context(), filepath.Join(root, "kapi.yaml"), "en", ReviewQueueOptions{})
	require.NoError(t, err)
	require.NotEmpty(t, queue.Pending)

	byLang := map[string][]string{}
	for _, it := range queue.Pending {
		byLang[it.LanguageTag()] = append(byLang[it.LanguageTag()], it.Key)
	}
	want := []string{"title", "mission", "about"}
	for lang, keys := range byLang {
		assert.Equal(t, want, keys, "the %s lane reads the file, not its key order", lang)
	}
	require.Contains(t, byLang, "en", "the source lane is listed")
	require.Contains(t, byLang, "nb", "the target lane is listed")
}
