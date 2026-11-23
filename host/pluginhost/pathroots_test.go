//go:build !js

package pluginhost

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
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
	file = filepath.Join(root, "docs", "deep", "a.fakefmt")
	require.NoError(t, os.WriteFile(file, []byte("x"), 0o644))
	return root, file
}

// looseFile is a file in no project.
func looseFile(t *testing.T) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "named.fakefmt")
	require.NoError(t, os.WriteFile(p, []byte("loose"), 0o644))
	return p
}

// A file a person named in a project declares the project's root; one in no
// project, its own directory; a directory, itself.
func TestPathRootsFor(t *testing.T) {
	root, file := rootsProject(t)
	assert.Equal(t, []string{realDir(t, root)}, PathRootsFor(file, filepath.Join(root, "b.docx")),
		"two files of one project share its root")

	loose := looseFile(t)
	assert.Equal(t, []string{realDir(t, filepath.Dir(loose))}, PathRootsFor(loose))
	dir := t.TempDir()
	assert.Equal(t, []string{realDir(t, dir)}, PathRootsFor(dir))
	assert.Empty(t, PathRootsFor(""))
}

// Roots added to a context accumulate, resolved and sorted with no repeats;
// an output directory that does not exist yet is spelled as it will resolve.
func TestAddPathRoots(t *testing.T) {
	a, b := t.TempDir(), t.TempDir()
	ctx := AddPathRoots(context.Background(), b, "")
	ctx = AddPathRoots(ctx, a, b, filepath.Join(a, "out", "fr"))
	want := []string{realDir(t, a), filepath.Join(realDir(t, a), "out", "fr"), realDir(t, b)}
	assert.ElementsMatch(t, want, PathRootsFrom(ctx))
	assert.Empty(t, PathRootsFrom(context.Background()))
}

// processCall mirrors a line of the fake daemon's FAKE_DAEMON_PROCESS_LOG.
type processCall struct {
	Input      string `json:"input"`
	InputPath  string `json:"input_path,omitempty"`
	OutputPath string `json:"output_path,omitempty"`
}

// rootsHarness is one long-lived pool of the fake bridge daemon, with the
// logs it writes the roots it was started with and each Process call to.
type rootsHarness struct {
	pool               *DaemonPool
	plugin             *Plugin
	rootsLog, procsLog string
}

func newRootsHarness(t *testing.T) *rootsHarness {
	t.Helper()
	bin := buildFakeDaemon(t)
	h := &rootsHarness{
		plugin:   makePluginWithBridge(t, "fmt-plugin", bin, "fakefmt", []string{".fakefmt"}),
		rootsLog: filepath.Join(t.TempDir(), "roots.log"),
		procsLog: filepath.Join(t.TempDir(), "process.log"),
	}
	t.Setenv("FAKE_DAEMON_ROOTS_LOG", h.rootsLog)
	t.Setenv("FAKE_DAEMON_PROCESS_LOG", h.procsLog)
	t.Setenv("FAKE_DAEMON_OUTPUT", "written")
	t.Setenv("FAKE_DAEMON_BRIDGE", "1")
	h.pool = NewDaemonPool(DaemonPoolOptions{MaxDaemons: 4, StartupTimeout: 10 * time.Second, ShutdownGrace: 500 * time.Millisecond})
	t.Cleanup(h.pool.Shutdown)
	return h
}

// read runs the plugin's reader over path, opened as the host opens a file,
// under ctx.
func (h *rootsHarness) read(t *testing.T, ctx context.Context, path string) {
	t.Helper()
	f, err := os.Open(path)
	require.NoError(t, err)
	defer f.Close()
	r := newDaemonReader(h.pool, h.plugin, "fakefmt", format.FormatSignature{Extensions: []string{".fakefmt"}}, "Fake")
	require.NoError(t, r.Open(ctx, &model.RawDocument{URI: path, Reader: f, ReaderAt: f, SourceLocale: "en"}))
	for res := range r.Read(ctx) {
		require.NoError(t, res.Error)
	}
}

// write runs the plugin's writer from source to out under ctx.
func (h *rootsHarness) write(t *testing.T, ctx context.Context, source, out string) {
	t.Helper()
	w := newDaemonWriter(h.pool, h.plugin, "fakefmt")
	w.SetSourcePath(source)
	require.NoError(t, w.SetOutput(out))
	w.SetLocale("fr")
	parts := make(chan *model.Part)
	close(parts)
	require.NoError(t, w.Write(ctx, parts))
}

func (h *rootsHarness) roots(t *testing.T) []string {
	t.Helper()
	body, err := os.ReadFile(h.rootsLog)
	require.NoError(t, err)
	return strings.Split(strings.TrimSpace(string(body)), "\n")
}

func (h *rootsHarness) calls(t *testing.T) []processCall {
	t.Helper()
	body, err := os.ReadFile(h.procsLog)
	require.NoError(t, err)
	var calls []processCall
	for line := range strings.SplitSeq(strings.TrimSpace(string(body)), "\n") {
		var c processCall
		require.NoError(t, json.Unmarshal([]byte(line), &c))
		calls = append(calls, c)
	}
	return calls
}

func testCtx(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	t.Cleanup(cancel)
	return ctx
}

// Each daemon receives in KAPI_PLUGIN_ROOTS the roots the run declared on its
// context, whatever path the call sends: the project root for a project run,
// the project root and the named directories for an explicit input written
// with -o elsewhere, and a second project's root, all in one long-lived pool.
// A run declaring the same roots again reuses its daemon.
func TestDaemonReceivesDeclaredRoots(t *testing.T) {
	h := newRootsHarness(t)
	ctx := testCtx(t)
	alpha, alphaFile := rootsProject(t)
	beta, betaFile := rootsProject(t)
	named := looseFile(t)
	outDir := t.TempDir()

	// A project run over alpha.
	h.read(t, WithPathRoots(ctx, []string{alpha}), alphaFile)
	// In alpha, an explicit input in no project, written with -o elsewhere.
	explicit := []string{alpha, filepath.Dir(named), outDir}
	h.write(t, WithPathRoots(ctx, explicit), named, filepath.Join(outDir, "out.fakefmt"))
	// A project run over beta in the same host.
	h.read(t, WithPathRoots(ctx, []string{beta}), betaFile)
	// alpha again.
	h.read(t, WithPathRoots(ctx, []string{alpha}), alphaFile)

	sep := string(os.PathListSeparator)
	assert.ElementsMatch(t, []string{
		realDir(t, alpha),
		strings.Join(PathRootsFrom(WithPathRoots(ctx, explicit)), sep),
		realDir(t, beta),
	}, h.roots(t), "one daemon per declared set of roots, each started with exactly that set")
	assert.ElementsMatch(t, []string{realDir(t, alpha), realDir(t, filepath.Dir(named)), realDir(t, outDir)},
		PathRootsFrom(WithPathRoots(ctx, explicit)))

	realAlphaFile, err := filepath.EvalSymlinks(alphaFile)
	require.NoError(t, err)
	realNamed, err := filepath.EvalSymlinks(named)
	require.NoError(t, err)
	assert.Equal(t, []processCall{
		{Input: "path", InputPath: realAlphaFile},
		{Input: "path", InputPath: realNamed, OutputPath: filepath.Join(realDir(t, outDir), "out.fakefmt")},
		{Input: "path", InputPath: filepath.Join(realDir(t, beta), "docs", "deep", "a.fakefmt")},
		{Input: "path", InputPath: realAlphaFile},
	}, h.calls(t), "a path beneath the declared roots goes as a path")
}

// A path outside the declared roots never reaches the daemon: the source goes
// inline, and the output comes back inline for the host to write. With no
// roots declared (a library caller) nothing goes as a path.
func TestPathOutsideRootsIsSentInline(t *testing.T) {
	h := newRootsHarness(t)
	ctx := testCtx(t)
	alpha, alphaFile := rootsProject(t)
	outside := looseFile(t)
	outDir := t.TempDir()
	out := filepath.Join(outDir, "out.fakefmt")

	// A project run over alpha that reaches a file outside it.
	h.read(t, WithPathRoots(ctx, []string{alpha}), outside)
	// A write from outside the roots to a destination outside them.
	h.write(t, WithPathRoots(ctx, []string{alpha}), outside, out)
	// No roots at all.
	h.read(t, ctx, alphaFile)

	assert.Equal(t, []processCall{{Input: "inline"}, {Input: "inline"}, {Input: "inline"}}, h.calls(t),
		"no ContentRef path and no OutputRef path reached the daemon")
	written, err := os.ReadFile(out)
	require.NoError(t, err)
	assert.Equal(t, "written", string(written), "the host wrote the bytes the daemon returned")
}
