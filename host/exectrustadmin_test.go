package host

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/host/output"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// execRecipeAt writes the exec-step recipe into a fresh directory and returns
// the recipe path and the digest of its surface.
func execRecipeAt(t *testing.T) (string, string) {
	t.Helper()
	recipePath := filepath.Join(t.TempDir(), project.RecipeFileName)
	require.NoError(t, os.WriteFile(recipePath, []byte(execStepRecipe), 0o644))
	return recipePath, project.ExecSurfaceDigest(project.ExecSurface(execProject(t)))
}

// trustCommand is a Command with captured IO, for the verbs that confirm.
func trustCommand(input string) (*EnvCommand, *bytes.Buffer) {
	cmd := NewEnvCommand(context.Background(), "trust")
	var errW bytes.Buffer
	cmd.SetIn(readerOf(input))
	cmd.SetOut(&errW)
	cmd.SetErr(&errW)
	return cmd, &errW
}

func TestExecTrustList(t *testing.T) {
	isolateTrust(t)
	a := &App{}

	t.Run("an empty record lists nothing and names the file", func(t *testing.T) {
		out := a.ExecTrustList()
		assert.Empty(t, out.Entries)
		assert.Equal(t, ExecTrustPath(), out.Record)
	})

	current, digest := execRecipeAt(t)
	require.NoError(t, recordExecTrust(current, digest, execTrustAllow))

	changed, _ := execRecipeAt(t)
	require.NoError(t, recordExecTrust(changed, "stale-digest", execTrustDeny))

	missing := filepath.Join(t.TempDir(), project.RecipeFileName)
	require.NoError(t, recordExecTrust(missing, digest, execTrustAllow))

	unarmed, _ := execRecipeAt(t)
	require.NoError(t, os.WriteFile(unarmed, []byte("version: v1\n"), 0o644))
	require.NoError(t, recordExecTrust(unarmed, digest, execTrustAllow))

	unreadable, _ := execRecipeAt(t)
	require.NoError(t, os.WriteFile(unreadable, []byte("flows: [not a mapping"), 0o644))
	require.NoError(t, recordExecTrust(unreadable, digest, execTrustAllow))

	formatter := filepath.Join(t.TempDir(), "vite.config.ts")
	require.NoError(t, os.WriteFile(formatter, []byte("export default {}\n"), 0o644))
	require.NoError(t, recordExecTrust(formatter, "formatter-digest", execTrustAllow))

	out := a.ExecTrustList()
	byPath := map[string]output.TrustListEntry{}
	for _, e := range out.Entries {
		byPath[e.Path] = e
	}
	require.Len(t, byPath, 6)

	assert.Equal(t, output.TrustStatusCurrent, byPath[current].Status)
	assert.Equal(t, "allow", byPath[current].Decision)
	assert.Equal(t, "recipe", byPath[current].Kind)
	assert.NotEmpty(t, byPath[current].RecordedAt)

	assert.Equal(t, output.TrustStatusChanged, byPath[changed].Status)
	assert.Equal(t, "deny", byPath[changed].Decision)

	assert.Equal(t, output.TrustStatusMissing, byPath[missing].Status)
	assert.Equal(t, output.TrustStatusChanged, byPath[unarmed].Status, "a recipe that runs nothing now has an inert decision")
	assert.Contains(t, byPath[unarmed].Detail, "no longer runs commands")
	assert.Equal(t, output.TrustStatusUnreadable, byPath[unreadable].Status)
	assert.NotEmpty(t, byPath[unreadable].Detail)

	assert.Equal(t, "formatter", byPath[formatter].Kind)
	assert.Equal(t, output.TrustStatusUnverified, byPath[formatter].Status)

	t.Run("entries come in path order", func(t *testing.T) {
		for i := 1; i < len(out.Entries); i++ {
			assert.Less(t, out.Entries[i-1].Path, out.Entries[i].Path)
		}
	})

	t.Run("the text rendering names every path and the record", func(t *testing.T) {
		var b bytes.Buffer
		require.NoError(t, out.FormatText(&b))
		for path := range byPath {
			assert.Contains(t, b.String(), path)
		}
		assert.Contains(t, b.String(), ExecTrustPath())
	})
}

func TestExecTrustShow(t *testing.T) {
	isolateTrust(t)
	a := &App{}
	recipePath, digest := execRecipeAt(t)

	t.Run("undecided before any answer", func(t *testing.T) {
		out, err := a.ExecTrustShow(recipePath)
		require.NoError(t, err)
		assert.Equal(t, output.TrustShowUndecided, out.Status)
		assert.Equal(t, digest, out.Digest)
		assert.Nil(t, out.Recorded)
		require.Len(t, out.Sites, 1)
		assert.Equal(t, "external-command", out.Sites[0].Name)
		assert.Equal(t, "flows.default.steps[0]", out.Sites[0].Where)
		assert.Contains(t, out.Sites[0].Detail, "/bin/echo")
		assert.False(t, out.EnvGranted)
	})

	t.Run("allowed at the current digest", func(t *testing.T) {
		require.NoError(t, recordExecTrust(recipePath, digest, execTrustAllow))
		out, err := a.ExecTrustShow(recipePath)
		require.NoError(t, err)
		assert.Equal(t, output.TrustShowAllowed, out.Status)
		require.NotNil(t, out.Recorded)
		assert.Equal(t, "allow", out.Recorded.Decision)
	})

	t.Run("declined at the current digest", func(t *testing.T) {
		require.NoError(t, recordExecTrust(recipePath, digest, execTrustDeny))
		out, err := a.ExecTrustShow(recipePath)
		require.NoError(t, err)
		assert.Equal(t, output.TrustShowDeclined, out.Status)
	})

	t.Run("changed when the record is for another surface", func(t *testing.T) {
		require.NoError(t, recordExecTrust(recipePath, "other", execTrustAllow))
		out, err := a.ExecTrustShow(recipePath)
		require.NoError(t, err)
		assert.Equal(t, output.TrustShowChanged, out.Status)
		assert.Equal(t, "other", out.Recorded.Digest)
	})

	t.Run("the environment grant is reported, not recorded", func(t *testing.T) {
		t.Setenv(execTrustEnvVar, "1")
		out, err := a.ExecTrustShow(recipePath)
		require.NoError(t, err)
		assert.True(t, out.EnvGranted)
		assert.Equal(t, output.TrustShowChanged, out.Status, "the record is reported as it is")
	})

	t.Run("a recipe that runs nothing has nothing to decide", func(t *testing.T) {
		plain := filepath.Join(t.TempDir(), project.RecipeFileName)
		require.NoError(t, os.WriteFile(plain, []byte("version: v1\n"), 0o644))
		out, err := a.ExecTrustShow(plain)
		require.NoError(t, err)
		assert.Equal(t, output.TrustShowNothingToDecide, out.Status)
		assert.Empty(t, out.Sites)
		assert.Empty(t, out.Digest)
	})

	t.Run("a missing recipe is an error", func(t *testing.T) {
		_, err := a.ExecTrustShow(filepath.Join(t.TempDir(), project.RecipeFileName))
		require.Error(t, err)
	})

	t.Run("every status renders as text", func(t *testing.T) {
		for _, status := range []string{output.TrustShowAllowed, output.TrustShowDeclined, output.TrustShowChanged, output.TrustShowUndecided, output.TrustShowNothingToDecide} {
			out := output.TrustShowOutput{Path: recipePath, Status: status, Record: ExecTrustPath(),
				Recorded: &output.TrustRecorded{Decision: "allow", RecordedAt: "2026-10-09T00:00:00Z"}}
			if status != output.TrustShowNothingToDecide {
				out.Sites = []output.TrustSite{{Where: "flows.default.steps[0]", Kind: "tool", Name: "external-command", Detail: "/bin/echo"}}
			}
			var b bytes.Buffer
			require.NoError(t, out.FormatText(&b), status)
			assert.Contains(t, b.String(), recipePath, status)
		}
	})
}

func TestExecTrustRevoke(t *testing.T) {
	isolateTrust(t)
	a := &App{}
	recipePath, digest := execRecipeAt(t)
	require.NoError(t, recordExecTrust(recipePath, digest, execTrustDeny))

	t.Run("the directory names the recipe in it", func(t *testing.T) {
		out, err := a.ExecTrustRevoke(filepath.Dir(recipePath))
		require.NoError(t, err)
		assert.True(t, out.Removed)
		assert.Equal(t, "deny", out.Decision)
		assert.Equal(t, recipePath, out.Path)
		_, found := lookupExecTrust(recipePath, digest)
		assert.False(t, found)
	})

	t.Run("revoking again is not an error", func(t *testing.T) {
		out, err := a.ExecTrustRevoke(recipePath)
		require.NoError(t, err)
		assert.False(t, out.Removed)
		assert.Empty(t, out.Decision)
	})

	t.Run("a withdrawn deny lets the gate ask again", func(t *testing.T) {
		require.NoError(t, recordExecTrust(recipePath, digest, execTrustDeny))
		err := (&App{}).ensureExecTrust(recipePath, execProject(t), LoadProjectInteractiveOptions{IsTTYFn: func() bool { return true }})
		require.ErrorIs(t, err, ErrExecNotTrusted)
		assert.Contains(t, err.Error(), "kapi trust revoke "+recipePath)

		_, err = a.ExecTrustRevoke(recipePath)
		require.NoError(t, err)
		require.NoError(t, (&App{}).ensureExecTrust(recipePath, execProject(t), LoadProjectInteractiveOptions{
			IsTTYFn: func() bool { return true }, In: readerOf("y\n"), Out: &bytes.Buffer{},
		}))
	})

	t.Run("other entries stay", func(t *testing.T) {
		other, _ := execRecipeAt(t)
		require.NoError(t, recordExecTrust(other, digest, execTrustAllow))
		_, err := a.ExecTrustRevoke(recipePath)
		require.NoError(t, err)
		_, found := lookupExecTrust(other, digest)
		assert.True(t, found)
	})

	t.Run("the text rendering says what happened", func(t *testing.T) {
		var b bytes.Buffer
		require.NoError(t, output.TrustRevokeOutput{Path: recipePath, Removed: true, Decision: "allow"}.FormatText(&b))
		assert.Contains(t, b.String(), "Withdrew the allow")
		b.Reset()
		require.NoError(t, output.TrustRevokeOutput{Path: recipePath}.FormatText(&b))
		assert.Contains(t, b.String(), "No decision recorded")
	})
}

func TestExecTrustAllow(t *testing.T) {
	isolateTrust(t)
	recipePath, digest := execRecipeAt(t)

	t.Run("a terminal is shown the commands and asked", func(t *testing.T) {
		isolateTrust(t)
		a := &App{isTTY: func() bool { return true }}
		cmd, errW := trustCommand("y\n")
		out, err := a.ExecTrustAllow(cmd, recipePath)
		require.NoError(t, err)
		assert.True(t, out.Recorded)
		assert.Equal(t, digest, out.Digest)
		assert.Contains(t, errW.String(), "/bin/echo")
		assert.Contains(t, errW.String(), "[y/N]")
		decision, found := lookupExecTrust(recipePath, digest)
		assert.True(t, found)
		assert.Equal(t, execTrustAllow, decision)
	})

	t.Run("a declined confirmation records nothing", func(t *testing.T) {
		isolateTrust(t)
		a := &App{isTTY: func() bool { return true }}
		cmd, _ := trustCommand("n\n")
		out, err := a.ExecTrustAllow(cmd, recipePath)
		require.NoError(t, err)
		assert.False(t, out.Recorded)
		assert.Equal(t, "not confirmed", out.Reason)
		assert.Empty(t, loadExecTrust().Projects, "a declined allow is not a deny")
	})

	t.Run("no terminal and no --yes refuses and records nothing", func(t *testing.T) {
		isolateTrust(t)
		a := &App{isTTY: func() bool { return false }}
		cmd, errW := trustCommand("y\n")
		_, err := a.ExecTrustAllow(cmd, recipePath)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "--yes")
		assert.Contains(t, errW.String(), "/bin/echo", "the commands are shown before the refusal")
		assert.Empty(t, loadExecTrust().Projects)
	})

	t.Run("no terminal with --yes records the allow", func(t *testing.T) {
		isolateTrust(t)
		a := &App{isTTY: func() bool { return false }, AssumeYes: true}
		cmd, errW := trustCommand("")
		out, err := a.ExecTrustAllow(cmd, recipePath)
		require.NoError(t, err)
		assert.True(t, out.Recorded)
		assert.NotContains(t, errW.String(), "[y/N]")
		decision, found := lookupExecTrust(recipePath, digest)
		assert.True(t, found)
		assert.Equal(t, execTrustAllow, decision)
	})

	t.Run("an allow replaces a recorded deny", func(t *testing.T) {
		isolateTrust(t)
		require.NoError(t, recordExecTrust(recipePath, digest, execTrustDeny))
		a := &App{isTTY: func() bool { return false }, AssumeYes: true}
		cmd, _ := trustCommand("")
		_, err := a.ExecTrustAllow(cmd, recipePath)
		require.NoError(t, err)
		decision, _ := lookupExecTrust(recipePath, digest)
		assert.Equal(t, execTrustAllow, decision)
	})

	t.Run("the environment grant is not recorded by allow either", func(t *testing.T) {
		isolateTrust(t)
		t.Setenv(execTrustEnvVar, "1")
		a := &App{isTTY: func() bool { return false }}
		cmd, _ := trustCommand("")
		_, err := a.ExecTrustAllow(cmd, recipePath)
		require.Error(t, err, "KAPI_TRUST_EXEC is process-scoped; it does not stand in for --yes")
		assert.Empty(t, loadExecTrust().Projects)
	})

	t.Run("a recipe that runs nothing records nothing", func(t *testing.T) {
		isolateTrust(t)
		plain := filepath.Join(t.TempDir(), project.RecipeFileName)
		require.NoError(t, os.WriteFile(plain, []byte("version: v1\n"), 0o644))
		a := &App{AssumeYes: true}
		cmd, _ := trustCommand("")
		out, err := a.ExecTrustAllow(cmd, plain)
		require.NoError(t, err)
		assert.False(t, out.Recorded)
		assert.Contains(t, out.Reason, "nothing to approve")
		assert.Empty(t, loadExecTrust().Projects)
	})

	t.Run("the gate honours the recorded allow silently", func(t *testing.T) {
		isolateTrust(t)
		a := &App{AssumeYes: true}
		cmd, _ := trustCommand("")
		_, err := a.ExecTrustAllow(cmd, recipePath)
		require.NoError(t, err)
		var prompted bytes.Buffer
		require.NoError(t, (&App{}).ensureExecTrust(recipePath, execProject(t), LoadProjectInteractiveOptions{
			IsTTYFn: func() bool { return false }, Out: &prompted,
		}))
		assert.Empty(t, prompted.String())
	})
}
