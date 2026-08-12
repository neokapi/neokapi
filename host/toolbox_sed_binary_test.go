package host

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// copyFixtureDocx puts a real .docx in a temp dir, so an edit has somewhere to
// go that is not the fixture itself.
func copyFixtureDocx(t *testing.T) string {
	t.Helper()
	src, err := os.ReadFile(filepath.Join("..", "core", "formats", "openxml", "testdata", "simple.docx"))
	if err != nil {
		t.Skip("no openxml .docx fixture available")
	}
	dst := filepath.Join(t.TempDir(), "report.docx")
	require.NoError(t, os.WriteFile(dst, src, 0o644))
	return dst
}

// TestSedRefusesBinaryAtTerminal: the whole point. A .docx edit reconstructs a
// zip, and streaming that at a terminal is what the guard exists to stop.
func TestSedRefusesBinaryAtTerminal(t *testing.T) {
	withStdoutTerminal(t, true)
	app := newToolboxApp(t)
	prog, err := ParseSedProgram([]string{"s/colour/color/g"})
	require.NoError(t, err)
	tool := NewSedTool(prog, "", true)
	doc := copyFixtureDocx(t)

	var runErr error
	var stderr string
	stdout, _ := captureStdout(t, func() error {
		stderr, runErr = captureStderr(t, func() error {
			return app.RunSed(context.Background(), []string{doc}, tool, SedOptions{})
		})
		return nil
	})

	require.Error(t, runErr)
	assert.Equal(t, ExitUsage, ExitCode(nil, runErr), "a refusal is trouble (2), like the rest of the toolbox")
	assert.Empty(t, stdout, "not one byte of the document may reach the terminal")
	assert.Contains(t, stderr, "binary output not written to a terminal")
	assert.Contains(t, stderr, "-i", "the message must name the way out")
	assert.Contains(t, stderr, "--force")

	// The input is untouched: a refused write is not a half-done edit.
	after, rerr := os.ReadFile(doc)
	require.NoError(t, rerr)
	assert.Equal(t, "PK", string(after[:2]))
}

// The guard is a terminal rule, not a format rule: redirected output is exactly
// the document it always was, which is what keeps `ksed … > out.docx` working.
func TestSedWritesBinaryWhenRedirected(t *testing.T) {
	withStdoutTerminal(t, false)
	app := newToolboxApp(t)
	prog, err := ParseSedProgram([]string{"s/colour/color/g"})
	require.NoError(t, err)
	tool := NewSedTool(prog, "", true)
	doc := copyFixtureDocx(t)

	stdout, runErr := captureStdout(t, func() error {
		return app.RunSed(context.Background(), []string{doc}, tool, SedOptions{})
	})
	require.NoError(t, runErr)
	require.NotEmpty(t, stdout)
	assert.Equal(t, "PK", stdout[:2], "the reconstructed .docx must stream unchanged")
}

// --force is the gzip escape hatch: the user asked for the bytes, they get them.
func TestSedForceWritesBinaryAtTerminal(t *testing.T) {
	withStdoutTerminal(t, true)
	app := newToolboxApp(t)
	prog, err := ParseSedProgram([]string{"s/colour/color/g"})
	require.NoError(t, err)
	tool := NewSedTool(prog, "", true)
	doc := copyFixtureDocx(t)

	stdout, runErr := captureStdout(t, func() error {
		return app.RunSed(context.Background(), []string{doc}, tool, SedOptions{Force: true})
	})
	require.NoError(t, runErr)
	require.NotEmpty(t, stdout)
	assert.Equal(t, "PK", stdout[:2])
}

// Text at a terminal is the ordinary case and must be untouched by any of this.
func TestSedTextAtTerminalIsUnaffected(t *testing.T) {
	withStdoutTerminal(t, true)
	app := newToolboxApp(t)
	prog, err := ParseSedProgram([]string{"s/colour/color/g"})
	require.NoError(t, err)
	tool := NewSedTool(prog, "", true)

	doc := writeToolboxFile(t, t.TempDir(), "guide.md", "The colour is bold.\n")
	stdout, runErr := captureStdout(t, func() error {
		return app.RunSed(context.Background(), []string{doc}, tool, SedOptions{})
	})
	require.NoError(t, runErr)
	assert.Contains(t, stdout, "color")
	assert.NotContains(t, stdout, "colour")
}

// In-place editing never touches stdout, so the guard must not interfere with
// it — including for the binary formats whose round-trip is the reason `-i`
// exists.
func TestSedInPlaceUnaffectedByGuard(t *testing.T) {
	withStdoutTerminal(t, true)
	app := newToolboxApp(t)
	prog, err := ParseSedProgram([]string{"s/colour/color/g"})
	require.NoError(t, err)
	tool := NewSedTool(prog, "", true)
	doc := copyFixtureDocx(t)

	stdout, runErr := captureStdout(t, func() error {
		return app.RunSed(context.Background(), []string{doc}, tool, SedOptions{InPlace: true})
	})
	require.NoError(t, runErr)
	assert.Empty(t, stdout)

	after, rerr := os.ReadFile(doc)
	require.NoError(t, rerr)
	assert.Equal(t, "PK", string(after[:2]), "the document is still a .docx")
}
