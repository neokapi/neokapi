//go:build e2e

package e2e

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A run the content memory answers completely needs no key for the provider
// the recipe names. Compass converges once on the offline demo provider, then
// its recipe names anthropic, with no key anywhere, and its German catalog
// goes missing. The plan reads every German block as recycled, and the run
// writes de.json from the content memory and exits 0: the translate step is
// built without the key and is never called.
func TestUpRecycledOnlyRunNeedsNoKey(t *testing.T) {
	dir, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	dataRoot, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	copyFixture(t, filepath.Join(findRoot(), "samples", "compass"), dir)

	run := func(args ...string) (string, error) {
		t.Helper()
		cmd := exec.Command(kapiBin, append(args, "-p", ".")...)
		cmd.Dir = dir
		cmd.Env = append(append(os.Environ(), isoEnv...),
			"KAPI_DATA_DIR="+dataRoot, "KAPI_AI_PROVIDER=", "KAPI_AI_MODEL=",
			"ANTHROPIC_API_KEY=", "OPENAI_API_KEY=")
		out, err := cmd.CombinedOutput()
		return string(out), err
	}
	out, err := run("store", "import", "--yes", "./context")
	require.NoError(t, err, out)
	out, err = run("up")
	require.NoError(t, err, "the demo run:\n%s", out)

	// The step's own provider line, not the comment above it that names demo.
	recipePath := filepath.Join(dir, "kapi.yaml")
	recipe, err := os.ReadFile(recipePath)
	require.NoError(t, err)
	const demoStep = "          provider: demo\n"
	require.Contains(t, string(recipe), demoStep)
	edited := strings.Replace(string(recipe), demoStep, "          provider: anthropic\n", 1)
	// Dutch still waits for drafting after the demo run, and a demo draft does
	// not serve an anthropic run, so it is dropped: what is left the content
	// memory answers completely.
	const languages = "target_languages: [nb, de, nl]"
	require.Contains(t, edited, languages)
	edited = strings.Replace(edited, languages, "target_languages: [nb, de]", 1)
	require.NoError(t, os.WriteFile(recipePath, []byte(edited), 0o644))
	german := filepath.Join(dir, "site", "locales", "de.json")
	want, err := os.ReadFile(german)
	require.NoError(t, err)
	require.NoError(t, os.Remove(german))

	plan, err := run("up", "--plan")
	require.NoError(t, err, plan)
	require.Contains(t, plan, "AI provider", "the plan ran")

	out, err = run("up")
	require.NoError(t, err, "a recycled-only run needs no key:\n%s\nplan:\n%s", out, plan)
	assert.NotContains(t, out, "no saved credentials")
	got, err := os.ReadFile(german)
	require.NoError(t, err, "the German catalog is written from the content memory:\n%s\nplan:\n%s", out, plan)
	assert.JSONEq(t, string(want), string(got))
}
