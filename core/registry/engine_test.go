package registry

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// engineRegistry registers a built-in json and xml and a bridge okf_json and
// okf_xml, each claiming the extension the other does, the way the real
// registry looks with okapi-bridge installed.
func engineRegistry() *FormatRegistry {
	reg := NewFormatRegistry()
	regStubSig(reg, "json", "JSON", []string{"application/json"}, []string{".json"})
	regStubSig(reg, "xml", "XML", []string{"application/xml"}, []string{".xml"})
	for _, name := range []string{"json", "xml"} {
		reg.RegisterWriter(FormatID(name), func() format.DataFormatWriter { return &stubWriter{} })
	}
	for _, name := range []string{"okf_json", "okf_xml"} {
		reg.RegisterFormatInfo(FormatID(name), FormatInfo{
			DisplayName: name,
			Extensions:  []string{"." + name[len("okf_"):]},
			Source:      "okapi-bridge",
			HasReader:   true,
			HasWriter:   true,
		})
	}
	return reg
}

func TestResolve_NativeFirstByDefault(t *testing.T) {
	reg := engineRegistry()

	res, err := reg.Resolve("file.json", DetectOptions{ExtensionOnly: true})
	require.NoError(t, err)
	assert.Equal(t, FormatID("json"), res.Format)
	assert.Equal(t, EngineNative, res.Engine)
	assert.Equal(t, RuleNativeFirst, res.Rule)
	assert.Equal(t, []string{EngineNative, "okapi-bridge"}, res.Order)
	assert.Equal(t, []FormatID{"json", "okf_json"}, res.Candidates)
}

// A priority never crosses the engine boundary: the bridge format raised
// above every built-in still loses the extension to the built-in.
func TestResolve_PriorityStaysInsideEngine(t *testing.T) {
	reg := engineRegistry()
	reg.SetFormatPriority("okf_json", 1000)

	name, err := reg.Detect("file.json", DetectOptions{ExtensionOnly: true})
	require.NoError(t, err)
	assert.Equal(t, FormatID("json"), name)

	name, err = reg.Detect("file.json", DetectOptions{ExtensionOnly: true, PriorityOverrides: map[string]int{"okf_json": 1000}})
	require.NoError(t, err)
	assert.Equal(t, FormatID("json"), name)
}

func TestResolve_PluginServesWhatNoBuiltInClaims(t *testing.T) {
	reg := engineRegistry()
	reg.RegisterFormatInfo("okf_idml", FormatInfo{Extensions: []string{".idml"}, Source: "okapi-bridge", HasReader: true})

	res, err := reg.Resolve("book.idml", DetectOptions{ExtensionOnly: true})
	require.NoError(t, err)
	assert.Equal(t, FormatID("okf_idml"), res.Format)
	assert.Equal(t, "okapi-bridge", res.Engine)
	assert.Equal(t, RuleEngineOrder, res.Rule)
}

// The engine precedence, most specific first: a per-format pin, the host's
// override (--engine), the detection's preference (defaults.engine), the
// host's default (formats.engine), native.
func TestResolve_EnginePrecedence(t *testing.T) {
	reg := engineRegistry()
	opts := DetectOptions{ExtensionOnly: true}

	reg.SetDefaultEngine("okapi-bridge")
	res, err := reg.Resolve("file.json", opts)
	require.NoError(t, err)
	assert.Equal(t, FormatID("okf_json"), res.Format)
	assert.Equal(t, RuleEngineDefault, res.Rule)

	opts.Engine = EngineNative
	res, err = reg.Resolve("file.json", opts)
	require.NoError(t, err)
	assert.Equal(t, FormatID("json"), res.Format)
	assert.Equal(t, RuleEnginePreference, res.Rule)

	reg.SetEngineOverride("okapi-bridge")
	res, err = reg.Resolve("file.json", opts)
	require.NoError(t, err)
	assert.Equal(t, FormatID("okf_json"), res.Format)
	assert.Equal(t, RuleEngineOverride, res.Rule)

	opts.FormatEngines = map[string]string{"json": EngineNative}
	res, err = reg.Resolve("file.json", opts)
	require.NoError(t, err)
	assert.Equal(t, FormatID("json"), res.Format)
	assert.Equal(t, RuleFormatEngine, res.Rule)

	// The pin names an extension as well as a format, in either case.
	opts.FormatEngines = map[string]string{".JSON": EngineNative}
	res, err = reg.Resolve("file.json", opts)
	require.NoError(t, err)
	assert.Equal(t, FormatID("json"), res.Format)
	assert.Equal(t, RuleFormatEngine, res.Rule)

	// A pin for one format leaves every other extension to the engine order.
	opts.FormatEngines = map[string]string{"json": EngineNative}
	res, err = reg.Resolve("file.xml", opts)
	require.NoError(t, err)
	assert.Equal(t, FormatID("okf_xml"), res.Format)
	assert.Equal(t, RuleEngineOverride, res.Rule)

	// Clearing the override and the default restores native first.
	reg.SetEngineOverride("")
	reg.SetDefaultEngine("")
	res, err = reg.Resolve("file.xml", DetectOptions{ExtensionOnly: true})
	require.NoError(t, err)
	assert.Equal(t, FormatID("xml"), res.Format)
	assert.Equal(t, RuleNativeFirst, res.Rule)
}

// "built-in" and "native" name the same engine wherever an engine is named.
func TestResolve_NativeAliases(t *testing.T) {
	reg := engineRegistry()
	reg.SetDefaultEngine("okapi-bridge")
	for _, alias := range []string{EngineNative, SourceBuiltIn} {
		name, err := reg.Detect("file.json", DetectOptions{ExtensionOnly: true, Engine: alias})
		require.NoError(t, err)
		assert.Equal(t, FormatID("json"), name, alias)
	}
	assert.Equal(t, SourceBuiltIn, NormalizeEngine("native"))
	assert.Equal(t, SourceBuiltIn, NormalizeEngine("built-in"))
	assert.Equal(t, "okapi-bridge", NormalizeEngine(" okapi-bridge "))
	assert.Equal(t, EngineNative, EngineName(""))
	assert.Equal(t, EngineNative, EngineName(SourceBuiltIn))
	assert.Equal(t, "okapi-bridge", EngineName("okapi-bridge"))
}

// An engine the allowed sources leave out is never chosen, however it is
// preferred: a recipe that declares no plugin scopes every preference to the
// built-in formats.
func TestResolve_AllowedSourcesBoundEveryPreference(t *testing.T) {
	reg := engineRegistry()
	reg.SetEngineOverride("okapi-bridge")
	opts := DetectOptions{ExtensionOnly: true, AllowedSources: []string{SourceBuiltIn}, Engine: "okapi-bridge", FormatEngines: map[string]string{"json": "okapi-bridge"}}

	name, err := reg.Detect("file.json", opts)
	require.NoError(t, err)
	assert.Equal(t, FormatID("json"), name)
}

// EngineOrder ranks the engines after native: two plugins claiming an
// extension no built-in does resolve to the one ranked first, not the one
// that sorts first.
func TestResolve_EngineOrderRanksPlugins(t *testing.T) {
	reg := engineRegistry()
	reg.RegisterFormatInfo("okf_regex", FormatInfo{Extensions: []string{".srt"}, Source: "okapi-bridge", HasReader: true})
	reg.RegisterFormatInfo("av_srt", FormatInfo{Extensions: []string{".srt"}, Source: "kapi-av", HasReader: true})

	name, err := reg.Detect("clip.srt", DetectOptions{ExtensionOnly: true})
	require.NoError(t, err)
	assert.Equal(t, FormatID("av_srt"), name, "name order without a ranking")

	name, err = reg.Detect("clip.srt", DetectOptions{ExtensionOnly: true, EngineOrder: []string{"okapi-bridge", "kapi-av"}})
	require.NoError(t, err)
	assert.Equal(t, FormatID("okf_regex"), name)
}

// Within one engine, priority and then name decide, as before.
func TestResolve_PriorityDecidesWithinEngine(t *testing.T) {
	reg := NewFormatRegistry()
	for _, name := range []string{"okf_regex", "okf_vtt"} {
		reg.RegisterFormatInfo(FormatID(name), FormatInfo{Extensions: []string{".srt"}, Source: "okapi-bridge", HasReader: true})
	}
	name, err := reg.Detect("clip.srt", DetectOptions{ExtensionOnly: true})
	require.NoError(t, err)
	assert.Equal(t, FormatID("okf_regex"), name)

	name, err = reg.Detect("clip.srt", DetectOptions{ExtensionOnly: true, PriorityOverrides: map[string]int{"okf_vtt": 110}})
	require.NoError(t, err)
	assert.Equal(t, FormatID("okf_vtt"), name)
}

// The file's content chooses among the chosen engine's formats only. The
// built-in sniffer that names a format of another engine does not move the
// file there.
func TestResolve_ContentChoosesWithinEngine(t *testing.T) {
	reg := NewFormatRegistry()
	reg.RegisterReader("xml", func() format.DataFormatReader {
		return newStubReaderWithSig("xml", "XML", nil, []string{".xml"})
	}, format.FormatSignature{Extensions: []string{".xml"}}, "XML")
	reg.RegisterReader("android", func() format.DataFormatReader {
		return newStubReaderWithSig("android", "Android", nil, []string{".xml"})
	}, format.FormatSignature{
		Extensions: []string{".xml"},
		Sniff:      func(b []byte) bool { return len(b) > 0 && string(b[:min(len(b), 11)]) == "<resources>" },
	}, "Android")
	reg.RegisterFormatInfo("okf_xml", FormatInfo{Extensions: []string{".xml"}, Source: "okapi-bridge", HasReader: true})

	path := filepath.Join(t.TempDir(), "strings.xml")
	require.NoError(t, os.WriteFile(path, []byte(`<resources><string name="a">b</string></resources>`), 0o644))

	res, err := reg.Resolve(path, DetectOptions{})
	require.NoError(t, err)
	assert.Equal(t, FormatID("android"), res.Format)
	assert.True(t, res.Content)

	res, err = reg.Resolve(path, DetectOptions{Engine: "okapi-bridge"})
	require.NoError(t, err)
	assert.Equal(t, FormatID("okf_xml"), res.Format)
	assert.False(t, res.Content)
}

// A writer resolution takes only formats that write, and the reader's
// engine pins the writer: a bridge reader converting to .xml gets the bridge
// XML writer, a built-in reader the built-in one.
func TestWriterFormatFor_PinsTheReadersEngine(t *testing.T) {
	reg := engineRegistry()

	assert.Equal(t, FormatID("json"), reg.WriterFormatFor("json", "out.json"))
	assert.Equal(t, FormatID("xml"), reg.WriterFormatFor("json", "out.xml"))
	assert.Equal(t, FormatID("okf_json"), reg.WriterFormatFor("okf_json", "out.json"))
	assert.Equal(t, FormatID("okf_xml"), reg.WriterFormatFor("okf_json", "out.xml"))

	// Even with the bridge preferred for the run, a built-in reader keeps a
	// built-in writer.
	reg.SetEngineOverride("okapi-bridge")
	assert.Equal(t, FormatID("xml"), reg.WriterFormatFor("json", "out.xml"))

	// An engine with no writer for the extension yields to the order.
	reg.SetEngineOverride("")
	reg.RegisterFormatInfo("av_srt", FormatInfo{Extensions: []string{".srt"}, Source: "kapi-av", HasReader: true})
	assert.Equal(t, FormatID("xml"), reg.WriterFormatFor("av_srt", "out.xml"))

	// No writer claims the extension: the reader's format stays.
	assert.Equal(t, FormatID("json"), reg.WriterFormatFor("json", "out.idml"))
}

func TestResolve_WriterTakesOnlyWritableFormats(t *testing.T) {
	reg := NewFormatRegistry()
	regStubSig(reg, "mo", "MO", nil, []string{".mo"})
	reg.RegisterFormatInfo("okf_mo", FormatInfo{Extensions: []string{".mo"}, Source: "okapi-bridge", HasReader: true, HasWriter: true})

	name, err := reg.Detect("x.mo", DetectOptions{ExtensionOnly: true})
	require.NoError(t, err)
	assert.Equal(t, FormatID("mo"), name)

	name, err = reg.Detect("x.mo", DetectOptions{ExtensionOnly: true, Writer: true})
	require.NoError(t, err)
	assert.Equal(t, FormatID("okf_mo"), name)
}

// The lazy loader fires only when nothing claims the extension: a built-in
// format satisfying the request never starts a plugin daemon.
func TestResolve_OnMissNeverFiresWhenNativeSatisfies(t *testing.T) {
	reg := engineRegistry()
	fired := 0
	reg.SetOnMiss(func() { fired++ })

	for _, opts := range []DetectOptions{
		{ExtensionOnly: true},
		{ExtensionOnly: true, Engine: "okapi-bridge"},
		{ExtensionOnly: true, AllowedSources: []string{SourceBuiltIn}},
	} {
		_, err := reg.Detect("file.json", opts)
		require.NoError(t, err)
	}
	_, err := reg.NewReader("json")
	require.NoError(t, err)
	assert.Equal(t, 0, fired)

	_, err = reg.Detect("file.unknown", DetectOptions{ExtensionOnly: true})
	require.Error(t, err)
	assert.Equal(t, 1, fired, "an extension nothing claims asks the loader once")
}

func TestSourcesAndCheckEngine(t *testing.T) {
	reg := engineRegistry()
	reg.RegisterFormatInfo("av_srt", FormatInfo{Extensions: []string{".srt"}, Source: "kapi-av", HasReader: true})

	assert.Equal(t, []string{SourceBuiltIn, "kapi-av", "okapi-bridge"}, reg.Sources())
	assert.Equal(t, SourceBuiltIn, reg.FormatSource("json"))
	assert.Equal(t, "okapi-bridge", reg.FormatSource("okf_json"))
	assert.Equal(t, SourceBuiltIn, reg.FormatSource("nothing"))

	require.NoError(t, reg.CheckEngine(""))
	require.NoError(t, reg.CheckEngine("native"))
	require.NoError(t, reg.CheckEngine("built-in"))
	require.NoError(t, reg.CheckEngine("okapi-bridge"))
	err := reg.CheckEngine("okapi")
	require.ErrorIs(t, err, ErrNoEngine)
	assert.Contains(t, err.Error(), "native, kapi-av, okapi-bridge")
}
