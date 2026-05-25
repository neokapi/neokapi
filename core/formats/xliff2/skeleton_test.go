package xliff2_test

import (
	"bytes"
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/formats/xliff2"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func snippetRoundtripWithSkeleton(t *testing.T, input string) string {
	t.Helper()
	ctx := t.Context()

	reader := xliff2.NewReader()
	writer := xliff2.NewWriter()

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

func TestSkeletonStore_ByteExact_SimpleXLIFF2(t *testing.T) {
	input := `<?xml version="1.0" encoding="UTF-8"?>
<xliff version="2.0" xmlns="urn:oasis:names:tc:xliff:document:2.0"
       srcLang="en" trgLang="fr">
  <file id="f1">
    <unit id="u1">
      <segment id="s1">
        <source>Hello World</source>
        <target>Bonjour le monde</target>
      </segment>
    </unit>
  </file>
</xliff>`
	output := snippetRoundtripWithSkeleton(t, input)
	assert.Equal(t, input, output, "simple XLIFF 2 roundtrip should be byte-exact")
}

func TestSkeletonStore_ByteExact_MultipleUnits(t *testing.T) {
	input := `<?xml version="1.0" encoding="UTF-8"?>
<xliff version="2.0" xmlns="urn:oasis:names:tc:xliff:document:2.0"
       srcLang="en" trgLang="fr">
  <file id="f1">
    <unit id="u1" name="greeting">
      <segment id="s1">
        <source>Hello World</source>
        <target>Bonjour le monde</target>
      </segment>
    </unit>
    <unit id="u2">
      <segment id="s1">
        <source>Goodbye</source>
        <target>Au revoir</target>
      </segment>
    </unit>
    <unit id="u3">
      <segment id="s1">
        <source>Welcome</source>
      </segment>
    </unit>
  </file>
</xliff>`
	output := snippetRoundtripWithSkeleton(t, input)
	assert.Equal(t, input, output, "multiple units XLIFF 2 roundtrip should be byte-exact")
}

func TestSkeletonStore_ByteExact_XmlEntities(t *testing.T) {
	input := `<?xml version="1.0" encoding="UTF-8"?>
<xliff version="2.0" xmlns="urn:oasis:names:tc:xliff:document:2.0"
       srcLang="en" trgLang="fr">
  <file id="f1">
    <unit id="u1">
      <segment id="s1">
        <source>A &amp; B &lt; C</source>
        <target>A &amp; B &lt; C</target>
      </segment>
    </unit>
  </file>
</xliff>`
	output := snippetRoundtripWithSkeleton(t, input)
	assert.Equal(t, input, output, "XLIFF 2 with XML entities should be byte-exact")
}

func TestSkeletonStore_WithTranslation(t *testing.T) {
	input := `<?xml version="1.0" encoding="UTF-8"?>
<xliff version="2.0" xmlns="urn:oasis:names:tc:xliff:document:2.0"
       srcLang="en" trgLang="fr">
  <file id="f1">
    <unit id="u1">
      <segment id="s1">
        <source>Hello</source>
        <target>Bonjour</target>
      </segment>
    </unit>
  </file>
</xliff>`
	ctx := t.Context()

	reader := xliff2.NewReader()
	writer := xliff2.NewWriter()

	store, err := format.NewSkeletonStore()
	require.NoError(t, err)
	defer store.Close()
	reader.SetSkeletonStore(store)
	writer.SetSkeletonStore(store)

	err = reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	// Modify the French translation
	for _, p := range parts {
		if p.Type == model.PartBlock {
			b := p.Resource.(*model.Block)
			if b.SourceText() == "Hello" {
				b.SetTargetRuns(model.LocaleID("fr"), []model.Run{{Text: &model.TextRun{Text: "Salut"}}})
			}
		}
	}

	var buf bytes.Buffer
	require.NoError(t, writer.SetOutputWriter(&buf))

	ch := testutil.PartsToChannel(parts)
	require.NoError(t, writer.Write(ctx, ch))
	writer.Close()

	expected := `<?xml version="1.0" encoding="UTF-8"?>
<xliff version="2.0" xmlns="urn:oasis:names:tc:xliff:document:2.0"
       srcLang="en" trgLang="fr">
  <file id="f1">
    <unit id="u1">
      <segment id="s1">
        <source>Hello</source>
        <target>Salut</target>
      </segment>
    </unit>
  </file>
</xliff>`
	assert.Equal(t, expected, buf.String())
}

func TestSkeletonStore_WithTranslation_Escaping(t *testing.T) {
	input := `<?xml version="1.0" encoding="UTF-8"?>
<xliff version="2.0" xmlns="urn:oasis:names:tc:xliff:document:2.0"
       srcLang="en" trgLang="fr">
  <file id="f1">
    <unit id="u1">
      <segment id="s1">
        <source>Hello</source>
        <target>Bonjour</target>
      </segment>
    </unit>
  </file>
</xliff>`
	ctx := t.Context()

	reader := xliff2.NewReader()
	writer := xliff2.NewWriter()

	store, err := format.NewSkeletonStore()
	require.NoError(t, err)
	defer store.Close()
	reader.SetSkeletonStore(store)
	writer.SetSkeletonStore(store)

	err = reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	// Set a translation with XML special characters
	for _, p := range parts {
		if p.Type == model.PartBlock {
			b := p.Resource.(*model.Block)
			b.SetTargetRuns(model.LocaleID("fr"), []model.Run{{Text: &model.TextRun{Text: "A & B < C"}}})
		}
	}

	var buf bytes.Buffer
	require.NoError(t, writer.SetOutputWriter(&buf))

	ch := testutil.PartsToChannel(parts)
	require.NoError(t, writer.Write(ctx, ch))
	writer.Close()

	assert.Contains(t, buf.String(), "<target>A &amp; B &lt; C</target>")
}

func TestSkeletonStore_ByteExact_SourceOnly(t *testing.T) {
	input := `<?xml version="1.0" encoding="UTF-8"?>
<xliff version="2.0" xmlns="urn:oasis:names:tc:xliff:document:2.0"
       srcLang="en">
  <file id="f1">
    <unit id="u1">
      <segment id="s1">
        <source>Hello</source>
      </segment>
    </unit>
  </file>
</xliff>`
	output := snippetRoundtripWithSkeleton(t, input)
	assert.Equal(t, input, output, "source-only XLIFF 2 roundtrip should be byte-exact")
}

func TestSkeletonStore_ByteExact_WithNotes(t *testing.T) {
	input := `<?xml version="1.0" encoding="UTF-8"?>
<xliff version="2.0" xmlns="urn:oasis:names:tc:xliff:document:2.0"
       srcLang="en" trgLang="fr">
  <file id="f1">
    <unit id="u1">
      <notes>
        <note>This needs review</note>
      </notes>
      <segment id="s1">
        <source>Welcome</source>
        <target>Bienvenue</target>
      </segment>
    </unit>
  </file>
</xliff>`
	output := snippetRoundtripWithSkeleton(t, input)
	assert.Equal(t, input, output, "XLIFF 2 with notes should be byte-exact")
}
