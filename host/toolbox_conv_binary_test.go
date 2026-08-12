package host

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A same-format conversion round-trips a .docx through the skeleton and writes
// a zip back out, so the terminal rule applies to kconv exactly as to ksed —
// and here the escape hatch is `-o -`, curl's spelling, rather than --force.
// (A cross-format conversion cannot reach this: a skeleton-bound format is
// refused as a conversion target long before any bytes are written.)
func TestConvRefusesBinaryAtTerminal(t *testing.T) {
	withStdoutTerminal(t, true)
	app := newToolboxApp(t)
	if !app.FormatReg.HasWriter("openxml") {
		t.Skip("no openxml writer registered")
	}
	src := copyFixtureDocx(t)

	var runErr error
	var stderr string
	stdout, _ := captureStdout(t, func() error {
		stderr, runErr = captureStderr(t, func() error {
			return app.RunConv(context.Background(), []string{src}, ConvOptions{To: "openxml"})
		})
		return nil
	})

	require.Error(t, runErr)
	assert.Equal(t, ExitUsage, ExitCode(nil, runErr))
	assert.Empty(t, stdout, "not one byte of the document may reach the terminal")
	assert.Contains(t, stderr, "binary output not written to a terminal")
	assert.Contains(t, stderr, "-o")
}

// `-o -` is standard output named explicitly: it writes the bytes, and it does
// not create a file called "-" in the working directory.
func TestConvDashOutputIsStdoutNotAFile(t *testing.T) {
	withStdoutTerminal(t, true)
	app := newToolboxApp(t)
	dir := t.TempDir()
	src := writeToolboxFile(t, dir, "guide.md", "# Title\n\nA paragraph.\n")

	cwd, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(dir))
	t.Cleanup(func() { _ = os.Chdir(cwd) })

	stdout, runErr := captureStdout(t, func() error {
		return app.RunConv(context.Background(), []string{src}, ConvOptions{To: "html", Output: StdoutName})
	})
	require.NoError(t, runErr)
	assert.Contains(t, stdout, "<h1")

	_, statErr := os.Stat("-")
	assert.True(t, os.IsNotExist(statErr), `-o - must not create a file named "-"`)
}

// The opt-in does opt in: `-o -` writes the zip at the terminal.
func TestConvDashOutputWritesBinaryAtTerminal(t *testing.T) {
	withStdoutTerminal(t, true)
	app := newToolboxApp(t)
	if !app.FormatReg.HasWriter("openxml") {
		t.Skip("no openxml writer registered")
	}
	src := copyFixtureDocx(t)

	stdout, runErr := captureStdout(t, func() error {
		return app.RunConv(context.Background(), []string{src}, ConvOptions{To: "openxml", Output: StdoutName})
	})
	require.NoError(t, runErr)
	require.NotEmpty(t, stdout)
	assert.Equal(t, "PK", stdout[:2])
}

// Text conversion at a terminal — the common case — is untouched.
func TestConvTextAtTerminalIsUnaffected(t *testing.T) {
	withStdoutTerminal(t, true)
	app := newToolboxApp(t)
	src := writeToolboxFile(t, t.TempDir(), "guide.md", "# Title\n\nA paragraph.\n")

	stdout, runErr := captureStdout(t, func() error {
		return app.RunConv(context.Background(), []string{src}, ConvOptions{To: "html"})
	})
	require.NoError(t, runErr)
	assert.Contains(t, stdout, "<h1")
}

// `-o -` with several inputs is a stream, not one output file that cannot hold
// them all — so the single-file rule must not fire on it.
func TestConvDashOutputAcceptsSeveralInputs(t *testing.T) {
	withStdoutTerminal(t, false)
	app := newToolboxApp(t)
	dir := t.TempDir()
	a := writeToolboxFile(t, dir, "one.md", "# One\n")
	b := writeToolboxFile(t, dir, "two.md", "# Two\n")

	stdout, runErr := captureStdout(t, func() error {
		return app.RunConv(context.Background(), []string{a, b}, ConvOptions{To: "html", Output: StdoutName})
	})
	require.NoError(t, runErr)
	assert.Contains(t, stdout, "One")
	assert.Contains(t, stdout, "Two")
}
