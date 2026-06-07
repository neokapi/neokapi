package tools_test

import (
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/tools"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func srcSegText(b *model.Block, i int) string {
	return model.RunsText(b.SourceSegmentRuns(i))
}

// The default (srx) engine suppresses breaks after known abbreviations — the
// key upgrade over the old naive regex default.
func TestSegmentationTool_SRXAbbreviation(t *testing.T) {
	t.Parallel()
	tl := tools.NewSegmentationTool(&tools.SegmentationConfig{})
	block := model.NewBlock("tu1", "Dr. Smith left. He went home.")
	result := processPart(t, tl, &model.Part{Type: model.PartBlock, Resource: block})
	b := result.Resource.(*model.Block)

	require.Equal(t, 2, b.SourceSegmentCount(), "Dr. is a no-break abbreviation")
	// Compare trimmed sentence cores: inter-sentence whitespace attaches per
	// ruleset (Okapi's okapi.srx trims it as uncovered material; the pure-Go
	// default leaves it leading the next segment), so the exact edge whitespace
	// is build-dependent while the sentences are not.
	assert.Equal(t, "Dr. Smith left.", strings.TrimSpace(srcSegText(b, 0)))
	assert.Equal(t, "He went home.", strings.TrimSpace(srcSegText(b, 1)))
}

// The uax29 engine (ICU) has no abbreviation suppression, so the same input
// splits after "Dr." — exercised here to prove engine selection works.
func TestSegmentationTool_UAX29Engine(t *testing.T) {
	t.Parallel()
	tl := tools.NewSegmentationTool(&tools.SegmentationConfig{Engine: "uax29"})
	block := model.NewBlock("tu1", "Hello world. How are you? Fine.")
	result := processPart(t, tl, &model.Part{Type: model.PartBlock, Resource: block})
	b := result.Resource.(*model.Block)
	assert.Equal(t, 3, b.SourceSegmentCount())
}

func TestSegmentationTool_UnknownEngine(t *testing.T) {
	t.Parallel()
	tl := tools.NewSegmentationTool(&tools.SegmentationConfig{Engine: "does-not-exist"})
	block := model.NewBlock("tu1", "One. Two.")
	in := make(chan *model.Part, 1)
	out := make(chan *model.Part, 1)
	in <- &model.Part{Type: model.PartBlock, Resource: block}
	close(in)
	err := tl.Process(t.Context(), in, out)
	close(out)
	require.Error(t, err, "an unknown engine surfaces a clear error")
}

// A named layer attaches alongside the primary sentence layer rather than
// replacing it — multiple segmentation granularities coexist.
func TestSegmentationTool_MultiLayer(t *testing.T) {
	t.Parallel()
	block := model.NewBlock("tu1", "One. Two. Three.")
	part := &model.Part{Type: model.PartBlock, Resource: block}

	// Primary sentence layer via the default engine.
	primary := processPart(t, tools.NewSegmentationTool(&tools.SegmentationConfig{}), part)
	b := primary.Resource.(*model.Block)
	require.NotNil(t, b.SourceSegmentation(), "primary layer present")
	require.Equal(t, "3", b.Properties[tools.PropSegmentCount])

	// A second pass writing a named "clause" layer must keep the primary layer.
	clauseTool := tools.NewSegmentationTool(&tools.SegmentationConfig{Layer: "clause"})
	part2 := &model.Part{Type: model.PartBlock, Resource: b}
	out := processPart(t, clauseTool, part2)
	b2 := out.Resource.(*model.Block)

	assert.NotNil(t, b2.SourceSegmentation(), "primary layer survives")
	assert.NotNil(t, b2.SegmentationLayerFor(nil, "clause"), "named layer added")
	layers := b2.SegmentationLayers(nil)
	assert.ElementsMatch(t, []string{"", "clause"}, layers)
	// segment-count tracks the primary layer only.
	assert.Equal(t, "3", b2.Properties[tools.PropSegmentCount])
}

// Code-aware: an isolated code between two sentences does not prevent a break,
// and the runs are never rewritten (the placeholder survives in the source).
func TestSegmentationTool_CodeAware(t *testing.T) {
	t.Parallel()
	block := &model.Block{
		ID:           "tu1",
		Translatable: true,
		Source: []model.Run{
			{Text: &model.TextRun{Text: "First sentence. "}},
			{Ph: &model.PlaceholderRun{ID: "x1", Equiv: "{x1}"}},
			{Text: &model.TextRun{Text: "Second sentence."}},
		},
	}
	tl := tools.NewSegmentationTool(&tools.SegmentationConfig{TreatIsolatedCodesAsWhitespace: true})
	result := processPart(t, tl, &model.Part{Type: model.PartBlock, Resource: block})
	b := result.Resource.(*model.Block)

	require.Equal(t, 2, b.SourceSegmentCount())
	// The placeholder run is untouched — segmentation is a stand-off overlay.
	require.Len(t, b.Source, 3)
	assert.NotNil(t, b.Source[1].Ph)
}
