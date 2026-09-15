package host

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/comment"
)

// markingOxfmt writes an executable named oxfmt into dir. Each run appends a
// line to marker and then runs this repository's oxfmt, so a test can tell
// whether kapi executed it, and a run kapi allows still formats the file. It
// reads the repository's oxfmt by a path relative to the package, so a test
// calls it before changing directory.
func markingOxfmt(t *testing.T, dir, marker string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the fixture formatter is a shell script")
	}
	require.NoError(t, os.MkdirAll(dir, 0o755))
	script := "#!/bin/sh\necho ran >> '" + marker + "'\nexec '" + repoOxfmt(t) + "' \"$@\"\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "oxfmt"), []byte(script), 0o755))
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
// for in the project and on PATH.
func declaredFormatterApp(t *testing.T) *App {
	t.Helper()
	return sourcecodeApp(t, func(map[string]any) {})
}

// configureTrustedDirs writes kapi's own configuration, in the directory the
// test isolates it to, with formatters.trusted_dirs set to value, a YAML value.
func configureTrustedDirs(t *testing.T, value string) {
	t.Helper()
	dir := os.Getenv("KAPI_CONFIG_DIR")
	require.NotEmpty(t, dir, "the App under test isolates kapi's configuration")
	require.NoError(t, os.MkdirAll(dir, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "kapi.yaml"), []byte("formatters:\n  trusted_dirs: "+value+"\n"), 0o600))
}

// assertFormatterNotTrusted asserts that an edit to file's line comment did not
// run, wrote nothing, and never executed the formatter that writes marker.
func assertFormatterNotTrusted(t *testing.T, a *App, file, marker string) {
	t.Helper()
	out, err := applyWith(t, a, pluginCommentEntry(t, a, file, "func/parse", "Parses the input."))
	assert.Equal(t, ExitGate, ExitCode(nil, err))
	assert.False(t, formatterRan(t, marker), "kapi executed a formatter the user has not trusted")
	require.Len(t, out.Comments, 1)
	edit := out.Comments[0].Edits[0]
	assert.Equal(t, commentNotRun, edit.Status, edit.Detail)
	assert.Equal(t, string(comment.RefusedFormatter), edit.Reason)
	assert.Contains(t, edit.Detail, "--trust-project-formatters")
	assert.Contains(t, edit.Detail, "formatters.trusted_dirs")
	assertUnchanged(t, file, rewriteTS)
}

// assertFormatterTrusted asserts that an edit to file's line comment ran the
// formatter that writes marker and was written.
func assertFormatterTrusted(t *testing.T, a *App, file, marker string) {
	t.Helper()
	out, err := applyWith(t, a, pluginCommentEntry(t, a, file, "func/parse", "Parses the input."))
	require.NoError(t, err)
	assert.True(t, formatterRan(t, marker), "kapi did not run the formatter the user trusts")
	require.Len(t, out.Comments, 1)
	edit := out.Comments[0].Edits[0]
	assert.Equal(t, commentWritten, edit.Status, edit.Detail)
	assertUnchanged(t, file, strings.Replace(rewriteTS, "the the", "the", 1))
}

// applyEditsMCPWith runs MCP apply_edits through a over entries.
func applyEditsMCPWith(t *testing.T, a *App, entries ...map[string]any) applyEditsMCPOutput {
	t.Helper()
	data, err := json.Marshal(map[string]any{"changeset": entries})
	require.NoError(t, err)
	var in applyEditsInput
	require.NoError(t, json.Unmarshal(data, &in))
	_, out, err := a.applyEditsMCP(t.Context(), in)
	require.NoError(t, err)
	return out
}

// A project's formatter runs code the project controls: its executable when the
// project installs it, and its configuration and plugins wherever it is
// installed. A comment edit runs it only when the user trusts the project's
// formatters from outside the project, and otherwise did not run.
//
// The subtests named "must fail" hold a project nobody trusted, or a trust that
// does not reach it, and assert that its formatter never runs.
func TestCommentFormatterTrust(t *testing.T) {
	t.Run("must fail: a formatter the project installs does not run untrusted", func(t *testing.T) {
		a := declaredFormatterApp(t)
		file := tsProject(t, nil)
		marker := filepath.Join(t.TempDir(), "ran")
		markingOxfmt(t, filepath.Join(filepath.Dir(file), "node_modules", ".bin"), marker)
		assertFormatterNotTrusted(t, a, file, marker)
	})

	t.Run("must fail: a formatter on PATH does not run untrusted", func(t *testing.T) {
		a := declaredFormatterApp(t)
		file := tsProject(t, nil)
		marker := filepath.Join(t.TempDir(), "ran")
		bin := t.TempDir()
		markingOxfmt(t, bin, marker)
		t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
		assertFormatterNotTrusted(t, a, file, marker)
	})

	t.Run("a formatter the project installs runs when the command trusts project formatters", func(t *testing.T) {
		a := declaredFormatterApp(t)
		a.TrustProjectFormatters = true
		file := tsProject(t, nil)
		marker := filepath.Join(t.TempDir(), "ran")
		markingOxfmt(t, filepath.Join(filepath.Dir(file), "node_modules", ".bin"), marker)
		assertFormatterTrusted(t, a, file, marker)
	})

	t.Run("a formatter on PATH runs for a project under a directory the configuration lists", func(t *testing.T) {
		a := declaredFormatterApp(t)
		file := tsProject(t, nil)
		configureTrustedDirs(t, "["+strconv.Quote(filepath.Dir(file))+"]")
		marker := filepath.Join(t.TempDir(), "ran")
		bin := t.TempDir()
		markingOxfmt(t, bin, marker)
		t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
		assertFormatterTrusted(t, a, file, marker)
	})

	t.Run("directories joined into one string, as kapi config set stores them, are each trusted", func(t *testing.T) {
		a := declaredFormatterApp(t)
		file := tsProject(t, nil)
		configureTrustedDirs(t, strconv.Quote(t.TempDir()+string(os.PathListSeparator)+filepath.Dir(filepath.Dir(file))))
		marker := filepath.Join(t.TempDir(), "ran")
		markingOxfmt(t, filepath.Join(filepath.Dir(file), "node_modules", ".bin"), marker)
		assertFormatterTrusted(t, a, file, marker)
	})

	t.Run("must fail: a directory the configuration lists grants nothing outside it", func(t *testing.T) {
		a := declaredFormatterApp(t)
		file := tsProject(t, nil)
		configureTrustedDirs(t, "["+strconv.Quote(t.TempDir())+"]")
		marker := filepath.Join(t.TempDir(), "ran")
		markingOxfmt(t, filepath.Join(filepath.Dir(file), "node_modules", ".bin"), marker)
		assertFormatterNotTrusted(t, a, file, marker)
	})

	t.Run("must fail: a relative directory in the configuration grants nothing", func(t *testing.T) {
		a := declaredFormatterApp(t)
		file := tsProject(t, nil)
		configureTrustedDirs(t, `["."]`)
		marker := filepath.Join(t.TempDir(), "ran")
		markingOxfmt(t, filepath.Join(filepath.Dir(file), "node_modules", ".bin"), marker)
		t.Chdir(filepath.Dir(file))
		assertFormatterNotTrusted(t, a, file, marker)
	})

	t.Run("must fail: a kapi.yaml in the project grants nothing", func(t *testing.T) {
		a := declaredFormatterApp(t)
		file := tsProject(t, map[string]string{"kapi.yaml": "formatters:\n  trusted_dirs: [\"/\"]\n"})
		marker := filepath.Join(t.TempDir(), "ran")
		markingOxfmt(t, filepath.Join(filepath.Dir(file), "node_modules", ".bin"), marker)
		t.Chdir(filepath.Dir(file))
		assertFormatterNotTrusted(t, a, file, marker)
	})

	t.Run("must fail: a PATH entry that is not an absolute path is skipped and named", func(t *testing.T) {
		a := declaredFormatterApp(t)
		a.TrustProjectFormatters = true
		file := tsProject(t, nil)
		cwd := t.TempDir()
		marker := filepath.Join(t.TempDir(), "ran")
		markingOxfmt(t, filepath.Join(cwd, "bin"), marker)
		markingOxfmt(t, cwd, marker)
		entry := pluginCommentEntry(t, a, file, "func/parse", "Parses the input.")
		t.Chdir(cwd)
		t.Setenv("PATH", "bin"+string(os.PathListSeparator)+".")
		out, err := applyWith(t, a, entry)
		assert.Equal(t, ExitGate, ExitCode(nil, err))
		assert.False(t, formatterRan(t, marker), "kapi executed a formatter found through a relative PATH entry")
		require.Len(t, out.Comments, 1)
		edit := out.Comments[0].Edits[0]
		assert.Equal(t, commentNotRun, edit.Status, edit.Detail)
		assert.Equal(t, string(comment.RefusedFormatter), edit.Reason)
		assert.Contains(t, edit.Detail, "is not installed")
		assert.Contains(t, edit.Detail, `skips the entries that are not absolute paths: "bin", "."`)
		assertUnchanged(t, file, rewriteTS)
	})

	t.Run("must fail: apply_edits does not run a formatter its server was not started to trust", func(t *testing.T) {
		a := declaredFormatterApp(t)
		file := tsProject(t, nil)
		marker := filepath.Join(t.TempDir(), "ran")
		markingOxfmt(t, filepath.Join(filepath.Dir(file), "node_modules", ".bin"), marker)
		out := applyEditsMCPWith(t, a, pluginCommentEntry(t, a, file, "func/parse", "Parses the input."))
		assert.False(t, out.OK)
		assert.False(t, formatterRan(t, marker), "apply_edits executed a formatter the user has not trusted")
		require.Len(t, out.Comments, 1)
		edit := out.Comments[0].Edits[0]
		assert.Equal(t, commentNotRun, edit.Status, edit.Detail)
		assert.Equal(t, string(comment.RefusedFormatter), edit.Reason)
		assert.Contains(t, edit.Detail, "--trust-project-formatters")
		assertUnchanged(t, file, rewriteTS)
	})

	t.Run("apply_edits runs the formatter when its server trusts project formatters", func(t *testing.T) {
		a := declaredFormatterApp(t)
		a.TrustProjectFormatters = true
		file := tsProject(t, nil)
		marker := filepath.Join(t.TempDir(), "ran")
		markingOxfmt(t, filepath.Join(filepath.Dir(file), "node_modules", ".bin"), marker)
		out := applyEditsMCPWith(t, a, pluginCommentEntry(t, a, file, "func/parse", "Parses the input."))
		assert.True(t, formatterRan(t, marker), "apply_edits did not run the formatter the user trusts")
		require.Len(t, out.Comments, 1)
		edit := out.Comments[0].Edits[0]
		assert.Equal(t, commentWritten, edit.Status, edit.Detail)
		assertUnchanged(t, file, strings.Replace(rewriteTS, "the the", "the", 1))
	})

	t.Run("must fail: a formatter resolved for an untrusted project runs nothing when called directly", func(t *testing.T) {
		a := declaredFormatterApp(t)
		file := tsProject(t, nil)
		marker := filepath.Join(t.TempDir(), "ran")
		markingOxfmt(t, filepath.Join(filepath.Dir(file), "node_modules", ".bin"), marker)
		p, ok := a.commentProviderFor(file)
		require.True(t, ok)
		f := resolveCommentFormatter(p, file, p.(commentRewriteDeclarer).Rewrite(), formatterTrust{})
		_, err := f.run(file, []byte(rewriteTS))
		require.ErrorIs(t, err, comment.ErrFormatterNotRun)
		require.ErrorIs(t, f.verify(file), comment.ErrFormatterNotRun)
		assert.False(t, formatterRan(t, marker), "a direct call executed a formatter the user has not trusted")
	})
}
