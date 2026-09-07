package flow_test

import (
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/flow"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/tool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPartKey covers the identity a trace holds a part under: a boundary
// marker for the paired structural parts, and the plain resource id for
// everything else.
func TestPartKey(t *testing.T) {
	layer := &model.Layer{ID: "doc1", Name: "Document"}
	tests := []struct {
		name string
		part *model.Part
		want string
	}{
		{"layer start", &model.Part{Type: model.PartLayerStart, Resource: layer}, "doc1#start"},
		{"layer end", &model.Part{Type: model.PartLayerEnd, Resource: layer}, "doc1#end"},
		{"group start", &model.Part{Type: model.PartGroupStart, Resource: &model.GroupStart{ID: "g1"}}, "g1#start"},
		{"group end", &model.Part{Type: model.PartGroupEnd, Resource: &model.GroupEnd{ID: "g1"}}, "g1#end"},
		{"block", &model.Part{Type: model.PartBlock, Resource: model.NewBlock("b1", "hi")}, "b1"},
		{"data", &model.Part{Type: model.PartData, Resource: &model.Data{ID: "d1"}}, "d1"},
		{"media", &model.Part{Type: model.PartMedia, Resource: &model.Media{ID: "m1"}}, "m1"},
		{"no resource", &model.Part{Type: model.PartBlock}, ""},
		{"nil part", nil, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, flow.PartKey(tt.part))
		})
	}
}

// TestTraceKeepsEachLayerBoundarySeparate runs a two-layer document through a
// traced tool and proves each boundary keeps its own snapshot set.
//
// Both boundaries of a layer carry the same *model.Layer, so keying by the
// resource id alone let the end's initial snapshot replace the start's: a
// document part opened, in the Run view, in the state it closed in, and any
// per-node snapshot taken in between went with it.
func TestTraceKeepsEachLayerBoundarySeparate(t *testing.T) {
	rec := flow.NewTraceRecorder()
	upper := flow.NewTracingTool(&tool.BaseTool{
		ToolName: "upper",
		Produce: func(v tool.VariantView) error {
			if v.Translatable() {
				v.SetTargetText(model.LocaleFrench, strings.ToUpper(v.SourceText()))
			}
			return nil
		},
	}, "upper-node", rec)

	f, err := flow.NewFlow("two-layers").AddTool(upper).Build()
	require.NoError(t, err)

	in, out, wait := flow.NewExecutor().ExecuteWithChannels(t.Context(), f)

	// Two documents, each opened and closed around one translatable block,
	// fed the way a reader feeds them: an initial snapshot per part.
	var parts []*model.Part
	for _, doc := range []string{"doc1", "doc2"} {
		layer := &model.Layer{ID: doc, Name: doc}
		block := model.NewBlock(doc+"-b1", "hello "+doc)
		parts = append(parts,
			&model.Part{Type: model.PartLayerStart, Resource: layer},
			&model.Part{Type: model.PartBlock, Resource: block},
			&model.Part{Type: model.PartLayerEnd, Resource: layer},
		)
	}
	go func() {
		for _, p := range parts {
			rec.SnapshotPart(p, "reader", "initial")
			in <- p
		}
		close(in)
	}()
	for range out { //nolint:revive // draining is the point; the trace is what is asserted
	}
	require.NoError(t, wait())

	snaps := rec.Snapshots()
	require.Len(t, snaps, 6, "two documents: a start, a block and an end each")

	for _, doc := range []string{"doc1", "doc2"} {
		start := snaps[doc+"#start"]
		end := snaps[doc+"#end"]
		require.NotNil(t, start, "%s keeps its own start", doc)
		require.NotNil(t, end, "%s keeps its own end", doc)

		assert.Equal(t, "LayerStart", start.Initial.Type,
			"the initial snapshot of %s is the document's start state", doc)
		assert.Equal(t, "Layer: "+doc, start.Initial.Summary)
		assert.Equal(t, "LayerEnd", end.Initial.Type)
		assert.Equal(t, "end layer "+doc, end.Initial.Summary)

		// The display id stays the resource id: the key separates the two
		// boundaries, and the inspector still names the document.
		assert.Equal(t, doc, start.Initial.ID)
		assert.Equal(t, doc, end.Initial.ID)

		// The tool's snapshot of each boundary landed on that boundary.
		assert.Contains(t, start.AfterNode, "upper-node")
		assert.Contains(t, end.AfterNode, "upper-node")
		assert.Equal(t, "LayerStart", start.AfterNode["upper-node"].Type)
		assert.Equal(t, "LayerEnd", end.AfterNode["upper-node"].Type)
	}

	// The block between them is unaffected, and its target is the tool's work.
	block := snaps["doc1-b1"]
	require.NotNil(t, block)
	assert.Equal(t, "HELLO DOC1", block.AfterNode["upper-node"].TargetText)

	// Every event names one of the six keys, so the Run view's playback and
	// its snapshots agree on what a part is.
	keys := map[string]bool{}
	for k := range snaps {
		keys[k] = true
	}
	for _, ev := range rec.Events() {
		assert.True(t, keys[ev.PartID], "event names a known part: %q", ev.PartID)
	}
	assert.Contains(t, keys, "doc1#start")
	assert.Contains(t, keys, "doc1#end")
}
