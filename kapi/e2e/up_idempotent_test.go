//go:build e2e

package e2e

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const upTwiceRecipe = `version: v1
name: up-twice-e2e
defaults:
  source_language: en
  target_languages: [fr, de]
collections:
  - name: app
    path: src/messages_en.json
    target: src/messages_{lang}.json
  - name: docs
    path: docs/en/*.md
    target: docs/{lang}/*.md
`

const upTwiceGuide = `# Getting started

Install the tool, then run it in your project.

- Open the **settings** page.
- Choose a [language](https://example.com/languages).
`

// A second `kapi up` over a project the first one converged does nothing: it
// reports zero passes and changes no byte of the tree, the project state under
// .kapi/ included. Nothing is excluded from the comparison, so a run that
// rewrites a target, a cache entry or the store with equal content but a new
// timestamp inside it fails here.
func TestUpTwiceIsANoOp(t *testing.T) {
	dir, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "src"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "docs", "en"), 0o755))
	source, err := os.ReadFile(filepath.Join(testdata, "messages_en.json"))
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "src", "messages_en.json"), source, 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "docs", "en", "guide.md"), []byte(upTwiceGuide), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "kapi.yaml"), []byte(upTwiceRecipe), 0o644))

	// The built-in default flow translates with the configured provider; the
	// demo provider answers offline, with no key.
	t.Setenv("KAPI_AI_PROVIDER", "demo")
	t.Setenv("ANTHROPIC_API_KEY", "")
	t.Setenv("OPENAI_API_KEY", "")

	first := kapiIn(t, dir, "up", "-p", "kapi.yaml")
	require.Contains(t, first, "Up to date", "the first run converges")
	for _, target := range []string{"src/messages_fr.json", "src/messages_de.json", "docs/fr/guide.md", "docs/de/guide.md"} {
		require.FileExists(t, filepath.Join(dir, target))
	}
	before := snapshotTree(t, dir)

	type upResult struct {
		Type      string `json:"type"`
		Passes    int    `json:"passes"`
		Converged bool   `json:"converged"`
	}
	var result *upResult
	for line := range strings.Lines(kapiIn(t, dir, "up", "-p", "kapi.yaml", "--json")) {
		var r upResult
		if json.Unmarshal([]byte(line), &r) == nil && r.Type == "result" {
			result = &r
		}
	}
	require.NotNil(t, result, "the second run reports a result")
	assert.True(t, result.Converged, "the second run is converged")
	assert.Zero(t, result.Passes, "the second run has nothing to do")

	after := snapshotTree(t, dir)
	for path, sum := range before {
		if got, ok := after[path]; !ok {
			t.Errorf("the second run removed %s", path)
		} else if got != sum {
			t.Errorf("the second run changed %s", path)
		}
	}
	for path := range after {
		if _, ok := before[path]; !ok {
			t.Errorf("the second run created %s", path)
		}
	}
}

// snapshotTree maps every regular file under root to the SHA-256 of its bytes.
func snapshotTree(t *testing.T, root string) map[string]string {
	t.Helper()
	sums := map[string]string{}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || !d.Type().IsRegular() {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(data)
		sums[filepath.ToSlash(rel)] = hex.EncodeToString(sum[:])
		return nil
	})
	require.NoError(t, err)
	return sums
}
