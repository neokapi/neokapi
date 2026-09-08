package openxml

// okapi-filter: openxml

import (
	"bytes"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A tool such as `kapi sed` with no target locale edits a block's source runs
// in place. Every source-form replay compares a rendering against the source
// runs, and an edited source is its own comparison, so the reader stamps each
// block with a digest of the runs it emitted and a writer replays nothing
// over a block whose runs no longer match it. One test per surface.

func TestSourceFingerprint_TellsAnEditedSourceFromTheOneRead(t *testing.T) {
	b := model.NewBlock("tu1", "Hello")
	assert.False(t, sourceRunsAsRead(b), "a block nothing stamped is treated as edited")

	stampSourceFingerprint(b)
	assert.True(t, sourceRunsAsRead(b))

	b.Source = []model.Run{{Text: &model.TextRun{Text: "Hallo"}}}
	assert.False(t, sourceRunsAsRead(b), "an edit to the source runs is seen")

	b.Source = []model.Run{{Text: &model.TextRun{Text: "Hello"}}}
	assert.True(t, sourceRunsAsRead(b), "runs that say what was read match again")

	assert.False(t, sourceRunsAsRead(nil))
}

// editSourceInPlace round-trips a package with every translatable block's
// source runs rewritten by edit, and no target locale on the writer.
func editSourceInPlace(t *testing.T, original []byte, uri string, edit func(*model.Block)) []byte {
	t.Helper()
	skelStore, err := format.NewSkeletonStore()
	require.NoError(t, err)
	defer skelStore.Close()

	reader := NewReader()
	reader.SetSkeletonStore(skelStore)
	require.NoError(t, reader.Open(t.Context(), &model.RawDocument{
		URI:          uri,
		SourceLocale: model.LocaleEnglish,
		Encoding:     "UTF-8",
		Reader:       readCloserFromBytes(original),
	}))
	parts := testutil.CollectParts(t, reader.Read(t.Context()))
	require.NoError(t, reader.Close())

	for _, p := range parts {
		if b, ok := p.Resource.(*model.Block); ok && b.Translatable {
			edit(b)
		}
	}

	var buf bytes.Buffer
	writer := NewWriter()
	writer.SetOriginalContent(original)
	writer.SetSkeletonStore(skelStore)
	require.NoError(t, writer.SetOutputWriter(&buf))
	require.NoError(t, writer.Write(t.Context(), testutil.PartsToChannel(parts)))
	require.NoError(t, writer.Close())
	return buf.Bytes()
}

// replaceInSource rewrites every text run of a block's source in place.
func replaceInSource(from, to string) func(*model.Block) {
	return func(b *model.Block) {
		runs := make([]model.Run, len(b.Source))
		for i, r := range b.Source {
			if r.Text != nil {
				text := *r.Text
				text.Text = strings.ReplaceAll(text.Text, from, to)
				r.Text = &text
			}
			runs[i] = r
		}
		b.Source = runs
	}
}

func TestSourceFingerprint_EditedDocxParagraphIsRendered(t *testing.T) {
	pkg := replayDocx(t, replaySourceParagraph)
	out := editSourceInPlace(t, pkg, "edit.docx", replaceInSource("plain", "EDITED"))
	got := string(zipPartBytes(t, out, "word/document.xml"))
	assert.Contains(t, got, "EDITED", "the edit reaches the output")
	assert.NotContains(t, got, replaySourceParagraph, "the paragraph is rendered rather than replayed")
}

func TestSourceFingerprint_EditedSharedStringIsRendered(t *testing.T) {
	sst := sourceFormSST(`<si><t xml:space="preserve">Plain</t></si>`)
	out := editSourceInPlace(t, richTextPackage(t, sst), "edit.xlsx", replaceInSource("Plain", "EDITED"))
	got := string(zipPartBytes(t, out, "xl/sharedStrings.xml"))
	assert.Contains(t, got, "EDITED", "the edit reaches the output")
	assert.NotContains(t, got, "Plain", "the source form is not replayed over the edit")
}

func TestSourceFingerprint_EditedSlideParagraphIsRendered(t *testing.T) {
	slide := dmlSlide(dmlMergeableRuns)
	out := editSourceInPlace(t, dmlDeck(t, slide), "edit.pptx", replaceInSource("Two", "EDITED"))
	got := dmlSlideXML(t, out)
	assert.Contains(t, got, "EDITED", "the edit reaches the output")
	assert.NotEqual(t, slide, got, "the source form is not replayed over the edit")
}
