package registry

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
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

// regStub registers a stub reader with an empty signature.
func regStub(reg *FormatRegistry, name string) {
	reg.RegisterReader(FormatID(name), func() format.DataFormatReader {
		return newStubReader(name)
	}, format.FormatSignature{}, "")
}

// regStubSig registers a stub reader with the given signature and display name.
func regStubSig(reg *FormatRegistry, name, displayName string, mimes, exts []string) {
	reg.RegisterReader(FormatID(name), func() format.DataFormatReader {
		return newStubReaderWithSig(name, displayName, mimes, exts)
	}, format.FormatSignature{MIMETypes: mimes, Extensions: exts}, displayName)
}

func TestNewReaderExact(t *testing.T) {
	reg := NewFormatRegistry()
	regStub(reg, "csv")

	r, err := reg.NewReader("csv")
	require.NoError(t, err)
	assert.Equal(t, "csv", r.Name())
}

func TestNewReaderUnknown(t *testing.T) {
	reg := NewFormatRegistry()
	_, err := reg.NewReader("nonexistent")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown format")
}

func TestNewReaderVersionedExact(t *testing.T) {
	reg := NewFormatRegistry()
	reg.RegisterReader("okapi-html@1.46.0", func() format.DataFormatReader {
		return newStubReader("okapi-html@1.46.0")
	}, format.FormatSignature{}, "")

	r, err := reg.NewReader("okapi-html@1.46.0")
	require.NoError(t, err)
	assert.Equal(t, "okapi-html@1.46.0", r.Name())
}

func TestNewReaderVersionedFallback(t *testing.T) {
	reg := NewFormatRegistry()
	reg.RegisterReader("okapi-html@1.46.0", func() format.DataFormatReader {
		return newStubReader("okapi-html@1.46.0")
	}, format.FormatSignature{}, "")
	reg.RegisterReader("okapi-html@1.47.0", func() format.DataFormatReader {
		return newStubReader("okapi-html@1.47.0")
	}, format.FormatSignature{}, "")
	reg.RegisterReader("okapi-html@1.45.0", func() format.DataFormatReader {
		return newStubReader("okapi-html@1.45.0")
	}, format.FormatSignature{}, "")

	// Requesting bare name should fall back to latest version.
	r, err := reg.NewReader("okapi-html")
	require.NoError(t, err)
	assert.Equal(t, "okapi-html@1.47.0", r.Name())
}

func TestNewReaderBareNamePreferred(t *testing.T) {
	reg := NewFormatRegistry()
	reg.RegisterReader("okapi-html", func() format.DataFormatReader {
		return newStubReader("okapi-html-bare")
	}, format.FormatSignature{}, "")
	reg.RegisterReader("okapi-html@1.46.0", func() format.DataFormatReader {
		return newStubReader("okapi-html@1.46.0")
	}, format.FormatSignature{}, "")

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
	require.Error(t, err)
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
	regStubSig(reg, "html", "HTML", []string{"text/html"}, []string{".html", ".htm"})
	reg.RegisterWriter("html", func() format.DataFormatWriter {
		return newStubWriter("html")
	})

	infos := reg.FormatInfos()
	require.Len(t, infos, 1)
	assert.Equal(t, FormatID("html"), infos[0].Name)
	assert.Equal(t, "HTML", infos[0].DisplayName)
	assert.Equal(t, []string{"text/html"}, infos[0].MimeTypes)
	assert.Equal(t, []string{".html", ".htm"}, infos[0].Extensions)
	assert.True(t, infos[0].HasReader)
	assert.True(t, infos[0].HasWriter)
	assert.Equal(t, SourceBuiltIn, infos[0].Source)
}

func TestFormatInfosPluginSource(t *testing.T) {
	reg := NewFormatRegistry()
	regStubSig(reg, "okapi-html", "Okapi HTML", []string{"text/html"}, []string{".html"})
	reg.SetFormatSource("okapi-html", "okapi")

	infos := reg.FormatInfos()
	require.Len(t, infos, 1)
	assert.Equal(t, "okapi", infos[0].Source)
}

func TestFormatInfosSorted(t *testing.T) {
	reg := NewFormatRegistry()
	regStub(reg, "yaml")
	regStub(reg, "csv")
	regStub(reg, "html")

	infos := reg.FormatInfos()
	require.Len(t, infos, 3)
	assert.Equal(t, FormatID("csv"), infos[0].Name)
	assert.Equal(t, FormatID("html"), infos[1].Name)
	assert.Equal(t, FormatID("yaml"), infos[2].Name)
}

func TestFormatInfoSingleLookup(t *testing.T) {
	reg := NewFormatRegistry()
	regStubSig(reg, "html", "HTML", []string{"text/html"}, []string{".html"})
	reg.SetFormatSource("html", SourceBuiltIn)

	info := reg.FormatInfo("html")
	require.NotNil(t, info)
	assert.Equal(t, FormatID("html"), info.Name)
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
	regStubSig(reg, "html", "HTML", []string{"text/html"}, []string{".html"})

	info := reg.FormatInfo("html")
	require.NotNil(t, info)
	assert.Equal(t, format.DefaultBuiltInPriority, info.Priority)
}

func TestSetFormatSourceSetsPluginPriority(t *testing.T) {
	reg := NewFormatRegistry()
	regStubSig(reg, "okapi-html", "Okapi HTML", []string{"text/html"}, []string{".html"})
	reg.SetFormatSource("okapi-html", "okapi")

	info := reg.FormatInfo("okapi-html")
	require.NotNil(t, info)
	assert.Equal(t, format.DefaultPluginPriority, info.Priority)
}

func TestSetFormatSourceBuiltInKeepsDefaultPriority(t *testing.T) {
	reg := NewFormatRegistry()
	regStubSig(reg, "html", "HTML", []string{"text/html"}, []string{".html"})
	reg.SetFormatSource("html", SourceBuiltIn)

	info := reg.FormatInfo("html")
	require.NotNil(t, info)
	assert.Equal(t, format.DefaultBuiltInPriority, info.Priority)
}

func TestSetFormatPriority(t *testing.T) {
	reg := NewFormatRegistry()
	regStubSig(reg, "html", "HTML", []string{"text/html"}, []string{".html"})

	reg.SetFormatPriority("html", 200)

	info := reg.FormatInfo("html")
	require.NotNil(t, info)
	assert.Equal(t, 200, info.Priority)
}

func TestSetFormatPriorityOverridesPluginDefault(t *testing.T) {
	reg := NewFormatRegistry()
	regStubSig(reg, "okapi-html", "Okapi HTML", []string{"text/html"}, []string{".html"})
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
	regStubSig(reg, "html", "HTML", []string{"text/html"}, []string{".html"})
	regStubSig(reg, "okapi-html", "Okapi HTML", []string{"text/html"}, []string{".html"})
	reg.SetFormatSource("okapi-html", "okapi")

	// Plugin format should win by default (priority 100 > 50).
	name := reg.ResolveFormat("text/html")
	assert.Equal(t, FormatID("okapi-html"), name)
}

func TestResolveFormatConfigOverride(t *testing.T) {
	reg := NewFormatRegistry()
	regStubSig(reg, "html", "HTML", []string{"text/html"}, []string{".html"})
	regStubSig(reg, "okapi-html", "Okapi HTML", []string{"text/html"}, []string{".html"})
	reg.SetFormatSource("okapi-html", "okapi")

	// Override built-in to have higher priority.
	reg.SetFormatPriority("html", 200)

	name := reg.ResolveFormat("text/html")
	assert.Equal(t, FormatID("html"), name)
}

func TestResolveFormatNoMatch(t *testing.T) {
	reg := NewFormatRegistry()
	name := reg.ResolveFormat("application/octet-stream")
	assert.Equal(t, FormatID(""), name)
}

func TestFormatInfoIncludesPriority(t *testing.T) {
	reg := NewFormatRegistry()
	regStubSig(reg, "html", "HTML", []string{"text/html"}, []string{".html"})
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

	regStubSig(reg, "html", "HTML", []string{"text/html"}, []string{".html"})

	info := reg.FormatInfo("html")
	require.NotNil(t, info)
	// Priority set before registration should be preserved.
	assert.Equal(t, 300, info.Priority)
}

func TestDetectorUsedByResolveFormat(t *testing.T) {
	reg := NewFormatRegistry()
	regStubSig(reg, "json", "JSON", []string{"application/json"}, []string{".json"})

	name := reg.ResolveFormat("application/json")
	assert.Equal(t, FormatID("json"), name)
}

func TestSetFormatSourceDoesNotDowngradeExplicitPriority(t *testing.T) {
	reg := NewFormatRegistry()
	regStubSig(reg, "okapi-html", "Okapi HTML", []string{"text/html"}, []string{".html"})
	// Explicitly set a high priority first.
	reg.SetFormatPriority("okapi-html", 500)
	// Now set the source — should NOT downgrade to DefaultPluginPriority.
	reg.SetFormatSource("okapi-html", "okapi")

	info := reg.FormatInfo("okapi-html")
	require.NotNil(t, info)
	assert.Equal(t, 500, info.Priority)
}

func TestRegisterFormatInfo(t *testing.T) {
	reg := NewFormatRegistry()

	// Register metadata-only (no reader/writer factory).
	reg.RegisterFormatInfo("okapi-html@1.46.0", FormatInfo{
		DisplayName: "HTML Filter",
		MimeTypes:   []string{"text/html"},
		Extensions:  []string{".html", ".htm"},
		Source:      "okapi",
	})

	info := reg.FormatInfo("okapi-html@1.46.0")
	require.NotNil(t, info)
	assert.Equal(t, "HTML Filter", info.DisplayName)
	assert.Equal(t, []string{"text/html"}, info.MimeTypes)
	assert.Equal(t, []string{".html", ".htm"}, info.Extensions)
	assert.Equal(t, "okapi", info.Source)
	assert.False(t, info.HasReader)
	assert.False(t, info.HasWriter)

	// Registering a reader later should update the existing info.
	reg.RegisterReader("okapi-html@1.46.0", func() format.DataFormatReader {
		return newStubReaderWithSig("okapi-html", "HTML Filter", []string{"text/html"}, []string{".html"})
	}, format.FormatSignature{MIMETypes: []string{"text/html"}, Extensions: []string{".html"}}, "HTML Filter")
	info = reg.FormatInfo("okapi-html@1.46.0")
	require.NotNil(t, info)
	assert.True(t, info.HasReader)
	assert.Equal(t, "okapi", info.Source) // preserved from RegisterFormatInfo
}

func TestRegisterFormatInfoAppearsInList(t *testing.T) {
	reg := NewFormatRegistry()
	reg.RegisterFormatInfo("bridge-csv@2.0.0", FormatInfo{
		DisplayName: "CSV (Bridge)",
		Source:      "my-plugin",
	})
	infos := reg.FormatInfos()
	require.Len(t, infos, 1)
	assert.Equal(t, FormatID("bridge-csv@2.0.0"), infos[0].Name)
	assert.Equal(t, "CSV (Bridge)", infos[0].DisplayName)
	assert.Equal(t, "my-plugin", infos[0].Source)
}

func TestOnMissTriggeredOnReaderMiss(t *testing.T) {
	reg := NewFormatRegistry()
	regStub(reg, "html")

	var called bool
	reg.SetOnMiss(func() {
		called = true
		// Simulate lazy-loading a bridge format.
		regStub(reg, "okf_docx")
	})

	// Built-in format — no onMiss.
	r, err := reg.NewReader("html")
	require.NoError(t, err)
	assert.NotNil(t, r)
	assert.False(t, called, "onMiss should not fire for a built-in format")

	// Unknown format — triggers onMiss, which registers the format.
	r, err = reg.NewReader("okf_docx")
	require.NoError(t, err)
	assert.NotNil(t, r)
	assert.True(t, called, "onMiss should fire for a missing format")
}

func TestOnMissCalledOnlyOnce(t *testing.T) {
	reg := NewFormatRegistry()
	callCount := 0
	reg.SetOnMiss(func() {
		callCount++
	})

	_, _ = reg.NewReader("nonexistent1")
	_, _ = reg.NewReader("nonexistent2")
	assert.Equal(t, 1, callCount, "onMiss should be called only once")
}

func TestOnMissConcurrentCallersBlock(t *testing.T) {
	reg := NewFormatRegistry()

	var callCount int
	reg.SetOnMiss(func() {
		callCount++
		// Simulate slow bridge loading.
		time.Sleep(50 * time.Millisecond)
		regStub(reg, "okf_openxml")
	})

	// Launch concurrent readers — all should block until onMiss completes.
	var wg sync.WaitGroup
	errors := make([]error, 10)
	for i := range 10 {
		wg.Go(func() {
			_, errors[i] = reg.NewReader("okf_openxml")
		})
	}
	wg.Wait()

	assert.Equal(t, 1, callCount, "onMiss should be called exactly once")
	for i, err := range errors {
		require.NoError(t, err, "goroutine %d should find the format after onMiss completes", i)
	}
}

func TestOnMissTriggeredOnWriterMiss(t *testing.T) {
	reg := NewFormatRegistry()
	var called bool
	reg.SetOnMiss(func() {
		called = true
		reg.RegisterWriter("okf_docx", func() format.DataFormatWriter {
			return &stubWriter{}
		})
	})

	w, err := reg.NewWriter("okf_docx")
	require.NoError(t, err)
	assert.NotNil(t, w)
	assert.True(t, called)
}

// TestOnMissSetConcurrentWithLookup exercises the data race between SetOnMiss
// (which replaces the onMiss callback) and the lookup paths that fire it. Run
// with -race; the failure mode of the previous sync.Once design was a race on
// the reassigned Once value. The test only asserts that the invariants hold —
// every registered callback fires at most once for its registration, and the
// lookup path stays internally consistent under concurrency.
func TestOnMissSetConcurrentWithLookup(t *testing.T) {
	reg := NewFormatRegistry()

	const callbacks = 50
	var counts [callbacks]int64

	var wg sync.WaitGroup

	// Writer: continuously install fresh onMiss callbacks. Each callback, when
	// it fires, increments its own counter and registers a format.
	wg.Go(func() {
		for i := range callbacks {
			reg.SetOnMiss(func() {
				atomic.AddInt64(&counts[i], 1)
				regStub(reg, "lazy-format")
			})
		}
	})

	// Readers: hammer the miss path (and the success path) concurrently.
	for range 16 {
		wg.Go(func() {
			for range 100 {
				_, _ = reg.NewReader("lazy-format")
				_, _ = reg.NewWriter("never-registered")
				_, _ = reg.DetectByExtension(".nope")
			}
		})
	}

	wg.Wait()

	// Each callback must have fired at most once for its single registration.
	for i := range counts {
		got := atomic.LoadInt64(&counts[i])
		assert.LessOrEqualf(t, got, int64(1),
			"callback %d fired %d times; must fire at most once per registration", i, got)
	}
}

func TestRegisterFormatInfoDetectableByExtension(t *testing.T) {
	reg := NewFormatRegistry()

	// Register metadata-only (no reader/writer factory) — should still be detectable.
	reg.RegisterFormatInfo("okf_openxml@1.46.0", FormatInfo{
		DisplayName: "OpenXML Filter",
		Extensions:  []string{".docx", ".xlsx", ".pptx"},
		Source:      "okapi-bridge",
	})

	// Should find the format by extension without needing a reader factory.
	name, err := reg.DetectByExtension(".docx")
	require.NoError(t, err)
	assert.Equal(t, FormatID("okf_openxml@1.46.0"), name)

	name, err = reg.DetectByExtension(".xlsx")
	require.NoError(t, err)
	assert.Equal(t, FormatID("okf_openxml@1.46.0"), name)
}

func TestRegisterFormatInfoDetectableByMIME(t *testing.T) {
	reg := NewFormatRegistry()

	reg.RegisterFormatInfo("okf_html@1.46.0", FormatInfo{
		DisplayName: "HTML Filter",
		MimeTypes:   []string{"text/html"},
		Source:      "okapi-bridge",
	})

	name := reg.ResolveFormat("text/html")
	assert.Equal(t, FormatID("okf_html@1.46.0"), name)
}

func TestRegisterFormatInfoPriorityInDetection(t *testing.T) {
	reg := NewFormatRegistry()

	// Built-in HTML at default priority (50).
	regStubSig(reg, "html", "HTML", []string{"text/html"}, []string{".html"})

	// Bridge HTML registered via metadata only — gets plugin priority (100).
	reg.RegisterFormatInfo("okf_html@1.46.0", FormatInfo{
		DisplayName: "Okapi HTML",
		MimeTypes:   []string{"text/html"},
		Extensions:  []string{".html"},
		Source:      "okapi-bridge",
	})

	// Bridge format should win (priority 100 > 50).
	name, err := reg.DetectByExtension(".html")
	require.NoError(t, err)
	assert.Equal(t, FormatID("okf_html@1.46.0"), name)

	name = reg.ResolveFormat("text/html")
	assert.Equal(t, FormatID("okf_html@1.46.0"), name)

	// Override built-in to have higher priority — should win.
	reg.SetFormatPriority("html", 200)
	name, err = reg.DetectByExtension(".html")
	require.NoError(t, err)
	assert.Equal(t, FormatID("html"), name)
}

func TestDetectByExtensionTriggersOnMiss(t *testing.T) {
	reg := NewFormatRegistry()

	var called bool
	reg.SetOnMiss(func() {
		called = true
		// Simulate bridge loading registering a format with .docx extension.
		reg.RegisterReader("okf_openxml", func() format.DataFormatReader {
			return newStubReaderWithSig("okf_openxml", "OpenXML", nil, []string{".docx", ".docm"})
		}, format.FormatSignature{Extensions: []string{".docx", ".docm"}}, "OpenXML")
	})

	// Built-in extension — no onMiss needed.
	regStubSig(reg, "json", "JSON", nil, []string{".json"})

	name, err := reg.DetectByExtension(".json")
	require.NoError(t, err)
	assert.Equal(t, FormatID("json"), name)
	assert.False(t, called, "onMiss should not fire for a built-in extension")

	// Unknown extension — triggers onMiss, which registers the format.
	name, err = reg.DetectByExtension(".docx")
	require.NoError(t, err)
	assert.Equal(t, FormatID("okf_openxml"), name)
	assert.True(t, called, "onMiss should fire for a missing extension")
}

func TestDetectByExtensionForSources(t *testing.T) {
	reg := NewFormatRegistry()

	// Register a built-in JSON format.
	reg.RegisterReader("json", func() format.DataFormatReader {
		return newStubReaderWithSig("json", "JSON", []string{"application/json"}, []string{".json"})
	}, format.FormatSignature{Extensions: []string{".json"}}, "JSON")

	// Register a plugin JSON format (higher priority).
	reg.RegisterFormatInfo("okf_json", FormatInfo{
		DisplayName: "Okapi JSON",
		Extensions:  []string{".json"},
		Source:      "okapi-bridge",
		HasReader:   true,
	})
	reg.SetFormatPriority("okf_json", format.DefaultPluginPriority)

	// Without source filter: plugin wins (higher priority).
	name, err := reg.DetectByExtension(".json")
	require.NoError(t, err)
	assert.Equal(t, FormatID("okf_json"), name)

	// With source filter: only built-in allowed.
	name, err = reg.DetectByExtensionForSources(".json", []string{SourceBuiltIn})
	require.NoError(t, err)
	assert.Equal(t, FormatID("json"), name)

	// With source filter including the plugin.
	name, err = reg.DetectByExtensionForSources(".json", []string{SourceBuiltIn, "okapi-bridge"})
	require.NoError(t, err)
	assert.Equal(t, FormatID("okf_json"), name, "plugin format should win when its source is allowed")

	// Nil sources = no filter = same as DetectByExtension.
	name, err = reg.DetectByExtensionForSources(".json", nil)
	require.NoError(t, err)
	assert.Equal(t, FormatID("okf_json"), name)

	// Empty sources = no filter.
	name, err = reg.DetectByExtensionForSources(".json", []string{})
	require.NoError(t, err)
	assert.Equal(t, FormatID("okf_json"), name)

	// Unknown extension with restrictive filter.
	_, err = reg.DetectByExtensionForSources(".xyz", []string{SourceBuiltIn})
	assert.Error(t, err)
}
