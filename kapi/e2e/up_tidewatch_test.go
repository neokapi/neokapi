//go:build e2e

package e2e

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// tidewatchRun converges a copy of samples/tidewatch-docs with the demo
// provider and returns the combined output of `kapi up` run with args.
func tidewatchRun(t *testing.T, args ...string) string {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.CopyFS(dir, os.DirFS(filepath.Join(findRoot(), "samples", "tidewatch-docs"))))
	dataRoot := t.TempDir()
	env := append(append(os.Environ(), isoEnv...),
		"KAPI_DATA_DIR="+dataRoot, "ANTHROPIC_API_KEY=", "OPENAI_API_KEY=",
		// The sample's context is imported as the person working here would.
		"KAPI_ACTOR=person")
	run := func(args ...string) string {
		cmd := exec.Command(kapiBin, args...)
		cmd.Dir = dir
		cmd.Env = env
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "kapi %s:\n%s", strings.Join(args, " "), out)
		return string(out)
	}
	run("store", "import", "./context", "-p", "kapi.yaml")
	return run(append([]string{"up", "-p", "kapi.yaml"}, args...)...)
}

// On the Tidewatch sample, Norwegian ships and Dutch waits for review: the
// run writes Norwegian's 60 units and holds Dutch's 60, and counts nothing
// that is not one of a language's units (the sample's code block is written
// into the target file as it is and is no translation).
func TestUpTidewatchCountsEachLanguagesUnits(t *testing.T) {
	out := tidewatchRun(t, "--json")
	var result *upTwiceResult
	for line := range strings.Lines(out) {
		var e upTwiceResult
		if json.Unmarshal([]byte(line), &e) == nil && e.Type == "result" {
			result = &e
		}
	}
	require.NotNil(t, result, out)
	assert.Equal(t, 60, result.Produced, "Norwegian's units, written once each")
	assert.Equal(t, 60, result.WaitingForReview, "Dutch's units, held by its gate")
}

// The pass lines and the summary tell one story: a gated pass reports what
// it drafted and recycled, and produced appears where the delivery writes the
// files.
func TestUpTidewatchPassLinesAddUpToTheSummary(t *testing.T) {
	out := tidewatchRun(t)
	assert.Contains(t, out, "pass 1 done · drafted 97 · recycled 23 · waiting for review 60", out)
	assert.NotContains(t, out, "done · produced 0", "a pass that did work does not read as produced 0:\n%s", out)
	assert.Contains(t, out, "materialized 4 target file(s) · produced 60", out)
	assert.Contains(t, out, "  produced            60", out)
}
