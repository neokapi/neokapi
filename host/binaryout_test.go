package host

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// withStdoutTerminal makes stdoutIsTerminal report tty for the test, so the
// guard's terminal branch is reachable without one.
func withStdoutTerminal(t *testing.T, tty bool) {
	t.Helper()
	prev := stdoutIsTerminal
	stdoutIsTerminal = func() bool { return tty }
	t.Cleanup(func() { stdoutIsTerminal = prev })
}

func TestBinaryGuardPassesText(t *testing.T) {
	tests := []struct {
		name string
		in   string
	}{
		{"short document", "# Title\n\nA paragraph.\n"},
		{"empty document", ""},
		{"longer than the sniff prefix", strings.Repeat("prose and more prose\n", 1000)},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer
			g := &binaryGuard{w: &out}
			n, err := g.Write([]byte(tc.in))
			require.NoError(t, err)
			assert.Equal(t, len(tc.in), n)
			require.NoError(t, g.Flush())
			assert.Equal(t, tc.in, out.String(), "text must pass through byte-for-byte")
		})
	}
}

func TestBinaryGuardRefusesBinary(t *testing.T) {
	// A .docx is a zip: "PK\x03\x04", NUL bytes close behind.
	docx := append([]byte("PK\x03\x04\x14\x00\x00\x00\x08\x00"), bytes.Repeat([]byte{0x00, 0xff}, 100)...)

	var out bytes.Buffer
	g := &binaryGuard{w: &out}
	_, err := g.Write(docx)
	// Under the sniff length, so the verdict lands at Flush.
	require.NoError(t, err)
	require.ErrorIs(t, g.Flush(), ErrBinaryStdout)
	assert.Empty(t, out.Bytes(), "not one byte of a refused document may reach the terminal")

	// The refusal is sticky: a writer that ignores the error cannot slip the
	// rest of the document past it, and Flush keeps reporting failure.
	_, err = g.Write([]byte("more"))
	require.ErrorIs(t, err, ErrBinaryStdout)
	require.ErrorIs(t, g.Flush(), ErrBinaryStdout)
	assert.Empty(t, out.Bytes())
}

// A document long enough to cross the sniff length is decided mid-stream, and
// everything buffered to that point is released in order.
func TestBinaryGuardDecidesAtSniffLength(t *testing.T) {
	body := strings.Repeat("x", binarySniffLen*2)
	var out bytes.Buffer
	g := &binaryGuard{w: &out}
	for _, chunk := range []string{body[:10], body[10:], "tail"} {
		_, err := g.Write([]byte(chunk))
		require.NoError(t, err)
	}
	require.NoError(t, g.Flush())
	assert.Equal(t, body+"tail", out.String())
}

// A NUL arriving after the sniff prefix is past the guard — the same bound the
// input-side guard accepts, and the same one git uses.
func TestBinaryGuardOnlyInspectsThePrefix(t *testing.T) {
	body := strings.Repeat("x", binarySniffLen+10) + "\x00late"
	var out bytes.Buffer
	g := &binaryGuard{w: &out}
	_, err := g.Write([]byte(body))
	require.NoError(t, err)
	require.NoError(t, g.Flush())
	assert.Equal(t, body, out.String())
}

// UTF-16 text is full of NUL bytes and is still text; the BOM says so.
func TestBinaryGuardAcceptsBOMText(t *testing.T) {
	utf16 := append([]byte{0xFF, 0xFE}, []byte("h\x00e\x00l\x00l\x00o\x00")...)
	var out bytes.Buffer
	g := &binaryGuard{w: &out}
	_, err := g.Write(utf16)
	require.NoError(t, err)
	require.NoError(t, g.Flush())
	assert.Equal(t, utf16, out.Bytes())
}

func TestGuardedStdoutOnlyGuardsATerminal(t *testing.T) {
	t.Run("redirected output is os.Stdout untouched", func(t *testing.T) {
		withStdoutTerminal(t, false)
		a := &App{}
		w, flush := a.guardedStdout()
		assert.Same(t, os.Stdout, w, "off a terminal the byte path must not change")
		assert.NoError(t, flush())
	})

	t.Run("terminal output is guarded", func(t *testing.T) {
		withStdoutTerminal(t, true)
		a := &App{}
		w, _ := a.guardedStdout()
		assert.IsType(t, &binaryGuard{}, w)
	})

	// --encoding UTF-16 declares a text form whose bytes carry NULs; the NUL
	// rule is off the table for it, on this side exactly as on the read side.
	t.Run("a NUL-bearing encoding takes the guard off", func(t *testing.T) {
		withStdoutTerminal(t, true)
		a := &App{Encoding: "UTF-16LE"}
		w, flush := a.guardedStdout()
		assert.Same(t, os.Stdout, w)
		assert.NoError(t, flush())
	})
}
