package host

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/project"
)

// Files split by the policy that governs them, and files no policy governs
// share one group whatever resolved them: a collection that switches
// redaction off, beside one that declares nothing, splits nothing.
func TestGroupInputsByBinding_SplitsOnlyByAPolicyInForce(t *testing.T) {
	isolateKapiEnv(t)
	root := t.TempDir()
	write := func(rel, body string) {
		t.Helper()
		path := filepath.Join(root, filepath.FromSlash(rel))
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
		require.NoError(t, os.WriteFile(path, []byte(body), 0o600))
	}
	write(project.RecipeFileName, "version: v1\nname: groups\ndefaults:\n  source_language: en\n"+
		"collections:\n"+
		"  - name: legal\n    redaction:\n      enabled: false\n    content:\n      - path: \"legal/*.json\"\n"+
		"  - name: docs\n    content:\n      - path: \"docs/*.json\"\n")
	write("legal/memo.json", redactedPage)
	write("docs/page.json", redactedPage)
	recipe := filepath.Join(root, project.RecipeFileName)
	proj, err := project.Load(recipe)
	require.NoError(t, err)
	a := &App{SourceLang: "en"}
	a.InitRegistries()
	cmd, err := a.unitCommand(t.Context(), recipe)
	require.NoError(t, err)

	inputs := []string{filepath.Join(root, "legal", "memo.json"), filepath.Join(root, "docs", "page.json")}
	groups, err := a.groupInputsByBinding(cmd, proj, root, inputs)
	require.NoError(t, err)
	require.Len(t, groups, 1, "no policy is in force for either file")
	assert.Equal(t, inputs, groups[0].Inputs)
	assert.Nil(t, groups[0].Redaction.Spec)

	// The same recipe with the legal collection's policy switched on splits.
	write(project.RecipeFileName, "version: v1\nname: groups\ndefaults:\n  source_language: en\n"+
		"collections:\n"+
		"  - name: legal\n    redaction:\n      enabled: true\n      rules: redact.yaml\n      detectors: [rules]\n    content:\n      - path: \"legal/*.json\"\n"+
		"  - name: docs\n    content:\n      - path: \"docs/*.json\"\n")
	write("redact.yaml", redactRules)
	proj, err = project.Load(recipe)
	require.NoError(t, err)
	groups, err = a.groupInputsByBinding(cmd, proj, root, inputs)
	require.NoError(t, err)
	require.Len(t, groups, 2)
	assert.Equal(t, "collections[0].redaction", groups[0].Redaction.At)
	assert.Nil(t, groups[1].Redaction.Spec)
}
