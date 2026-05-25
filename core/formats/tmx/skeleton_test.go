package tmx_test

import (
	"bytes"
	"os"
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/formats/tmx"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func snippetRoundtripWithSkeleton(t *testing.T, input string) string {
	t.Helper()
	ctx := t.Context()

	reader := tmx.NewReader()
	writer := tmx.NewWriter()

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

func TestSkeletonStore_ByteExact_SimpleTMX(t *testing.T) {
	input := `<?xml version="1.0" encoding="UTF-8"?>
<tmx version="1.4">
  <header creationtool="test" srclang="en" datatype="plaintext"/>
  <body>
    <tu tuid="tu1">
      <tuv xml:lang="en">
        <seg>Hello World</seg>
      </tuv>
    </tu>
  </body>
</tmx>
`
	output := snippetRoundtripWithSkeleton(t, input)
	assert.Equal(t, input, output, "simple TMX roundtrip should be byte-exact")
}

func TestSkeletonStore_ByteExact_BilingualTMX(t *testing.T) {
	input := `<?xml version="1.0" encoding="UTF-8"?>
<tmx version="1.4">
  <header creationtool="neokapi" creationtoolversion="1.0" segtype="sentence" o-tmf="unknown" adminlang="en" srclang="en" datatype="plaintext"/>
  <body>
    <tu tuid="tu1">
      <tuv xml:lang="en">
        <seg>Hello World</seg>
      </tuv>
      <tuv xml:lang="fr">
        <seg>Bonjour le monde</seg>
      </tuv>
    </tu>
    <tu tuid="tu2">
      <tuv xml:lang="en">
        <seg>Goodbye</seg>
      </tuv>
      <tuv xml:lang="fr">
        <seg>Au revoir</seg>
      </tuv>
      <tuv xml:lang="de">
        <seg>Auf Wiedersehen</seg>
      </tuv>
    </tu>
  </body>
</tmx>
`
	output := snippetRoundtripWithSkeleton(t, input)
	assert.Equal(t, input, output, "bilingual TMX roundtrip should be byte-exact")
}

func TestSkeletonStore_ByteExact_SimpleFile(t *testing.T) {
	data, err := os.ReadFile("testdata/simple.tmx")
	require.NoError(t, err)
	input := string(data)
	output := snippetRoundtripWithSkeleton(t, input)
	assert.Equal(t, input, output, "simple.tmx file roundtrip should be byte-exact")
}

func TestSkeletonStore_ByteExact_EmptySeg(t *testing.T) {
	input := `<?xml version="1.0" encoding="UTF-8"?>
<tmx version="1.4">
  <header srclang="en"/>
  <body>
    <tu tuid="tu1">
      <tuv xml:lang="en">
        <seg></seg>
      </tuv>
    </tu>
  </body>
</tmx>
`
	output := snippetRoundtripWithSkeleton(t, input)
	assert.Equal(t, input, output, "TMX with empty seg should be byte-exact")
}

func TestSkeletonStore_WithTranslation(t *testing.T) {
	input := `<?xml version="1.0" encoding="UTF-8"?>
<tmx version="1.4">
  <header srclang="en" datatype="plaintext"/>
  <body>
    <tu tuid="tu1">
      <tuv xml:lang="en">
        <seg>Hello</seg>
      </tuv>
      <tuv xml:lang="fr">
        <seg>Bonjour</seg>
      </tuv>
    </tu>
  </body>
</tmx>
`
	ctx := t.Context()

	reader := tmx.NewReader()
	writer := tmx.NewWriter()

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
<tmx version="1.4">
  <header srclang="en" datatype="plaintext"/>
  <body>
    <tu tuid="tu1">
      <tuv xml:lang="en">
        <seg>Hello</seg>
      </tuv>
      <tuv xml:lang="fr">
        <seg>Salut</seg>
      </tuv>
    </tu>
  </body>
</tmx>
`
	assert.Equal(t, expected, buf.String())
}

func TestSkeletonStore_WithTranslation_Escaping(t *testing.T) {
	input := `<?xml version="1.0" encoding="UTF-8"?>
<tmx version="1.4">
  <header srclang="en"/>
  <body>
    <tu tuid="tu1">
      <tuv xml:lang="en">
        <seg>Hello</seg>
      </tuv>
      <tuv xml:lang="fr">
        <seg>Bonjour</seg>
      </tuv>
    </tu>
  </body>
</tmx>
`
	ctx := t.Context()

	reader := tmx.NewReader()
	writer := tmx.NewWriter()

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

	assert.Contains(t, buf.String(), "<seg>A &amp; B &lt; C</seg>")
}

func TestSkeletonStore_ByteExact_XmlEntities(t *testing.T) {
	input := `<?xml version="1.0" encoding="UTF-8"?>
<tmx version="1.4">
  <header srclang="en"/>
  <body>
    <tu tuid="tu1">
      <tuv xml:lang="en">
        <seg>A &amp; B &lt; C</seg>
      </tuv>
    </tu>
  </body>
</tmx>
`
	output := snippetRoundtripWithSkeleton(t, input)
	assert.Equal(t, input, output, "TMX with XML entities should be byte-exact")
}
