//go:build !js

package pluginhost

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/plugin/manifest"
)

// realDir is dir as the plugin resolves it.
func realDir(t *testing.T, dir string) string {
	t.Helper()
	real, err := filepath.EvalSymlinks(dir)
	require.NoError(t, err)
	return real
}

// rootsProject is a kapi project with one source file in a subdirectory.
func rootsProject(t *testing.T) (root, file string) {
	t.Helper()
	root = t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "kapi.yaml"), []byte("version: v1\nname: p\n"), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "docs", "deep"), 0o755))
	file = filepath.Join(root, "docs", "deep", "a.docx")
	require.NoError(t, os.WriteFile(file, []byte("x"), 0o644))
	return root, file
}

// A path in a project is allowed under the project's root; a file a person
// named in no project, under its own directory.
func TestPathRootsFor(t *testing.T) {
	root, file := rootsProject(t)
	assert.Equal(t, []string{realDir(t, root)}, PathRootsFor(file, filepath.Join(root, "b.docx")),
		"two files of one project share its root")

	loose := filepath.Join(t.TempDir(), "named.docx")
	require.NoError(t, os.WriteFile(loose, []byte("x"), 0o644))
	assert.Equal(t, []string{realDir(t, filepath.Dir(loose))}, PathRootsFor(loose))
	assert.Empty(t, PathRootsFor(""), "a call that sends content inline declares none")
}

// Every daemon the pool starts receives the roots of the paths its call
// sends in KAPI_PLUGIN_ROOTS, a daemon is reused only for the same roots, and
// a call that sends no path reuses any daemon of the plugin.
func TestDaemonPoolDeclaresPluginRoots(t *testing.T) {
	bin := buildFakeDaemon(t)
	plugin := makePlugin(t, "fake", bin, &manifest.DaemonConfig{StartupTimeoutSeconds: 5, IdleTimeoutSeconds: 300})
	log := filepath.Join(t.TempDir(), "roots.log")
	t.Setenv("FAKE_DAEMON_ROOTS_LOG", log)
	pool := NewDaemonPool(DaemonPoolOptions{MaxDaemons: 4})
	t.Cleanup(pool.Shutdown)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	alpha, alphaFile := rootsProject(t)
	beta, betaFile := rootsProject(t)
	named := filepath.Join(t.TempDir(), "named.docx")
	require.NoError(t, os.WriteFile(named, []byte("x"), 0o644))

	// A project run.
	a1, err := pool.Acquire(WithPathRoots(ctx, PathRootsFor(alphaFile)), plugin)
	require.NoError(t, err)
	// An explicit input in no project, written with -o to a directory
	// elsewhere: the daemon is allowed both directories.
	outDir := t.TempDir()
	n1, err := pool.Acquire(WithPathRoots(ctx, PathRootsFor(named, filepath.Join(outDir, "out.docx"))), plugin)
	require.NoError(t, err)
	// A second project in the same long-lived host.
	b1, err := pool.Acquire(WithPathRoots(ctx, PathRootsFor(betaFile)), plugin)
	require.NoError(t, err)
	// The first project again, and a call with content only.
	a2, err := pool.Acquire(WithPathRoots(ctx, PathRootsFor(alphaFile)), plugin)
	require.NoError(t, err)
	inline, err := pool.Acquire(ctx, plugin)
	require.NoError(t, err)

	assert.Same(t, a1, a2, "the same roots reuse the daemon")
	assert.NotEqual(t, a1.PID(), n1.PID(), "other roots start another daemon")
	assert.NotEqual(t, a1.PID(), b1.PID())
	assert.Contains(t, []int{a1.PID(), n1.PID(), b1.PID()}, inline.PID(), "a call with no path reuses a running daemon")

	body, err := os.ReadFile(log)
	require.NoError(t, err)
	got := strings.Split(strings.TrimSpace(string(body)), "\n")
	inputAndOutput := PathRootsFor(named, filepath.Join(outDir, "out.docx"))
	assert.ElementsMatch(t, []string{realDir(t, filepath.Dir(named)), realDir(t, outDir)}, inputAndOutput)
	assert.ElementsMatch(t, []string{
		realDir(t, alpha),
		strings.Join(inputAndOutput, string(os.PathListSeparator)),
		realDir(t, beta),
	}, got,
		"each daemon received exactly the roots of its call")
}
