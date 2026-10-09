package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/host/output"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const trustExecRecipe = `version: v1
flows:
  default:
    steps:
      - tool: external-command
        config:
          command: /bin/echo
`

// trustTestRecipe isolates the record under a throwaway config dir and writes
// a recipe with one exec site. Discovery is off under the isolation contract,
// so every command names the recipe with -p or an argument.
func trustTestRecipe(t *testing.T) string {
	t.Helper()
	t.Setenv("KAPI_CONFIG_DIR", t.TempDir())
	t.Setenv("KAPI_TRUST_EXEC", "")
	t.Setenv("KAPI_NO_PROJECT", "1")
	recipePath := filepath.Join(t.TempDir(), project.RecipeFileName)
	require.NoError(t, os.WriteFile(recipePath, []byte(trustExecRecipe), 0o644))
	return recipePath
}

// TestTrustCommands walks the family end to end through the cobra tree: show
// before any answer, allow with --yes (go test has no terminal), list, revoke,
// and list again.
func TestTrustCommands(t *testing.T) {
	recipePath := trustTestRecipe(t)
	run := func(args ...string) (string, error) {
		root, _ := newTestRoot(t)
		return runCLI(t, root, args...)
	}

	out, err := run("trust", "show", "-p", recipePath, "--json")
	require.NoError(t, err, out)
	var show output.TrustShowOutput
	require.NoError(t, json.Unmarshal([]byte(out), &show))
	assert.Equal(t, output.TrustShowUndecided, show.Status)
	require.Len(t, show.Sites, 1)
	assert.Equal(t, "external-command", show.Sites[0].Name)

	_, err = run("trust", "allow", "-p", recipePath, "--json")
	require.Error(t, err, "no terminal and no --yes refuses")
	assert.Contains(t, err.Error(), "--yes")

	out, err = run("trust", "allow", "-p", recipePath, "--yes", "--json")
	require.NoError(t, err, out)
	var allow output.TrustAllowOutput
	require.NoError(t, json.Unmarshal([]byte(out[lastJSONStart(out):]), &allow))
	assert.True(t, allow.Recorded)

	out, err = run("trust", "list", "--json")
	require.NoError(t, err, out)
	var list output.TrustListOutput
	require.NoError(t, json.Unmarshal([]byte(out), &list))
	require.Len(t, list.Entries, 1)
	assert.Equal(t, recipePath, list.Entries[0].Path)
	assert.Equal(t, "allow", list.Entries[0].Decision)
	assert.Equal(t, output.TrustStatusCurrent, list.Entries[0].Status)

	out, err = run("trust", "show", "-p", recipePath, "--json")
	require.NoError(t, err, out)
	require.NoError(t, json.Unmarshal([]byte(out), &show))
	assert.Equal(t, output.TrustShowAllowed, show.Status)

	out, err = run("trust", "revoke", filepath.Dir(recipePath), "--json")
	require.NoError(t, err, out)
	var revoke output.TrustRevokeOutput
	require.NoError(t, json.Unmarshal([]byte(out), &revoke))
	assert.True(t, revoke.Removed)
	assert.Equal(t, recipePath, revoke.Path)

	out, err = run("trust", "revoke", "-p", recipePath)
	require.NoError(t, err, out)
	assert.Contains(t, out, "No decision recorded")

	out, err = run("trust", "list")
	require.NoError(t, err, out)
	assert.Contains(t, out, "No execution-trust decisions recorded.")

	_, err = run("trust", "revoke")
	require.Error(t, err, "no project in scope and no path named")
}

// lastJSONStart finds the JSON document in output that also carries the
// prompt text allow writes to stderr, which runCLI merges into one buffer.
func lastJSONStart(s string) int {
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == '\n' && i+1 < len(s) && s[i+1] == '{' {
			return i + 1
		}
	}
	return 0
}
