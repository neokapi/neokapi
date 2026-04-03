package dtd_test

import (
	"bytes"
	"context"
	"os"
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/formats/dtd"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func snippetRoundtripWithSkeleton(t *testing.T, input string) string {
	t.Helper()
	ctx := context.Background()

	reader := dtd.NewReader()
	writer := dtd.NewWriter()

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

func TestSkeletonStore_ByteExact_SimpleEntity(t *testing.T) {
	input := `<!ENTITY greeting "Hello world">`
	output := snippetRoundtripWithSkeleton(t, input)
	assert.Equal(t, input, output, "single entity roundtrip should be byte-exact")
}

func TestSkeletonStore_ByteExact_MultipleEntities(t *testing.T) {
	input := "<!ENTITY greeting \"Hello\">\n<!ENTITY farewell \"Goodbye\">\n<!ENTITY thanks \"Thank you\">\n"
	output := snippetRoundtripWithSkeleton(t, input)
	assert.Equal(t, input, output, "multiple entities with newlines should be byte-exact")
}

func TestSkeletonStore_ByteExact_WithComments(t *testing.T) {
	input := "<!-- Window title -->\n<!ENTITY findWindow.title \"Find Files\">\n<!-- File menu -->\n<!ENTITY fileMenu.label \"File\">\n"
	output := snippetRoundtripWithSkeleton(t, input)
	assert.Equal(t, input, output, "entities with comments should be byte-exact")
}

func TestSkeletonStore_ByteExact_SingleQuoted(t *testing.T) {
	input := "<!ENTITY greeting 'Hello world'>"
	output := snippetRoundtripWithSkeleton(t, input)
	assert.Equal(t, input, output, "single-quoted entity should be byte-exact")
}

func TestSkeletonStore_ByteExact_WithEscapes(t *testing.T) {
	input := "<!ENTITY escaped \"Text with &amp; ampersand and &lt;angle brackets&gt;\">\n"
	output := snippetRoundtripWithSkeleton(t, input)
	assert.Equal(t, input, output, "entities with XML escapes should be byte-exact")
}

func TestSkeletonStore_ByteExact_WithNCRs(t *testing.T) {
	// NCRs (&#65;) are resolved to characters (A) during reading.
	// The skeleton preserves the structural bytes around the value, but
	// the value itself is re-encoded from the resolved block content.
	// Characters that don't need DTD escaping (like A, B) are written as-is.
	input := "<!ENTITY ncr \"Char &#65; and hex &#x42;\">\n"
	expected := "<!ENTITY ncr \"Char A and hex B\">\n"
	output := snippetRoundtripWithSkeleton(t, input)
	assert.Equal(t, expected, output, "NCRs should be resolved and re-encoded")
}

func TestSkeletonStore_ByteExact_EmptyEntity(t *testing.T) {
	input := `<!ENTITY empty "">`
	output := snippetRoundtripWithSkeleton(t, input)
	assert.Equal(t, input, output, "empty entity should be byte-exact")
}

func TestSkeletonStore_ByteExact_CRLF(t *testing.T) {
	input := "<!ENTITY greeting \"Hello\">\r\n<!ENTITY farewell \"Goodbye\">\r\n"
	output := snippetRoundtripWithSkeleton(t, input)
	assert.Equal(t, input, output, "CRLF line endings should be preserved")
}

func TestSkeletonStore_ByteExact_ParameterEntity(t *testing.T) {
	// Parameter entities are not translatable, they should be preserved as skeleton text
	input := "<!ENTITY % pent \"value\">\n<!ENTITY greeting \"Hello\">\n"
	output := snippetRoundtripWithSkeleton(t, input)
	assert.Equal(t, input, output, "parameter entities should be preserved as skeleton text")
}

func TestSkeletonStore_ByteExact_ComplexFile(t *testing.T) {
	data, err := os.ReadFile("testdata/complex.dtd")
	require.NoError(t, err)
	input := string(data)
	output := snippetRoundtripWithSkeleton(t, input)
	// NCRs (&#65; -> A, &#x42; -> B) are resolved during reading, so the
	// ncr entity value will differ. Everything else should be byte-exact.
	assert.Contains(t, output, "<!-- Window title -->\n<!ENTITY findWindow.title \"Find Files\">\n")
	assert.Contains(t, output, "<!-- File menu -->\n<!ENTITY fileMenu.label \"File\">\n")
	assert.Contains(t, output, "<!ENTITY editMenu.label \"Edit\">\n")
	assert.Contains(t, output, "<!ENTITY escaped \"Text with &amp; ampersand and &lt;angle brackets&gt;\">\n")
	assert.Contains(t, output, "<!ENTITY ncr \"Char A and hex B\">\n")
	assert.Contains(t, output, "<!ENTITY unicode \"\xe3\x81\x93\xe3\x82\x93\xe3\x81\xab\xe3\x81\xa1\xe3\x81\xaf\xe4\xb8\x96\xe7\x95\x8c\">\n")
}

func TestSkeletonStore_ByteExact_SimpleFile(t *testing.T) {
	data, err := os.ReadFile("testdata/simple.dtd")
	require.NoError(t, err)
	input := string(data)
	output := snippetRoundtripWithSkeleton(t, input)
	assert.Equal(t, input, output, "simple file roundtrip should be byte-exact")
}

func TestSkeletonStore_WithTranslation(t *testing.T) {
	input := "<!ENTITY greeting \"Hello\">\n<!ENTITY farewell \"Goodbye\">\n"
	ctx := context.Background()
	locale := model.LocaleID("fr")

	reader := dtd.NewReader()
	writer := dtd.NewWriter()

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
			case "Goodbye":
				b.Targets[locale] = []*model.Segment{{ID: "s1", Content: model.NewFragment("Au revoir")}}
			}
		}
	}

	var buf bytes.Buffer
	writer.SetLocale(locale)
	require.NoError(t, writer.SetOutputWriter(&buf))

	ch := testutil.PartsToChannel(parts)
	require.NoError(t, writer.Write(ctx, ch))
	writer.Close()

	expected := "<!ENTITY greeting \"Bonjour\">\n<!ENTITY farewell \"Au revoir\">\n"
	assert.Equal(t, expected, buf.String())
}

func TestSkeletonStore_WithTranslation_Escaping(t *testing.T) {
	input := "<!ENTITY greeting \"Hello\">\n"
	ctx := context.Background()
	locale := model.LocaleID("fr")

	reader := dtd.NewReader()
	writer := dtd.NewWriter()

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
			// Translation contains characters that need DTD escaping
			b.Targets[locale] = []*model.Segment{{ID: "s1", Content: model.NewFragment("A & B < C")}}
		}
	}

	var buf bytes.Buffer
	writer.SetLocale(locale)
	require.NoError(t, writer.SetOutputWriter(&buf))

	ch := testutil.PartsToChannel(parts)
	require.NoError(t, writer.Write(ctx, ch))
	writer.Close()

	expected := "<!ENTITY greeting \"A &amp; B &lt; C\">\n"
	assert.Equal(t, expected, buf.String())
}
