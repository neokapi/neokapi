package registry

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// indexRegistry is engineRegistry with a built-in format whose extension no
// plugin claims (markdown) and a plugin format no built-in claims (dtd).
func indexRegistry() *FormatRegistry {
	reg := engineRegistry()
	regStubSig(reg, "markdown", "Markdown", []string{"text/markdown"}, []string{".md", ".markdown"})
	reg.RegisterFormatInfo("okf_dtd", FormatInfo{
		DisplayName: "DTD", Extensions: []string{".dtd"}, Source: "okapi-bridge", HasReader: true,
	})
	return reg
}

func TestExtensionEngines(t *testing.T) {
	reg := indexRegistry()
	index := reg.ExtensionEngines()
	assert.Equal(t, []string{EngineNative, "okapi-bridge"}, index[".json"])
	assert.Equal(t, []string{EngineNative, "okapi-bridge"}, index[".xml"])
	assert.Equal(t, []string{EngineNative}, index[".md"])
	assert.Equal(t, []string{"okapi-bridge"}, index[".dtd"])
}

func TestSharedExtensions(t *testing.T) {
	reg := indexRegistry()
	assert.Equal(t, []string{".json"}, reg.SharedExtensions("json"))
	assert.Equal(t, []string{".json"}, reg.SharedExtensions("okf_json"))
	assert.Empty(t, reg.SharedExtensions("markdown"))
	assert.Empty(t, reg.SharedExtensions("okf_dtd"))
	assert.Nil(t, reg.SharedExtensions("no-such-format"))
}

func TestEngineFormat(t *testing.T) {
	reg := indexRegistry()

	// The format of another engine for the same extensions.
	got, err := reg.EngineFormat("json", "okapi-bridge")
	require.NoError(t, err)
	assert.Equal(t, FormatID("okf_json"), got)
	got, err = reg.EngineFormat("okf_json", EngineNative)
	require.NoError(t, err)
	assert.Equal(t, FormatID("json"), got)

	// A format already of the engine names itself, under either spelling of
	// the built-in engine.
	for _, engine := range []string{EngineNative, SourceBuiltIn} {
		got, err = reg.EngineFormat("json", engine)
		require.NoError(t, err)
		assert.Equal(t, FormatID("json"), got)
	}

	// No engine named: the name as given.
	got, err = reg.EngineFormat("okf_json", "")
	require.NoError(t, err)
	assert.Equal(t, FormatID("okf_json"), got)

	// A name no format carries reads as an extension.
	got, err = reg.EngineFormat("dtd", "okapi-bridge")
	require.NoError(t, err)
	assert.Equal(t, FormatID("okf_dtd"), got)

	// An engine with nothing for the extensions is an error naming both.
	_, err = reg.EngineFormat("markdown", "okapi-bridge")
	require.ErrorIs(t, err, ErrNoEngineFormat)
	assert.Contains(t, err.Error(), "okapi-bridge")
	assert.Contains(t, err.Error(), ".md")
	_, err = reg.EngineFormat("okf_dtd", EngineNative)
	require.ErrorIs(t, err, ErrNoEngineFormat)
}

// Among several formats of the engine claiming the extensions, the one
// claiming most of them wins, then priority, then name.
func TestEngineFormat_PrefersTheClosestClaimant(t *testing.T) {
	reg := indexRegistry()
	reg.RegisterFormatInfo("okf_md_lite", FormatInfo{
		DisplayName: "lite", Extensions: []string{".md"}, Source: "okapi-bridge", HasReader: true,
	})
	reg.RegisterFormatInfo("okf_markdown", FormatInfo{
		DisplayName: "full", Extensions: []string{".md", ".markdown"}, Source: "okapi-bridge", HasReader: true,
	})
	got, err := reg.EngineFormat("markdown", "okapi-bridge")
	require.NoError(t, err)
	assert.Equal(t, FormatID("okf_markdown"), got)

	reg.RegisterFormatInfo("okf_md_alt", FormatInfo{
		DisplayName: "alt", Extensions: []string{".md", ".markdown"}, Source: "okapi-bridge", HasReader: true,
	})
	got, err = reg.EngineFormat("markdown", "okapi-bridge")
	require.NoError(t, err)
	assert.Equal(t, FormatID("okf_markdown"), got, "name order breaks the tie")
	reg.SetFormatPriority("okf_md_alt", 500)
	got, err = reg.EngineFormat("markdown", "okapi-bridge")
	require.NoError(t, err)
	assert.Equal(t, FormatID("okf_md_alt"), got, "priority ranks above name order")
}
