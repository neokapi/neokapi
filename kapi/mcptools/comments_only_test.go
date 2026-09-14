package mcptools

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The run_flow tool runs a project's flow over its first content file when no
// path is given. A file declared for its comments alone holds no value a flow
// reads, so the tool passes over it, for a project flow and a built-in one.
func TestRunFlowWithProjectPassesOverCommentsOnlyItems(t *testing.T) {
	run := func(t *testing.T, spelled, flowName string) RunFlowOutput {
		t.Helper()
		dir := t.TempDir()
		t.Setenv("KAPI_NO_PROJECT", "1")
		for rel, body := range map[string]string{
			"config/app.yaml": "# Greets the reader.\ngreeting: Hello\n",
			"locales/en.json": `{"greeting":"Hello"}`,
		} {
			abs := filepath.Join(dir, filepath.FromSlash(rel))
			require.NoError(t, os.MkdirAll(filepath.Dir(abs), 0o755))
			require.NoError(t, os.WriteFile(abs, []byte(body), 0o644))
		}
		recipe := filepath.Join(dir, "kapi.yaml")
		require.NoError(t, os.WriteFile(recipe, []byte(`version: v1
name: comments-only
defaults:
  source_language: en
collections:
  - name: config
    content:
      - path: "config/app.yaml"
        comments: `+spelled+`
  - name: app
    content:
      - path: "locales/en.json"
flows:
  pseudo:
    steps:
      - tool: pseudo-translate
`), 0o644))

		a := testApp()
		a.AssumeYes = true
		_, out, err := handleRunFlowWithProject(t.Context(), a, RunFlowInput{
			FlowName:   flowName,
			Project:    recipe,
			TargetLang: "qps",
			OutputPath: filepath.Join(dir, "out", "result"),
		})
		require.NoError(t, err)
		return out
	}

	for name, flowName := range map[string]string{"a project flow": "pseudo", "a built-in flow": "pseudo-translate"} {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, "en.json", filepath.Base(run(t, "{only: true}", flowName).InputPath))
			assert.Equal(t, "app.yaml", filepath.Base(run(t, "true", flowName).InputPath), "must fail: comments: true makes the YAML file the first input")
		})
	}
}
