package editor

import (
	"encoding/json"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// layerStart, etc. build Parts for a content stream in document order.
func layerStart(l *model.Layer) *model.Part {
	return &model.Part{Type: model.PartLayerStart, Resource: l}
}
func layerEnd(id string) *model.Part {
	return &model.Part{Type: model.PartLayerEnd, Resource: &model.Layer{ID: id}}
}
func groupStart(g *model.GroupStart) *model.Part {
	return &model.Part{Type: model.PartGroupStart, Resource: g}
}
func groupEnd(id string) *model.Part {
	return &model.Part{Type: model.PartGroupEnd, Resource: &model.GroupEnd{ID: id}}
}
func blockPart(b *model.Block) *model.Part {
	return &model.Part{Type: model.PartBlock, Resource: b}
}
func dataPart(d *model.Data) *model.Part {
	return &model.Part{Type: model.PartData, Resource: d}
}

// richBlock is a multi-segment block carrying a text run, an inline
// placeholder, and an ICU plural — the interesting cases for a learner. The
// source is a flat run sequence (AD-002); the two sentence boundaries are a
// stand-off segmentation overlay rather than structural segments.
func richBlock() *model.Block {
	b := &model.Block{
		ID:           "b1",
		Translatable: true,
		Source: []model.Run{
			{Text: &model.TextRun{Text: "Hello "}},
			{Ph: &model.PlaceholderRun{ID: "1", Type: "var", Data: "{name}", Equiv: "{name}"}},
			{Plural: &model.PluralRun{
				Pivot: "count",
				Forms: map[model.PluralForm][]model.Run{
					model.PluralOne:   {{Text: &model.TextRun{Text: "1 item"}}},
					model.PluralOther: {{Text: &model.TextRun{Text: "# items"}}},
				},
			}},
		},
		Targets: map[model.VariantKey]*model.Target{
			model.Variant("fr"): {Runs: []model.Run{{Text: &model.TextRun{Text: "Bonjour"}}}},
		},
	}
	b.SetSegmentation(nil, []model.Span{
		{ID: "s1", Range: model.RunRange{StartRun: 0, EndRun: 2}},
		{ID: "s2", Range: model.RunRange{StartRun: 2, EndRun: 3}},
	})
	return b
}

func TestBuildContentTree_Hierarchy(t *testing.T) {
	// Root layer (JSON) → group → rich block; sibling data; an embedded
	// child layer (HTML-in-JSON) holding a simple block.
	parts := []*model.Part{
		layerStart(&model.Layer{ID: "root", Name: "document", Format: "json", Locale: "en"}),
		groupStart(&model.GroupStart{ID: "g1", Name: "messages"}),
		blockPart(richBlock()),
		groupEnd("g1"),
		dataPart(&model.Data{ID: "d1", Name: "punctuation", Skeleton: &model.Skeleton{}}),
		layerStart(&model.Layer{ID: "html1", Name: "embedded html", Format: "html", Locale: "en", ParentID: "root"}),
		blockPart(model.NewBlock("b2", "Click here")),
		layerEnd("html1"),
		layerEnd("root"),
	}

	tree := BuildContentTree(parts, "json")

	assert.Equal(t, "json", tree.Format)
	assert.Equal(t, ContentStats{Layers: 2, Groups: 1, Blocks: 2, Data: 1, Runs: 4}, tree.Stats)

	require.Len(t, tree.Root, 1, "single root layer")
	root := tree.Root[0]
	assert.Equal(t, "layer", root.Kind)
	assert.Equal(t, "json", root.Format)

	// Root children in document order: group, data, embedded layer.
	require.Len(t, root.Children, 3)
	assert.Equal(t, "group", root.Children[0].Kind)
	assert.Equal(t, "data", root.Children[1].Kind)
	assert.Equal(t, "layer", root.Children[2].Kind)

	// Group holds the rich block.
	grp := root.Children[0]
	require.Len(t, grp.Children, 1)
	b1 := grp.Children[0]
	assert.Equal(t, "block", b1.Kind)
	assert.Len(t, b1.Source, 3, "two text/ph runs + one plural run, flattened across segments")

	// Embedded child layer carries its own block, demonstrating nesting.
	embedded := root.Children[2]
	assert.Equal(t, "html", embedded.Format)
	assert.Equal(t, "root", embedded.ParentID)
	require.Len(t, embedded.Children, 1)
	assert.Equal(t, "block", embedded.Children[0].Kind)
}

func TestBuildContentTree_SegmentOverlayAndTargets(t *testing.T) {
	tree := BuildContentTree([]*model.Part{blockPart(richBlock())}, "json")
	require.Len(t, tree.Root, 1)
	b := tree.Root[0]

	// Segment boundaries are an overlay of half-open run-index ranges.
	require.Len(t, b.Segments, 2)
	assert.Equal(t, SegmentSpan{ID: "s1", Start: 0, End: 2}, b.Segments[0])
	assert.Equal(t, SegmentSpan{ID: "s2", Start: 2, End: 3}, b.Segments[1])

	// Targets are flattened run sequences keyed by locale string.
	require.Contains(t, b.Targets, "fr")
	require.Len(t, b.Targets["fr"], 1)
	require.NotNil(t, b.Targets["fr"][0].Text)
	assert.Equal(t, "Bonjour", b.Targets["fr"][0].Text.Text)
}

func TestBuildContentTree_SingleSegmentHasNoOverlay(t *testing.T) {
	// A single-segment block carries no meaningful boundary overlay.
	tree := BuildContentTree([]*model.Part{blockPart(model.NewBlock("b", "just text"))}, "json")
	require.Len(t, tree.Root, 1)
	assert.Nil(t, tree.Root[0].Segments)
	assert.Len(t, tree.Root[0].Source, 1)
}

func TestBuildContentTree_RunsSerializeWithDiscriminators(t *testing.T) {
	// The JSON must preserve the RFC 0001 run discriminators so the UI can
	// render text vs placeholder vs plural distinctly.
	tree := BuildContentTree([]*model.Part{blockPart(richBlock())}, "json")
	data, err := json.Marshal(tree)
	require.NoError(t, err)
	s := string(data)
	assert.Contains(t, s, `"text"`)
	assert.Contains(t, s, `"ph"`)
	assert.Contains(t, s, `"plural"`)
	assert.Contains(t, s, `"forms"`)
	assert.Contains(t, s, `"segments"`)
}

func TestBuildContentTree_MalformedStreamDoesNotPanic(t *testing.T) {
	// Stray End parts with no matching open container must be tolerated, and
	// content must still attach (to the root when no container is open).
	parts := []*model.Part{
		groupEnd("ghost"),
		layerEnd("ghost"),
		blockPart(model.NewBlock("b", "orphan")),
		layerStart(&model.Layer{ID: "l", Format: "json"}),
		blockPart(model.NewBlock("b2", "nested")),
		// missing layerEnd — unbalanced
	}

	var tree *ContentTree
	require.NotPanics(t, func() { tree = BuildContentTree(parts, "json") })

	// Orphan block sits at root; the layer (still open at EOF) also sits at
	// root and holds its nested block.
	require.Len(t, tree.Root, 2)
	assert.Equal(t, "block", tree.Root[0].Kind)
	assert.Equal(t, "layer", tree.Root[1].Kind)
	require.Len(t, tree.Root[1].Children, 1)
	assert.Equal(t, "b2", tree.Root[1].Children[0].ID)
}

func TestBuildContentTree_StructureAndGeometry(t *testing.T) {
	// A heading block carrying the WS1 structural layer: semantic role + level,
	// layout layer, and page geometry.
	h := model.NewBlock("b1", "Overview")
	h.SetSemanticRole(model.RoleHeading, 2)
	h.SetGeometry(&model.GeometryAnnotation{
		Page: 1, BBox: model.Rect{X: 72, Y: 60, W: 428, H: 32},
		Origin: "top-left", Resolution: 512,
	})
	// A furniture block (running header).
	ph := model.NewBlock("b2", "Confidential")
	ph.SetSemanticRole(model.RolePageHeader, 0)
	ph.SetLayoutLayer(model.LayerFurniture)

	tree := BuildContentTree([]*model.Part{blockPart(h), blockPart(ph)}, "docling")
	require.Len(t, tree.Root, 2)

	hn := tree.Root[0]
	require.NotNil(t, hn.Structure, "heading should carry structure")
	assert.Equal(t, model.RoleHeading, hn.Structure.Role)
	assert.Equal(t, 2, hn.Structure.Level)
	require.NotNil(t, hn.Geometry, "heading should carry geometry")
	assert.Equal(t, 1, hn.Geometry.Page)
	assert.Equal(t, GeometryView{Page: 1, X: 72, Y: 60, W: 428, H: 32, Origin: "top-left", Resolution: 512}, *hn.Geometry)

	phn := tree.Root[1]
	require.NotNil(t, phn.Structure)
	assert.Equal(t, model.RolePageHeader, phn.Structure.Role)
	assert.Equal(t, model.LayerFurniture, phn.Structure.Layer)
	assert.Nil(t, phn.Geometry, "no geometry set → nil")

	// Structure/geometry are first-class fields, NOT generic annotation rows.
	for _, a := range hn.Annotations {
		assert.NotEqual(t, model.AnnoStructure, a.Type, "structure must not appear as a generic annotation")
		assert.NotEqual(t, model.AnnoGeometry, a.Type, "geometry must not appear as a generic annotation")
	}

	// Round-trips through JSON with the expected field names.
	b, err := json.Marshal(hn)
	require.NoError(t, err)
	assert.Contains(t, string(b), `"structure":{"role":"heading","level":2}`)
	assert.Contains(t, string(b), `"geometry":{"page":1,"x":72`)
}

func TestBuildContentTree_NoStructureWhenUnset(t *testing.T) {
	tree := BuildContentTree([]*model.Part{blockPart(model.NewBlock("b", "plain"))}, "json")
	require.Len(t, tree.Root, 1)
	assert.Nil(t, tree.Root[0].Structure, "plain block has no structure")
	assert.Nil(t, tree.Root[0].Geometry, "plain block has no geometry")
}
