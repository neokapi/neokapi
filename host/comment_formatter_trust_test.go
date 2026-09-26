package host

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/comment"
	"github.com/neokapi/neokapi/core/contextop"
)

// markingOxfmt writes an executable named oxfmt into dir and returns its path.
// Each run appends a line to marker and then runs this repository's oxfmt, so a
// test can tell whether kapi executed it, and a run kapi allows still formats
// the file. It reads the repository's oxfmt by a path relative to the package,
// so a test calls it before changing directory.
func markingOxfmt(t *testing.T, dir, marker string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the fixture formatter is a shell script")
	}
	require.NoError(t, os.MkdirAll(dir, 0o755))
	script := "#!/bin/sh\necho ran >> '" + marker + "'\nexec '" + repoOxfmt(t) + "' \"$@\"\n"
	path := filepath.Join(dir, "oxfmt")
	require.NoError(t, os.WriteFile(path, []byte(script), 0o755))
	return path
}

// formatterRan reports whether a fixture formatter that writes marker ran.
func formatterRan(t *testing.T, marker string) bool {
	t.Helper()
	_, err := os.Stat(marker)
	if errors.Is(err, os.ErrNotExist) {
		return false
	}
	require.NoError(t, err)
	return true
}

// declaredFormatterApp returns an App reading comments through the sourcecode
// plugin with the formatter commands its manifest declares, which kapi looks
// for in the project and on PATH. Nothing grants execution trust: the
// environment grant is unset and no terminal is attached.
func declaredFormatterApp(t *testing.T) *App {
	t.Helper()
	a := sourcecodeApp(t, func(map[string]any) {})
	t.Setenv(execTrustEnvVar, "")
	a.isTTY = func() bool { return false }
	return a
}

// installedFormatterProject writes rewriteTS into a project whose oxfmt is a
// marking fixture installed in its node_modules/.bin, and returns the file, the
// marker the fixture writes, and the fixture's path.
func installedFormatterProject(t *testing.T) (file, marker, bin string) {
	t.Helper()
	file = tsProject(t, nil)
	marker = filepath.Join(t.TempDir(), "ran")
	bin = markingOxfmt(t, filepath.Join(filepath.Dir(file), "node_modules", ".bin"), marker)
	return file, marker, bin
}

// applyWithInput runs kapi apply through a over entries written as a change-set
// file, with stdin as the command's standard input, and returns the report and
// what the command wrote to standard error.
func applyWithInput(t *testing.T, a *App, stdin string, entries ...map[string]any) (applyOutput, string, error) {
	t.Helper()
	var lines []string
	for _, e := range entries {
		b, err := json.Marshal(e)
		require.NoError(t, err)
		lines = append(lines, string(b))
	}
	path := filepath.Join(t.TempDir(), "changeset.jsonl")
	require.NoError(t, os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o600))
	cmd := NewEnvCommand(t.Context(), "apply")
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetIn(strings.NewReader(stdin))
	err := a.RunApply(cmd, path, false, "", true)
	var out applyOutput
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &out), stdout.String()+stderr.String())
	return out, stderr.String(), err
}

// applyEditsMCPWith runs MCP apply_edits through a over entries.
func applyEditsMCPWith(t *testing.T, a *App, entries ...map[string]any) applyEditsMCPOutput {
	t.Helper()
	data, err := json.Marshal(map[string]any{"changeset": entries})
	require.NoError(t, err)
	var in applyEditsInput
	require.NoError(t, json.Unmarshal(data, &in))
	_, out, err := a.applyEditsMCP(t.Context(), contextop.Actor{Kind: contextop.ActorAgent}, in)
	require.NoError(t, err)
	return out
}

// repairEntry is the edit that repairs file's doubled word, guarded by the
// fingerprint the comment has now.
func repairEntry(t *testing.T, a *App, file string) map[string]any {
	t.Helper()
	return pluginCommentEntry(t, a, file, "func/parse", "Parses the input.")
}

// assertFormatterNotRun asserts that the one comment edit in comments did not
// run, with the reason formatter and a detail holding each of details, that
// file still holds rewriteTS, and that the fixture writing marker never ran.
func assertFormatterNotRun(t *testing.T, comments []commentFileResult, file, marker string, details ...string) {
	t.Helper()
	assert.False(t, formatterRan(t, marker), "kapi executed a formatter no one allowed")
	require.Len(t, comments, 1)
	edit := comments[0].Edits[0]
	assert.Equal(t, commentNotRun, edit.Status, edit.Detail)
	assert.Equal(t, string(comment.RefusedFormatter), edit.Reason)
	for _, d := range details {
		assert.Contains(t, edit.Detail, d)
	}
	assertUnchanged(t, file, rewriteTS)
}

// assertFormatterRan asserts that the one comment edit in comments was written
// and that the fixture writing marker ran. It then puts rewriteTS back and
// removes marker, so the same project can be edited again.
func assertFormatterRan(t *testing.T, comments []commentFileResult, file, marker string) {
	t.Helper()
	assert.True(t, formatterRan(t, marker), "kapi did not run the formatter that was allowed")
	require.Len(t, comments, 1)
	edit := comments[0].Edits[0]
	assert.Equal(t, commentWritten, edit.Status, edit.Detail)
	assertUnchanged(t, file, strings.Replace(rewriteTS, "the the", "the", 1))
	require.NoError(t, os.WriteFile(file, []byte(rewriteTS), 0o644))
	require.NoError(t, os.Remove(marker))
}

// A project's formatter runs code the project controls, so a comment edit runs
// it only under execution trust (host/exectrust.go): the KAPI_TRUST_EXEC grant
// for kapi apply, a decision recorded for the configuration file that selects
// the formatter, or an answer given at kapi apply's prompt. MCP apply_edits
// never runs it. Otherwise the edit did not run.
//
// The subtests named "must fail" hold a formatter no one allowed, or an allow
// that no longer matches what would run, and assert that it never runs.
func TestCommentFormatterTrust(t *testing.T) {
	t.Run("must fail: kapi apply with no terminal does not run a formatter no one allowed", func(t *testing.T) {
		a := declaredFormatterApp(t)
		file, marker, _ := installedFormatterProject(t)
		out, _, err := applyWithInput(t, a, "", repairEntry(t, a, file))
		assert.Equal(t, ExitGate, ExitCode(nil, err))
		assertFormatterNotRun(t, out.Comments, file, marker, "no one has allowed it to run", "kapi apply from a terminal", execTrustEnvVar)
	})

	t.Run("must fail: a formatter on PATH does not run for a project no one allowed", func(t *testing.T) {
		a := declaredFormatterApp(t)
		file := tsProject(t, nil)
		marker := filepath.Join(t.TempDir(), "ran")
		bin := t.TempDir()
		markingOxfmt(t, bin, marker)
		t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
		out, _, err := applyWithInput(t, a, "", repairEntry(t, a, file))
		assert.Equal(t, ExitGate, ExitCode(nil, err))
		assertFormatterNotRun(t, out.Comments, file, marker, "no one has allowed it to run")
	})

	t.Run("must fail: apply_edits does not run a formatter no one allowed", func(t *testing.T) {
		a := declaredFormatterApp(t)
		file, marker, _ := installedFormatterProject(t)
		out := applyEditsMCPWith(t, a, repairEntry(t, a, file))
		assert.False(t, out.OK)
		assertFormatterNotRun(t, out.Comments, file, marker, "apply_edits never runs a project's formatter", "kapi apply")
	})

	t.Run("must fail: apply_edits does not take KAPI_TRUST_EXEC as an allow", func(t *testing.T) {
		a := declaredFormatterApp(t)
		t.Setenv(execTrustEnvVar, "1")
		file, marker, _ := installedFormatterProject(t)
		out := applyEditsMCPWith(t, a, repairEntry(t, a, file))
		assertFormatterNotRun(t, out.Comments, file, marker, "apply_edits never runs a project's formatter")
	})

	t.Run("kapi apply runs the formatter under KAPI_TRUST_EXEC and records nothing", func(t *testing.T) {
		a := declaredFormatterApp(t)
		t.Setenv(execTrustEnvVar, "1")
		file, marker, _ := installedFormatterProject(t)
		out, _, err := applyWithInput(t, a, "", repairEntry(t, a, file))
		require.NoError(t, err)
		assertFormatterRan(t, out.Comments, file, marker)
		assert.Empty(t, loadExecTrust().Projects, "the environment grant is not recorded")
	})

	t.Run("an allow answered at kapi apply's prompt is recorded, and kapi apply then runs the formatter without asking", func(t *testing.T) {
		a := declaredFormatterApp(t)
		a.isTTY = func() bool { return true }
		file, marker, bin := installedFormatterProject(t)
		out, prompt, err := applyWithInput(t, a, "y\n", repairEntry(t, a, file))
		require.NoError(t, err)
		assert.Contains(t, prompt, ".oxfmtrc.json selects the formatter a comment edit runs")
		assert.Contains(t, prompt, filepath.Base(filepath.Dir(bin))+string(filepath.Separator)+"oxfmt")
		assert.Contains(t, prompt, "Allow this formatter to run? [y/N]")
		assertFormatterRan(t, out.Comments, file, marker)

		a.isTTY = func() bool { return false }
		again, _, err := applyWithInput(t, a, "", repairEntry(t, a, file))
		require.NoError(t, err)
		assertFormatterRan(t, again.Comments, file, marker)
	})

	t.Run("must fail: a decline answered at the prompt is recorded and holds", func(t *testing.T) {
		a := declaredFormatterApp(t)
		a.isTTY = func() bool { return true }
		file, marker, _ := installedFormatterProject(t)
		out, _, err := applyWithInput(t, a, "n\n", repairEntry(t, a, file))
		assert.Equal(t, ExitGate, ExitCode(nil, err))
		assertFormatterNotRun(t, out.Comments, file, marker, "was declined")

		a.isTTY = func() bool { return false }
		out, _, _ = applyWithInput(t, a, "", repairEntry(t, a, file))
		assertFormatterNotRun(t, out.Comments, file, marker, "was declined", ExecTrustPath())
		mcp := applyEditsMCPWith(t, a, repairEntry(t, a, file))
		assertFormatterNotRun(t, mcp.Comments, file, marker, "apply_edits never runs a project's formatter")
	})

	t.Run("must fail: a change to the configuration file that selects the formatter voids the allow", func(t *testing.T) {
		a := declaredFormatterApp(t)
		a.isTTY = func() bool { return true }
		file, marker, _ := installedFormatterProject(t)
		out, _, err := applyWithInput(t, a, "y\n", repairEntry(t, a, file))
		require.NoError(t, err)
		assertFormatterRan(t, out.Comments, file, marker)

		require.NoError(t, os.WriteFile(filepath.Join(filepath.Dir(file), ".oxfmtrc.json"), []byte("{ }\n"), 0o644))
		a.isTTY = func() bool { return false }
		changed, _, _ := applyWithInput(t, a, "", repairEntry(t, a, file))
		assertFormatterNotRun(t, changed.Comments, file, marker, "no one has allowed it to run")
	})

	t.Run("must fail: a change to the formatter executable voids the allow", func(t *testing.T) {
		a := declaredFormatterApp(t)
		a.isTTY = func() bool { return true }
		file, marker, bin := installedFormatterProject(t)
		out, _, err := applyWithInput(t, a, "y\n", repairEntry(t, a, file))
		require.NoError(t, err)
		assertFormatterRan(t, out.Comments, file, marker)

		script, err := os.ReadFile(bin)
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(bin, append(script, "# changed\n"...), 0o755))
		a.isTTY = func() bool { return false }
		changed, _, _ := applyWithInput(t, a, "", repairEntry(t, a, file))
		assertFormatterNotRun(t, changed.Comments, file, marker, "no one has allowed it to run")
	})

	t.Run("must fail: a change-set read from standard input leaves no one to ask", func(t *testing.T) {
		a := declaredFormatterApp(t)
		a.isTTY = func() bool { return true }
		cmd := NewEnvCommand(t.Context(), "apply")
		assert.Nil(t, a.applyFormatterTrust(cmd, true).ask, "the answer would be read from the change-set")
		assert.NotNil(t, a.applyFormatterTrust(cmd, false).ask)
		assert.Nil(t, mcpFormatterTrust().ask, "apply_edits never runs a project's formatter")
	})

	t.Run("must fail: a PATH entry that is not an absolute path is skipped and named", func(t *testing.T) {
		a := declaredFormatterApp(t)
		t.Setenv(execTrustEnvVar, "1")
		file := tsProject(t, nil)
		cwd := t.TempDir()
		marker := filepath.Join(t.TempDir(), "ran")
		markingOxfmt(t, filepath.Join(cwd, "bin"), marker)
		markingOxfmt(t, cwd, marker)
		entry := repairEntry(t, a, file)
		t.Chdir(cwd)
		t.Setenv("PATH", "bin"+string(os.PathListSeparator)+".")
		out, _, err := applyWithInput(t, a, "", entry)
		assert.Equal(t, ExitGate, ExitCode(nil, err))
		assertFormatterNotRun(t, out.Comments, file, marker, "is not installed", `skips the entries that are not absolute paths: "bin", "."`)
	})

	t.Run("must fail: a formatter resolved without trust runs nothing when called directly", func(t *testing.T) {
		a := declaredFormatterApp(t)
		file, marker, _ := installedFormatterProject(t)
		p, ok := a.commentProviderFor(file)
		require.True(t, ok)
		f := resolveCommentFormatter(p, file, p.(commentRewriteDeclarer).Rewrite(), newFormatterTrust(false))
		_, err := f.run(file, []byte(rewriteTS))
		require.ErrorIs(t, err, comment.ErrFormatterNotRun)
		require.ErrorIs(t, f.verify(file), comment.ErrFormatterNotRun)
		assert.False(t, formatterRan(t, marker), "a direct call executed a formatter no one allowed")
	})
}
