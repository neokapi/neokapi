package host

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSedRecursiveEditsTree: `ksed -R -i` walks a directory argument and edits
// every document beneath it in place, leaving files it cannot claim alone.
func TestSedRecursiveEditsTree(t *testing.T) {
	app := newToolboxApp(t)
	prog, err := ParseSedProgram([]string{"s/colour/color/g"})
	require.NoError(t, err)
	tool := NewSedTool(prog, "", true)

	dir := t.TempDir()
	docs := filepath.Join(dir, "docs")
	require.NoError(t, os.MkdirAll(filepath.Join(docs, "guide"), 0o755))
	top := writeToolboxFile(t, docs, "index.md", "The colour is bold.\n")
	nested := writeToolboxFile(t, filepath.Join(docs, "guide"), "intro.md", "Another colour here.\n")

	require.NoError(t, app.RunSed(context.Background(), []string{docs}, tool,
		SedOptions{InPlace: true, Recursive: true}))

	for _, p := range []string{top, nested} {
		got, rerr := os.ReadFile(p)
		require.NoError(t, rerr)
		assert.Contains(t, string(got), "color", "%s should have been edited", p)
		assert.NotContains(t, string(got), "colour")
	}
}

// TestSedWithoutRecursiveSkipsDirectory keeps the POSIX contract: without -R a
// directory argument is reported and skipped, not walked, and the run exits 2 —
// the same rule kgrep follows.
func TestSedWithoutRecursiveSkipsDirectory(t *testing.T) {
	app := newToolboxApp(t)
	prog, err := ParseSedProgram([]string{"s/colour/color/g"})
	require.NoError(t, err)
	tool := NewSedTool(prog, "", true)

	dir := t.TempDir()
	docs := filepath.Join(dir, "docs")
	require.NoError(t, os.Mkdir(docs, 0o755))
	untouched := writeToolboxFile(t, docs, "index.md", "The colour is bold.\n")

	stderr, err := captureStderr(t, func() error {
		return app.RunSed(context.Background(), []string{docs}, tool, SedOptions{InPlace: true})
	})
	require.Error(t, err)
	assert.Equal(t, ExitUsage, ExitCode(nil, err))
	assert.Contains(t, stderr, "is a directory")

	got, rerr := os.ReadFile(untouched)
	require.NoError(t, rerr)
	assert.Equal(t, "The colour is bold.\n", string(got), "nothing is edited without -R")
}

// TestSedRecursiveSkipsBinaryFile: a walked tree is exactly where an unreadable
// blob turns up. It is reported and left alone rather than rewritten as text,
// and the documents around it are still edited.
func TestSedRecursiveSkipsBinaryFile(t *testing.T) {
	app := newToolboxApp(t)
	prog, err := ParseSedProgram([]string{"s/colour/color/g"})
	require.NoError(t, err)
	tool := NewSedTool(prog, "", true)

	dir := t.TempDir()
	docs := filepath.Join(dir, "docs")
	require.NoError(t, os.Mkdir(docs, 0o755))
	doc := writeToolboxFile(t, docs, "index.md", "The colour is bold.\n")
	blob := filepath.Join(docs, "installer.dmg")
	require.NoError(t, os.WriteFile(blob, fakeDMG, 0o644))

	stderr, err := captureStderr(t, func() error {
		return app.RunSed(context.Background(), []string{docs}, tool,
			SedOptions{InPlace: true, Recursive: true})
	})
	require.Error(t, err)
	assert.Contains(t, stderr, "binary file")

	edited, rerr := os.ReadFile(doc)
	require.NoError(t, rerr)
	assert.Contains(t, string(edited), "color", "the good document is still edited")

	after, rerr := os.ReadFile(blob)
	require.NoError(t, rerr)
	assert.Equal(t, fakeDMG, after, "the binary file is byte-identical — never rewritten")
}
