package transtable_test

import (
	"bytes"
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/formats/transtable"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// snippetRoundtripWithSkeleton round-trips input through the reader +
// writer with a SkeletonStore wired between them.
func snippetRoundtripWithSkeleton(t *testing.T, input string) string {
	t.Helper()
	ctx := t.Context()

	reader := transtable.NewReader()
	writer := transtable.NewWriter()

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
	writer.SetLocale(model.LocaleFrench)

	ch := testutil.PartsToChannel(parts)
	require.NoError(t, writer.Write(ctx, ch))
	writer.Close()

	return buf.String()
}

// Round-trip a single source-only entry. The writer always renders the
// target column when a target locale is set so the second `\t""` cell
// shows up; we assert on the rendered shape, not byte-equality with
// the input.
func TestSkeletonStore_SingleEntry(t *testing.T) {
	input := "TransTableV1\ten\tfr\n" +
		"\"okpCtx:tu=1\"\t\"hello\"\n"
	output := snippetRoundtripWithSkeleton(t, input)
	assert.Contains(t, output, "TransTableV1\ten\tfr")
	assert.Contains(t, output, "\"okpCtx:tu=1\"\t\"hello\"")
}

func TestSkeletonStore_BilingualEntry(t *testing.T) {
	input := "TransTableV1\ten\tfr\n" +
		"\"okpCtx:tu=1\"\t\"hello\"\t\"bonjour\"\n"
	output := snippetRoundtripWithSkeleton(t, input)
	assert.Contains(t, output, "\"okpCtx:tu=1\"\t\"hello\"\t\"bonjour\"")
}

func TestSkeletonStore_MultipleEntries(t *testing.T) {
	input := "TransTableV1\ten\tfr\n" +
		"\"okpCtx:tu=1\"\t\"hello\"\n" +
		"\"okpCtx:tu=2\"\t\"goodbye\"\n"
	output := snippetRoundtripWithSkeleton(t, input)
	assert.Contains(t, output, "\"okpCtx:tu=1\"\t\"hello\"")
	assert.Contains(t, output, "\"okpCtx:tu=2\"\t\"goodbye\"")
}

func TestSkeletonStore_SegmentedEntry_RetainsSegmentation(t *testing.T) {
	input := "TransTableV1\ten\tfr\n" +
		"okpCtx:tu=1:s=0\tA\n" +
		"okpCtx:tu=1:s=1\tB\n"
	output := snippetRoundtripWithSkeleton(t, input)
	assert.Contains(t, output, "\"okpCtx:tu=1:s=0\"\t\"A\"")
	assert.Contains(t, output, "\"okpCtx:tu=1:s=1\"\t\"B\"")
}

// Updating a target before write produces the expected target column.
func TestSkeletonStore_WithTranslation(t *testing.T) {
	input := "TransTableV1\ten\tfr\n" +
		"\"okpCtx:tu=1\"\t\"Hello\"\n"
	ctx := t.Context()
	locale := model.LocaleFrench

	reader := transtable.NewReader()
	writer := transtable.NewWriter()

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
			if b.SourceText() == "Hello" {
				b.SetTargetText(locale, "Bonjour")
			}
		}
	}

	var buf bytes.Buffer
	writer.SetLocale(locale)
	require.NoError(t, writer.SetOutputWriter(&buf))

	ch := testutil.PartsToChannel(parts)
	require.NoError(t, writer.Write(ctx, ch))
	writer.Close()

	assert.Contains(t, buf.String(), "\"okpCtx:tu=1\"\t\"Hello\"\t\"Bonjour\"")
}
