package encoding

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewEncoderManager(t *testing.T) {
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
	em := NewEncoderManager()
	_, err := em.Get("nonexistent-encoding")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported encoding")
}

func TestEncoderManager_DecodeEncode_Latin1(t *testing.T) {
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
