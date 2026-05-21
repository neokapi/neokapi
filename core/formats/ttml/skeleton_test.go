package ttml_test

import (
	"bytes"
	"os"
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/formats/ttml"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Okapi's TTMLSkeletonWriterTest exercises net.sf.okapi.filters.ttml.TTMLSkeletonWriter,
// a caption-resplitting skeleton writer that, per <p> caption, re-wraps the
// translated text to a configured max chars/line and max lines/caption,
// re-inserts <br/> separators (escaped or literal per escapeBrMode),
// optionally splits words, and redistributes the begin/end timecodes across
// the resulting overflow captions. neokapi's native TTML writer is by design a
// content-replacement skeleton writer (byte-exact roundtrip; it only swaps the
// inner text of each <p> with the translated block text). It has no caption
// model, no line/caption wrapping, no word-splitting, and no timecode
// redistribution — the splitting config fields exist but are not consumed by
// the writer. These contracts are therefore not applicable to the native
// reader/writer and are skip-classified rather than fake-passed (#611).
//
// okapi-skip: TTMLSkeletonWriterTest#testProcessTextUnit — native TTML writer is content-replacement only; no caption-resplitting skeleton writer
// okapi-skip: TTMLSkeletonWriterTest#testProcessTextUnitNonEscapeBrMode — writer does not re-insert <br/> separators (escaped or literal) when wrapping captions
// okapi-skip: TTMLSkeletonWriterTest#testProcessTextUnitSplitLines — writer does not wrap caption text to maxCharsPerLine with <br/>
// okapi-skip: TTMLSkeletonWriterTest#testProcessTextUnitSplitCaptionsLines — writer does not split a caption into multiple <p> on line overflow
// okapi-skip: TTMLSkeletonWriterTest#testProcessTextUnitSplitCaptionsLineOverflow — writer does not split words/lines or redistribute timecodes on overflow
// okapi-skip: TTMLSkeletonWriterTest#testProcessTextUnitSplitCaptionsCaptionOverflow — writer does not redistribute begin/end timecodes across overflow captions
// okapi-skip: TTMLSkeletonWriterTest#testProcessTextUnitWithCodes — writer has no InlineCodeFinder-based code re-emission during caption resplitting

func snippetRoundtripWithSkeleton(t *testing.T, input string) string {
	t.Helper()
	ctx := t.Context()

	reader := ttml.NewReader()
	writer := ttml.NewWriter()

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

func TestSkeletonStore_ByteExact_SimpleTTML(t *testing.T) {
	input := `<?xml version="1.0" encoding="UTF-8"?>
<tt xml:lang="en" xmlns="http://www.w3.org/ns/ttml">
  <body><div>
    <p begin="00:00:01.000" end="00:00:04.000">Hello world</p>
    <p begin="00:00:05.000" end="00:00:08.000">Second subtitle</p>
  </div></body>
</tt>`
	output := snippetRoundtripWithSkeleton(t, input)
	assert.Equal(t, input, output, "simple TTML roundtrip should be byte-exact")
}

func TestSkeletonStore_ByteExact_WithXMLID(t *testing.T) {
	input := `<?xml version="1.0" encoding="UTF-8"?>
<tt xml:lang="en" xmlns="http://www.w3.org/ns/ttml">
  <body><div>
    <p xml:id="myCaption" begin="00:00:01.000" end="00:00:04.000">Hello</p>
  </div></body>
</tt>`
	output := snippetRoundtripWithSkeleton(t, input)
	assert.Equal(t, input, output, "TTML with xml:id roundtrip should be byte-exact")
}

func TestSkeletonStore_ByteExact_WithStyling(t *testing.T) {
	input := `<?xml version="1.0" encoding="UTF-8"?>
<tt xml:lang="en" xmlns="http://www.w3.org/ns/ttml" xmlns:tts="http://www.w3.org/ns/ttml#styling">
  <head>
    <styling>
      <style xml:id="defaultStyle" tts:fontFamily="Arial" tts:fontSize="100%" tts:textAlign="center"/>
    </styling>
    <layout>
      <region xml:id="bottom" tts:origin="10% 80%" tts:extent="80% 20%"/>
    </layout>
  </head>
  <body><div>
    <p xml:id="s1" begin="00:00:01.000" end="00:00:04.000" region="bottom" tts:textAlign="center">First subtitle.</p>
  </div></body>
</tt>`
	output := snippetRoundtripWithSkeleton(t, input)
	assert.Equal(t, input, output, "TTML with styling roundtrip should be byte-exact")
}

func TestSkeletonStore_ByteExact_MultipleDivs(t *testing.T) {
	input := `<?xml version="1.0" encoding="UTF-8"?>
<tt xml:lang="en" xmlns="http://www.w3.org/ns/ttml">
  <body>
    <div>
      <p begin="00:00:01.000" end="00:00:04.000">First div subtitle.</p>
    </div>
    <div>
      <p begin="00:00:05.000" end="00:00:08.000">Second div subtitle.</p>
    </div>
  </body>
</tt>`
	output := snippetRoundtripWithSkeleton(t, input)
	assert.Equal(t, input, output, "TTML with multiple divs roundtrip should be byte-exact")
}

func TestSkeletonStore_ByteExact_EmptyCaption(t *testing.T) {
	input := `<?xml version="1.0" encoding="UTF-8"?>
<tt xml:lang="en" xmlns="http://www.w3.org/ns/ttml">
  <body><div>
    <p xml:id="s1" begin="00:00:00.897" end="00:00:05.263"></p>
    <p xml:id="s2" begin="00:00:05.430" end="00:00:08.730">I am so excited to be with you.</p>
  </div></body>
</tt>`
	output := snippetRoundtripWithSkeleton(t, input)
	assert.Equal(t, input, output, "TTML with empty caption roundtrip should be byte-exact")
}

func TestSkeletonStore_ByteExact_TestdataSimple(t *testing.T) {
	data, err := os.ReadFile("testdata/simple.ttml")
	require.NoError(t, err)
	input := string(data)
	output := snippetRoundtripWithSkeleton(t, input)
	assert.Equal(t, input, output, "simple.ttml roundtrip should be byte-exact")
}

func TestSkeletonStore_PreservesFormatting(t *testing.T) {
	// Various indentation and whitespace patterns
	input := `<?xml version="1.0" encoding="UTF-8"?>
<tt xml:lang="en" xmlns="http://www.w3.org/ns/ttml">
  <body>
    <div>
      <p begin="00:00:01.000" end="00:00:04.000">Hello world</p>
      <p begin="00:00:05.000" end="00:00:08.000">Goodbye world</p>
    </div>
  </body>
</tt>
`
	output := snippetRoundtripWithSkeleton(t, input)
	assert.Equal(t, input, output, "formatting should be preserved byte-exact")
}

func TestSkeletonStore_WithTranslation(t *testing.T) {
	input := `<?xml version="1.0" encoding="UTF-8"?>
<tt xml:lang="en" xmlns="http://www.w3.org/ns/ttml">
  <body><div>
    <p begin="00:00:01.000" end="00:00:04.000">Hello</p>
    <p begin="00:00:05.000" end="00:00:08.000">World</p>
  </div></body>
</tt>`
	ctx := t.Context()
	locale := model.LocaleID("fr")

	reader := ttml.NewReader()
	writer := ttml.NewWriter()

	store, err := format.NewSkeletonStore()
	require.NoError(t, err)
	defer store.Close()
	reader.SetSkeletonStore(store)
	writer.SetSkeletonStore(store)

	err = reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	for _, p := range parts {
		if p.Type == model.PartBlock {
			b := p.Resource.(*model.Block)
			switch b.SourceText() {
			case "Hello":
				b.Targets[locale] = []*model.Segment{{ID: "s1", Runs: []model.Run{{Text: &model.TextRun{Text: "Bonjour"}}}}}
			case "World":
				b.Targets[locale] = []*model.Segment{{ID: "s1", Runs: []model.Run{{Text: &model.TextRun{Text: "Monde"}}}}}
			}
		}
	}

	var buf bytes.Buffer
	writer.SetLocale(locale)
	require.NoError(t, writer.SetOutputWriter(&buf))

	ch := testutil.PartsToChannel(parts)
	require.NoError(t, writer.Write(ctx, ch))
	writer.Close()

	expected := `<?xml version="1.0" encoding="UTF-8"?>
<tt xml:lang="en" xmlns="http://www.w3.org/ns/ttml">
  <body><div>
    <p begin="00:00:01.000" end="00:00:04.000">Bonjour</p>
    <p begin="00:00:05.000" end="00:00:08.000">Monde</p>
  </div></body>
</tt>`
	assert.Equal(t, expected, buf.String())
}
