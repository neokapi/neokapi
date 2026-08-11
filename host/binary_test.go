package host

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeDMG is the shape of the reported case: a file no format claims, whose
// bytes are binary. The NUL byte is what every real installer, image and
// archive has and no text file does.
var fakeDMG = append([]byte("koly\x00\x00\x00\x04"), bytes.Repeat([]byte{0x00, 0x7F, 0xB2, 0x11}, 64)...)

// TestCatRefusesBinaryFile: `kcat some.dmg` used to fall back to plaintext and
// print the raw bytes at the terminal. It now reports the file and prints
// nothing, while the good files in the same glob still print.
func TestCatRefusesBinaryFile(t *testing.T) {
	app := newToolboxApp(t)
	dir := t.TempDir()
	blob := filepath.Join(dir, "installer.dmg")
	require.NoError(t, os.WriteFile(blob, fakeDMG, 0o644))
	good := writeToolboxFile(t, dir, "en.json", `{"k":"Hello"}`)

	var out string
	stderr, err := captureStderr(t, func() error {
		var cerr error
		out, cerr = captureStdout(t, func() error {
			return app.RunCat(context.Background(), NewEnvCommand(context.Background(), "test"),
				[]string{blob, good}, CatOptions{})
		})
		return cerr
	})

	require.Error(t, err, "a binary input is trouble, not silence")
	assert.Equal(t, ExitUsage, ExitCode(nil, err))
	assert.Contains(t, stderr, "binary file")
	assert.Contains(t, stderr, "-f FORMAT", "the message names the escape hatch")
	assert.Equal(t, "Hello\n", out, "no raw bytes on stdout; the good file still prints")
}

// TestConvRefusesBinaryFile: the same guard covers kconv, which would otherwise
// write a Markdown file full of the input's bytes.
func TestConvRefusesBinaryFile(t *testing.T) {
	app := newToolboxApp(t)
	dir := t.TempDir()
	blob := filepath.Join(dir, "installer.dmg")
	require.NoError(t, os.WriteFile(blob, fakeDMG, 0o644))
	out := filepath.Join(dir, "converted")

	stderr, err := captureStderr(t, func() error {
		return app.RunConv(context.Background(), []string{blob},
			ConvOptions{To: "markdown", Output: out + string(os.PathSeparator)})
	})
	require.Error(t, err)
	assert.Contains(t, stderr, "binary file")
	assert.NoFileExists(t, filepath.Join(out, "installer.md"), "nothing is written for a refused input")
}

// TestExplicitFormatOverridesBinaryGuard: --format short-circuits detection, so
// a user who insists gets what they asked for. The guard only ever fires on the
// plaintext fallback.
func TestExplicitFormatOverridesBinaryGuard(t *testing.T) {
	app := newToolboxApp(t)
	app.FormatFlag = "plaintext"
	dir := t.TempDir()
	blob := filepath.Join(dir, "installer.dmg")
	require.NoError(t, os.WriteFile(blob, []byte("real text\x00and a NUL"), 0o644))

	name, err := app.resolveFormatFrom(blob, bytes.NewReader([]byte("x\x00y")))
	require.NoError(t, err)
	assert.Equal(t, "plaintext", name)
}

// TestBinaryGuardLeavesDetectedFormatsAlone: a format kapi understands is
// detected before the fallback, so a binary document (a .docx is a ZIP) is
// unaffected by the guard.
func TestBinaryGuardLeavesDetectedFormatsAlone(t *testing.T) {
	fixtures, _ := filepath.Glob("../core/formats/openxml/testdata/*.docx")
	if len(fixtures) == 0 {
		t.Skip("no .docx fixtures available")
	}
	content, err := os.ReadFile(fixtures[0])
	require.NoError(t, err)

	app := newToolboxApp(t)
	name, err := app.ResolveFormatName(StdinName, content)
	require.NoError(t, err, "a detected binary format must not trip the guard")
	assert.Equal(t, "openxml", name)
}

// TestLooksBinary covers the rule itself: git's NUL test, with Unicode
// byte-order marks exempted because UTF-16/32 text is full of NUL bytes and
// says so in its first bytes.
func TestLooksBinary(t *testing.T) {
	tests := []struct {
		name  string
		bytes []byte
		want  bool
	}{
		{name: "ascii prose", bytes: []byte("Hello, world.\n"), want: false},
		{name: "utf-8 prose", bytes: []byte("Håkon skrev — tekst\n"), want: false},
		{name: "empty", bytes: nil, want: false},
		{name: "nul byte", bytes: []byte("head\x00tail"), want: true},
		{name: "utf-16le bom", bytes: []byte{0xFF, 0xFE, 'H', 0x00, 'i', 0x00}, want: false},
		{name: "utf-16be bom", bytes: []byte{0xFE, 0xFF, 0x00, 'H', 0x00, 'i'}, want: false},
		{name: "utf-32be bom", bytes: []byte{0x00, 0x00, 0xFE, 0xFF, 0x00, 0x00, 0x00, 'H'}, want: false},
		{name: "utf-8 bom", bytes: []byte{0xEF, 0xBB, 0xBF, 'H', 'i'}, want: false},
		{name: "utf-16 without a bom", bytes: []byte{'H', 0x00, 'i', 0x00}, want: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, looksBinary(tc.bytes))
		})
	}
}

// TestEncodingOptsOutOfBinaryGuard: BOM-less UTF-16 is unrecognisable from its
// bytes, so --encoding is how a user declares it. That declaration takes the
// NUL rule off the table.
func TestEncodingOptsOutOfBinaryGuard(t *testing.T) {
	app := newToolboxApp(t)
	bomless := []byte{'H', 0x00, 'i', 0x00}

	assert.True(t, app.binaryBytes(bomless), "UTF-8 is the default, so this reads as binary")

	app.Encoding = "UTF-16LE"
	assert.False(t, app.binaryBytes(bomless), "--encoding UTF-16LE declares it text")
}

// TestBinaryGuardRewindsSource: the guard reads a prefix to decide, and the
// reader that follows must still see the whole document from byte zero.
func TestBinaryGuardRewindsSource(t *testing.T) {
	app := newToolboxApp(t)
	dir := t.TempDir()
	path := writeToolboxFile(t, dir, "prose.txt", "first line\nsecond line\n")

	src, err := openDocSource(context.Background(), path)
	require.NoError(t, err)
	defer src.Close()
	sniff, err := src.seeker()
	require.NoError(t, err)

	name, err := app.resolveFormatFrom(path, sniff)
	require.NoError(t, err)
	assert.Equal(t, "plaintext", name)

	rest, err := src.bytes()
	require.NoError(t, err)
	assert.Equal(t, "first line\nsecond line\n", string(rest))
}
