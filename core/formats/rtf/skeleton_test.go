package rtf_test

import (
	"bytes"
	"os"
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/formats/rtf"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func snippetRoundtripWithSkeleton(t *testing.T, input string) string {
	t.Helper()
	ctx := t.Context()

	reader := rtf.NewReader()
	writer := rtf.NewWriter()

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

func TestSkeletonStore_ByteExact_SimpleRTF(t *testing.T) {
	input := "{\\rtf1\\ansi\\deff0\n\\pard Hello World\\par\n}\n"
	output := snippetRoundtripWithSkeleton(t, input)
	assert.Equal(t, input, output, "simple RTF roundtrip should be byte-exact")
}

func TestSkeletonStore_ByteExact_MultipleParagraphs(t *testing.T) {
	input := "{\\rtf1\\ansi\\deff0\n\\pard First paragraph\\par\n\\pard Second paragraph\\par\n}\n"
	output := snippetRoundtripWithSkeleton(t, input)
	assert.Equal(t, input, output, "multi-paragraph RTF roundtrip should be byte-exact")
}

func TestSkeletonStore_ByteExact_SimpleFile(t *testing.T) {
	data, err := os.ReadFile("testdata/simple.rtf")
	require.NoError(t, err)
	input := string(data)
	output := snippetRoundtripWithSkeleton(t, input)
	assert.Equal(t, input, output, "simple.rtf file roundtrip should be byte-exact")
}

func TestSkeletonStore_WithTranslation(t *testing.T) {
	input := "{\\rtf1\\ansi\\deff0\n\\pard Hello\\par\n}\n"
	ctx := t.Context()

	reader := rtf.NewReader()
	writer := rtf.NewWriter()

	store, err := format.NewSkeletonStore()
	require.NoError(t, err)
	defer store.Close()
	reader.SetSkeletonStore(store)
	writer.SetSkeletonStore(store)

	err = reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	// Modify text
	for _, p := range parts {
		if p.Type == model.PartBlock {
			b := p.Resource.(*model.Block)
			if b.SourceText() == "Hello" {
				b.SetSourceText("Bonjour")
			}
		}
	}

	var buf bytes.Buffer
	require.NoError(t, writer.SetOutputWriter(&buf))

	ch := testutil.PartsToChannel(parts)
	require.NoError(t, writer.Write(ctx, ch))
	writer.Close()

	expected := "{\\rtf1\\ansi\\deff0\n\\pard Bonjour\\par\n}\n"
	assert.Equal(t, expected, buf.String())
}
