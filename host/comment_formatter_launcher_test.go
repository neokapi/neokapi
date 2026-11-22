package host

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The fixtures in testdata/launchers are node_modules/.bin launchers as the
// named pnpm release wrote them, verbatim except that the project directory is
// /project, and a launcher whose script kapi cannot know without running it.
var launcherFixtures = []struct {
	file, target string
}{
	{"pnpm11.5.0-oxfmt", "oxfmt/bin/oxfmt"},
	{"pnpm11.13.0-semver", ".pnpm/semver@7.7.2/node_modules/semver/bin/semver.js"},
	{"pnpm11.28.13-semver", ".pnpm/semver@7.7.2/node_modules/semver/bin/semver.js"},
	{"pnpm12.0.0-semver", "semver/bin/semver.js"},
	{"pnpm12.11.2-semver", ".pnpm/semver@7.7.2/node_modules/semver/bin/semver.js"},
	{"pnpm12.21.0-oxfmt", ".pnpm/oxfmt@0.79.0_vite-plus@1.7.0_@types+node@26.6.6_@vitest+ui@5.1.0_esbuild@0.28.2_jiti@2._2ce4b6ad2894019449197584a0f63826/node_modules/oxfmt/bin/oxfmt"},
}

func readLauncherFixture(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "launchers", name))
	require.NoError(t, err)
	return string(data)
}

// installLauncher writes body to <root>/node_modules/.bin/tool and returns
// root, resolved to its physical location, and the launcher's path.
func installLauncher(t *testing.T, body string) (root, bin string) {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	bin = filepath.Join(root, "node_modules", ".bin", "tool")
	require.NoError(t, os.MkdirAll(filepath.Dir(bin), 0o755))
	require.NoError(t, os.WriteFile(bin, []byte(body), 0o755))
	return root, bin
}

// echoNode writes an executable node beside the launcher that prints the
// script it is handed, so a test can ask the shell what the launcher runs.
func echoNode(t *testing.T, bin string) string {
	t.Helper()
	node := filepath.Join(filepath.Dir(bin), "node")
	require.NoError(t, os.WriteFile(node, []byte("#!/bin/sh\nprintf '%s' \"$1\"\n"), 0o755))
	return node
}

// shellRuns runs the launcher at path and returns, cleaned, the script its
// node received.
func shellRuns(t *testing.T, path string) string {
	t.Helper()
	out, err := exec.CommandContext(t.Context(), path, "--version").Output()
	require.NoError(t, err)
	return filepath.Clean(string(out))
}

func TestReadLauncherLayouts(t *testing.T) {
	skipShellFixture(t)
	for _, fx := range launcherFixtures {
		t.Run(fx.file, func(t *testing.T) {
			root, bin := installLauncher(t, readLauncherFixture(t, fx.file))
			node := echoNode(t, bin)
			rt, err := formatterRuntimeOf(bin)
			require.NoError(t, err)
			want := filepath.Join(root, "node_modules", filepath.FromSlash(fx.target))
			assert.Equal(t, formatterRuntime{interpreter: node, script: want}, rt)
			assert.Equal(t, want, shellRuns(t, bin), "the shell runs the script kapi names")
		})
	}
}

// With no executable node beside it, a launcher runs the node on PATH.
func TestReadLauncherNodeOnPath(t *testing.T) {
	skipShellFixture(t)
	pathDir := t.TempDir()
	onPath := filepath.Join(pathDir, "node")
	require.NoError(t, os.WriteFile(onPath, []byte("#!/bin/sh\n"), 0o755))
	t.Setenv("PATH", "."+string(os.PathListSeparator)+pathDir)
	_, bin := installLauncher(t, readLauncherFixture(t, "pnpm12.21.0-oxfmt"))
	rt, err := formatterRuntimeOf(bin)
	require.NoError(t, err)
	assert.Equal(t, onPath, rt.interpreter)
}

// A launcher that resolves $0 through symbolic links runs the script beside
// the launcher, not beside a link to it.
func TestReadLauncherFollowsLinks(t *testing.T) {
	skipShellFixture(t)
	root, bin := installLauncher(t, readLauncherFixture(t, "pnpm12.21.0-oxfmt"))
	node := echoNode(t, bin)
	link := filepath.Join(t.TempDir(), "tool")
	require.NoError(t, os.Symlink(bin, link))
	rt, err := formatterRuntimeOf(link)
	require.NoError(t, err)
	want := filepath.Join(root, "node_modules", filepath.FromSlash(launcherFixtures[len(launcherFixtures)-1].target))
	assert.Equal(t, formatterRuntime{interpreter: node, script: want}, rt)
	assert.Equal(t, want, shellRuns(t, link))
}

func TestReadLauncherRefuses(t *testing.T) {
	pnpm11 := readLauncherFixture(t, "pnpm11.5.0-oxfmt")
	pnpm12 := readLauncherFixture(t, "pnpm12.21.0-oxfmt")
	replace := func(s, old, new string) string {
		require.Contains(t, s, old)
		return strings.Replace(s, old, new, 1)
	}

	for name, body := range map[string]string{
		"a script computed at run time":        readLauncherFixture(t, "unknown-oxfmt"),
		"a basedir reassignment appended":      pnpm12 + "basedir=/tmp\n",
		"a basedir reassignment before exec":   replace(pnpm11, "\nif [ -x \"$basedir/node\" ]", "\nbasedir=/tmp\nif [ -x \"$basedir/node\" ]"),
		"an exec after true;":                  replace(pnpm12, "\nfi\n# cmd-shim-target", "\nfi\ntrue; exec /bin/sh -c id\n# cmd-shim-target"),
		"an exec after a tab":                  replace(pnpm11, "\nfi\n#", "\nfi\n\texec /bin/sh -c id\n#"),
		"an exec after &&":                     replace(pnpm11, "\nfi\n#", "\nfi\ntrue && exec /bin/sh -c id\n#"),
		"a node.exe branch added":              replace(pnpm11, "if [ -x \"$basedir/node\" ]; then", "if [ -x \"$basedir/node.exe\" ]; then\n  exec \"$basedir/node.exe\"  \"$basedir/../other/bin/x\" \"$@\"\nelif [ -x \"$basedir/node\" ]; then"),
		"exec lines naming two scripts":        replace(pnpm12, `  exec node  "$basedir_abs"/'../.pnpm/`, `  exec node  "$basedir_abs"/'../.pnpm/other/`),
		"a command substitution in NODE_PATH":  replace(pnpm11, `export NODE_PATH="`, `export NODE_PATH="$(id)`),
		"a script that stays in its directory": strings.ReplaceAll(pnpm11, "$basedir/../oxfmt/bin/oxfmt", "$basedir/oxfmt/bin/oxfmt"),
		"a script with a .. after a name":      strings.ReplaceAll(pnpm11, "$basedir/../oxfmt/bin/oxfmt", "$basedir/../oxfmt/../../x"),
		"one byte changed":                     replace(pnpm12, "Cap hops at", "Cap hops at "),
		"padded past the read limit":           pnpm12 + strings.Repeat("#", formatterShebangBytes),
	} {
		t.Run(name, func(t *testing.T) {
			_, bin := installLauncher(t, body)
			rt, err := formatterRuntimeOf(bin)
			require.Error(t, err)
			assert.Equal(t, formatterRuntime{}, rt)
			assert.Contains(t, err.Error(), bin, "the message names the file")
		})
	}
}

// Each template is the exact text of a fixture, with only the script, the
// NODE_PATH value and the cmd-shim-target comment standing in, and each
// fixture matches exactly one template.
func TestLauncherTemplatesMatchTheirFixtures(t *testing.T) {
	all, err := layouts()
	require.NoError(t, err)
	require.Len(t, all, len(launcherLayouts))
	matched := map[string]bool{}
	for _, fx := range launcherFixtures {
		data := []byte(readLauncherFixture(t, fx.file))
		var hits []string
		for _, l := range all {
			if l.re.Match(data) {
				hits = append(hits, l.file)
			}
		}
		require.Len(t, hits, 1, "%s matches exactly one template", fx.file)
		matched[hits[0]] = true
	}
	assert.Len(t, matched, len(launcherLayouts), "every template has a fixture")
}

// An executable kapi cannot read is never asked about, so it never runs.
func TestFormatterTrustRefusesAnUnreadableLauncher(t *testing.T) {
	_, bin := installLauncher(t, readLauncherFixture(t, "unknown-oxfmt"))
	config := filepath.Join(filepath.Dir(bin), ".oxfmtrc.json")
	require.NoError(t, os.WriteFile(config, []byte("{}"), 0o644))
	trust := &formatterTrust{hashed: map[string]string{}}
	_, err := trust.site(config, "oxfmt", []string{bin}, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "read what the oxfmt executable runs before asking whether it may run")
}

// A shell script past the read limit is refused; a script for another
// interpreter is read by its first line.
func TestFormatterRuntimeOfReadLimit(t *testing.T) {
	dir := t.TempDir()
	sh := filepath.Join(dir, "fmt")
	require.NoError(t, os.WriteFile(sh, []byte("#!/bin/sh\n"+strings.Repeat("#", formatterShebangBytes)), 0o755))
	_, err := formatterRuntimeOf(sh)
	require.Error(t, err)

	js := filepath.Join(dir, "fmt.js")
	require.NoError(t, os.WriteFile(js, []byte("#!/opt/node/bin/node\n"+strings.Repeat("/", formatterShebangBytes)), 0o755))
	rt, err := formatterRuntimeOf(js)
	require.NoError(t, err)
	assert.Equal(t, formatterRuntime{interpreter: "/opt/node/bin/node"}, rt)
}
