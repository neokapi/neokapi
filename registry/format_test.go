package registry

import (
	"context"
	"testing"

	"github.com/gokapi/gokapi/format"
	"github.com/gokapi/gokapi/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubReader is a minimal DataFormatReader for testing.
type stubReader struct {
	format.BaseFormatReader
	sig format.FormatSignature
}

func newStubReader(name string) *stubReader {
	return &stubReader{
		BaseFormatReader: format.BaseFormatReader{FormatName: name},
	}
}

func newStubReaderWithSig(name, displayName string, mimes, exts []string) *stubReader {
	return &stubReader{
		BaseFormatReader: format.BaseFormatReader{
			FormatName:        name,
			FormatDisplayName: displayName,
		},
		sig: format.FormatSignature{
			MIMETypes:  mimes,
			Extensions: exts,
		},
	}
}

func (s *stubReader) Signature() format.FormatSignature {
	return s.sig
}
func (s *stubReader) Open(_ context.Context, _ *model.RawDocument) error {
	return nil
}
func (s *stubReader) Read(_ context.Context) <-chan model.PartResult {
	ch := make(chan model.PartResult)
	close(ch)
	return ch
}
func (s *stubReader) Close() error { return nil }

// stubWriter is a minimal DataFormatWriter for testing.
type stubWriter struct {
	format.BaseFormatWriter
}

func newStubWriter(name string) *stubWriter {
	return &stubWriter{
		BaseFormatWriter: format.BaseFormatWriter{FormatName: name},
	}
}

func (s *stubWriter) Write(_ context.Context, _ <-chan *model.Part) error { return nil }

func TestNewReaderExact(t *testing.T) {
	reg := NewFormatRegistry()
	reg.RegisterReader("csv", func() format.DataFormatReader {
		return newStubReader("csv")
	})

	r, err := reg.NewReader("csv")
	require.NoError(t, err)
	assert.Equal(t, "csv", r.Name())
}

func TestNewReaderUnknown(t *testing.T) {
	reg := NewFormatRegistry()
	_, err := reg.NewReader("nonexistent")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown format")
}

func TestNewReaderVersionedExact(t *testing.T) {
	reg := NewFormatRegistry()
	reg.RegisterReader("okapi-html@1.46.0", func() format.DataFormatReader {
		return newStubReader("okapi-html@1.46.0")
	})

	r, err := reg.NewReader("okapi-html@1.46.0")
	require.NoError(t, err)
	assert.Equal(t, "okapi-html@1.46.0", r.Name())
}

func TestNewReaderVersionedFallback(t *testing.T) {
	reg := NewFormatRegistry()
	// Register versioned entries only (no bare name).
	reg.RegisterReader("okapi-html@1.46.0", func() format.DataFormatReader {
		return newStubReader("okapi-html@1.46.0")
	})
	reg.RegisterReader("okapi-html@1.47.0", func() format.DataFormatReader {
		return newStubReader("okapi-html@1.47.0")
	})
	reg.RegisterReader("okapi-html@1.45.0", func() format.DataFormatReader {
		return newStubReader("okapi-html@1.45.0")
	})

	// Requesting bare name should fall back to latest version.
	r, err := reg.NewReader("okapi-html")
	require.NoError(t, err)
	assert.Equal(t, "okapi-html@1.47.0", r.Name())
}

func TestNewReaderBareNamePreferred(t *testing.T) {
	reg := NewFormatRegistry()
	// Register both bare name and versioned entries.
	reg.RegisterReader("okapi-html", func() format.DataFormatReader {
		return newStubReader("okapi-html-bare")
	})
	reg.RegisterReader("okapi-html@1.46.0", func() format.DataFormatReader {
		return newStubReader("okapi-html@1.46.0")
	})

	// Bare name should match directly, not fall through to versioned.
	r, err := reg.NewReader("okapi-html")
	require.NoError(t, err)
	assert.Equal(t, "okapi-html-bare", r.Name())
}

func TestNewWriterVersionedFallback(t *testing.T) {
	reg := NewFormatRegistry()
	reg.RegisterWriter("okapi-html@1.46.0", func() format.DataFormatWriter {
		return newStubWriter("okapi-html@1.46.0")
	})
	reg.RegisterWriter("okapi-html@1.47.0", func() format.DataFormatWriter {
		return newStubWriter("okapi-html@1.47.0")
	})

	w, err := reg.NewWriter("okapi-html")
	require.NoError(t, err)
	assert.Equal(t, "okapi-html@1.47.0", w.Name())
}

func TestNewWriterUnknown(t *testing.T) {
	reg := NewFormatRegistry()
	_, err := reg.NewWriter("nonexistent")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown format writer")
}

func TestInternalCompareSemver(t *testing.T) {
	assert.Equal(t, 0, compareSemver("1.0.0", "1.0.0"))
	assert.Equal(t, -1, compareSemver("1.0.0", "2.0.0"))
	assert.Equal(t, 1, compareSemver("2.0.0", "1.0.0"))
	assert.Equal(t, 1, compareSemver("1.47.0", "1.46.0"))
}

func TestFormatInfosBuiltIn(t *testing.T) {
	reg := NewFormatRegistry()
	reg.RegisterReader("html", func() format.DataFormatReader {
		return newStubReaderWithSig("html", "HTML", []string{"text/html"}, []string{".html", ".htm"})
	})
	reg.RegisterWriter("html", func() format.DataFormatWriter {
		return newStubWriter("html")
	})

	infos := reg.FormatInfos()
	require.Len(t, infos, 1)
	assert.Equal(t, "html", infos[0].Name)
	assert.Equal(t, "HTML", infos[0].DisplayName)
	assert.Equal(t, []string{"text/html"}, infos[0].MimeTypes)
	assert.Equal(t, []string{".html", ".htm"}, infos[0].Extensions)
	assert.True(t, infos[0].HasReader)
	assert.True(t, infos[0].HasWriter)
	assert.Equal(t, "built-in", infos[0].Source)
}

func TestFormatInfosPluginSource(t *testing.T) {
	reg := NewFormatRegistry()
	reg.RegisterReader("okapi-html", func() format.DataFormatReader {
		return newStubReaderWithSig("okapi-html", "Okapi HTML", []string{"text/html"}, []string{".html"})
	})
	reg.SetFormatSource("okapi-html", "okapi")

	infos := reg.FormatInfos()
	require.Len(t, infos, 1)
	assert.Equal(t, "okapi", infos[0].Source)
}

func TestFormatInfosSorted(t *testing.T) {
	reg := NewFormatRegistry()
	reg.RegisterReader("yaml", func() format.DataFormatReader {
		return newStubReader("yaml")
	})
	reg.RegisterReader("csv", func() format.DataFormatReader {
		return newStubReader("csv")
	})
	reg.RegisterReader("html", func() format.DataFormatReader {
		return newStubReader("html")
	})

	infos := reg.FormatInfos()
	require.Len(t, infos, 3)
	assert.Equal(t, "csv", infos[0].Name)
	assert.Equal(t, "html", infos[1].Name)
	assert.Equal(t, "yaml", infos[2].Name)
}

func TestFormatInfoSingleLookup(t *testing.T) {
	reg := NewFormatRegistry()
	reg.RegisterReader("html", func() format.DataFormatReader {
		return newStubReaderWithSig("html", "HTML", []string{"text/html"}, []string{".html"})
	})
	reg.SetFormatSource("html", "built-in")

	info := reg.FormatInfo("html")
	require.NotNil(t, info)
	assert.Equal(t, "html", info.Name)
	assert.Equal(t, "HTML", info.DisplayName)

	// Non-existent format returns nil.
	assert.Nil(t, reg.FormatInfo("nonexistent"))
}

func TestFormatInfoWriterOnly(t *testing.T) {
	reg := NewFormatRegistry()
	reg.RegisterWriter("custom", func() format.DataFormatWriter {
		return newStubWriter("custom")
	})

	infos := reg.FormatInfos()
	require.Len(t, infos, 1)
	assert.False(t, infos[0].HasReader)
	assert.True(t, infos[0].HasWriter)
}

// --- Priority tests ---

func TestDefaultPriorityBuiltIn(t *testing.T) {
	reg := NewFormatRegistry()
	reg.RegisterReader("html", func() format.DataFormatReader {
		return newStubReaderWithSig("html", "HTML", []string{"text/html"}, []string{".html"})
	})

	info := reg.FormatInfo("html")
	require.NotNil(t, info)
	assert.Equal(t, format.DefaultBuiltInPriority, info.Priority)
}

func TestSetFormatSourceSetsPluginPriority(t *testing.T) {
	reg := NewFormatRegistry()
	reg.RegisterReader("okapi-html", func() format.DataFormatReader {
		return newStubReaderWithSig("okapi-html", "Okapi HTML", []string{"text/html"}, []string{".html"})
	})
	reg.SetFormatSource("okapi-html", "okapi")

	info := reg.FormatInfo("okapi-html")
	require.NotNil(t, info)
	assert.Equal(t, format.DefaultPluginPriority, info.Priority)
}

func TestSetFormatSourceBuiltInKeepsDefaultPriority(t *testing.T) {
	reg := NewFormatRegistry()
	reg.RegisterReader("html", func() format.DataFormatReader {
		return newStubReaderWithSig("html", "HTML", []string{"text/html"}, []string{".html"})
	})
	reg.SetFormatSource("html", "built-in")

	info := reg.FormatInfo("html")
	require.NotNil(t, info)
	assert.Equal(t, format.DefaultBuiltInPriority, info.Priority)
}

func TestSetFormatPriority(t *testing.T) {
	reg := NewFormatRegistry()
	reg.RegisterReader("html", func() format.DataFormatReader {
		return newStubReaderWithSig("html", "HTML", []string{"text/html"}, []string{".html"})
	})

	reg.SetFormatPriority("html", 200)

	info := reg.FormatInfo("html")
	require.NotNil(t, info)
	assert.Equal(t, 200, info.Priority)
}

func TestSetFormatPriorityOverridesPluginDefault(t *testing.T) {
	reg := NewFormatRegistry()
	reg.RegisterReader("okapi-html", func() format.DataFormatReader {
		return newStubReaderWithSig("okapi-html", "Okapi HTML", []string{"text/html"}, []string{".html"})
	})
	reg.SetFormatSource("okapi-html", "okapi")

	// Priority should now be plugin default.
	info := reg.FormatInfo("okapi-html")
	require.NotNil(t, info)
	assert.Equal(t, format.DefaultPluginPriority, info.Priority)

	// Override it.
	reg.SetFormatPriority("okapi-html", 25)
	info = reg.FormatInfo("okapi-html")
	require.NotNil(t, info)
	assert.Equal(t, 25, info.Priority)
}

func TestResolveFormatWithPriority(t *testing.T) {
	reg := NewFormatRegistry()
	reg.RegisterReader("html", func() format.DataFormatReader {
		return newStubReaderWithSig("html", "HTML", []string{"text/html"}, []string{".html"})
	})
	reg.RegisterReader("okapi-html", func() format.DataFormatReader {
		return newStubReaderWithSig("okapi-html", "Okapi HTML", []string{"text/html"}, []string{".html"})
	})
	reg.SetFormatSource("okapi-html", "okapi")

	// Plugin format should win by default (priority 100 > 50).
	name := reg.ResolveFormat("text/html")
	assert.Equal(t, "okapi-html", name)
}

func TestResolveFormatConfigOverride(t *testing.T) {
	reg := NewFormatRegistry()
	reg.RegisterReader("html", func() format.DataFormatReader {
		return newStubReaderWithSig("html", "HTML", []string{"text/html"}, []string{".html"})
	})
	reg.RegisterReader("okapi-html", func() format.DataFormatReader {
		return newStubReaderWithSig("okapi-html", "Okapi HTML", []string{"text/html"}, []string{".html"})
	})
	reg.SetFormatSource("okapi-html", "okapi")

	// Override built-in to have higher priority.
	reg.SetFormatPriority("html", 200)

	name := reg.ResolveFormat("text/html")
	assert.Equal(t, "html", name)
}

func TestResolveFormatNoMatch(t *testing.T) {
	reg := NewFormatRegistry()
	name := reg.ResolveFormat("application/octet-stream")
	assert.Equal(t, "", name)
}

func TestFormatInfoIncludesPriority(t *testing.T) {
	reg := NewFormatRegistry()
	reg.RegisterReader("html", func() format.DataFormatReader {
		return newStubReaderWithSig("html", "HTML", []string{"text/html"}, []string{".html"})
	})
	reg.SetFormatPriority("html", 150)

	infos := reg.FormatInfos()
	require.Len(t, infos, 1)
	assert.Equal(t, 150, infos[0].Priority)

	info := reg.FormatInfo("html")
	require.NotNil(t, info)
	assert.Equal(t, 150, info.Priority)
}

func TestSetFormatPriorityBeforeRegister(t *testing.T) {
	reg := NewFormatRegistry()
	// Set priority before registering the reader.
	reg.SetFormatPriority("html", 300)

	reg.RegisterReader("html", func() format.DataFormatReader {
		return newStubReaderWithSig("html", "HTML", []string{"text/html"}, []string{".html"})
	})

	info := reg.FormatInfo("html")
	require.NotNil(t, info)
	// Priority set before registration should be preserved.
	assert.Equal(t, 300, info.Priority)
}

func TestDetectorUsedByResolveFormat(t *testing.T) {
	reg := NewFormatRegistry()
	reg.RegisterReader("json", func() format.DataFormatReader {
		return newStubReaderWithSig("json", "JSON", []string{"application/json"}, []string{".json"})
	})

	name := reg.ResolveFormat("application/json")
	assert.Equal(t, "json", name)
}

func TestSetFormatSourceDoesNotDowngradeExplicitPriority(t *testing.T) {
	reg := NewFormatRegistry()
	reg.RegisterReader("okapi-html", func() format.DataFormatReader {
		return newStubReaderWithSig("okapi-html", "Okapi HTML", []string{"text/html"}, []string{".html"})
	})
	// Explicitly set a high priority first.
	reg.SetFormatPriority("okapi-html", 500)
	// Now set the source — should NOT downgrade to DefaultPluginPriority.
	reg.SetFormatSource("okapi-html", "okapi")

	info := reg.FormatInfo("okapi-html")
	require.NotNil(t, info)
	assert.Equal(t, 500, info.Priority)
}
