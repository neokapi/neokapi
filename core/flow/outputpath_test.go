package flow_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/neokapi/neokapi/core/flow"
	"github.com/neokapi/neokapi/core/formats"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/neokapi/neokapi/core/tool"
	"github.com/neokapi/neokapi/core/tools"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// #1449: a destination that cannot be written must refuse the run with a reason
// a person can act on — never a swallowed error, and never a bare errno.

func TestCheckOutputPath(t *testing.T) {
	dir := t.TempDir()

	regularFile := filepath.Join(dir, "existing.json")
	require.NoError(t, os.WriteFile(regularFile, []byte("{}"), 0o644))

	blockingDir := filepath.Join(dir, "fr-FR.json")
	require.NoError(t, os.Mkdir(blockingDir, 0o755))

	fileAsParent := filepath.Join(dir, "notadir")
	require.NoError(t, os.WriteFile(fileAsParent, []byte("x"), 0o644))

	tests := []struct {
		name string
		path string
		kind flow.OutputPathErrorKind
		ok   bool
	}{
		{name: "empty path is not a destination", path: "", ok: true},
		{name: "free path", path: filepath.Join(dir, "new.json"), ok: true},
		{name: "free path under a directory to create", path: filepath.Join(dir, "a", "b", "new.json"), ok: true},
		{name: "existing regular file is replaceable", path: regularFile, ok: true},
		{name: "directory occupies the path", path: blockingDir, kind: flow.OutputPathIsDir},
		{name: "an ancestor is a file", path: filepath.Join(fileAsParent, "out.json"), kind: flow.OutputPathParentNotDir},
		{name: "a deeper ancestor is a file", path: filepath.Join(fileAsParent, "x", "y", "out.json"), kind: flow.OutputPathParentNotDir},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := flow.CheckOutputPath(tc.path)
			if tc.ok {
				assert.NoError(t, err)
				return
			}
			var ope *flow.OutputPathError
			require.ErrorAs(t, err, &ope, "the fault must be typed so a UI can classify it")
			assert.Equal(t, tc.kind, ope.Kind)
			assert.Equal(t, tc.path, ope.Path)
			assert.Contains(t, err.Error(), tc.path, "the message must name the path")
		})
	}
}

// A character device is not a regular file: renaming output over it would
// replace the device node, so it is refused like any other obstruction.
func TestCheckOutputPath_IrregularFile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("no device-node paths on Windows")
	}
	err := flow.CheckOutputPath("/dev/null")
	var ope *flow.OutputPathError
	require.ErrorAs(t, err, &ope)
	assert.Equal(t, flow.OutputPathIrregular, ope.Kind)
	assert.Contains(t, err.Error(), "not a regular file")
}

func TestCheckOutputPath_PermissionDeniedDirectory(t *testing.T) {
	requireNonRoot(t)
	dir := t.TempDir()
	locked := filepath.Join(dir, "locked")
	require.NoError(t, os.Mkdir(locked, 0o755))
	// A path *inside* an unlistable directory cannot even be stat'd.
	require.NoError(t, os.Chmod(locked, 0o000))
	t.Cleanup(func() { _ = os.Chmod(locked, 0o755) })

	err := flow.CheckOutputPath(filepath.Join(locked, "sub", "out.json"))
	var ope *flow.OutputPathError
	require.ErrorAs(t, err, &ope)
	assert.Equal(t, flow.OutputPathPermission, ope.Kind)
	assert.Contains(t, err.Error(), "permission denied")
}

func TestClassifyOutputPathError(t *testing.T) {
	dir := t.TempDir()

	t.Run("nil stays nil", func(t *testing.T) {
		assert.NoError(t, flow.ClassifyOutputPathError(filepath.Join(dir, "x"), dir, nil))
	})

	t.Run("a rename onto a directory reports the directory, not the errno", func(t *testing.T) {
		// os.Rename over an existing directory reports EEXIST on macOS,
		// ENOTEMPTY/EISDIR elsewhere — none of which name the obstruction.
		blocked := filepath.Join(dir, "blocked.json")
		require.NoError(t, os.Mkdir(blocked, 0o755))
		src := filepath.Join(dir, "tmp-out")
		require.NoError(t, os.WriteFile(src, []byte("x"), 0o644))
		rerr := os.Rename(src, blocked)
		require.Error(t, rerr, "renaming onto a directory must fail for the test to mean anything")

		err := flow.ClassifyOutputPathError(blocked, dir, rerr)
		var ope *flow.OutputPathError
		require.ErrorAs(t, err, &ope)
		assert.Equal(t, flow.OutputPathIsDir, ope.Kind)
		assert.ErrorIs(t, err, rerr, "the underlying error stays reachable")
	})

	t.Run("an already-classified error passes through", func(t *testing.T) {
		orig := &flow.OutputPathError{Kind: flow.OutputPathReadOnly, Path: "/x"}
		assert.Same(t, orig, flow.ClassifyOutputPathError("/x", "/", orig))
	})
}

// TestRunFile_BlockedOutputPath is the fixture #1442 could not build: a run
// whose destination is occupied by a directory must FAIL — with the path and the
// remedy named — and must leave no partial output behind.
func TestRunFile_BlockedOutputPath(t *testing.T) {
	dir, inputPath, runner, pseudo := newBlockedPathRunner(t)

	outputPath := filepath.Join(dir, "out", "qps.json")
	require.NoError(t, os.MkdirAll(outputPath, 0o755), "a directory takes the writer's path")

	err := runner.RunFile(context.Background(), "pseudo-translate", []tool.Tool{pseudo}, inputPath, outputPath, "qps")

	var ope *flow.OutputPathError
	require.ErrorAs(t, err, &ope, "a blocked destination must fail the run, not be swallowed")
	assert.Equal(t, flow.OutputPathIsDir, ope.Kind)
	assert.Contains(t, err.Error(), outputPath)
	assert.Contains(t, err.Error(), "a directory occupies that path")

	// The pre-flight refuses before the pipeline runs, so nothing is staged.
	entries, rerr := os.ReadDir(outputPath)
	require.NoError(t, rerr)
	assert.Empty(t, entries, "no temp output may be left inside the blocking directory")
}

// A destination whose parent is a FILE cannot have its output directory created.
func TestRunFile_OutputParentIsAFile(t *testing.T) {
	dir, inputPath, runner, pseudo := newBlockedPathRunner(t)

	parent := filepath.Join(dir, "out")
	require.NoError(t, os.WriteFile(parent, []byte("not a directory"), 0o644))
	outputPath := filepath.Join(parent, "qps.json")

	err := runner.RunFile(context.Background(), "pseudo-translate", []tool.Tool{pseudo}, inputPath, outputPath, "qps")

	var ope *flow.OutputPathError
	require.ErrorAs(t, err, &ope)
	assert.Equal(t, flow.OutputPathParentNotDir, ope.Kind)
	assert.Contains(t, err.Error(), "is not a directory")
	assert.Contains(t, err.Error(), parent, "the message must name the blocking ancestor")
}

// A read-only output directory reports permission denied and names the
// directory to fix — not "set output: open …: permission denied".
func TestRunFile_OutputDirectoryNotWritable(t *testing.T) {
	requireNonRoot(t)
	dir, inputPath, runner, pseudo := newBlockedPathRunner(t)

	outDir := filepath.Join(dir, "out")
	require.NoError(t, os.Mkdir(outDir, 0o555))
	t.Cleanup(func() { _ = os.Chmod(outDir, 0o755) })
	outputPath := filepath.Join(outDir, "qps.json")

	err := runner.RunFile(context.Background(), "pseudo-translate", []tool.Tool{pseudo}, inputPath, outputPath, "qps")

	var ope *flow.OutputPathError
	require.ErrorAs(t, err, &ope, "an unwritable output directory must fail the run")
	assert.Equal(t, flow.OutputPathPermission, ope.Kind)
	assert.Contains(t, err.Error(), outDir, "the message must name the directory to fix")
	assert.NoFileExists(t, outputPath)
}

// The ordinary write path is untouched: a free destination still writes.
func TestRunFile_FreeOutputPathStillWrites(t *testing.T) {
	dir, inputPath, runner, pseudo := newBlockedPathRunner(t)
	outputPath := filepath.Join(dir, "out", "qps.json")
	require.NoError(t, runner.RunFile(context.Background(), "pseudo-translate",
		[]tool.Tool{pseudo}, inputPath, outputPath, "qps"))
	assert.FileExists(t, outputPath)
}

// newBlockedPathRunner builds the shared fixture for the output-path tests: a
// one-key JSON source and a pseudo-translate runner over the real registry.
func newBlockedPathRunner(t *testing.T) (dir, inputPath string, runner *flow.FileRunner, pseudo tool.Tool) {
	t.Helper()
	reg := registry.NewFormatRegistry()
	formats.RegisterAll(reg)

	dir = t.TempDir()
	inputPath = filepath.Join(dir, "input.json")
	require.NoError(t, os.WriteFile(inputPath, []byte(`{"greeting":"Hello World"}`), 0o644))

	p, err := tools.NewPseudoTranslateFromConfig(map[string]any{"target_locale": "qps"}, "qps")
	require.NoError(t, err)

	return dir, inputPath, flow.NewFileRunner(flow.FileRunnerConfig{
		FormatReg:    reg,
		SourceLocale: "en-US",
	}), p
}

// requireNonRoot skips a permission test running as root, where the mode bits
// are advisory and the assertion would pass vacuously (root writes anyway, so
// the test would silently stop testing anything in a root CI container).
func requireNonRoot(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("POSIX mode bits do not gate writes on Windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("running as root: mode bits do not deny writes, so this test cannot fail")
	}
	// Prove the mode bits actually bite in this sandbox before relying on them.
	probe := filepath.Join(t.TempDir(), "probe")
	require.NoError(t, os.Mkdir(probe, 0o555))
	t.Cleanup(func() { _ = os.Chmod(probe, 0o755) })
	if f, err := os.Create(filepath.Join(probe, "x")); err == nil {
		_ = f.Close()
		t.Skip("this filesystem ignores directory mode bits")
	} else if !errors.Is(err, os.ErrPermission) {
		t.Skipf("unexpected probe error, cannot rely on mode bits: %v", err)
	}
}
