// Canonical wire-schema compat suite (AD-034).
//
// protoconvert is the canonical converter between core/model and the
// content-model wire schema in core/proto/content/v1. These tests are the
// compatibility oracle for that schema: model → proto → model must be
// identity for every message kind the wire carries. A failure here means the
// canonical schema (or its converter) changed behavior — treat it as a
// wire-compatibility break, not a test to update casually.
//
// Known, deliberate non-carriage (schema limitations, not regressions):
//   - Target variants are keyed by locale on the wire (TargetEntry.locale);
//     tone/channel variant dimensions and Target status/origin/score do not
//     cross this schema. The sync protocol stashes those in segment
//     properties (core/venue).
//   - Segmentation overlays are reconstructed from segment boundaries rather
//     than carried as OverlayMessages.
package protoconvert_test

import (
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/plugin/protoconvert"
	"github.com/stretchr/testify/require"
)

// compatRunCorpus returns one run of every kind, with every field populated,
// including nested Plural/Select recursion.
func compatRunCorpus() []model.Run {
	return []model.Run{
		{Text: &model.TextRun{Text: "plain text — ünïcode 日本語"}},
		{Ph: &model.PlaceholderRun{
			ID: "ph1", Type: "jsx:var", SubType: "expr", Data: "{name}",
			Equiv: "name", Disp: "Name",
			Constraints: &model.RunConstraints{Deletable: true, Cloneable: true, Reorderable: true},
		}},
		{PcOpen: &model.PcOpenRun{
			ID: "pc1", Type: "html:element", SubType: "inline", Data: `<span class="muted">`,
			Equiv: "span", Disp: "Span",
			Constraints: &model.RunConstraints{Deletable: false, Cloneable: true, Reorderable: false},
		}},
		{Text: &model.TextRun{Text: "wrapped"}},
		{PcClose: &model.PcCloseRun{
			ID: "pc1", Type: "html:element", SubType: "inline", Data: "</span>", Equiv: "span",
		}},
		{Sub: &model.SubRun{ID: "sub1", Ref: "block-42", Equiv: "sub"}},
		// Canonical inline attributes (model Run.Attrs): a hyperlink pair with
		// href/title on the opening half, and a self-closing image placeholder
		// with src/alt/title.
		{PcOpen: &model.PcOpenRun{
			ID: "lnk1", Type: "link:hyperlink", Data: `<a href="https://example.com" title="Docs">`,
			Equiv: "a", Disp: "Link",
			Attrs: map[string]string{model.AttrHref: "https://example.com", model.AttrTitle: "Docs"},
		}},
		{Text: &model.TextRun{Text: "docs"}},
		{PcClose: &model.PcCloseRun{ID: "lnk1", Type: "link:hyperlink", Data: "</a>", Equiv: "a"}},
		{Ph: &model.PlaceholderRun{
			ID: "img1", Type: "link:image", Data: `![Logo](img/logo.png "Kapi")`,
			Equiv: "img", Disp: "Image",
			Attrs: map[string]string{
				model.AttrSrc: "img/logo.png", model.AttrAlt: "Logo", model.AttrTitle: "Kapi",
			},
		}},
		{Plural: &model.PluralRun{
			Pivot: "count",
			Forms: map[model.PluralForm][]model.Run{
				model.PluralZero: {{Text: &model.TextRun{Text: "none"}}},
				model.PluralOne: {
					{Ph: &model.PlaceholderRun{ID: "p1", Type: "var", Data: "{n}", Equiv: "n"}},
					{Text: &model.TextRun{Text: " item"}},
				},
				model.PluralTwo:  {{Text: &model.TextRun{Text: "a pair"}}},
				model.PluralFew:  {{Text: &model.TextRun{Text: "a few"}}},
				model.PluralMany: {{Text: &model.TextRun{Text: "many"}}},
				model.PluralOther: {
					{Ph: &model.PlaceholderRun{ID: "p1", Type: "var", Data: "{n}", Equiv: "n"}},
					{Text: &model.TextRun{Text: " items"}},
				},
			},
		}},
		{Select: &model.SelectRun{
			Pivot: "gender",
			Cases: map[string][]model.Run{
				"male":   {{Text: &model.TextRun{Text: "He"}}},
				"female": {{Text: &model.TextRun{Text: "She"}}},
				"other": {
					{PcOpen: &model.PcOpenRun{ID: "b1", Type: "element", Data: "<b>", Equiv: "b"}},
					{Text: &model.TextRun{Text: "They"}},
					{PcClose: &model.PcCloseRun{ID: "b1", Type: "element", Data: "</b>"}},
				},
			},
		}},
	}
}

// TestCompatRunKinds pins model → proto → model identity for every Run kind.
func TestCompatRunKinds(t *testing.T) {
	runs := compatRunCorpus()
	got := protoconvert.ProtoToRuns(protoconvert.RunsToProto(runs))
	require.Equal(t, runs, got, "canonical Run wire schema must round-trip every kind")
}

// TestCompatRunAttrsNilEmpty pins the Attrs nil/empty normalization: nil
// Attrs stay nil through the wire, and an empty (but non-nil) map — which
// proto3 cannot distinguish from an absent one — decodes back to nil, matching
// the model's `omitempty` JSON semantics where nil and empty are equivalent.
func TestCompatRunAttrsNilEmpty(t *testing.T) {
	nilRuns := []model.Run{
		{Ph: &model.PlaceholderRun{ID: "p1", Type: "var", Data: "{n}", Equiv: "n"}},
		{PcOpen: &model.PcOpenRun{ID: "b1", Type: "element", Data: "<b>", Equiv: "b"}},
		{PcClose: &model.PcCloseRun{ID: "b1", Type: "element", Data: "</b>"}},
	}
	require.Equal(t, nilRuns, protoconvert.ProtoToRuns(protoconvert.RunsToProto(nilRuns)),
		"nil Attrs must round-trip as nil")

	emptyRuns := []model.Run{
		{Ph: &model.PlaceholderRun{ID: "p1", Type: "var", Data: "{n}", Equiv: "n",
			Attrs: map[string]string{}}},
		{PcOpen: &model.PcOpenRun{ID: "b1", Type: "element", Data: "<b>", Equiv: "b",
			Attrs: map[string]string{}}},
	}
	got := protoconvert.ProtoToRuns(protoconvert.RunsToProto(emptyRuns))
	require.Nil(t, got[0].Ph.Attrs, "empty Attrs normalize to nil (absent on the wire)")
	require.Nil(t, got[1].PcOpen.Attrs, "empty Attrs normalize to nil (absent on the wire)")
}

// TestCompatBlockCorpus pins the full Block round-trip: rich source runs,
// multi-locale targets, source and target segmentation, stand-off overlays
// (variant-scoped, layered, with props and typed payloads), skeleton text +
// ref parts, display hints, typed + generic annotations, and properties.
func TestCompatBlockCorpus(t *testing.T) {
	b := model.NewRunsBlock("compat-1", compatRunCorpus())
	b.Name = "greeting.title"
	b.Type = "paragraph"
	b.MimeType = "text/html"
	b.PreserveWhitespace = true
	b.IsReferent = true
	b.Properties["context"] = "landing page"
	b.Properties["resname"] = "title"

	// Multi-locale targets, one segmented.
	b.SetTargetRuns(model.LocaleID("fr-FR"), []model.Run{
		{Text: &model.TextRun{Text: "Bonjour. "}},
		{Text: &model.TextRun{Text: "Monde."}},
	})
	frKey := model.Variant(model.LocaleID("fr-FR"))
	b.SetSegmentation(&frKey, []model.Span{
		{ID: "s1", Range: model.SpanAnchor(model.RunPos{Run: 0}, model.RunPos{Run: 1})},
		{ID: "s2", Range: model.SpanAnchor(model.RunPos{Run: 1}, model.RunPos{Run: 2})},
	})
	b.SetTargetText(model.LocaleID("de-DE"), "Hallo Welt")

	// Source segmentation (reconstructed from segment boundaries on the wire).
	b.SetSegmentation(nil, []model.Span{
		{ID: "s1", Range: model.SpanAnchor(model.RunPos{Run: 0}, model.RunPos{Run: 3})},
		{ID: "s2", Range: model.SpanAnchor(model.RunPos{Run: 3}, model.RunPos{Run: len(b.Source)})},
	})

	// Stand-off overlays: source-side term overlay with props + typed payload,
	// and a variant-scoped, layered check overlay.
	b.Overlays = append(b.Overlays, model.Overlay{
		Type: model.OverlayType("term"),
		Spans: []model.Span{{
			ID:    "t1",
			Range: model.SpanAnchor(model.RunPos{Run: 0}, model.RunPos{Run: 1, Offset: 5}),
			Props: map[string]string{"concept": "c-42", "status": "approved"},
			Value: &model.TermAnnotation{SourceTerm: "plain", ConceptID: "c-42", Score: 1.0},
		}},
	})
	frVariant := model.Variant(model.LocaleID("fr-FR"))
	b.Overlays = append(b.Overlays, model.Overlay{
		Type:    model.OverlayType("qa"),
		Variant: &frVariant,
		Layer:   "review",
		Spans: []model.Span{{
			ID:    "q1",
			Range: model.SpanAnchor(model.RunPos{Run: 0}, model.RunPos{Run: 1}),
			Props: map[string]string{"severity": "minor"},
		}},
	})

	// Skeleton with text and locale-scoped ref parts.
	b.Skeleton = &model.Skeleton{
		Strategy:  model.SkeletonStrategy(1),
		SourceURI: "source://file.html",
		Parts: []model.SkeletonPart{
			&model.SkeletonText{Text: `<title>`},
			&model.SkeletonRef{ResourceID: "compat-1", Property: "target", Locale: "fr-FR"},
			&model.SkeletonText{Text: `</title>`},
		},
	}

	// Display hint.
	b.DisplayHint = &model.DisplayHint{
		MaxLength: 120, ContentType: "heading", Context: "page header", Preview: "Greeting…",
	}

	// Typed annotations. (Unregistered annotation types degrade to
	// GenericAnnotation on decode by design — see ProtoToAnnotation — so the
	// identity corpus uses registered types.)
	b.SetAnno("note", &model.Notes{Items: []*model.NoteAnnotation{
		{Text: "keep the greeting informal", From: "reviewer", Priority: 2},
	}})
	b.SetAnno("term", &model.TermAnnotation{SourceTerm: "greeting", ConceptID: "c-7", Score: 0.9})

	got := protoconvert.ProtoToBlock(protoconvert.BlockToProto(b))
	require.NotNil(t, got)

	require.Equal(t, b.ID, got.ID)
	require.Equal(t, b.Name, got.Name)
	require.Equal(t, b.Type, got.Type)
	require.Equal(t, b.MimeType, got.MimeType)
	require.Equal(t, b.Translatable, got.Translatable)
	require.Equal(t, b.PreserveWhitespace, got.PreserveWhitespace)
	require.Equal(t, b.IsReferent, got.IsReferent)
	require.Equal(t, b.Properties, got.Properties)
	require.Equal(t, b.Source, got.Source, "source runs")
	require.Equal(t, b.SourceSegmentation(), got.SourceSegmentation(), "source segmentation")
	require.ElementsMatch(t, b.TargetLocales(), got.TargetLocales(), "target locales")
	for _, loc := range b.TargetLocales() {
		require.Equal(t, b.TargetRuns(loc), got.TargetRuns(loc), "target runs %s", loc)
	}
	require.Equal(t, b.SegmentationFor(&frKey), got.SegmentationFor(&frKey), "fr target segmentation")
	require.Equal(t, b.Skeleton, got.Skeleton)
	require.Equal(t, b.DisplayHint, got.DisplayHint)
	require.Equal(t, b.AnnoMap(), got.AnnoMap(), "annotations")

	// Non-segmentation overlays round-trip in order.
	var wantOverlays, gotOverlays []model.Overlay
	for _, o := range b.Overlays {
		if o.Type != model.OverlaySegmentation {
			wantOverlays = append(wantOverlays, o)
		}
	}
	for _, o := range got.Overlays {
		if o.Type != model.OverlaySegmentation {
			gotOverlays = append(gotOverlays, o)
		}
	}
	require.Equal(t, wantOverlays, gotOverlays, "stand-off overlays")
}

// TestCompatPartKinds pins the Part round-trip for every part type the wire
// carries: layer start/end, data (with skeleton ref), group start/end, media,
// and block.
func TestCompatPartKinds(t *testing.T) {
	parts := []*model.Part{
		{Type: model.PartLayerStart, Resource: &model.Layer{
			ID: "l1", Name: "doc.html", Format: "html", Locale: model.LocaleID("en-US"),
			Encoding: "utf-8", MimeType: "text/html", LineBreak: "\n",
			IsMultilingual: true, ParentID: "l0",
			Properties: map[string]string{"k": "v"}, HasBOM: true,
		}},
		{Type: model.PartBlock, Resource: model.NewBlock("b1", "Hello")},
		{Type: model.PartData, Resource: &model.Data{
			ID: "d1", Name: "header", Properties: map[string]string{"raw": "<!doctype html>"},
			Skeleton: &model.Skeleton{Parts: []model.SkeletonPart{
				&model.SkeletonText{Text: "<!doctype html>"},
				&model.SkeletonRef{ResourceID: "b1", Property: "source"},
			}},
			IsReferent: true,
		}},
		{Type: model.PartGroupStart, Resource: &model.GroupStart{
			ID: "g1", Name: "table", Type: "table", Properties: map[string]string{"rows": "2"},
		}},
		{Type: model.PartMedia, Resource: &model.Media{
			ID: "m1", MimeType: "image/png", Data: []byte{0x89, 'P', 'N', 'G'},
			URI: "img/logo.png", AltText: "Logo", Properties: map[string]string{"w": "64"},
		}},
		{Type: model.PartGroupEnd, Resource: &model.GroupEnd{ID: "g1"}},
		{Type: model.PartLayerEnd, Resource: &model.Layer{ID: "l1"}},
	}

	got := protoconvert.ProtoToParts(protoconvert.PartsToProto(parts))
	require.Len(t, got, len(parts))
	for i, p := range parts {
		require.Equal(t, p.Type, got[i].Type, "part %d type", i)
		if p.Type == model.PartBlock {
			// Block equality is covered structurally by TestCompatBlockCorpus;
			// here assert the content survives the Part envelope.
			wb := p.Resource.(*model.Block)
			gb := got[i].Resource.(*model.Block)
			require.Equal(t, wb.ID, gb.ID)
			require.Equal(t, wb.Source, gb.Source)
			continue
		}
		require.Equal(t, p.Resource, got[i].Resource, "part %d resource", i)
	}
}
