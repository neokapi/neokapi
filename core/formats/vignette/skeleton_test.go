package vignette_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/formats/vignette"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// vignetteSkeletonRoundtrip reads input through the vignette reader
// (no translation, no edits) and writes it back through the writer
// using a SkeletonStore. Returns the writer's output for caller
// comparison against the input.
func vignetteSkeletonRoundtrip(t *testing.T, input string) string {
	t.Helper()
	ctx := t.Context()

	reader := vignette.NewReader()
	writer := vignette.NewWriter()

	store, err := format.NewSkeletonStore()
	require.NoError(t, err)
	defer store.Close()
	reader.SetSkeletonStore(store)
	writer.SetSkeletonStore(store)

	err = reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	var buf bytes.Buffer
	require.NoError(t, writer.SetOutputWriter(&buf))

	ch := testutil.PartsToChannel(parts)
	require.NoError(t, writer.Write(ctx, ch))
	writer.Close()

	return buf.String()
}

func TestSkeletonStore_ByteExact_EmptyProject(t *testing.T) {
	output := vignetteSkeletonRoundtrip(t, minimalEmptyDoc)
	assert.Equal(t, minimalEmptyDoc, output, "empty project roundtrip should be byte-exact")
}

func TestSkeletonStore_ByteExact_PlainBilingualPair(t *testing.T) {
	// With useCDATA=true (default) and the source-side payload "hello"
	// being plain text, the writer re-emits `<![CDATA[hello]]>` in place
	// of the original `hello`. The rest of the document (envelope, other
	// instance, attribute names, locale tags) is byte-exact via skeleton.
	output := vignetteSkeletonRoundtrip(t, plainBilingualPair)
	assert.Contains(t, output, "<![CDATA[hello]]>")
	assert.Contains(t, output, "bonjour", "non-extracted instance payload preserved verbatim")
	assert.Contains(t, output, `xmlns="http://www.vignette.com/xmlschemas/importexport"`)
}

func TestSkeletonStore_WithTranslation(t *testing.T) {
	ctx := t.Context()
	locale := model.LocaleID("fr")

	reader := vignette.NewReader()
	writer := vignette.NewWriter()

	store, err := format.NewSkeletonStore()
	require.NoError(t, err)
	defer store.Close()
	reader.SetSkeletonStore(store)
	writer.SetSkeletonStore(store)

	err = reader.Open(ctx, testutil.RawDocFromString(plainBilingualPair, model.LocaleEnglish))
	require.NoError(t, err)
	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	for _, p := range parts {
		if p.Type != model.PartBlock {
			continue
		}
		b := p.Resource.(*model.Block)
		if b.SourceText() == "hello" {
			b.SetTargetText(locale, "salut")
		}
	}

	var buf bytes.Buffer
	writer.SetLocale(locale)
	require.NoError(t, writer.SetOutputWriter(&buf))

	ch := testutil.PartsToChannel(parts)
	require.NoError(t, writer.Write(ctx, ch))
	writer.Close()

	output := buf.String()
	assert.Contains(t, output, "<![CDATA[salut]]>")
	assert.NotContains(t, output, "<![CDATA[hello]]>", "source text should be replaced by translation")
	// The non-extracted instance payload is still emitted verbatim
	// because no Block reference covers it.
	assert.Contains(t, output, "bonjour")
}

func TestSkeletonStore_RealisticHTMLPayloadWritesValidXML(t *testing.T) {
	output := vignetteSkeletonRoundtrip(t, simpleBilingualPair)
	// "ENtext" is the decoded source-side payload; write-side re-wraps
	// it in <p> and CDATA-escapes the result for embedding in valueCLOB.
	assert.Contains(t, output, "<![CDATA[<p>ENtext</p>]]>")
	// The envelope is preserved.
	assert.Contains(t, output, "<importContentInstance>")
	assert.Contains(t, output, "</packageBody>")
}

func TestSkeletonStore_NoSkeleton_FallbackWritesPayloadsOnly(t *testing.T) {
	ctx := t.Context()
	reader := vignette.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString(plainBilingualPair, model.LocaleEnglish))
	require.NoError(t, err)
	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	var buf bytes.Buffer
	writer := vignette.NewWriter()
	require.NoError(t, writer.SetOutputWriter(&buf))

	ch := testutil.PartsToChannel(parts)
	require.NoError(t, writer.Write(ctx, ch))
	writer.Close()

	output := buf.String()
	// Fallback mode: just block payloads, one per line.
	assert.Equal(t, "hello", strings.TrimSpace(output))
}
