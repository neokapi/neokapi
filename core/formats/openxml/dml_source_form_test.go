package openxml

// okapi-filter: openxml

import (
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A slide's run split is a fact about the source that the model cannot hold.
//
// Upstream Okapi's RunMerger fuses two adjacent runs whose properties match
// (RunMerger.java:156-229) and #2512 kept that rule, so a translator sees one
// stretch of text rather than an arbitrary split. A run holding an empty
// <a:t/> carries no text at all, so nothing opens a run for it. Both are the
// right answer for the content model and the wrong answer for the bytes.
//
// So the reader keeps the paragraph's run-bearing children as the source wrote
// them, and the writer replays those bytes for a paragraph whose content
// nothing has changed. It is what #2537 does for a CT_Rst element, on the same
// terms: kept only where the writer would spell it differently, replayed only
// where the rendered content still says what the source said.

// dmlMergeableRuns is the shape #2532 reported against 1058.pptx: two adjacent
// runs with identical properties, which the model holds as one.
const dmlMergeableRuns = `<a:p>` +
	`<a:r><a:rPr lang="en-US" smtClean="0"/><a:t>Two</a:t></a:r>` +
	`<a:r><a:rPr lang="en-US" smtClean="0"/><a:t>rows</a:t></a:r>` +
	`<a:endParaRPr lang="en-US" dirty="0"/></a:p>`

// dmlEmptyTextRun is the second shape: a run whose <a:t/> holds nothing, beside
// a run that holds text. 1421-line-break.pptx, 977.pptx, sample.pptx and four
// others each carry one.
const dmlEmptyTextRun = `<a:p>` +
	`<a:r><a:rPr lang="fr-FR" dirty="0"/><a:t>Ligne</a:t></a:r>` +
	`<a:br><a:rPr lang="fr-FR" dirty="0"/></a:br>` +
	`<a:r><a:rPr lang="fr-FR" dirty="0"/><a:t/></a:r>` +
	`<a:endParaRPr lang="fr-FR" dirty="0"/></a:p>`

// dmlIndentedRuns is a paragraph a producer pretty-printed, so whitespace sits
// between the runs and inside each of them.
const dmlIndentedRuns = "<a:p>\r\n" +
	"                        <a:r>\r\n" +
	"                            <a:t>Para 1-1:</a:t>\r\n" +
	"                        </a:r>\r\n" +
	"                        <a:r>\r\n" +
	"                            <a:t> done.</a:t>\r\n" +
	"                        </a:r>\r\n" +
	"                    </a:p>"

// TestDMLSourceForm_AdjacentSamePropertyRunsKeepTheirBoundary is #2532's first
// shape: the merge that the content model needs must not reach the bytes.
func TestDMLSourceForm_AdjacentSamePropertyRunsKeepTheirBoundary(t *testing.T) {
	slide := dmlSlide(dmlMergeableRuns)
	out := skeletonRoundtripBytes(t, dmlDeck(t, slide), "deck.pptx")
	assert.Equal(t, slide, dmlSlideXML(t, out),
		"an untranslated shape keeps the run envelopes it was read with")
}

// TestDMLSourceForm_EmptyTextRunSurvives is #2532's second shape.
func TestDMLSourceForm_EmptyTextRunSurvives(t *testing.T) {
	slide := dmlSlide(dmlEmptyTextRun)
	out := skeletonRoundtripBytes(t, dmlDeck(t, slide), "deck.pptx")
	got := dmlSlideXML(t, out)
	assert.Equal(t, slide, got, "the run holding an empty <a:t/> keeps its envelope")
	assert.Contains(t, got, `<a:t/>`, "and the empty text element with it")
}

// TestDMLSourceForm_IndentedRunsKeepTheirWhitespace covers the third form the
// bytes carry and the model does not: the whitespace between the runs and
// inside them. The whitespace on either side of the run sequence sits outside
// the span the block holds and comes back through the skeleton (#2533).
func TestDMLSourceForm_IndentedRunsKeepTheirWhitespace(t *testing.T) {
	slide := dmlSlide(dmlIndentedRuns)
	out := skeletonRoundtripBytes(t, dmlDeck(t, slide), "deck.pptx")
	assert.Contains(t, dmlSlideXML(t, out),
		"<a:r>\r\n                            <a:t>Para 1-1:</a:t>\r\n"+
			"                        </a:r>\r\n"+
			"                        <a:r>",
		"the whitespace a producer indented the runs with comes back")
}

// TestDMLSourceForm_MergedRunsReachTheModelAsOne states the rule the source
// form must not undo: a translator still sees one stretch of text.
func TestDMLSourceForm_MergedRunsReachTheModelAsOne(t *testing.T) {
	blocks := dmlBlocks(t, dmlDeck(t, dmlSlide(dmlMergeableRuns)))
	require.Len(t, blocks, 1)
	assert.Equal(t, "Tworows", blocks[0].SourceText())
	texts := 0
	for _, r := range blocks[0].Source {
		if r.Text != nil {
			texts++
		}
	}
	assert.Equal(t, 1, texts, "the two source runs are one text run in the model")
}

// TestDMLSourceForm_TranslatedParagraphIsRendered is the other half: a target
// that says something else cannot carry the source's run split, so the writer
// rebuilds the paragraph from the runs.
func TestDMLSourceForm_TranslatedParagraphIsRendered(t *testing.T) {
	out := dmlWriteBack(t, dmlDeck(t, dmlSlide(dmlMergeableRuns)))
	got := dmlSlideXML(t, out)
	assert.Contains(t, got, "TWOROWS", "the translation is written back")
	assert.Equal(t, 1, strings.Count(got, "<a:r>"),
		"a rebuilt paragraph carries the merged run the model holds")
}

// TestDMLSourceForm_TargetSayingTheSourceReplays states the test the writer
// applies: the rendered content, not the presence of a target.
func TestDMLSourceForm_TargetSayingTheSourceReplays(t *testing.T) {
	slide := dmlSlide(dmlMergeableRuns)
	out := dmlWriteBackRuns(t, dmlDeck(t, slide), func(src []model.Run) []model.Run {
		return src
	})
	assert.Equal(t, slide, dmlSlideXML(t, out),
		"a target that says what the source said is untranslated as far as the bytes go")
}

// TestDMLSourceForm_KeptOnlyWhereTheWriterWouldDiffer keeps the property off
// every block whose paragraph the writer rebuilds byte for byte.
func TestDMLSourceForm_KeptOnlyWhereTheWriterWouldDiffer(t *testing.T) {
	plain := `<a:p><a:r><a:rPr lang="en-US"/><a:t>Title</a:t></a:r></a:p>`
	blocks := dmlBlocks(t, dmlDeck(t, dmlSlide(plain)))
	require.Len(t, blocks, 1)
	assert.Empty(t, blocks[0].Properties[dmlSourceProp],
		"a paragraph the writer spells the same way carries no source bytes")

	blocks = dmlBlocks(t, dmlDeck(t, dmlSlide(dmlMergeableRuns)))
	require.Len(t, blocks, 1)
	assert.Equal(t,
		`<a:r><a:rPr lang="en-US" smtClean="0"/><a:t>Two</a:t></a:r>`+
			`<a:r><a:rPr lang="en-US" smtClean="0"/><a:t>rows</a:t></a:r>`,
		blocks[0].Properties[dmlSourceProp],
		"the kept bytes span the run-bearing children and nothing else")
}
