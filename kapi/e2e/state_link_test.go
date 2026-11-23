//go:build e2e && unix

package e2e

import (
	"crypto/sha256"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stateFilesDigest hashes a project's work store and the files SQLite keeps
// beside it, so a test sees any write that reached them.
func stateFilesDigest(t *testing.T, recipe string) map[string][32]byte {
	t.Helper()
	out := map[string][32]byte{}
	work := filepath.Join(statePath(t, recipe), "work")
	for _, name := range []string{"store.db", "store.db-wal", "store.db-shm"} {
		b, err := os.ReadFile(filepath.Join(work, name))
		if err == nil {
			out[name] = sha256.Sum256(b)
		}
	}
	return out
}

// A repository that commits a link under .kapi/, here where an earlier kapi
// kept the work store, pointing at another project's store, is refused by every
// command before it opens anything, and the other project's store is left
// exactly as it was.
func TestAStateFileThatIsALinkIsRefused(t *testing.T) {
	_, otherRecipe := writeBareProject(t, "other")
	kapi(t, "status", "-p", otherRecipe)
	before := stateFilesDigest(t, otherRecipe)
	require.Contains(t, before, "store.db", "the other project keeps a work store")

	victim, victimRecipe := writeBareProject(t, "victim")
	require.NoError(t, os.MkdirAll(filepath.Join(victim, ".kapi", "work"), 0o755))
	require.NoError(t, os.Symlink(filepath.Join(statePath(t, otherRecipe), "work", "store.db"),
		filepath.Join(victim, ".kapi", "work", "store.db")))

	for _, args := range [][]string{
		{"status", "-p", victimRecipe},
		{"run", "pseudo-translate", "-p", victimRecipe},
	} {
		out, err := kapiAllowFail(t, args...)
		require.Error(t, err, "%v succeeded:\n%s", args, out)
		assert.Contains(t, out, "is a symbolic link", args)
	}
	assert.Equal(t, before, stateFilesDigest(t, otherRecipe), "nothing reached the other project's store")
}

// A target that is a link out of the project is passed over and named, as a
// source whose name cannot fill its target is: kapi status and kapi up --plan
// report on every other file and exit 0, and nothing is read through it.
func TestATargetThatLinksOutIsNamedByStatusAndThePlan(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "locales"), 0o755))
	recipe := filepath.Join(root, "kapi.yaml")
	require.NoError(t, os.WriteFile(recipe, []byte(`version: v1
name: links
defaults:
  source_language: en
  target_languages: [fr]
  translate_after: none
collections:
  - name: App
    content:
      - path: "locales/en.json"
        target: "locales/{lang}.json"
      - path: "locales/b-en.json"
        target: "locales/b-{lang}.json"
`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "locales", "en.json"), []byte(`{"a":"Hello"}`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "locales", "b-en.json"), []byte(`{"b":"Bye"}`), 0o644))
	outside := filepath.Join(t.TempDir(), "fr.json")
	require.NoError(t, os.WriteFile(outside, []byte(`{"a":"held outside"}`), 0o644))
	require.NoError(t, os.Symlink(outside, filepath.Join(root, "locales", "fr.json")))

	for _, args := range [][]string{{"status", "-p", recipe}, {"up", "--plan", "-p", recipe}} {
		out, err := kapiAllowFail(t, args...)
		require.NoError(t, err, "%v:\n%s", args, out)
		assert.Contains(t, out, "locales/en.json has no fr target", args)
		assert.NotContains(t, out, "held outside", args)
	}
}
