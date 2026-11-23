package host

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/host/pluginhost"
)

// An -o value declares the directory it names for the run's outputs.
func TestOutputRoot(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"", ""},
		{"-", ""},
		{"out/", "out"},
		{"out", "out"},
		{"out/x.docx", "out"},
		{"x.docx", "."},
		{"out/{lang}/", "out"},
		{"build/l10n/x_{lang}.docx", "build/l10n"},
		{"{dir}/{name}.{lang}.docx", ""},
	} {
		assert.Equal(t, tc.want, outputRoot(tc.in), "outputRoot(%q)", tc.in)
	}
}

// Named inputs declare the directory each names: a file's directory, a
// directory itself, a glob's directory before its first wildcard, an archive
// locator's archive, and nothing for standard input.
func TestNamedInputRoots(t *testing.T) {
	dir := t.TempDir()
	real, err := filepath.EvalSymlinks(dir)
	require.NoError(t, err)
	sub := filepath.Join(dir, "sub")
	require.NoError(t, os.MkdirAll(sub, 0o755))
	file := filepath.Join(dir, "a.txt")
	require.NoError(t, os.WriteFile(file, []byte("x"), 0o644))

	assert.Equal(t, []string{real}, namedInputRoots([]string{file, StdinName}))
	assert.Equal(t, []string{filepath.Join(real, "sub")}, namedInputRoots([]string{sub}))
	assert.Equal(t, []string{filepath.Join(real, "sub")}, namedInputRoots([]string{filepath.Join(sub, "**", "*.md")}))
	assert.Empty(t, namedInputRoots([]string{StdinName}))
}

// A run declares its roots on the command's context for the rest of the
// command, adding to any declared before.
func TestDeclarePathRoots(t *testing.T) {
	a, b := t.TempDir(), t.TempDir()
	cmd := NewEnvCommand(context.Background(), "run")
	declarePathRoots(cmd, a)
	ctx := declarePathRoots(cmd, b, "")
	ra, err := filepath.EvalSymlinks(a)
	require.NoError(t, err)
	rb, err := filepath.EvalSymlinks(b)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{ra, rb}, pluginhost.PathRootsFrom(cmd.Context()))
	assert.Equal(t, pluginhost.PathRootsFrom(ctx), pluginhost.PathRootsFrom(cmd.Context()))
}
