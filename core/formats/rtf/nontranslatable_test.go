package rtf_test

import (
	"bytes"
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/formats/rtf"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// collect returns all PartBlock blocks (translatable and not).
func readBlocks(t *testing.T, input string, configure func(*rtf.Config)) []*model.Block {
	t.Helper()
	ctx := t.Context()
	reader := rtf.NewReader()
	if configure != nil {
		configure(reader.Config().(*rtf.Config))
	}
	require.NoError(t, reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish)))
	defer reader.Close()
	return testutil.CollectBlocks(t, reader.Read(ctx))
}

func findBlock(blocks []*model.Block, text string) *model.Block {
	for _, b := range blocks {
		if b.SourceText() == text {
			return b
		}
	}
	return nil
}

// roundtripSkeleton reads with a skeleton store and writes back, returning the
// merged bytes. Mirrors skeleton_test.go's helper but exposed here.
func roundtripSkeleton(t *testing.T, input string, configure func(*rtf.Config)) string {
	t.Helper()
	ctx := t.Context()

	reader := rtf.NewReader()
	if configure != nil {
		configure(reader.Config().(*rtf.Config))
	}
	writer := rtf.NewWriter()

	store, err := format.NewSkeletonStore()
	require.NoError(t, err)
	defer store.Close()
	reader.SetSkeletonStore(store)
	writer.SetSkeletonStore(store)

	require.NoError(t, reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish)))
	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	var buf bytes.Buffer
	require.NoError(t, writer.SetOutputWriter(&buf))
	require.NoError(t, writer.Write(ctx, testutil.PartsToChannel(parts)))
	writer.Close()
	return buf.String()
}

// --- Treatment A: header/footer content blocks ---

func TestNonTranslatableFooter(t *testing.T) {
	input := "{\\rtf1\\ansi\\deff0\n{\\footer Page footer text}\n\\pard Body.\\par\n}"
	blocks := readBlocks(t, input, nil)

	footer := findBlock(blocks, "Page footer text")
	require.NotNil(t, footer, "footer text should surface as a content block")
	assert.False(t, footer.Translatable)
	assert.Equal(t, model.RolePageFooter, footer.SemanticRole())
	assert.True(t, footer.PreserveWhitespace)

	body := findBlock(blocks, "Body.")
	require.NotNil(t, body)
	assert.True(t, body.Translatable)
}

func TestHeaderTranslatableWhenEnabled(t *testing.T) {
	// ExtractHeadersFooters on keeps the legacy translatable behavior even with
	// ExtractNonTranslatableContent on.
	input := "{\\rtf1\\ansi\\deff0\n{\\header Header text}\n\\pard Body.\\par\n}"
	blocks := readBlocks(t, input, func(c *rtf.Config) { c.ExtractHeadersFooters = true })

	h := findBlock(blocks, "Header text")
	require.NotNil(t, h)
	assert.True(t, h.Translatable, "header is translatable when ExtractHeadersFooters is on")
}

// --- Treatment A: \info title + doccomm ---

func TestNonTranslatableInfoTitleAndDocComment(t *testing.T) {
	input := "{\\rtf1\\ansi\\deff0\n{\\info{\\title Document Title}{\\doccomm A review comment}{\\author Yves}}\n\\pard Body.\\par\n}"
	blocks := readBlocks(t, input, nil)

	title := findBlock(blocks, "Document Title")
	require.NotNil(t, title, "\\info title should surface as a content block")
	assert.False(t, title.Translatable)
	assert.Equal(t, model.RoleTitle, title.SemanticRole())

	doccomm := findBlock(blocks, "A review comment")
	require.NotNil(t, doccomm, "\\info doccomm should surface as a content block")
	assert.False(t, doccomm.Translatable)
	assert.Equal(t, "comment", doccomm.SemanticRole())

	// \author is metadata, not surfaced.
	assert.Nil(t, findBlock(blocks, "Yves"), "\\author should stay in skeleton")

	body := findBlock(blocks, "Body.")
	require.NotNil(t, body)
	assert.True(t, body.Translatable)
}

// --- Treatment A: \xe index entry mid-paragraph ---

func TestNonTranslatableIndexEntry(t *testing.T) {
	input := "{\\rtf1\\ansi\\deff0\n\\pard Before {\\xe entry text}after.\\par\n}"
	blocks := readBlocks(t, input, nil)

	idx := findBlock(blocks, "entry text")
	require.NotNil(t, idx, "\\xe entry should surface as a content block")
	assert.False(t, idx.Translatable)
	assert.Equal(t, model.RoleIndex, idx.SemanticRole())

	body := findBlock(blocks, "Before after.")
	require.NotNil(t, body, "body paragraph continues past the index entry")
	assert.True(t, body.Translatable)
}

func TestNonTranslatableTOCEntry(t *testing.T) {
	input := "{\\rtf1\\ansi\\deff0\n\\pard {\\tc Chapter One}Heading\\par\n}"
	blocks := readBlocks(t, input, nil)

	toc := findBlock(blocks, "Chapter One")
	require.NotNil(t, toc, "\\tc entry should surface as a content block")
	assert.False(t, toc.Translatable)
	assert.Equal(t, model.RoleIndex, toc.SemanticRole())
}

// --- Treatment B: \annotation -> NoteAnnotation ---

func TestAnnotationCarriedAsNote(t *testing.T) {
	// Realistic \*-ignorable annotation structure: an author group then the
	// annotation comment group, anchored to body text.
	input := "{\\rtf1\\ansi\\deff0\n" +
		"\\pard Please review this line.{\\*\\atnid n}{\\*\\atnauthor Reviewer Name}{\\*\\annotation{\\*\\atnref 12345}Nice comment}\\par\n" +
		"}"
	ctx := t.Context()
	reader := rtf.NewReader()
	require.NoError(t, reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish)))
	defer reader.Close()
	blocks := testutil.CollectBlocks(t, reader.Read(ctx))

	body := findBlock(blocks, "Please review this line.")
	require.NotNil(t, body, "annotation IDs/text must not leak into the body block")
	assert.True(t, body.Translatable)

	// The annotation comment is a note, not a translatable block.
	assert.Nil(t, findBlock(blocks, "Nice comment"))
	assert.Nil(t, findBlock(blocks, "12345"), "atnref id must stay in skeleton")

	notes := body.Notes()
	require.Len(t, notes, 1)
	assert.Equal(t, "Nice comment", notes[0].Text)
	assert.Equal(t, "Reviewer Name", notes[0].From)
}

// On the legacy (ExtractNonTranslatableContent off) path the pre-existing
// ExtractAnnotations flag still extracts a bare {\annotation ...} as
// translatable text. (The \*-ignorable form is not detected on this path — that
// is the historical baseline preserved for byte-identical parity.)
func TestAnnotationTranslatableLegacyPath(t *testing.T) {
	input := "{\\rtf1\\ansi\\deff0\n" +
		"{\\annotation Nice comment}\n" +
		"\\pard Body.\\par\n" +
		"}"
	blocks := readBlocks(t, input, func(c *rtf.Config) {
		c.SetExtractNonTranslatableContent(false)
		c.ExtractAnnotations = true
	})
	nice := findBlock(blocks, "Nice comment")
	require.NotNil(t, nice, "annotation is translatable on the legacy path when ExtractAnnotations is on")
	assert.True(t, nice.Translatable)
}

// --- Opt-out: ExtractNonTranslatableContent off keeps everything in skeleton ---

func TestOptOutKeepsContentInSkeleton(t *testing.T) {
	input := "{\\rtf1\\ansi\\deff0\n" +
		"{\\footer Footer text}\n" +
		"{\\info{\\title Document Title}}\n" +
		"\\pard Before {\\xe entry text}after.\\par\n" +
		"}"
	blocks := readBlocks(t, input, func(c *rtf.Config) { c.SetExtractNonTranslatableContent(false) })

	// Only the body paragraph is emitted; the index entry is skeleton, so the
	// body text differs from the ENTC-on form ("Before after.").
	require.Len(t, blocks, 1)
	assert.True(t, blocks[0].Translatable)
	assert.Nil(t, findBlock(blocks, "Footer text"))
	assert.Nil(t, findBlock(blocks, "Document Title"))
	assert.Nil(t, findBlock(blocks, "entry text"))
}

// --- Byte-exact round-trip with content blocks present ---

func TestRoundTripByteExactWithContentBlocks(t *testing.T) {
	cases := map[string]string{
		"footer":      "{\\rtf1\\ansi\\deff0\n{\\footer Footer text}\n\\pard Body.\\par\n}\n",
		"info_title":  "{\\rtf1\\ansi\\deff0\n{\\info{\\title Document Title}{\\author Yves}}\n\\pard Body.\\par\n}\n",
		"index_entry": "{\\rtf1\\ansi\\deff0\n\\pard Before {\\xe entry text}after.\\par\n}\n",
		"annotation":  "{\\rtf1\\ansi\\deff0\n\\pard Body.{\\*\\atnauthor Bob}{\\*\\annotation Nice comment}\\par\n}\n",
	}
	for name, input := range cases {
		t.Run(name, func(t *testing.T) {
			// Both ENTC on (default) and off must round-trip byte-exactly.
			assert.Equal(t, input, roundtripSkeleton(t, input, nil), "ENTC on")
			assert.Equal(t, input, roundtripSkeleton(t, input, func(c *rtf.Config) {
				c.SetExtractNonTranslatableContent(false)
			}), "ENTC off")
		})
	}
}

// --- Config flag plumbing ---

func TestExtractNonTranslatableContentDefaultAndApplyMap(t *testing.T) {
	cfg := &rtf.Config{}
	cfg.Reset()
	assert.True(t, cfg.ExtractNonTranslatableContent(), "default is on")

	require.NoError(t, cfg.ApplyMap(map[string]any{"extractNonTranslatableContent": false}))
	assert.False(t, cfg.ExtractNonTranslatableContent())

	require.NoError(t, cfg.ApplyMap(map[string]any{"extractNonTranslatableContent": true}))
	assert.True(t, cfg.ExtractNonTranslatableContent())

	cfg.SetExtractNonTranslatableContent(false)
	assert.False(t, cfg.ExtractNonTranslatableContent())

	require.Error(t, cfg.ApplyMap(map[string]any{"extractNonTranslatableContent": "nope"}))
}

func TestSchemaHasExtractNonTranslatableContent(t *testing.T) {
	cfg := &rtf.Config{}
	sch := cfg.Schema()
	require.NotNil(t, sch)
	prop, ok := sch.Properties["extractNonTranslatableContent"]
	require.True(t, ok, "schema must declare extractNonTranslatableContent")
	assert.Equal(t, "boolean", prop.Type)
	assert.Equal(t, true, prop.Default)
}
