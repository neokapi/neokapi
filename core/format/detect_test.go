package format_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newDetector() *format.FormatDetector {
	d := format.NewFormatDetector()
	d.Register("html", format.FormatSignature{
		MIMETypes:  []string{"text/html", "application/xhtml+xml"},
		Extensions: []string{".html", ".htm", ".xhtml"},
		MagicBytes: [][]byte{[]byte("<!DOCTYPE"), []byte("<html")},
	})
	d.Register("xml", format.FormatSignature{
		MIMETypes:  []string{"text/xml", "application/xml"},
		Extensions: []string{".xml"},
		MagicBytes: [][]byte{[]byte("<?xml")},
	})
	d.Register("json", format.FormatSignature{
		MIMETypes:  []string{"application/json"},
		Extensions: []string{".json"},
		Sniff: func(data []byte) bool {
			trimmed := bytes.TrimSpace(data)
			return len(trimmed) > 0 && (trimmed[0] == '{' || trimmed[0] == '[')
		},
	})
	d.Register("plaintext", format.FormatSignature{
		MIMETypes:  []string{"text/plain"},
		Extensions: []string{".txt"},
	})
	return d
}

func TestDetectByMIME(t *testing.T) {
	d := newDetector()

	tests := []struct {
		mime string
		want string
	}{
		{"text/html", "html"},
		{"application/xhtml+xml", "html"},
		{"text/xml", "xml"},
		{"application/xml", "xml"},
		{"application/json", "json"},
		{"text/plain", "plaintext"},
		{"TEXT/HTML", "html"}, // case insensitive
	}

	for _, tt := range tests {
		t.Run(tt.mime, func(t *testing.T) {
			name, err := d.DetectByMIME(tt.mime)
			require.NoError(t, err)
			assert.Equal(t, tt.want, name)
		})
	}
}

func TestDetectByMIMEUnknown(t *testing.T) {
	d := newDetector()
	_, err := d.DetectByMIME("application/octet-stream")
	assert.Error(t, err)
}

func TestDetectByExtension(t *testing.T) {
	d := newDetector()

	tests := []struct {
		ext  string
		want string
	}{
		{".html", "html"},
		{".htm", "html"},
		{".xhtml", "html"},
		{".xml", "xml"},
		{".json", "json"},
		{".txt", "plaintext"},
		{".HTML", "html"}, // case insensitive
	}

	for _, tt := range tests {
		t.Run(tt.ext, func(t *testing.T) {
			name, err := d.DetectByExtension(tt.ext)
			require.NoError(t, err)
			assert.Equal(t, tt.want, name)
		})
	}
}

func TestDetectByExtensionUnknown(t *testing.T) {
	d := newDetector()
	_, err := d.DetectByExtension(".docx")
	assert.Error(t, err)
}

func TestDetectByExtensionEmpty(t *testing.T) {
	d := newDetector()
	_, err := d.DetectByExtension("")
	assert.Error(t, err)
}

func TestDetectByContentMagicBytes(t *testing.T) {
	d := newDetector()

	tests := []struct {
		name    string
		content string
		want    string
	}{
		{"html doctype", "<!DOCTYPE html><html></html>", "html"},
		{"html tag", "<html><body>Hello</body></html>", "html"},
		{"xml declaration", "<?xml version=\"1.0\"?><root/>", "xml"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := strings.NewReader(tt.content)
			name, err := d.DetectByContent(reader)
			require.NoError(t, err)
			assert.Equal(t, tt.want, name)
		})
	}
}

func TestDetectByContentSniff(t *testing.T) {
	d := newDetector()

	reader := strings.NewReader(`{"key": "value"}`)
	name, err := d.DetectByContent(reader)
	require.NoError(t, err)
	assert.Equal(t, "json", name)
}

func TestDetectByContentArray(t *testing.T) {
	d := newDetector()

	reader := strings.NewReader(`[1, 2, 3]`)
	name, err := d.DetectByContent(reader)
	require.NoError(t, err)
	assert.Equal(t, "json", name)
}

func TestDetectByContentUnknown(t *testing.T) {
	d := newDetector()
	reader := strings.NewReader("just some random text")
	_, err := d.DetectByContent(reader)
	assert.Error(t, err)
}

func TestDetectCascade(t *testing.T) {
	d := newDetector()

	// MIME takes priority
	reader := strings.NewReader("not really html")
	name, err := d.Detect("file.txt", reader, "text/html")
	require.NoError(t, err)
	assert.Equal(t, "html", name)

	// Fall back to extension
	reader = strings.NewReader("some content")
	name, err = d.Detect("file.json", reader, "")
	require.NoError(t, err)
	assert.Equal(t, "json", name)

	// Fall back to content sniffing
	reader = strings.NewReader(`<?xml version="1.0"?><root/>`)
	name, err = d.Detect("file.unknown", reader, "")
	require.NoError(t, err)
	assert.Equal(t, "xml", name)
}

func TestDetectAllFail(t *testing.T) {
	d := newDetector()
	reader := strings.NewReader("random content")
	_, err := d.Detect("file.unknown", reader, "")
	assert.Error(t, err)
}

func TestDetectReaderSeekReset(t *testing.T) {
	d := newDetector()

	content := `<?xml version="1.0"?><root>data</root>`
	reader := strings.NewReader(content)

	// Detect should reset the reader
	name, err := d.DetectByContent(reader)
	require.NoError(t, err)
	assert.Equal(t, "xml", name)

	// Reader should be back at start
	buf := make([]byte, 5)
	n, _ := reader.Read(buf)
	assert.Equal(t, "<?xml", string(buf[:n]))
}

// --- Priority tests ---

func TestDefaultPriorityIsBuiltIn(t *testing.T) {
	d := format.NewFormatDetector()
	d.Register("html", format.FormatSignature{
		MIMETypes:  []string{"text/html"},
		Extensions: []string{".html"},
	})
	assert.Equal(t, format.DefaultBuiltInPriority, d.Priority("html"))
}

func TestSetPriority(t *testing.T) {
	d := format.NewFormatDetector()
	d.Register("html", format.FormatSignature{
		MIMETypes: []string{"text/html"},
	})
	d.SetPriority("html", 200)
	assert.Equal(t, 200, d.Priority("html"))
}

func TestPriorityUnregisteredFormatIsZero(t *testing.T) {
	d := format.NewFormatDetector()
	assert.Equal(t, 0, d.Priority("nonexistent"))
}

func TestDetectByMIMEPriority(t *testing.T) {
	d := format.NewFormatDetector()
	// Two formats claim the same MIME type.
	d.Register("html", format.FormatSignature{
		MIMETypes: []string{"text/html"},
	})
	d.Register("okapi-html", format.FormatSignature{
		MIMETypes: []string{"text/html"},
	})

	// Both start at DefaultBuiltInPriority (50). Give okapi-html plugin priority.
	d.SetPriority("okapi-html", format.DefaultPluginPriority)

	name, err := d.DetectByMIME("text/html")
	require.NoError(t, err)
	assert.Equal(t, "okapi-html", name)
}

func TestDetectByMIMEPriorityOverride(t *testing.T) {
	d := format.NewFormatDetector()
	d.Register("html", format.FormatSignature{
		MIMETypes: []string{"text/html"},
	})
	d.Register("okapi-html", format.FormatSignature{
		MIMETypes: []string{"text/html"},
	})

	// Plugin gets default higher priority.
	d.SetPriority("okapi-html", format.DefaultPluginPriority)
	// Config override makes built-in preferred.
	d.SetPriority("html", 200)

	name, err := d.DetectByMIME("text/html")
	require.NoError(t, err)
	assert.Equal(t, "html", name)
}

func TestDetectByExtensionPriority(t *testing.T) {
	d := format.NewFormatDetector()
	d.Register("html", format.FormatSignature{
		Extensions: []string{".html", ".htm"},
	})
	d.Register("okapi-html", format.FormatSignature{
		Extensions: []string{".html"},
	})
	d.SetPriority("okapi-html", format.DefaultPluginPriority)

	name, err := d.DetectByExtension(".html")
	require.NoError(t, err)
	assert.Equal(t, "okapi-html", name)
}

func TestDetectByExtensionPriorityOverride(t *testing.T) {
	d := format.NewFormatDetector()
	d.Register("html", format.FormatSignature{
		Extensions: []string{".html"},
	})
	d.Register("okapi-html", format.FormatSignature{
		Extensions: []string{".html"},
	})
	d.SetPriority("okapi-html", format.DefaultPluginPriority)
	d.SetPriority("html", 200)

	name, err := d.DetectByExtension(".html")
	require.NoError(t, err)
	assert.Equal(t, "html", name)
}

func TestDetectByContentMagicBytesPriority(t *testing.T) {
	d := format.NewFormatDetector()
	d.Register("html", format.FormatSignature{
		MagicBytes: [][]byte{[]byte("<html")},
	})
	d.Register("okapi-html", format.FormatSignature{
		MagicBytes: [][]byte{[]byte("<html")},
	})
	d.SetPriority("okapi-html", format.DefaultPluginPriority)

	reader := strings.NewReader("<html><body>test</body></html>")
	name, err := d.DetectByContent(reader)
	require.NoError(t, err)
	assert.Equal(t, "okapi-html", name)
}

func TestDetectByContentSniffPriority(t *testing.T) {
	jsonSniff := func(data []byte) bool {
		trimmed := bytes.TrimSpace(data)
		return len(trimmed) > 0 && (trimmed[0] == '{' || trimmed[0] == '[')
	}
	d := format.NewFormatDetector()
	d.Register("json", format.FormatSignature{
		Sniff: jsonSniff,
	})
	d.Register("okapi-json", format.FormatSignature{
		Sniff: jsonSniff,
	})
	d.SetPriority("okapi-json", format.DefaultPluginPriority)

	reader := strings.NewReader(`{"key": "value"}`)
	name, err := d.DetectByContent(reader)
	require.NoError(t, err)
	assert.Equal(t, "okapi-json", name)
}

func TestDetectCascadeWithPriority(t *testing.T) {
	d := format.NewFormatDetector()
	d.Register("html", format.FormatSignature{
		MIMETypes:  []string{"text/html"},
		Extensions: []string{".html"},
		MagicBytes: [][]byte{[]byte("<html")},
	})
	d.Register("okapi-html", format.FormatSignature{
		MIMETypes:  []string{"text/html"},
		Extensions: []string{".html"},
		MagicBytes: [][]byte{[]byte("<html")},
	})
	d.SetPriority("okapi-html", format.DefaultPluginPriority)

	// MIME detection should pick higher-priority format.
	reader := strings.NewReader("content")
	name, err := d.Detect("file.html", reader, "text/html")
	require.NoError(t, err)
	assert.Equal(t, "okapi-html", name)

	// Extension detection should pick higher-priority format.
	reader = strings.NewReader("content")
	name, err = d.Detect("file.html", reader, "")
	require.NoError(t, err)
	assert.Equal(t, "okapi-html", name)

	// Content detection should pick higher-priority format.
	reader = strings.NewReader("<html><body>test</body></html>")
	name, err = d.Detect("file.unknown", reader, "")
	require.NoError(t, err)
	assert.Equal(t, "okapi-html", name)
}

func TestDetectByExtensionUniqueFormatsUnaffectedByPriority(t *testing.T) {
	// When only one format matches, priority doesn't matter.
	d := format.NewFormatDetector()
	d.Register("csv", format.FormatSignature{
		Extensions: []string{".csv"},
	})
	d.Register("json", format.FormatSignature{
		Extensions: []string{".json"},
	})
	d.SetPriority("json", 200)

	name, err := d.DetectByExtension(".csv")
	require.NoError(t, err)
	assert.Equal(t, "csv", name)
}

func TestDefaultPriorityConstants(t *testing.T) {
	assert.Equal(t, 50, format.DefaultBuiltInPriority)
	assert.Equal(t, 100, format.DefaultPluginPriority)
	assert.Greater(t, format.DefaultPluginPriority, format.DefaultBuiltInPriority)
}
