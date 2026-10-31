package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// `kapi run` stops on a recipe that carries a retired or incomplete shape
// before it reads any content, names the recipe file and the key, and exits
// with the code every other recipe load error exits with.
func TestRunRefusesARecipeShapeWithTheLoadErrorExitCode(t *testing.T) {
	t.Setenv("KAPI_CONFIG_DIR", t.TempDir())
	t.Setenv("KAPI_DATA_DIR", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("KAPI_PLUGINS_DIR_ONLY", "1")
	t.Setenv("KAPI_PLUGINS_DIR", "")
	t.Setenv("KAPI_NO_PROJECT", "1")

	run := func(t *testing.T, recipe string) error {
		t.Helper()
		dir := t.TempDir()
		require.NoError(t, os.MkdirAll(filepath.Join(dir, "docs"), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "docs", "a.json"), []byte(`{"a":"Hello"}`), 0o644))
		path := filepath.Join(dir, "kapi.yaml")
		require.NoError(t, os.WriteFile(path, []byte(recipe), 0o644))
		app := &App{Quiet: true}
		app.InitRegistries()
		defer app.Shutdown()
		cmd := NewRunCmd(app)
		cmd.SetArgs([]string{"review", "-p", path})
		cmd.SilenceUsage = true
		cmd.SilenceErrors = true
		err := cmd.Execute()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid project file "+path+": ")
		return err
	}

	const recipe = "version: %s\nname: shapes\ndefaults:\n  source_language: en\n  target_languages: [fr]\n%scollections:\n  - path: \"docs/*.json\"\nflows:\n  review:\n    steps:\n      - tool: qa\n%s"
	other := run(t, fmt.Sprintf(recipe, "v0", "", ""))
	want := ExitCode(nil, other)
	require.Equal(t, ExitError, want, "an unsupported version is an ordinary load error")

	for name, tc := range map[string]struct{ defaults, step, msg string }{
		"checkTerminology": {step: "        config:\n          checkTerminology: true\n",
			msg: "flows.review.steps[0].config.checkTerminology is retired and nothing reads it. Remove the key"},
		"defaults.redaction without enabled": {defaults: "  redaction:\n    rules: redact.yaml\n",
			msg: "defaults.redaction sets rules without enabled: true"},
		"a qa provider without mode": {step: "        config:\n          provider: anthropic\n",
			msg: "flows.review.steps[0] (qa) sets provider: anthropic without mode"},
	} {
		t.Run(name, func(t *testing.T) {
			err := run(t, fmt.Sprintf(recipe, "v1", tc.defaults, tc.step))
			assert.Contains(t, err.Error(), tc.msg)
			assert.Equal(t, want, ExitCode(nil, err), "the exit code of any other recipe load error")
		})
	}
}
