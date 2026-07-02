package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/flow"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Regression for the target-overlay id collision: format readers assign
// file-local block ids that repeat across files (both these JSON files have a
// single "greeting" block with the same id), so keying the store's
// `targets/<locale>` overlays on the raw id let the two files share one
// overlay — the second process-only run overwrote the first, and merge then
// materialized the SAME (wrong) target into both localized files.
//
// Keying overlays by blockstore.StoreKey(sourceRel, id) keeps them distinct, so
// the run → merge round-trip must produce a different pseudo-translation per
// file. On the pre-fix code the two outputs come out identical.
func TestOverlayKey_NoCollisionAcrossFiles(t *testing.T) {
	a := processOnlyApp(t)
	dir := t.TempDir()
	root, err := filepath.EvalSymlinks(dir)
	require.NoError(t, err)

	recipe := filepath.Join(root, "app.kapi")
	proj := &project.KapiProject{
		Version:  "v1",
		Name:     "OverlayCollision",
		Defaults: project.Defaults{SourceLanguage: "en-US", TargetLanguages: []model.LocaleID{"fr-FR"}},
		Content: []project.ContentCollection{{
			Path:   "src/locales/en/*.json",
			Format: &project.FormatSpec{Name: "json"},
			Target: "src/locales/{lang}/*.json",
		}},
		Flows: map[string]*flow.StepsSpec{
			"pseudo": {Steps: []flow.FlowStep{{Tool: "pseudo-translate"}}},
		},
	}
	require.NoError(t, project.Save(recipe, proj))
	require.NoError(t, os.MkdirAll(filepath.Join(root, project.StateDirName), 0o755))

	sd := filepath.Join(root, "src/locales/en")
	require.NoError(t, os.MkdirAll(sd, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(sd, "a.json"), []byte(`{"greeting":"Alpha"}`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(sd, "b.json"), []byte(`{"greeting":"Bravo"}`), 0o644))

	// Two process-only runs, one per file — each commits its file's targets/fr-FR
	// overlay. On the buggy code the second overwrites the first (shared id).
	for _, f := range []string{"a.json", "b.json"} {
		out, rerr := runRunCmd(t, a, recipe, "pseudo", "-i", filepath.Join(sd, f))
		require.NoError(t, rerr, "run %s: %s", f, out)
	}

	// merge (no -i) materializes both localized files from the store.
	mOut, err := runMergeCmd(t, recipe)
	require.NoError(t, err, "merge output: %s", mOut)

	readGreeting := func(rel string) string {
		data, rerr := os.ReadFile(filepath.Join(root, rel))
		require.NoError(t, rerr, "merge must write %s", rel)
		var m map[string]string
		require.NoError(t, json.Unmarshal(data, &m))
		return m["greeting"]
	}
	ga := readGreeting("src/locales/fr-FR/a.json")
	gb := readGreeting("src/locales/fr-FR/b.json")

	assert.NotEqual(t, "Alpha", ga, "a.json should be pseudo-translated")
	assert.NotEqual(t, "Bravo", gb, "b.json should be pseudo-translated")
	// The crux: distinct source ⇒ distinct target. A collision makes them equal.
	assert.NotEqual(t, ga, gb, "per-file overlays collided: both files got the same target")
}
