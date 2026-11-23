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

// A translation the project's committed record teaches and its delivery
// writes back is one content-memory entry. The record absorber creates it;
// the merge after the run adds its origin to that entry instead of storing
// the same source and target a second time.
func TestContentMemoryHoldsOneEntryPerTranslation(t *testing.T) {
	dir, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	dataRoot, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	copyFixture(t, filepath.Join(findRoot(), "samples", "compass"), dir)

	run := func(args ...string) string {
		t.Helper()
		cmd := exec.Command(kapiBin, append(args, "-p", ".")...)
		cmd.Dir = dir
		cmd.Env = append(append(os.Environ(), isoEnv...),
			"KAPI_DATA_DIR="+dataRoot, "KAPI_AI_PROVIDER=demo", "ANTHROPIC_API_KEY=", "OPENAI_API_KEY=")
		out, err := cmd.Output()
		require.NoError(t, err, "kapi %v failed:\n%s", args, out)
		return string(out)
	}
	run("store", "import", "--yes", "./context")
	run("up")

	var lookup struct {
		Matches []struct {
			Target  string `json:"target"`
			EntryID string `json:"entry_id"`
		} `json:"matches"`
	}
	require.NoError(t, json.Unmarshal([]byte(run("memory", "lookup", "Berth plan", "-s", "en-GB", "-t", "nb", "--json")), &lookup))
	require.Len(t, lookup.Matches, 1, "the store holds one entry for the translation")
	assert.Equal(t, "Kaiplan", lookup.Matches[0].Target)
	assert.True(t, strings.HasPrefix(lookup.Matches[0].EntryID, "record:"),
		"the record absorber's entry holds the pair: %s", lookup.Matches[0].EntryID)
}
