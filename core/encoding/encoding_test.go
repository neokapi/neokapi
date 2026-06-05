package encoding

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewEncoderManager(t *testing.T) {
	t.Parallel()
	em := NewEncoderManager()
	names := em.Names()
	assert.NotEmpty(t, names)

	// Verify key encodings are registered.
	for _, name := range []string{"utf-8", "iso-8859-1", "windows-1252", "shift_jis"} {
		enc, err := em.Get(name)
		require.NoError(t, err, "encoding %s not found", name)
		assert.NotNil(t, enc)
	}
}

func TestEncoderManager_GetUnknown(t *testing.T) {
	t.Parallel()
	em := NewEncoderManager()
	_, err := em.Get("nonexistent-encoding")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported encoding")
}

func TestEncoderManager_DecodeEncode_Latin1(t *testing.T) {
	t.Parallel()
	em := NewEncoderManager()

	// "café" in ISO-8859-1: 0x63 0x61 0x66 0xe9
	latin1Bytes := []byte{0x63, 0x61, 0x66, 0xe9}
	decoded, err := em.Decode(latin1Bytes, "iso-8859-1")
	require.NoError(t, err)
	assert.Equal(t, "café", decoded)

	// Round-trip.
	encoded, err := em.Encode(decoded, "iso-8859-1")
	require.NoError(t, err)
	assert.Equal(t, latin1Bytes, encoded)
}

func TestEncoderManager_DecodeEncode_UTF8Passthrough(t *testing.T) {
	t.Parallel()
	em := NewEncoderManager()

	input := []byte("Hello, world!")
	decoded, err := em.Decode(input, "utf-8")
	require.NoError(t, err)
	assert.Equal(t, "Hello, world!", decoded)

	// Empty encoding name also passes through.
	decoded2, err := em.Decode(input, "")
	require.NoError(t, err)
	assert.Equal(t, "Hello, world!", decoded2)
}

func TestEncoderManager_DecodeEncode_Windows1252(t *testing.T) {
	t.Parallel()
	em := NewEncoderManager()

	// "smart quotes" in Windows-1252: 0x93 = left double, 0x94 = right double
	win1252Bytes := []byte{0x93, 0x48, 0x69, 0x94}
	decoded, err := em.Decode(win1252Bytes, "windows-1252")
	require.NoError(t, err)
	assert.Equal(t, "\u201cHi\u201d", decoded)

	encoded, err := em.Encode(decoded, "windows-1252")
	require.NoError(t, err)
	assert.Equal(t, win1252Bytes, encoded)
}

func TestEncoderManager_Register(t *testing.T) {
	em := NewEncoderManager()
	utf8, _ := em.Get("utf-8")

	em.Register("my-custom", utf8)
	enc, err := em.Get("my-custom")
	require.NoError(t, err)
	assert.NotNil(t, enc)
}

func TestEncoderManager_NormalizeName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"UTF-8", "utf-8"},
		{"ISO_8859_1", "iso-8859-1"},
		{" Shift_JIS ", "shift-jis"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			assert.Equal(t, tt.expected, normalizeEncodingName(tt.input))
		})
	}
}

func TestEncoderManager_GB2312_DecodesEUCCN(t *testing.T) {
	em := NewEncoderManager()

	// "中文" in EUC-CN / GB2312 double-byte encoding:
	//   中 = 0xD6 0xD0, 文 = 0xCE 0xC4
	// Files labelled charset=gb2312 (e.g. via the PO Content-Type header)
	// carry this double-byte data, not the 7-bit HZ escape transport. The
	// "gb2312" alias must therefore decode it correctly (regression: it
	// previously mapped to HZGB2312 and produced U+FFFD).
	euccn := []byte{0xD6, 0xD0, 0xCE, 0xC4}

	for _, name := range []string{"gb2312", "euc-cn", "gbk", "gb18030"} {
		t.Run(name, func(t *testing.T) {
			decoded, err := em.Decode(euccn, name)
			require.NoError(t, err)
			assert.Equal(t, "中文", decoded)
			assert.NotContains(t, decoded, "�", "decoding produced replacement chars")
		})
	}
}

func TestEncoderManager_GB2312_RoundTrip(t *testing.T) {
	em := NewEncoderManager()

	euccn := []byte{0xD6, 0xD0, 0xCE, 0xC4}
	decoded, err := em.Decode(euccn, "gb2312")
	require.NoError(t, err)
	assert.Equal(t, "中文", decoded)

	encoded, err := em.Encode(decoded, "gb2312")
	require.NoError(t, err)
	assert.Equal(t, euccn, encoded)
}

func TestEncoderManager_HZGB2312_StillReachable(t *testing.T) {
	em := NewEncoderManager()

	// HZ-GB-2312 remains available under its own real name and is distinct
	// from the "gb2312" alias (which now means GBK/EUC-CN).
	for _, name := range []string{"hz-gb-2312", "hz", "hz_gb_2312"} {
		t.Run(name, func(t *testing.T) {
			enc, err := em.Get(name)
			require.NoError(t, err)
			assert.NotNil(t, enc)
		})
	}

	// HZ uses a 7-bit escape transport: "~{...~}" delimits GB-coded text.
	// "~{<text>~}" with the same two characters as the EUC-CN test.
	hz := []byte("~{VPND~}")
	decoded, err := em.Decode(hz, "hz-gb-2312")
	require.NoError(t, err)
	assert.Equal(t, "中文", decoded)
}

func TestDetect(t *testing.T) {
	tests := []struct {
		name     string
		data     []byte
		encoding string
		content  []byte
	}{
		{
			name:     "UTF-8 BOM",
			data:     []byte{0xEF, 0xBB, 0xBF, 0x48, 0x69},
			encoding: "utf-8",
			content:  []byte{0x48, 0x69},
		},
		{
			name:     "UTF-16 BE BOM",
			data:     []byte{0xFE, 0xFF, 0x00, 0x48},
			encoding: "utf-16be",
			content:  []byte{0x00, 0x48},
		},
		{
			name:     "UTF-16 LE BOM",
			data:     []byte{0xFF, 0xFE, 0x48, 0x00},
			encoding: "utf-16le",
			content:  []byte{0x48, 0x00},
		},
		{
			name:     "No BOM defaults to UTF-8",
			data:     []byte("Hello"),
			encoding: "utf-8",
			content:  []byte("Hello"),
		},
		{
			name:     "Empty data",
			data:     []byte{},
			encoding: "utf-8",
			content:  []byte{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			enc, content := Detect(tt.data)
			assert.Equal(t, tt.encoding, enc)
			assert.Equal(t, tt.content, content)
		})
	}
}
