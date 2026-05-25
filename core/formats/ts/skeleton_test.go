package ts_test

import (
	"bytes"
	"os"
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/formats/ts"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func snippetRoundtripWithSkeleton(t *testing.T, input string) string {
	t.Helper()
	ctx := t.Context()

	reader := ts.NewReader()
	writer := ts.NewWriter()

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

func TestSkeletonStore_ByteExact_SimpleTS(t *testing.T) {
	t.Parallel()
	input := `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE TS []>
<TS version="2.1" language="fr" sourcelanguage="en">
<context>
    <name>MainWindow</name>
    <message>
        <source>Hello</source>
        <translation>Bonjour</translation>
    </message>
    <message>
        <source>Goodbye</source>
        <translation type="unfinished"></translation>
    </message>
</context>
</TS>
`
	output := snippetRoundtripWithSkeleton(t, input)
	assert.Equal(t, input, output, "simple TS roundtrip should be byte-exact")
}

func TestSkeletonStore_ByteExact_SimpleFile(t *testing.T) {
	t.Parallel()
	data, err := os.ReadFile("testdata/simple.ts")
	require.NoError(t, err)
	input := string(data)
	output := snippetRoundtripWithSkeleton(t, input)
	assert.Equal(t, input, output, "simple.ts file roundtrip should be byte-exact")
}

func TestSkeletonStore_ByteExact_BilingualFile(t *testing.T) {
	t.Parallel()
	data, err := os.ReadFile("testdata/bilingual.ts")
	require.NoError(t, err)
	input := string(data)
	output := snippetRoundtripWithSkeleton(t, input)
	assert.Equal(t, input, output, "bilingual.ts file roundtrip should be byte-exact")
}

func TestSkeletonStore_ByteExact_MultipleContexts(t *testing.T) {
	t.Parallel()
	input := `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE TS []>
<TS version="2.1" language="de" sourcelanguage="en">
<context>
    <name>FileDialog</name>
    <message>
        <source>Open</source>
        <translation>Oeffnen</translation>
    </message>
</context>
<context>
    <name>MainWindow</name>
    <message>
        <source>Save</source>
        <translation>Speichern</translation>
    </message>
</context>
</TS>
`
	output := snippetRoundtripWithSkeleton(t, input)
	assert.Equal(t, input, output, "multiple contexts should be byte-exact")
}

func TestSkeletonStore_ByteExact_WithComments(t *testing.T) {
	t.Parallel()
	input := `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE TS []>
<TS version="2.1" language="fr" sourcelanguage="en">
<context>
    <name>MainWindow</name>
    <message>
        <source>Hello</source>
        <comment>Greeting message</comment>
        <translation>Bonjour</translation>
    </message>
</context>
</TS>
`
	output := snippetRoundtripWithSkeleton(t, input)
	assert.Equal(t, input, output, "TS with comments should be byte-exact")
}

func TestSkeletonStore_ByteExact_EmptyTranslation(t *testing.T) {
	t.Parallel()
	input := `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE TS []>
<TS version="2.1" language="fr" sourcelanguage="en">
<context>
    <name>Test</name>
    <message>
        <source>Hello</source>
        <translation type="unfinished"></translation>
    </message>
</context>
</TS>
`
	output := snippetRoundtripWithSkeleton(t, input)
	assert.Equal(t, input, output, "empty translation should be byte-exact")
}

func TestSkeletonStore_WithTranslation(t *testing.T) {
	t.Parallel()
	input := `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE TS []>
<TS version="2.1" language="fr" sourcelanguage="en">
<context>
    <name>MainWindow</name>
    <message>
        <source>Hello</source>
        <translation type="unfinished"></translation>
    </message>
</context>
</TS>
`
	ctx := t.Context()
	locale := model.LocaleID("fr")

	reader := ts.NewReader()
	writer := ts.NewWriter()

	store, err := format.NewSkeletonStore()
	require.NoError(t, err)
	defer store.Close()
	reader.SetSkeletonStore(store)
	writer.SetSkeletonStore(store)

	err = reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	// Add translation
	for _, p := range parts {
		if p.Type == model.PartBlock {
			b := p.Resource.(*model.Block)
			if b.SourceText() == "Hello" {
				b.SetTargetRuns(locale, []model.Run{{Text: &model.TextRun{Text: "Bonjour"}}})
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
<!DOCTYPE TS []>
<TS version="2.1" language="fr" sourcelanguage="en">
<context>
    <name>MainWindow</name>
    <message>
        <source>Hello</source>
        <translation type="unfinished">Bonjour</translation>
    </message>
</context>
</TS>
`
	assert.Equal(t, expected, buf.String())
}

func TestSkeletonStore_WithTranslation_Escaping(t *testing.T) {
	t.Parallel()
	input := `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE TS []>
<TS version="2.1" language="fr" sourcelanguage="en">
<context>
    <name>Test</name>
    <message>
        <source>Hello</source>
        <translation></translation>
    </message>
</context>
</TS>
`
	ctx := t.Context()
	locale := model.LocaleID("fr")

	reader := ts.NewReader()
	writer := ts.NewWriter()

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
			b.SetTargetRuns(locale, []model.Run{{Text: &model.TextRun{Text: "A & B < C"}}})
		}
	}

	var buf bytes.Buffer
	writer.SetLocale(locale)
	require.NoError(t, writer.SetOutputWriter(&buf))

	ch := testutil.PartsToChannel(parts)
	require.NoError(t, writer.Write(ctx, ch))
	writer.Close()

	// The original translation was empty and we now write "A & B < C",
	// so the writer flips the opening tag to `type="unfinished"` to
	// mirror okapi's APPROVED-property → unfinished behavior on
	// content change.
	assert.Contains(t, buf.String(), `<translation type="unfinished">A &amp; B &lt; C</translation>`)
}

func TestSkeletonStore_ByteExact_Unicode(t *testing.T) {
	t.Parallel()
	input := `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE TS []>
<TS version="2.1" language="ja" sourcelanguage="en">
<context>
    <name>Test</name>
    <message>
        <source>Hello</source>
        <translation>こんにちは</translation>
    </message>
</context>
</TS>
`
	output := snippetRoundtripWithSkeleton(t, input)
	assert.Equal(t, input, output, "TS with Unicode should be byte-exact")
}
