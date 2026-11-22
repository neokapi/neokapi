//go:build e2e

package e2e

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// runFlagProject writes a project with one JSON collection and the given
// extra recipe lines, and returns its directory and an environment with no
// provider configured anywhere: no environment variable, no saved credential
// and no app config.
func runFlagProject(t *testing.T, extra string) (string, []string) {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	dataRoot, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "src"), 0o755))
	source, err := os.ReadFile(filepath.Join(testdata, "messages_en.json"))
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "src", "messages_en.json"), source, 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "kapi.yaml"), []byte(`version: v1
name: run-flags-e2e
defaults:
  source_language: en
  target_languages: [fr]
`+extra+`collections:
  - name: app
    path: src/messages_en.json
    target: src/messages_{lang}.json
`), 0o644))
	env := append(append(os.Environ(), isoEnv...),
		"KAPI_DATA_DIR="+dataRoot, "KAPI_AI_PROVIDER=", "ANTHROPIC_API_KEY=", "OPENAI_API_KEY=")
	return dir, env
}

// A run flag reaches every flow a run executes, by one rule: a step's own
// config, then the flag, then the recipe's presets and the configured default.
func TestRunFlagsReachEveryFlow(t *testing.T) {
	for _, tc := range []struct {
		name, recipe string
		args         []string
		note         string
	}{
		{name: "kapi up, the built-in default", args: []string{"up", "-p", "kapi.yaml", "--provider", "demo"}},
		{
			name:   "kapi up, a recipe flow whose step names no provider",
			recipe: "  flow: mine\n  translate_after: none\nflows:\n  mine:\n    steps:\n      - tool: translate\n",
			args:   []string{"up", "-p", "kapi.yaml", "--provider", "demo"},
		},
		{
			name:   "kapi up, a recipe flow whose step names its own provider",
			recipe: "  flow: mine\n  translate_after: none\nflows:\n  mine:\n    steps:\n      - tool: translate\n        config:\n          provider: demo\n",
			args:   []string{"up", "-p", "kapi.yaml", "--provider", "openai"},
			note:   `sets provider "demo", so --provider "openai" does not apply to it`,
		},
		{
			name:   "kapi run, a recipe flow",
			recipe: "flows:\n  mine:\n    steps:\n      - tool: translate\n",
			args:   []string{"run", "mine", "-p", "kapi.yaml", "--provider", "demo", "-o", "src/messages_{lang}.json"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir, env := runFlagProject(t, tc.recipe)
			cmd := exec.Command(kapiBin, tc.args...)
			cmd.Dir = dir
			cmd.Env = env
			out, err := cmd.CombinedOutput()
			require.NoError(t, err, "kapi %v:\n%s", tc.args, out)
			got, err := os.ReadFile(filepath.Join(dir, "src", "messages_fr.json"))
			require.NoError(t, err, "the fr target is written:\n%s", out)
			assert.Contains(t, string(got), "⟦fr⟧", "the demo provider drafted the fr target")
			if tc.note != "" {
				assert.Contains(t, string(out), tc.note, "the run names the flag the step overrides")
			}
		})
	}
}

// A flag a command accepts acts. `kapi up` refuses the flags that choose
// files, languages and outputs for one run, a server flag on a run on this
// machine, and `--pack` with no package.
func TestRunFlagsThatWouldNotActAreRefused(t *testing.T) {
	for _, tc := range []struct {
		args []string
		says string
	}{
		{[]string{"up", "-p", "kapi.yaml", "-o", "out/"}, "unknown shorthand flag: 'o'"},
		{[]string{"up", "-p", "kapi.yaml", "--target-lang", "de"}, "unknown flag: --target-lang"},
		{[]string{"up", "-p", "kapi.yaml", "--stats"}, "unknown flag: --stats"},
		{[]string{"up", "-p", "kapi.yaml", "--timeout", "1m"}, "--timeout applies to a server run"},
		{[]string{"translate", "src/messages_en.json", "--target-lang", "fr", "--pack"}, "--pack applies to a .kpz input"},
	} {
		t.Run(tc.args[0]+" "+tc.args[len(tc.args)-2], func(t *testing.T) {
			dir, env := runFlagProject(t, "")
			cmd := exec.Command(kapiBin, tc.args...)
			cmd.Dir = dir
			cmd.Env = env
			out, err := cmd.CombinedOutput()
			require.Error(t, err, "kapi %v must fail:\n%s", tc.args, out)
			assert.Contains(t, string(out), tc.says)
		})
	}
}
