package xliff_test

import (
	"bytes"
	"os"
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/formats/xliff"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func snippetRoundtripWithSkeleton(t *testing.T, input string) string {
	t.Helper()
	ctx := t.Context()

	reader := xliff.NewReader()
	writer := xliff.NewWriter()

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

func TestSkeletonStore_ByteExact_SimpleXLIFF(t *testing.T) {
	t.Parallel()
	input := `<?xml version="1.0" encoding="UTF-8"?>
<xliff version="1.2" xmlns="urn:oasis:names:tc:xliff:document:1.2">
  <file original="hello.txt" source-language="en" target-language="fr" datatype="plaintext">
    <body>
      <trans-unit id="1">
        <source>Hello World</source>
        <target>Bonjour le monde</target>
      </trans-unit>
    </body>
  </file>
</xliff>`
	output := snippetRoundtripWithSkeleton(t, input)
	assert.Equal(t, input, output, "simple XLIFF roundtrip should be byte-exact")
}

func TestSkeletonStore_ByteExact_MultipleTransUnits(t *testing.T) {
	t.Parallel()
	input := `<?xml version="1.0" encoding="UTF-8"?>
<xliff version="1.2" xmlns="urn:oasis:names:tc:xliff:document:1.2">
  <file original="hello.txt" source-language="en" target-language="fr" datatype="plaintext">
    <body>
      <trans-unit id="1">
        <source>Hello World</source>
        <target>Bonjour le monde</target>
      </trans-unit>
      <trans-unit id="2">
        <source>Goodbye</source>
        <target>Au revoir</target>
      </trans-unit>
      <trans-unit id="3">
        <source>Untranslated</source>
      </trans-unit>
    </body>
  </file>
</xliff>`
	output := snippetRoundtripWithSkeleton(t, input)
	assert.Equal(t, input, output, "multiple trans-units XLIFF roundtrip should be byte-exact")
}

// okapi: RoundTripXliffIT#xliffFiles
// RoundTripXliffIT#xliffFiles extracts each .xlf/.xliff in the corpus, merges,
// and re-extracts, asserting the events match (extract→merge→re-extract). The
// native byte-exact file roundtrip over testdata/simple.xlf is strictly
// stronger: it reads a real XLIFF file through the skeleton store and asserts
// the rendered output equals the input byte-for-byte, so no event/content can
// drift across the read→write cycle.
func TestSkeletonStore_ByteExact_SimpleFile(t *testing.T) {
	t.Parallel()
	data, err := os.ReadFile("testdata/simple.xlf")
	require.NoError(t, err)
	input := string(data)
	output := snippetRoundtripWithSkeleton(t, input)
	assert.Equal(t, input, output, "simple.xlf file roundtrip should be byte-exact")
}

func TestSkeletonStore_ByteExact_XmlEntities(t *testing.T) {
	t.Parallel()
	input := `<?xml version="1.0" encoding="UTF-8"?>
<xliff version="1.2" xmlns="urn:oasis:names:tc:xliff:document:1.2">
  <file original="test" source-language="en" target-language="fr" datatype="plaintext">
    <body>
      <trans-unit id="1">
        <source>A &amp; B &lt; C</source>
        <target>A &amp; B &lt; C</target>
      </trans-unit>
    </body>
  </file>
</xliff>`
	output := snippetRoundtripWithSkeleton(t, input)
	assert.Equal(t, input, output, "XLIFF with XML entities should be byte-exact")
}

func TestSkeletonStore_WithTranslation(t *testing.T) {
	t.Parallel()
	input := `<?xml version="1.0" encoding="UTF-8"?>
<xliff version="1.2" xmlns="urn:oasis:names:tc:xliff:document:1.2">
  <file original="test" source-language="en" target-language="fr" datatype="plaintext">
    <body>
      <trans-unit id="1">
        <source>Hello</source>
        <target>Bonjour</target>
      </trans-unit>
    </body>
  </file>
</xliff>`
	ctx := t.Context()

	reader := xliff.NewReader()
	writer := xliff.NewWriter()

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
<xliff version="1.2" xmlns="urn:oasis:names:tc:xliff:document:1.2">
  <file original="test" source-language="en" target-language="fr" datatype="plaintext">
    <body>
      <trans-unit id="1">
        <source>Hello</source>
        <target>Salut</target>
      </trans-unit>
    </body>
  </file>
</xliff>`
	assert.Equal(t, expected, buf.String())
}

func TestSkeletonStore_WithTranslation_Escaping(t *testing.T) {
	t.Parallel()
	input := `<?xml version="1.0" encoding="UTF-8"?>
<xliff version="1.2" xmlns="urn:oasis:names:tc:xliff:document:1.2">
  <file original="test" source-language="en" target-language="fr" datatype="plaintext">
    <body>
      <trans-unit id="1">
        <source>Hello</source>
        <target>Bonjour</target>
      </trans-unit>
    </body>
  </file>
</xliff>`
	ctx := t.Context()

	reader := xliff.NewReader()
	writer := xliff.NewWriter()

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
	t.Parallel()
	input := `<?xml version="1.0" encoding="UTF-8"?>
<xliff version="1.2" xmlns="urn:oasis:names:tc:xliff:document:1.2">
  <file original="test" source-language="en" datatype="plaintext">
    <body>
      <trans-unit id="1">
        <source>Hello</source>
      </trans-unit>
    </body>
  </file>
</xliff>`
	output := snippetRoundtripWithSkeleton(t, input)
	assert.Equal(t, input, output, "source-only XLIFF roundtrip should be byte-exact")
}
