package icml_test

import (
	"bytes"
	"context"
	"os"
	"testing"

	"github.com/gokapi/gokapi/core/format"
	"github.com/gokapi/gokapi/core/formats/icml"
	"github.com/gokapi/gokapi/core/model"
	"github.com/gokapi/gokapi/core/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func icmlSkeletonRoundtrip(t *testing.T, input string) string {
	t.Helper()
	ctx := context.Background()

	reader := icml.NewReader()
	writer := icml.NewWriter()

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

// ---------------------------------------------------------------------------
// Byte-exact roundtrip tests
// ---------------------------------------------------------------------------

func TestSkeletonStore_ByteExact_MinimalICML(t *testing.T) {
	input := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<?aid style="50" type="snippet" readerVersion="6.0" featureSet="513" product="8.0(370)" ?>
<Document DOMVersion="8.0">
  <Story Self="story1" AppliedTOCStyle="n" TrackChanges="false" StoryTitle="$ID/" AppliedNamedGrid="n">
    <StoryPreference OpticalMarginAlignment="false" OpticalMarginSize="12" FrameType="TextFrameType" StoryOrientation="Horizontal" StoryDirection="LeftToRightDirection" />
    <ParagraphStyleRange AppliedParagraphStyle="ParagraphStyle/$ID/NormalParagraphStyle">
      <CharacterStyleRange AppliedCharacterStyle="CharacterStyle/$ID/[No character style]">
        <Content>Hello World</Content>
      </CharacterStyleRange>
    </ParagraphStyleRange>
  </Story>
</Document>`
	output := icmlSkeletonRoundtrip(t, input)
	assert.Equal(t, input, output, "minimal ICML roundtrip should be byte-exact")
}

func TestSkeletonStore_ByteExact_MinimalICMLFile(t *testing.T) {
	data, err := os.ReadFile("testdata/minimal.icml")
	require.NoError(t, err)
	input := string(data)
	output := icmlSkeletonRoundtrip(t, input)
	assert.Equal(t, input, output, "minimal.icml file roundtrip should be byte-exact")
}

func TestSkeletonStore_ByteExact_MultipleParagraphs(t *testing.T) {
	input := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Document DOMVersion="8.0">
  <Story Self="story1">
    <ParagraphStyleRange AppliedParagraphStyle="ParagraphStyle/Body">
      <CharacterStyleRange AppliedCharacterStyle="CharacterStyle/$ID/[No character style]">
        <Content>Hello</Content>
      </CharacterStyleRange>
    </ParagraphStyleRange>
    <ParagraphStyleRange AppliedParagraphStyle="ParagraphStyle/Body">
      <CharacterStyleRange AppliedCharacterStyle="CharacterStyle/$ID/[No character style]">
        <Content>World</Content>
      </CharacterStyleRange>
    </ParagraphStyleRange>
  </Story>
</Document>`
	output := icmlSkeletonRoundtrip(t, input)
	assert.Equal(t, input, output, "multiple paragraphs roundtrip should be byte-exact")
}

func TestSkeletonStore_ByteExact_MultipleCharacterRanges(t *testing.T) {
	input := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Document DOMVersion="8.0">
  <Story Self="story1">
    <ParagraphStyleRange AppliedParagraphStyle="ParagraphStyle/Body">
      <CharacterStyleRange AppliedCharacterStyle="CharacterStyle/$ID/[No character style]">
        <Content>Hello </Content>
      </CharacterStyleRange>
      <CharacterStyleRange AppliedCharacterStyle="CharacterStyle/Italic">
        <Content>beautiful</Content>
      </CharacterStyleRange>
      <CharacterStyleRange AppliedCharacterStyle="CharacterStyle/$ID/[No character style]">
        <Content> world</Content>
      </CharacterStyleRange>
    </ParagraphStyleRange>
  </Story>
</Document>`
	output := icmlSkeletonRoundtrip(t, input)
	assert.Equal(t, input, output, "multiple character ranges roundtrip should be byte-exact")
}

func TestSkeletonStore_ByteExact_Properties(t *testing.T) {
	input := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Document DOMVersion="8.0">
  <Story Self="story1">
    <ParagraphStyleRange AppliedParagraphStyle="ParagraphStyle/Body">
      <CharacterStyleRange AppliedCharacterStyle="CharacterStyle/$ID/[No character style]">
        <Properties>
          <Leading type="unit">14</Leading>
        </Properties>
        <Content>Visible text only</Content>
      </CharacterStyleRange>
    </ParagraphStyleRange>
  </Story>
</Document>`
	output := icmlSkeletonRoundtrip(t, input)
	assert.Equal(t, input, output, "properties element roundtrip should be byte-exact")
}

func TestSkeletonStore_ByteExact_Entities(t *testing.T) {
	input := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Document DOMVersion="8.0">
  <Story Self="story1">
    <ParagraphStyleRange AppliedParagraphStyle="ParagraphStyle/Body">
      <CharacterStyleRange AppliedCharacterStyle="CharacterStyle/$ID/[No character style]">
        <Content>Cats &amp; Dogs</Content>
      </CharacterStyleRange>
    </ParagraphStyleRange>
  </Story>
</Document>`
	output := icmlSkeletonRoundtrip(t, input)
	assert.Equal(t, input, output, "entities roundtrip should be byte-exact")
}

func TestSkeletonStore_ByteExact_NoStory(t *testing.T) {
	input := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Document DOMVersion="8.0">
  <RootCharacterStyleGroup Self="rootCSG">
  </RootCharacterStyleGroup>
</Document>`
	output := icmlSkeletonRoundtrip(t, input)
	assert.Equal(t, input, output, "document without Story should roundtrip byte-exact")
}

// ---------------------------------------------------------------------------
// Translation tests
// ---------------------------------------------------------------------------

func TestSkeletonStore_WithTranslation(t *testing.T) {
	input := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Document DOMVersion="8.0">
  <Story Self="story1">
    <ParagraphStyleRange AppliedParagraphStyle="ParagraphStyle/Body">
      <CharacterStyleRange AppliedCharacterStyle="CharacterStyle/$ID/[No character style]">
        <Content>Hello</Content>
      </CharacterStyleRange>
    </ParagraphStyleRange>
    <ParagraphStyleRange AppliedParagraphStyle="ParagraphStyle/Body">
      <CharacterStyleRange AppliedCharacterStyle="CharacterStyle/$ID/[No character style]">
        <Content>World</Content>
      </CharacterStyleRange>
    </ParagraphStyleRange>
  </Story>
</Document>`
	ctx := context.Background()
	locale := model.LocaleID("fr")

	reader := icml.NewReader()
	writer := icml.NewWriter()

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
				b.Targets[locale] = []*model.Segment{{ID: "s1", Content: model.NewFragment("Bonjour")}}
			case "World":
				b.Targets[locale] = []*model.Segment{{ID: "s1", Content: model.NewFragment("Monde")}}
			}
		}
	}

	var buf bytes.Buffer
	writer.SetLocale(locale)
	require.NoError(t, writer.SetOutputWriter(&buf))

	ch := testutil.PartsToChannel(parts)
	require.NoError(t, writer.Write(ctx, ch))
	writer.Close()

	expected := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Document DOMVersion="8.0">
  <Story Self="story1">
    <ParagraphStyleRange AppliedParagraphStyle="ParagraphStyle/Body">
      <CharacterStyleRange AppliedCharacterStyle="CharacterStyle/$ID/[No character style]">
        <Content>Bonjour</Content>
      </CharacterStyleRange>
    </ParagraphStyleRange>
    <ParagraphStyleRange AppliedParagraphStyle="ParagraphStyle/Body">
      <CharacterStyleRange AppliedCharacterStyle="CharacterStyle/$ID/[No character style]">
        <Content>Monde</Content>
      </CharacterStyleRange>
    </ParagraphStyleRange>
  </Story>
</Document>`
	assert.Equal(t, expected, buf.String())
}

func TestSkeletonStore_TranslationWithEntities(t *testing.T) {
	input := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Document DOMVersion="8.0">
  <Story Self="story1">
    <ParagraphStyleRange AppliedParagraphStyle="ParagraphStyle/Body">
      <CharacterStyleRange AppliedCharacterStyle="CharacterStyle/$ID/[No character style]">
        <Content>Cats &amp; Dogs</Content>
      </CharacterStyleRange>
    </ParagraphStyleRange>
  </Story>
</Document>`
	ctx := context.Background()
	locale := model.LocaleID("fr")

	reader := icml.NewReader()
	writer := icml.NewWriter()

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
			if b.SourceText() == "Cats & Dogs" {
				b.Targets[locale] = []*model.Segment{{ID: "s1", Content: model.NewFragment("Chats & Chiens")}}
			}
		}
	}

	var buf bytes.Buffer
	writer.SetLocale(locale)
	require.NoError(t, writer.SetOutputWriter(&buf))

	ch := testutil.PartsToChannel(parts)
	require.NoError(t, writer.Write(ctx, ch))
	writer.Close()

	// The writer should XML-escape the ampersand in the translated text
	expected := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Document DOMVersion="8.0">
  <Story Self="story1">
    <ParagraphStyleRange AppliedParagraphStyle="ParagraphStyle/Body">
      <CharacterStyleRange AppliedCharacterStyle="CharacterStyle/$ID/[No character style]">
        <Content>Chats &amp; Chiens</Content>
      </CharacterStyleRange>
    </ParagraphStyleRange>
  </Story>
</Document>`
	assert.Equal(t, expected, buf.String())
}
