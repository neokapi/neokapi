package xliff2_test

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/formats/xliff2"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// xliff2xFixture returns a minimal, single-unit XLIFF 2.x document using the
// given version string and its matching OASIS document namespace.
//
// Per OASIS schemas, 2.0 and 2.1 share the namespace `...:document:2.0`
// (the 2.1 spec ships `xliff_core_2.0.xsd` as its core schema, only the
// `version` attribute distinguishes them). 2.2 uses a new namespace.
func xliff2xFixture(version string) string {
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<xliff version="%s" xmlns="%s" srcLang="en" trgLang="fr">
  <file id="f1">
    <unit id="u1">
      <segment id="s1">
        <source>Hello</source>
        <target>Bonjour</target>
      </segment>
    </unit>
  </file>
</xliff>`, version, xliff2.NamespaceForVersion(version))
}

// TestRead_AcceptsXliff2xNamespaces verifies that the reader accepts the 2.0,
// 2.1 and 2.2 OASIS document namespaces as a compatible family.
func TestRead_AcceptsXliff2xNamespaces(t *testing.T) {
	for _, version := range []string{"2.0", "2.1", "2.2"} {
		t.Run(version, func(t *testing.T) {
			ctx := t.Context()
			reader := xliff2.NewReader()
			err := reader.Open(ctx, testutil.RawDocFromString(xliff2xFixture(version), model.LocaleEnglish))
			require.NoError(t, err)
			defer reader.Close()

			blocks := testutil.CollectBlocks(t, reader.Read(ctx))
			require.Len(t, blocks, 1)
			assert.Equal(t, "Hello", blocks[0].SourceText())
			assert.True(t, blocks[0].HasTarget(model.LocaleFrench))
			assert.Equal(t, "Bonjour", blocks[0].TargetText(model.LocaleFrench))
		})
	}
}

// TestRead_SignatureSniffsXliff2x verifies that the format's Sniff detector
// accepts all three 2.x namespaces.
func TestRead_SignatureSniffsXliff2x(t *testing.T) {
	sig := xliff2.NewReader().Signature()
	require.NotNil(t, sig.Sniff)

	for _, version := range []string{"2.0", "2.1", "2.2"} {
		t.Run(version, func(t *testing.T) {
			assert.True(t, sig.Sniff([]byte(xliff2xFixture(version))),
				"Sniff should detect XLIFF %s as an XLIFF 2.x document", version)
		})
	}

	// A completely unrelated document must not be detected as XLIFF 2.x.
	assert.False(t, sig.Sniff([]byte(`<?xml version="1.0"?><foo/>`)))
}

// TestRoundTrip_ByteExact_AllVersions verifies that the skeleton-based
// byte-exact roundtrip preserves the input version/namespace verbatim for
// all three 2.x versions.
func TestRoundTrip_ByteExact_AllVersions(t *testing.T) {
	for _, version := range []string{"2.0", "2.1", "2.2"} {
		t.Run(version, func(t *testing.T) {
			input := xliff2xFixture(version)
			output := snippetRoundtripWithSkeleton(t, input)
			assert.Equal(t, input, output,
				"byte-exact roundtrip should preserve XLIFF %s input verbatim", version)
		})
	}
}

// TestRoundTrip_DomPreservesInputVersion verifies that the DOM roundtrip (no
// skeleton store) preserves the input document's version and matching
// namespace when no explicit writer version override is set.
func TestRoundTrip_DomPreservesInputVersion(t *testing.T) {
	for _, version := range []string{"2.0", "2.1", "2.2"} {
		t.Run(version, func(t *testing.T) {
			ctx := t.Context()
			reader := xliff2.NewReader()
			err := reader.Open(ctx, testutil.RawDocFromString(xliff2xFixture(version), model.LocaleEnglish))
			require.NoError(t, err)
			parts := testutil.CollectParts(t, reader.Read(ctx))
			reader.Close()

			var buf bytes.Buffer
			writer := xliff2.NewWriter()
			require.NoError(t, writer.SetOutputWriter(&buf))

			ch := testutil.PartsToChannel(parts)
			require.NoError(t, writer.Write(ctx, ch))

			output := buf.String()
			assert.Contains(t, output, fmt.Sprintf(`version="%s"`, version),
				"DOM roundtrip should preserve the input version attribute")
			assert.Contains(t, output, xliff2.NamespaceForVersion(version),
				"DOM roundtrip should preserve the matching XLIFF %s namespace", version)
		})
	}
}

// TestWriter_DefaultsTo2_2 verifies that a writer with no input layer and no
// explicit version override emits XLIFF 2.2 (the latest OASIS version).
func TestWriter_DefaultsTo2_2(t *testing.T) {
	ctx := t.Context()

	block := &model.Block{
		ID:           "u1",
		Translatable: true,
		Source:       []model.Run{{Text: &model.TextRun{Text: "Hello"}}},
	}
	block.SetSegmentation(nil, []model.Span{
		{ID: "s1", Range: model.RunRange{StartRun: 0, EndRun: 1}},
	})
	parts := []*model.Part{
		{Type: model.PartBlock, Resource: block},
	}

	var buf bytes.Buffer
	writer := xliff2.NewWriter()
	require.NoError(t, writer.SetOutputWriter(&buf))

	ch := testutil.PartsToChannel(parts)
	require.NoError(t, writer.Write(ctx, ch))

	output := buf.String()
	assert.Contains(t, output, `version="2.2"`,
		"writer with no input and no override should default to XLIFF 2.2")
	assert.Contains(t, output, "urn:oasis:names:tc:xliff:document:2.2",
		"writer default namespace must match the 2.2 version")
}

// TestWriter_VersionOverride verifies that SetVersion / Config.Version
// selects the emitted version and matching namespace for each 2.x value.
func TestWriter_VersionOverride(t *testing.T) {
	for _, version := range []string{"2.0", "2.1", "2.2"} {
		t.Run(version, func(t *testing.T) {
			ctx := t.Context()
			// Start from a 2.0 input to prove the override wins over the
			// preserved input version.
			reader := xliff2.NewReader()
			err := reader.Open(ctx, testutil.RawDocFromString(xliff2xFixture("2.0"), model.LocaleEnglish))
			require.NoError(t, err)
			parts := testutil.CollectParts(t, reader.Read(ctx))
			reader.Close()

			var buf bytes.Buffer
			writer := xliff2.NewWriter()
			require.NoError(t, writer.SetOutputWriter(&buf))
			require.NoError(t, writer.SetVersion(version))

			ch := testutil.PartsToChannel(parts)
			require.NoError(t, writer.Write(ctx, ch))

			output := buf.String()
			assert.Contains(t, output, fmt.Sprintf(`version="%s"`, version),
				"writer override should emit version=%q", version)
			assert.Contains(t, output, xliff2.NamespaceForVersion(version),
				"writer override should emit matching %s namespace", version)
		})
	}
}

// TestWriter_SetVersionRejectsUnknown verifies SetVersion refuses values
// outside the supported 2.x family.
func TestWriter_SetVersionRejectsUnknown(t *testing.T) {
	writer := xliff2.NewWriter()
	require.Error(t, writer.SetVersion("1.2"))
	require.Error(t, writer.SetVersion("2.3"))
	require.Error(t, writer.SetVersion("nonsense"))

	// Empty string is valid (means auto-resolve).
	require.NoError(t, writer.SetVersion(""))
	for _, v := range xliff2.SupportedXLIFFVersions {
		require.NoError(t, writer.SetVersion(v))
	}
}

// TestConfig_ApplyMapVersion verifies version round-trips through ApplyMap
// and that invalid values are rejected.
func TestConfig_ApplyMapVersion(t *testing.T) {
	c := &xliff2.Config{}
	c.Reset()
	require.NoError(t, c.ApplyMap(map[string]any{"version": "2.1"}))
	assert.Equal(t, "2.1", c.Version)

	require.NoError(t, c.ApplyMap(map[string]any{"version": ""}))
	assert.Equal(t, "", c.Version)

	require.Error(t, c.ApplyMap(map[string]any{"version": "1.2"}))
	require.Error(t, c.ApplyMap(map[string]any{"version": 22}))
}

// TestRead_UnknownAttrsRoundTripDom verifies that unknown 2.x attributes on
// the root <xliff> element survive a DOM round-trip (non-skeleton path).
func TestRead_UnknownAttrsRoundTripDom(t *testing.T) {
	input := `<?xml version="1.0" encoding="UTF-8"?>
<xliff version="2.2" xmlns="urn:oasis:names:tc:xliff:document:2.2" xmlns:xlf="urn:oasis:names:tc:xliff:matches:2.0" xml:lang="en" srcLang="en" trgLang="fr">
  <file id="f1">
    <unit id="u1">
      <segment id="s1"><source>Hi</source><target>Salut</target></segment>
    </unit>
  </file>
</xliff>`
	ctx := t.Context()

	reader := xliff2.NewReader()
	require.NoError(t, reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish)))
	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	var buf bytes.Buffer
	writer := xliff2.NewWriter()
	require.NoError(t, writer.SetOutputWriter(&buf))
	ch := testutil.PartsToChannel(parts)
	require.NoError(t, writer.Write(ctx, ch))

	output := buf.String()
	// The matches-namespace declaration and xml:lang attribute must survive.
	assert.True(t,
		strings.Contains(output, "urn:oasis:names:tc:xliff:matches:2.0"),
		"unknown xlf matches namespace should round-trip (got: %s)", output)
	assert.True(t,
		strings.Contains(output, `xml:lang="en"`) || strings.Contains(output, `lang="en"`),
		"unknown xml:lang attribute should round-trip (got: %s)", output)
}

// TestRead_UnknownAttrsRoundTripSkeleton verifies that unknown 2.x attributes
// survive a byte-exact skeleton round-trip.
func TestRead_UnknownAttrsRoundTripSkeleton(t *testing.T) {
	input := `<?xml version="1.0" encoding="UTF-8"?>
<xliff version="2.2" xmlns="urn:oasis:names:tc:xliff:document:2.2" xmlns:xlf="urn:oasis:names:tc:xliff:matches:2.0" srcLang="en" trgLang="fr">
  <file id="f1">
    <unit id="u1" xlf:custom="keep-me">
      <segment id="s1"><source>Hi</source><target>Salut</target></segment>
    </unit>
  </file>
</xliff>`
	output := snippetRoundtripWithSkeleton(t, input)
	assert.Equal(t, input, output,
		"byte-exact roundtrip must preserve unknown attributes verbatim")
}

// TestRead_LayerCarriesVersionProperty verifies the reader stores the input
// document's version attribute on the emitted Layer's Properties map so the
// writer can default to the input version on roundtrip.
func TestRead_LayerCarriesVersionProperty(t *testing.T) {
	for _, version := range []string{"2.0", "2.1", "2.2"} {
		t.Run(version, func(t *testing.T) {
			ctx := t.Context()
			reader := xliff2.NewReader()
			require.NoError(t, reader.Open(ctx, testutil.RawDocFromString(xliff2xFixture(version), model.LocaleEnglish)))
			defer reader.Close()

			parts := testutil.CollectParts(t, reader.Read(ctx))
			var layer *model.Layer
			for _, p := range parts {
				if p.Type == model.PartLayerStart {
					layer = p.Resource.(*model.Layer)
					break
				}
			}
			require.NotNil(t, layer, "reader should emit a layer start")
			assert.Equal(t, version, layer.Properties["xliff-version"])
		})
	}
}

// Ensure the skeleton store interface is still satisfied by the version
// tests above — compile-time check via this unused reference.
var _ = format.SkeletonText
