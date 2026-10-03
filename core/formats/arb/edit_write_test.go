package arb_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/formats/arb"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// arbEditWithSkeleton reads input as kapi apply reads a file, with the
// reader's skeleton store wired to the writer, lets edit change each block,
// and writes the document back. streaming reads from a file, which takes the
// reader's two-pass path; otherwise the whole input is read at once.
func arbEditWithSkeleton(t *testing.T, input string, streaming bool, edit func(*model.Block)) (string, error) {
	t.Helper()
	ctx := t.Context()
	reader, writer := arb.NewReader(), arb.NewWriter()
	store, err := format.NewSkeletonStore()
	require.NoError(t, err)
	defer store.Close()
	reader.SetSkeletonStore(store)
	writer.SetSkeletonStore(store)

	doc := testutil.RawDocFromString(input, model.LocaleEnglish)
	if streaming {
		path := filepath.Join(t.TempDir(), "app.arb")
		require.NoError(t, os.WriteFile(path, []byte(input), 0o600))
		f, err := os.Open(path)
		require.NoError(t, err)
		doc = &model.RawDocument{URI: path, SourceLocale: model.LocaleEnglish, Reader: f}
	}
	require.NoError(t, reader.Open(ctx, doc))
	parts := testutil.CollectParts(t, reader.Read(ctx))
	require.NoError(t, reader.Close())
	for _, p := range parts {
		if b, ok := p.Resource.(*model.Block); ok && p.Type == model.PartBlock {
			edit(b)
		}
	}
	var buf bytes.Buffer
	require.NoError(t, writer.SetOutputWriter(&buf))
	werr := writer.Write(ctx, testutil.PartsToChannel(parts))
	require.NoError(t, writer.Close())
	return buf.String(), werr
}

// setSource gives the block named key the source text value.
func setSource(key, value string) func(*model.Block) {
	return func(b *model.Block) {
		if b.Name == key {
			b.SetSourceRuns([]model.Run{model.TextR(value)})
		}
	}
}

// An edit to one message leaves every other message's bytes as they were: a
// value the file escapes its own way (\/, é, an upper-case \u escape)
// keeps its escapes, and a message with a plural that ICU rejects as a whole
// does not stop the write. Only the edited value is spelled anew.
func TestEditWritesOnlyTheEditedValue(t *testing.T) {
	input := "{\n" +
		"  \"@@locale\": \"en\",\n" +
		"  \"menu\": \"Caf\\u00e9 \\/ bar \\u00C9t\\u00e9\",\n" +
		"  \"broken\": \"{count, plural, one{a} other{b}} {oops\",\n" +
		"  \"stray\": \"{count, plural, one{a} other{b}} and a } brace\",\n" +
		"  \"other\": \"x\"\n" +
		"}\n"
	for _, streaming := range []bool{false, true} {
		name := map[bool]string{false: "buffered", true: "streaming"}[streaming]
		t.Run(name, func(t *testing.T) {
			untouched, err := arbEditWithSkeleton(t, input, streaming, func(*model.Block) {})
			require.NoError(t, err)
			assert.Equal(t, input, untouched, "an untouched file writes back byte for byte")

			out, err := arbEditWithSkeleton(t, input, streaming, setSource("other", "y"))
			require.NoError(t, err)
			assert.Equal(t, strings.Replace(input, `"other": "x"`, `"other": "y"`, 1), out)

			// The edited value itself is written as the writer spells it.
			out, err = arbEditWithSkeleton(t, input, streaming, setSource("menu", "Café / bar"))
			require.NoError(t, err)
			assert.Equal(t, strings.Replace(input, "\"Caf\\u00e9 \\/ bar \\u00C9t\\u00e9\"", `"Café / bar"`, 1), out)
		})
	}
}

// A target written in another language than the file keeps the escapes of a
// value its translation left as it was.
func TestTranslationKeepsTheEscapesOfAnUntranslatedValue(t *testing.T) {
	input := "{\n  \"path\": \"a\\/b\",\n  \"hello\": \"Hello\"\n}\n"
	ctx := t.Context()
	reader, writer := arb.NewReader(), arb.NewWriter()
	store, err := format.NewSkeletonStore()
	require.NoError(t, err)
	defer store.Close()
	reader.SetSkeletonStore(store)
	writer.SetSkeletonStore(store)
	require.NoError(t, reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish)))
	parts := testutil.CollectParts(t, reader.Read(ctx))
	require.NoError(t, reader.Close())
	fr := model.LocaleID("fr")
	for _, p := range parts {
		if b, ok := p.Resource.(*model.Block); ok && b.Name == "hello" {
			b.SetTargetText(fr, "Bonjour")
		}
	}
	var buf bytes.Buffer
	writer.SetLocale(fr)
	require.NoError(t, writer.SetOutputWriter(&buf))
	require.NoError(t, writer.Write(ctx, testutil.PartsToChannel(parts)))
	require.NoError(t, writer.Close())
	assert.Equal(t, "{\n  \"path\": \"a\\/b\",\n  \"hello\": \"Bonjour\"\n}\n", buf.String())
}
