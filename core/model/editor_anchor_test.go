package model_test

import (
	"encoding/json"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// editorAnchorCases covers the four per-ecosystem payload shapes named in
// format-maturity §2.3: Word content-control tag, Figma node id (+ sub-node
// range), Google Docs named range, and a headless-CMS entry+field path.
func editorAnchorCases() []struct {
	name   string
	anchor *model.EditorAnchor
} {
	figmaSub := &model.RunRange{StartRun: 0, StartOffset: 2, EndRun: 1, EndOffset: 0}
	return []struct {
		name   string
		anchor *model.EditorAnchor
	}{
		{
			name: "word",
			anchor: &model.EditorAnchor{
				System: model.EditorSystemWord,
				Ref:    "cc-tag-42",
				Extra:  map[string]string{"docId": "Document.docx"},
			},
		},
		{
			name: "figma",
			anchor: &model.EditorAnchor{
				System: model.EditorSystemFigma,
				Ref:    "1:23",
				Range:  figmaSub, // optional sub-node character span
				Extra:  map[string]string{"fileKey": "abc123"},
			},
		},
		{
			name: "gdocs",
			anchor: &model.EditorAnchor{
				System: model.EditorSystemGDocs,
				Ref:    "kix.namedrange.7",
				Extra:  map[string]string{"revisionId": "r9"},
			},
		},
		{
			name: "cms",
			anchor: &model.EditorAnchor{
				System: model.EditorSystemCMS,
				Ref:    "entry-123#title",
				Extra:  map[string]string{"locale": "en-US", "space": "marketing"},
			},
		},
	}
}

func TestEditorAnchor_TypeNameMatchesOverlayKind(t *testing.T) {
	t.Parallel()
	assert.Equal(t, model.OverlayEditorAnchor, model.OverlayType("editor-anchor"))
	assert.Equal(t, string(model.OverlayEditorAnchor), (&model.EditorAnchor{}).TypeName())
}

func TestEditorAnchor_RegisteredInRegistry(t *testing.T) {
	t.Parallel()
	p, ok := model.NewPayload(string(model.OverlayEditorAnchor))
	require.True(t, ok, "editor-anchor payload must be registered")
	require.NotNil(t, p)
	_, isAnchor := p.(*model.EditorAnchor)
	assert.True(t, isAnchor, "registry must produce a *EditorAnchor")
	assert.Equal(t, "editor-anchor", p.TypeName())
}

// TestEditorAnchor_JSONRoundTrip asserts each per-ecosystem payload marshals
// and unmarshals losslessly as a plain struct.
func TestEditorAnchor_JSONRoundTrip(t *testing.T) {
	t.Parallel()
	for _, tc := range editorAnchorCases() {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			data, err := json.Marshal(tc.anchor)
			require.NoError(t, err)

			var decoded model.EditorAnchor
			require.NoError(t, json.Unmarshal(data, &decoded))
			assert.Equal(t, *tc.anchor, decoded, "payload must round-trip losslessly")
		})
	}
}

// TestEditorAnchor_RegistryEnvelopeRoundTrip exercises the model's typed-payload
// serialization contract — the {type,data} envelope plus model.NewPayload that
// the wire (core/plugin/protoconvert) and store (bowrain/core/sync) layers use,
// because a plain json.Unmarshal cannot reconstruct the polymorphic Span.Value
// interface. The concrete *EditorAnchor (and its optional Range) must survive.
func TestEditorAnchor_RegistryEnvelopeRoundTrip(t *testing.T) {
	t.Parallel()
	for _, tc := range editorAnchorCases() {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var src model.Payload = tc.anchor
			data, err := json.Marshal(src)
			require.NoError(t, err)
			typeName := model.PayloadTypeName(src)
			assert.Equal(t, "editor-anchor", typeName)

			rebuilt, ok := model.NewPayload(typeName)
			require.True(t, ok)
			require.NoError(t, json.Unmarshal(data, rebuilt))

			got, ok := rebuilt.(*model.EditorAnchor)
			require.True(t, ok)
			assert.Equal(t, tc.anchor, got)
		})
	}
}

// TestEditorAnchor_BlockAccessors covers the add/get helpers and confirms the
// anchor rides on Block.Overlays (positional) and never leaks into the
// block-scoped annotation map.
func TestEditorAnchor_BlockAccessors(t *testing.T) {
	t.Parallel()
	b := model.NewRunsBlock("b1", []model.Run{
		{Text: &model.TextRun{Text: "Hello "}},
		{Text: &model.TextRun{Text: "world"}},
	})

	assert.Nil(t, b.OverlayOf(model.OverlayEditorAnchor))
	assert.Empty(t, b.EditorAnchors())
	assert.Nil(t, b.EditorAnchorByID("a1"))

	b.AddEditorAnchor("a1", model.RunRange{StartRun: 0, EndRun: 2},
		&model.EditorAnchor{System: model.EditorSystemWord, Ref: "cc-tag-1"})
	b.AddEditorAnchor("a2", model.RunRange{StartRun: 1, EndRun: 2},
		&model.EditorAnchor{System: model.EditorSystemFigma, Ref: "1:23"})

	o := b.OverlayOf(model.OverlayEditorAnchor)
	require.NotNil(t, o)
	assert.Equal(t, model.OverlayEditorAnchor, o.Type)
	assert.True(t, o.OnSource())
	require.Len(t, o.Spans, 2)

	anchors := b.EditorAnchors()
	require.Len(t, anchors, 2)
	assert.Equal(t, model.EditorSystemWord, anchors[0].System)
	assert.Equal(t, "1:23", anchors[1].Ref)

	got := b.EditorAnchorByID("a2")
	require.NotNil(t, got)
	assert.Equal(t, model.EditorSystemFigma, got.System)

	// Overlays never leak into the annotation map.
	assert.Empty(t, b.AnnoMap())
}

// TestEditorAnchor_SurvivesBlockCopy asserts the editor-anchor overlay survives
// a structural Block copy (the model has no deep Clone; this mirrors the
// element-wise copy used across the codebase, e.g. core/model bench_test.go).
func TestEditorAnchor_SurvivesBlockCopy(t *testing.T) {
	t.Parallel()
	src := model.NewRunsBlock("b1", []model.Run{{Text: &model.TextRun{Text: "Title"}}})
	subRange := &model.RunRange{StartRun: 0, StartOffset: 0, EndRun: 0, EndOffset: 5}
	src.AddEditorAnchor("a1", model.RunRange{StartRun: 0, EndRun: 1},
		&model.EditorAnchor{
			System: model.EditorSystemGDocs,
			Ref:    "kix.namedrange.7",
			Range:  subRange,
			Extra:  map[string]string{"revisionId": "r9"},
		})

	clone := &model.Block{
		ID:       src.ID,
		Source:   append([]model.Run(nil), src.Source...),
		Overlays: append([]model.Overlay(nil), src.Overlays...),
	}

	got := clone.EditorAnchorByID("a1")
	require.NotNil(t, got, "anchor must survive the copy")
	assert.Equal(t, model.EditorSystemGDocs, got.System)
	assert.Equal(t, "kix.namedrange.7", got.Ref)
	require.NotNil(t, got.Range)
	assert.Equal(t, *subRange, *got.Range)
	assert.Equal(t, "r9", got.Extra["revisionId"])
}

// blockJSONEnvelope is a faithful model-JSON projection of the slice of a Block
// that an editor anchor touches: the source runs (via Run's own JSON codec) and
// the run-anchored overlay spans, each span's polymorphic Value carried as the
// {type,data} typed-payload envelope. It mirrors the contract in
// core/plugin/protoconvert and bowrain/core/sync so the round-trip below is the
// same one a read→write→read across the wire/store performs.
type blockJSONEnvelope struct {
	ID       string           `json:"id"`
	Source   []model.Run      `json:"source"`
	Overlays []overlayJSONEnv `json:"overlays,omitempty"`
}

type overlayJSONEnv struct {
	Type  model.OverlayType `json:"type"`
	Spans []spanJSONEnv     `json:"spans"`
}

type spanJSONEnv struct {
	ID    string          `json:"id"`
	Range model.RunRange  `json:"range"`
	Type  string          `json:"valueType,omitempty"`
	Value json.RawMessage `json:"value,omitempty"`
}

func encodeBlock(t *testing.T, b *model.Block) []byte {
	t.Helper()
	env := blockJSONEnvelope{ID: b.ID, Source: b.Source}
	for i := range b.Overlays {
		o := &b.Overlays[i]
		oe := overlayJSONEnv{Type: o.Type}
		for _, s := range o.Spans {
			se := spanJSONEnv{ID: s.ID, Range: s.Range}
			if s.Value != nil {
				data, err := json.Marshal(s.Value)
				require.NoError(t, err)
				se.Type = model.PayloadTypeName(s.Value)
				se.Value = data
			}
			oe.Spans = append(oe.Spans, se)
		}
		env.Overlays = append(env.Overlays, oe)
	}
	out, err := json.Marshal(env)
	require.NoError(t, err)
	return out
}

func decodeBlock(t *testing.T, data []byte) *model.Block {
	t.Helper()
	var env blockJSONEnvelope
	require.NoError(t, json.Unmarshal(data, &env))
	b := &model.Block{ID: env.ID, Source: env.Source}
	for _, oe := range env.Overlays {
		o := model.Overlay{Type: oe.Type}
		for _, se := range oe.Spans {
			s := model.Span{ID: se.ID, Range: se.Range}
			if se.Type != "" {
				p, ok := model.NewPayload(se.Type)
				require.True(t, ok, "payload type %q must be registered", se.Type)
				require.NoError(t, json.Unmarshal(se.Value, p))
				s.Value = p
			}
			o.Spans = append(o.Spans, s)
		}
		b.Overlays = append(b.Overlays, o)
	}
	return b
}

// TestEditorAnchor_SurvivesInOutOperation is the anchor-survivability check: a
// Block with source runs + an editor-anchor overlay is marshalled to the
// model's JSON and read back, and both the anchor payload and its run-anchored
// Range must survive the round-trip intact.
//
// The end-to-end spec-case "anchor-survivability in-out operation"
// (format-spec-cases.md) — proving an anchor survives a real edit cycle in the
// native editor — rides issue #847 and is not wired here.
func TestEditorAnchor_SurvivesInOutOperation(t *testing.T) {
	t.Parallel()
	for _, tc := range editorAnchorCases() {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			b := model.NewRunsBlock("b1", []model.Run{
				{Text: &model.TextRun{Text: "Read, change, "}},
				{Text: &model.TextRun{Text: "and ship content."}},
			})
			spanRange := model.RunRange{StartRun: 0, EndRun: 2}
			b.AddEditorAnchor("a1", spanRange, tc.anchor)

			// read → write → read.
			got := decodeBlock(t, encodeBlock(t, b))

			assert.Equal(t, "Read, change, and ship content.", got.SourceText(),
				"source runs survive the round-trip")

			o := got.OverlayOf(model.OverlayEditorAnchor)
			require.NotNil(t, o, "editor-anchor overlay survives")
			require.Len(t, o.Spans, 1)
			assert.Equal(t, "a1", o.Spans[0].ID)
			assert.Equal(t, spanRange, o.Spans[0].Range, "carrying span Range survives")

			anchor := got.EditorAnchorByID("a1")
			require.NotNil(t, anchor)
			assert.Equal(t, tc.anchor, anchor, "anchor payload survives losslessly")
			if tc.anchor.Range != nil {
				require.NotNil(t, anchor.Range, "optional sub-block Range survives")
				assert.Equal(t, *tc.anchor.Range, *anchor.Range)
			}
		})
	}
}
