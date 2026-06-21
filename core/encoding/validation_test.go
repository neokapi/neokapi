package encoding

import (
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDiagnose_Off confirms the encoding scan is default-off: no diagnostics for
// any input when the mode is ValidationOff.
func TestDiagnose_Off(t *testing.T) {
	t.Parallel()
	badUTF8 := []byte{'h', 'i', 0xff, '!'}
	utf16 := []byte{0xff, 0xfe, 'h', 0, 'i', 0}

	assert.Nil(t, Diagnose(badUTF8, "UTF-8", format.ValidationOff))
	assert.Nil(t, Diagnose(utf16, "UTF-8", format.ValidationOff))
}

// TestDiagnose_InvalidUTF8 asserts an invalid UTF-8 byte sequence in a file
// declared UTF-8 yields a located encoding.invalid-utf8 diagnostic (Major).
func TestDiagnose_InvalidUTF8(t *testing.T) {
	t.Parallel()
	// "hi" then a lone 0xFF (not valid UTF-8) at offset 2.
	raw := []byte{'h', 'i', 0xff, '!'}

	diags := Diagnose(raw, "UTF-8", format.ValidationReport)
	require.Len(t, diags, 1, "expected one invalid-utf8 diagnostic: %+v", diags)
	d := diags[0]
	assert.Equal(t, "encoding.invalid-utf8", d.Category)
	assert.Equal(t, format.SeverityMajor, d.Severity)
	assert.Equal(t, 2, d.ByteOffset, "offset of the first bad byte")
	assert.Equal(t, 1, d.Line)
	assert.Equal(t, 3, d.Column, "1-based column of the bad byte")
}

// TestDiagnose_CharsetMismatch asserts a UTF-16 BOM under a declared UTF-8
// charset yields a Minor encoding.charset-mismatch and does NOT additionally
// report invalid UTF-8 (the mismatch already accounts for the non-UTF-8 bytes).
func TestDiagnose_CharsetMismatch(t *testing.T) {
	t.Parallel()
	// UTF-16LE BOM + "hi".
	raw := []byte{0xff, 0xfe, 'h', 0, 'i', 0}

	diags := Diagnose(raw, "UTF-8", format.ValidationReport)
	require.Len(t, diags, 1, "expected only a charset-mismatch diagnostic: %+v", diags)
	d := diags[0]
	assert.Equal(t, "encoding.charset-mismatch", d.Category)
	assert.Equal(t, format.SeverityMinor, d.Severity)
}

// TestDiagnose_CleanUTF8 confirms valid UTF-8 declared UTF-8 records nothing.
func TestDiagnose_CleanUTF8(t *testing.T) {
	t.Parallel()
	assert.Empty(t, Diagnose([]byte("héllo wörld"), "UTF-8", format.ValidationReport))
	// A UTF-8 BOM is valid and matches the declared charset.
	withBOM := append([]byte{0xEF, 0xBB, 0xBF}, []byte("clean")...)
	assert.Empty(t, Diagnose(withBOM, "UTF-8", format.ValidationReport))
	// An empty declared charset is treated as UTF-8.
	assert.Empty(t, Diagnose([]byte("plain"), "", format.ValidationReport))
}
