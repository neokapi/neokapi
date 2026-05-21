package wiki_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/formats/wiki"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func snippetRoundtripWithSkeleton(t *testing.T, input string) string {
	t.Helper()
	ctx := t.Context()

	reader := wiki.NewReader()
	writer := wiki.NewWriter()

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

func TestSkeletonStore_ByteExact_SimpleParagraph(t *testing.T) {
	input := "Hello world"
	output := snippetRoundtripWithSkeleton(t, input)
	assert.Equal(t, input, output, "simple paragraph roundtrip should be byte-exact")
}

func TestSkeletonStore_ByteExact_MultipleParagraphs(t *testing.T) {
	input := "First paragraph\n\nSecond paragraph"
	output := snippetRoundtripWithSkeleton(t, input)
	assert.Equal(t, input, output, "multiple paragraphs should be byte-exact")
}

func TestSkeletonStore_ByteExact_TrailingNewline(t *testing.T) {
	input := "Hello world\n"
	output := snippetRoundtripWithSkeleton(t, input)
	assert.Equal(t, input, output, "trailing newline should be preserved")
}

func TestSkeletonStore_ByteExact_Header(t *testing.T) {
	input := "== My Header ==\n"
	output := snippetRoundtripWithSkeleton(t, input)
	assert.Equal(t, input, output, "header should be byte-exact")
}

func TestSkeletonStore_ByteExact_HeaderAndParagraph(t *testing.T) {
	input := "== Title ==\nSome text here\n"
	output := snippetRoundtripWithSkeleton(t, input)
	assert.Equal(t, input, output, "header and paragraph should be byte-exact")
}

func TestSkeletonStore_ByteExact_EmptyInput(t *testing.T) {
	input := ""
	output := snippetRoundtripWithSkeleton(t, input)
	assert.Equal(t, input, output, "empty input should produce empty output")
}

// A mid-line `<file>` opener truncates the translatable text unit at the
// tag while the opener and the rest of the line stay in the skeleton, so
// the line still round-trips byte-for-byte.
func TestSkeletonStore_ByteExact_MidLineFileTag(t *testing.T) {
	input := "This is <file> a test."
	output := snippetRoundtripWithSkeleton(t, input)
	assert.Equal(t, input, output, "mid-line <file> tag should round-trip byte-exact")
}

func TestSkeletonStore_ByteExact_BlankLines(t *testing.T) {
	input := "First\n\n\nSecond"
	output := snippetRoundtripWithSkeleton(t, input)
	assert.Equal(t, input, output, "multiple blank lines should be preserved")
}

// snippetRoundtripTranslated roundtrips a snippet through the skeleton
// store, replacing each block's source text with translate(source) before
// the writer runs. Used to prove header delimiter level + spacing are
// reproduced from the stored layout, independent of the (changed) title.
func snippetRoundtripTranslated(t *testing.T, input string, translate func(src string) string) string {
	t.Helper()
	ctx := t.Context()
	locale := model.LocaleID("fr")

	reader := wiki.NewReader()
	writer := wiki.NewWriter()

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
		if p.Type != model.PartBlock {
			continue
		}
		b := p.Resource.(*model.Block)
		tgt := translate(b.SourceText())
		b.Targets[locale] = []*model.Segment{{ID: "s1", Runs: []model.Run{{Text: &model.TextRun{Text: tgt}}}}}
	}

	var buf bytes.Buffer
	writer.SetLocale(locale)
	require.NoError(t, writer.SetOutputWriter(&buf))
	require.NoError(t, writer.Write(ctx, testutil.PartsToChannel(parts)))
	writer.Close()

	return buf.String()
}

// TestSkeletonStore_ByteExact_HeaderLevels verifies that the header
// delimiter level (number of `=`) and the canonical single-space spacing
// round-trip byte-for-byte across every MediaWiki/DokuWiki heading level,
// using the stored layout (not a regex re-parse of the source line).
func TestSkeletonStore_ByteExact_HeaderLevels(t *testing.T) {
	for _, input := range []string{
		"== Level 2 ==\n",
		"=== Level 3 ===\n",
		"==== Level 4 ====\n",
		"===== Level 5 =====\n",
		"====== Level 6 ======\n",
	} {
		t.Run(input, func(t *testing.T) {
			output := snippetRoundtripWithSkeleton(t, input)
			assert.Equal(t, input, output, "header level should round-trip byte-exact")
		})
	}
}

// TestSkeletonStore_HeaderLevels_TranslatedTitle confirms that when the
// title text changes (translation), the surrounding delimiter level and
// spacing are reconstructed exactly from the stored layout — the closing
// delimiter run keeps the same length as the opening run.
func TestSkeletonStore_HeaderLevels_TranslatedTitle(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"level2", "== Title ==\n", "== TITLE ==\n"},
		{"level3", "=== Title ===\n", "=== TITLE ===\n"},
		{"level4", "==== Title ====\n", "==== TITLE ====\n"},
		{"level6", "====== Title ======\n", "====== TITLE ======\n"},
		// Header followed by body to exercise the multi-block skeleton path.
		{"level3_with_body", "=== Title ===\nBody text.\n", "=== TITLE ===\nBODY TEXT.\n"},
	}
	upper := func(s string) string { return strings.ToUpper(s) }
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output := snippetRoundtripTranslated(t, tt.input, upper)
			assert.Equal(t, tt.want, output,
				"delimiter level + spacing must survive a title change")
		})
	}
}

// TestSkeletonStore_Header_NonCanonicalSpacing verifies that non-canonical
// whitespace around the title (extra spaces, asymmetric delimiter runs,
// and trailing whitespace) is preserved byte-for-byte from the stored
// layout rather than normalized to a single space.
func TestSkeletonStore_Header_NonCanonicalSpacing(t *testing.T) {
	for _, input := range []string{
		"==  Wide Spacing  ==\n", // two spaces each side
		"==No Spacing==\n",       // no spaces at all
		"== Trailing ==  \n",     // trailing whitespace after closing delims
		"== Tab\tTitle ==\n",     // tab inside the title
	} {
		t.Run(input, func(t *testing.T) {
			output := snippetRoundtripWithSkeleton(t, input)
			assert.Equal(t, input, output, "non-canonical header spacing should be preserved")
		})
	}
}

func TestSkeletonStore_WithTranslation(t *testing.T) {
	input := "Hello World\n\nGoodbye\n"
	ctx := t.Context()
	locale := model.LocaleID("fr")

	reader := wiki.NewReader()
	writer := wiki.NewWriter()

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
			case "Hello World":
				b.Targets[locale] = []*model.Segment{{ID: "s1", Runs: []model.Run{{Text: &model.TextRun{Text: "Bonjour le monde"}}}}}
			case "Goodbye":
				b.Targets[locale] = []*model.Segment{{ID: "s1", Runs: []model.Run{{Text: &model.TextRun{Text: "Au revoir"}}}}}
			}
		}
	}

	var buf bytes.Buffer
	writer.SetLocale(locale)
	require.NoError(t, writer.SetOutputWriter(&buf))

	ch := testutil.PartsToChannel(parts)
	require.NoError(t, writer.Write(ctx, ch))
	writer.Close()

	assert.Equal(t, "Bonjour le monde\n\nAu revoir\n", buf.String())
}
