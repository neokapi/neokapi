package host

import (
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A recorded formatter allow covers what the formatter would run, and nothing
// else stands in for it. These routes reached code outside the record: an agent
// surface that ran a formatter on an allow, a configuration file the marker list
// did not name, a package.json that named prettier only as a dependency, the
// interpreter a node_modules/.bin shim runs, and a relative PATH entry the
// formatter's child inherited.

// markingFormatter writes an executable named name into dir that appends a line
// to marker, runs the lines in extra, and then execs target with its arguments.
func markingFormatter(t *testing.T, dir, name, marker, target, extra string) string {
	t.Helper()
	require.NoError(t, os.MkdirAll(dir, 0o755))
	script := "#!/bin/sh\necho ran >> '" + marker + "'\n" + extra + "exec '" + target + "' \"$@\"\n"
	path := filepath.Join(dir, name)
	require.NoError(t, os.WriteFile(path, []byte(script), 0o755))
	return path
}

// repoPrettier is the prettier installed by `vp install` at the repository root.
func repoPrettier(t *testing.T) string {
	t.Helper()
	path, err := filepath.Abs(filepath.Join("..", "node_modules", ".bin", "prettier"))
	require.NoError(t, err)
	_, err = os.Stat(path)
	require.NoError(t, err, "the prettier fixture runs the repository's prettier; run `vp install` at the repository root")
	return path
}

// prettierProject writes rewriteTS into a project whose configuration files are
// files, with no oxfmt configuration, and a marking prettier installed in its
// node_modules/.bin. It returns the file, the marker the fixture writes, and the
// project directory.
func prettierProject(t *testing.T, files map[string]string) (file, marker, dir string) {
	t.Helper()
	skipShellFixture(t)
	withoutOxfmt := map[string]string{".oxfmtrc.json": ""}
	maps.Copy(withoutOxfmt, files)
	file = tsProject(t, withoutOxfmt)
	dir = filepath.Dir(file)
	marker = filepath.Join(t.TempDir(), "ran")
	markingFormatter(t, filepath.Join(dir, "node_modules", ".bin"), "prettier", marker, repoPrettier(t), "")
	return file, marker, dir
}

func skipShellFixture(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the fixture formatter is a shell script")
	}
}

// nestedFile writes rewriteTS at rel under dir and returns its path.
func nestedFile(t *testing.T, dir, rel string) string {
	t.Helper()
	path := filepath.Join(dir, filepath.FromSlash(rel))
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(rewriteTS), 0o644))
	return path
}

// allowAtPrompt edits file through kapi apply at a terminal, answers yes, and
// asserts the formatter ran.
func allowAtPrompt(t *testing.T, a *App, file, marker string) {
	t.Helper()
	a.isTTY = func() bool { return true }
	out, _, err := applyWithInput(t, a, "y\n", repairEntry(t, a, file))
	require.NoError(t, err)
	assertFormatterRan(t, out.Comments, file, marker)
	a.isTTY = func() bool { return false }
}

func TestFormatterTrustRoutes(t *testing.T) {
	t.Run("must fail: apply_edits never runs a project formatter, even one allowed at the prompt", func(t *testing.T) {
		a := declaredFormatterApp(t)
		file, marker, _ := installedFormatterProject(t)
		allowAtPrompt(t, a, file, marker)

		out := applyEditsMCPWith(t, a, repairEntry(t, a, file))
		assert.False(t, out.OK)
		assertFormatterNotRun(t, out.Comments, file, marker, "apply_edits never runs a project's formatter", "kapi apply")
	})

	t.Run("must fail: a prettier configuration nested under an allowed one voids the allow", func(t *testing.T) {
		a := declaredFormatterApp(t)
		file, marker, dir := prettierProject(t, map[string]string{".prettierrc": "{}\n"})
		allowAtPrompt(t, a, file, marker)

		nested := nestedFile(t, dir, "packages/x/parse.ts")
		require.NoError(t, os.WriteFile(filepath.Join(dir, "packages", "x", "prettier.config.ts"), []byte("export default {};\n"), 0o644))
		out, _, err := applyWithInput(t, a, "", repairEntry(t, a, nested))
		assert.Equal(t, ExitGate, ExitCode(nil, err))
		assertFormatterNotRun(t, out.Comments, nested, marker, "no one has allowed it to run")
	})

	t.Run("must fail: a configuration file oxfmt loads beside the edited file voids the allow", func(t *testing.T) {
		a := declaredFormatterApp(t)
		file, marker, _ := installedFormatterProject(t)
		allowAtPrompt(t, a, file, marker)

		dir := filepath.Dir(file)
		nested := nestedFile(t, dir, "packages/x/parse.ts")
		require.NoError(t, os.WriteFile(filepath.Join(dir, "packages", "x", "vite.config.cts"), []byte("module.exports = {};\n"), 0o644))
		out, _, err := applyWithInput(t, a, "", repairEntry(t, a, nested))
		assert.Equal(t, ExitGate, ExitCode(nil, err))
		assertFormatterNotRun(t, out.Comments, nested, marker, "no one has allowed it to run")
	})

	t.Run("must fail: a package.json naming prettier only as a dependency configures no formatter", func(t *testing.T) {
		a := declaredFormatterApp(t)
		t.Setenv(execTrustEnvVar, "1")
		file, marker, _ := prettierProject(t, map[string]string{"package.json": `{"devDependencies": {"prettier": "3.9.5"}}` + "\n"})
		out, _, err := applyWithInput(t, a, "", repairEntry(t, a, file))
		assert.Equal(t, ExitGate, ExitCode(nil, err))
		assertFormatterNotRun(t, out.Comments, file, marker, "no formatter for")
	})

	t.Run("a package.json with a top-level prettier key configures prettier", func(t *testing.T) {
		a := declaredFormatterApp(t)
		t.Setenv(execTrustEnvVar, "1")
		file, marker, _ := prettierProject(t, map[string]string{"package.json": `{"name": "p", "prettier": {}}` + "\n"})
		out, _, err := applyWithInput(t, a, "", repairEntry(t, a, file))
		require.NoError(t, err)
		assertFormatterRan(t, out.Comments, file, marker)
	})

	t.Run("must fail: a change to the node a node_modules/.bin shim runs voids the allow", func(t *testing.T) {
		a := declaredFormatterApp(t)
		skipShellFixture(t)
		file := tsProject(t, nil)
		bin := filepath.Join(filepath.Dir(file), "node_modules", ".bin")
		marker := filepath.Join(t.TempDir(), "ran")
		realNode, err := exec.LookPath("node")
		require.NoError(t, err, "the shim fixture runs node")
		entryPath, err := filepath.Abs(filepath.Join("..", "node_modules", "vite-plus", "bin", "oxfmt"))
		require.NoError(t, err)
		entry, err := filepath.EvalSymlinks(entryPath)
		require.NoError(t, err, "the shim fixture runs the repository's vite-plus oxfmt; run `vp install` at the repository root")
		require.NoError(t, os.MkdirAll(bin, 0o755))
		node := filepath.Join(bin, "node")
		require.NoError(t, os.WriteFile(node, []byte("#!/bin/sh\nexec '"+realNode+"' \"$@\"\n"), 0o755))
		// The repository's own pnpm shim, marking each run and pointing at the
		// vite-plus entry by its absolute path, so it keeps the NODE_PATH the
		// entry resolves its modules through.
		repoShim, err := os.ReadFile(repoOxfmt(t))
		require.NoError(t, err)
		shim := strings.Replace(string(repoShim), "#!/bin/sh\n", "#!/bin/sh\necho ran >> '"+marker+"'\n", 1)
		require.Contains(t, shim, `"$basedir/node"`, "the repository's oxfmt is a pnpm shim")
		shim = strings.ReplaceAll(shim, `"$basedir/../vite-plus/bin/oxfmt"`, "'"+entry+"'")
		require.NoError(t, os.WriteFile(filepath.Join(bin, "oxfmt"), []byte(shim), 0o755))
		allowAtPrompt(t, a, file, marker)

		script, err := os.ReadFile(node)
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(node, append(script, "# changed\n"...), 0o755))
		out, _, err := applyWithInput(t, a, "", repairEntry(t, a, file))
		assert.Equal(t, ExitGate, ExitCode(nil, err))
		assertFormatterNotRun(t, out.Comments, file, marker, "no one has allowed it to run")
	})

	t.Run("must fail: a relative PATH entry does not reach the formatter", func(t *testing.T) {
		a := declaredFormatterApp(t)
		skipShellFixture(t)
		t.Setenv(execTrustEnvVar, "1")
		file := tsProject(t, nil)
		marker := filepath.Join(t.TempDir(), "ran")
		seen := filepath.Join(t.TempDir(), "path")
		markingFormatter(t, filepath.Join(filepath.Dir(file), "node_modules", ".bin"), "oxfmt", marker, repoOxfmt(t),
			"printf '%s' \"$PATH\" > '"+seen+"'\n")
		sep := string(os.PathListSeparator)
		t.Setenv("PATH", "relbin"+sep+"."+sep+sep+os.Getenv("PATH"))
		out, _, err := applyWithInput(t, a, "", repairEntry(t, a, file))
		require.NoError(t, err)
		assertFormatterRan(t, out.Comments, file, marker)

		got, err := os.ReadFile(seen)
		require.NoError(t, err)
		entries := strings.Split(string(got), sep)
		require.NotEmpty(t, entries)
		for _, e := range entries {
			assert.True(t, filepath.IsAbs(e), "the formatter's PATH holds %q, which is not an absolute path", e)
		}
	})
}
