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

// The launcher fixtures in testdata/launchers are node_modules/.bin/oxfmt as
// pnpm 11 and pnpm 12 wrote it, verbatim except for the project path, and a
// launcher whose script kapi cannot know without running it.
const (
	pnpm11Target = "oxfmt/bin/oxfmt"
	pnpm12Target = ".pnpm/oxfmt@0.79.0_vite-plus@1.7.0_@types+node@26.6.6_@vitest+ui@5.1.0_esbuild@0.28.2_jiti@2._2ce4b6ad2894019449197584a0f63826/node_modules/oxfmt/bin/oxfmt"
)

// installLauncher copies the named fixture to <root>/node_modules/.bin/oxfmt
// and returns that path, with root resolved to its physical location.
func installLauncher(t *testing.T, fixture string) (root, bin string) {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	data, err := os.ReadFile(filepath.Join("testdata", "launchers", fixture))
	require.NoError(t, err)
	bin = filepath.Join(root, "node_modules", ".bin", "oxfmt")
	require.NoError(t, os.MkdirAll(filepath.Dir(bin), 0o755))
	require.NoError(t, os.WriteFile(bin, data, 0o755))
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

// shellRuns runs the launcher at path and returns the script its node received.
func shellRuns(t *testing.T, path string) string {
	t.Helper()
	out, err := exec.CommandContext(t.Context(), path, "--version").Output()
	require.NoError(t, err)
	return filepath.Clean(string(out))
}

func TestReadLauncherLayouts(t *testing.T) {
	skipShellFixture(t)
	for _, tc := range []struct {
		fixture, target string
	}{
		{"pnpm11-oxfmt", pnpm11Target},
		{"pnpm12-oxfmt", pnpm12Target},
	} {
		t.Run(tc.fixture, func(t *testing.T) {
			root, bin := installLauncher(t, tc.fixture)
			node := echoNode(t, bin)
			rt, err := formatterRuntimeOf(bin)
			require.NoError(t, err)
			want := filepath.Join(root, "node_modules", filepath.FromSlash(tc.target))
			assert.Equal(t, formatterRuntime{interpreter: node, script: want}, rt)
			assert.Equal(t, want, shellRuns(t, bin), "the shell runs the script kapi names")
		})
	}
}

// A pnpm 12 launcher resolves $0 through symbolic links, so a link to it
// runs the script beside the launcher, not beside the link.
func TestReadLauncherPnpm12FollowsLinks(t *testing.T) {
	skipShellFixture(t)
	root, bin := installLauncher(t, "pnpm12-oxfmt")
	node := echoNode(t, bin)
	link := filepath.Join(t.TempDir(), "oxfmt")
	require.NoError(t, os.Symlink(bin, link))
	rt, err := formatterRuntimeOf(link)
	require.NoError(t, err)
	want := filepath.Join(root, "node_modules", filepath.FromSlash(pnpm12Target))
	assert.Equal(t, formatterRuntime{interpreter: node, script: want}, rt)
	assert.Equal(t, want, shellRuns(t, link))
}

func TestReadLauncherRefuses(t *testing.T) {
	pnpm12, err := os.ReadFile(filepath.Join("testdata", "launchers", "pnpm12-oxfmt"))
	require.NoError(t, err)
	unknown, err := os.ReadFile(filepath.Join("testdata", "launchers", "unknown-oxfmt"))
	require.NoError(t, err)
	disagree := strings.Replace(string(pnpm12),
		`exec node  "$basedir_abs"/'../.pnpm/`, `exec node  "$basedir_abs"/'../.pnpm/other/`, 1)
	require.NotEqual(t, string(pnpm12), disagree)

	for name, tc := range map[string]struct {
		body, reason string
	}{
		"a script computed at run time":       {string(unknown), "does not resolve"},
		"exec lines naming two scripts":       {disagree, "name two scripts"},
		"an unknown basedir assignment":       {"#!/bin/sh\nbasedir=$HOME\nexec node \"$basedir/../x/bin/x\" \"$@\"\n", "basedir assignment"},
		"basedir_abs without link resolution": {strings.Replace(string(pnpm12), `while [ -L "$link" ]`, `while false && [ -L "$link" ]`, 1), "symbolic links"},
		"a launcher that runs no node":        {"#!/bin/sh\nbasedir=$(dirname \"$0\")\nexec \"$basedir/../x/bin/x\" \"$@\"\n", "exec line"},
		"no exec line":                        {"#!/bin/sh\nbasedir=$(dirname \"$0\")\n", "no exec line"},
		"an escape in the script":             {"#!/bin/sh\nbasedir=$(dirname \"$0\")\nexec node \"$basedir/x\\\"y\" \"$@\"\n", "inside double quotes"},
	} {
		t.Run(name, func(t *testing.T) {
			bin := filepath.Join(t.TempDir(), "oxfmt")
			require.NoError(t, os.WriteFile(bin, []byte(tc.body), 0o755))
			rt, err := formatterRuntimeOf(bin)
			require.Error(t, err)
			assert.Equal(t, formatterRuntime{}, rt)
			assert.Contains(t, err.Error(), "launcher whose target kapi cannot read")
			assert.Contains(t, err.Error(), tc.reason)
		})
	}
}

// An executable kapi cannot read is never asked about, so it never runs.
func TestFormatterTrustRefusesAnUnreadableLauncher(t *testing.T) {
	_, bin := installLauncher(t, "unknown-oxfmt")
	config := filepath.Join(filepath.Dir(bin), ".oxfmtrc.json")
	require.NoError(t, os.WriteFile(config, []byte("{}"), 0o644))
	trust := &formatterTrust{hashed: map[string]string{}}
	_, err := trust.site(config, "oxfmt", []string{bin}, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "read what the oxfmt executable runs before asking whether it may run")
}
