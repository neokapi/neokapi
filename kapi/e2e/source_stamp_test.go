//go:build e2e

package e2e

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const stampRecipe = `version: v1
name: stamp-e2e
defaults:
  source_language: en
  target_languages: [fr]
collections:
  - name: app
    path: src/en.json
    target: src/{lang}.json
flows:
  redacted:
    steps:
      - tool: redact
        config:
          detectors: [rules]
          rules:
            - term: Mr Bean
              category: person
      - tool: translate
        config:
          provider: demo
      - tool: unredact
`

// A project flow that redacts the source before it translates and restores it
// after: every translation the run commits lands in the merged file, with the
// withheld value restored. An edit to one source block afterwards makes the
// merge count that block's translation as stale and leave it out of a target
// file written afresh.
func TestRedactedFlowMerge_StaleSourceStamp(t *testing.T) {
	// The project root and the input must name the same path, so the temp
	// dir is resolved the way kapi resolves the input (macOS links /var).
	dir, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "src"), 0o755))
	src := filepath.Join(dir, "src", "en.json")
	require.NoError(t, os.WriteFile(src, []byte(`{"greeting":"Hello Mr Bean","farewell":"Goodbye Mr Bean"}`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "kapi.yaml"), []byte(stampRecipe), 0o644))
	recipe := filepath.Join(dir, "kapi.yaml")
	target := filepath.Join(dir, "src", "fr.json")

	t.Setenv("ANTHROPIC_API_KEY", "")
	t.Setenv("OPENAI_API_KEY", "")
	kapiIn(t, dir, "run", "redacted", "-p", recipe, "-i", "src/en.json", "--target-lang", "fr", "--provider", "demo")

	type mergeResult struct {
		Written int `json:"written"`
		Stale   int `json:"stale"`
	}
	merge := func() (mergeResult, map[string]string) {
		t.Helper()
		var res mergeResult
		require.NoError(t, json.Unmarshal([]byte(kapiIn(t, dir, "merge", "-p", recipe, "--json")), &res))
		var got map[string]string
		require.NoError(t, json.Unmarshal([]byte(readFile(t, target)), &got))
		return res, got
	}

	res, got := merge()
	assert.Equal(t, mergeResult{Written: 1}, res)
	for key, source := range map[string]string{"greeting": "Hello Mr Bean", "farewell": "Goodbye Mr Bean"} {
		assert.NotEqual(t, source, got[key], "%s: the translation lands", key)
		assert.Contains(t, got[key], "Mr Bean", "%s: the redacted value is restored", key)
	}
	farewell := got["farewell"]

	require.NoError(t, os.WriteFile(src, []byte(`{"greeting":"Hello again Mr Bean","farewell":"Goodbye Mr Bean"}`), 0o644))
	require.NoError(t, os.Remove(target))
	res, got = merge()
	assert.Equal(t, mergeResult{Written: 1, Stale: 1}, res)
	assert.Equal(t, "Hello again Mr Bean", got["greeting"], "the stale translation is not written")
	assert.Equal(t, farewell, got["farewell"])
}
