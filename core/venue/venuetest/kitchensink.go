// Package synctest provides the canonical "kitchen-sink" content-model fixture
// shared by every kapi↔bowrain sync wire-path parity test: the proto push
// converter (core/venue), the JSON pull converter (host/venue/client),
// and the full model→wire→store→pull chain (bowrain/store).
//
// One fixture, reused everywhere, is the durable guard: a Block with every
// exported field, every Run kind, and every Overlay kind populated. If any wire
// path drops a field the fixture sets, that path's round-trip test fails; if
// model.Block grows a field, the reflect completeness guard (which walks this
// fixture) fails until the fixture and converters are updated.
//
// See web/docs/contribute/implementation/foundations/content-parity.md.
package venuetest

import (
	"time"

	"github.com/neokapi/neokapi/core/model"
)

// AllRunKinds enumerates every model.Run discriminator the sync wire must carry.
// Adding a new Run kind? Add it here, wire it through the converter (both
// directions on every wire path), and add a run of that kind to KitchenSinkBlock.
var AllRunKinds = []model.RunKind{
	model.RunKindText,
	model.RunKindPh,
	model.RunKindPcOpen,
	model.RunKindPcClose,
	model.RunKindSub,
	model.RunKindPlural,
	model.RunKindSelect,
}

// AllOverlayKinds enumerates every built-in model.Overlay type the sync wire
// must carry. Adding a new overlay kind? Add it here, wire it through the
// converter, and add an overlay of that kind to KitchenSinkOverlays.
var AllOverlayKinds = []model.OverlayType{
	model.OverlaySegmentation,
	model.OverlayTerm,
	model.OverlayEntity,
	model.OverlayCheck,
	model.OverlayAlignment,
	model.OverlayTermCandidate,
}

// BlockDerivedFields names model.Block fields intentionally NOT carried on the
// sync wire because they are derived/recomputed rather than authored content.
// The reflect completeness guard skips them. The content-addressable Identity is
// recomputed from source+context (its ContentHash rides in SyncBlock.content_hash).
var BlockDerivedFields = map[string]string{
	"Identity": "derived: recomputed by model.ComputeIdentity; ContentHash rides in SyncBlock.content_hash",
}

// KitchenSinkBlock returns a Block with EVERY exported field populated and EVERY
// Run kind + Overlay kind present, so a round-trip through it proves the whole
// content model survives a given sync wire path.
func KitchenSinkBlock() *model.Block {
	frVariant := model.VariantKey{Locale: model.LocaleFrench}
	deFormal := model.VariantKey{Locale: model.LocaleGerman, Tone: "formal"}
	esVariant := model.VariantKey{Locale: model.LocaleSpanish}

	b := &model.Block{
		ID:                 "ks-1",
		Name:               "kitchen.sink",
		Unit:               "u-0123456789abcdef",
		Type:               "text",
		MimeType:           "text/plain",
		Translatable:       true,
		SourceLocale:       model.LocaleEnglish,
		SourceStatus:       model.SourceStatusChecked,
		PreserveWhitespace: true,
		IsReferent:         true,
		Properties:         map[string]string{"context": "homepage", "max": "80"},
		Skeleton: &model.Skeleton{
			Strategy:  model.SkeletonStrategy(1),
			SourceURI: "source://kitchen.json",
			Parts: []model.SkeletonPart{
				&model.SkeletonText{Text: `{"k":`},
				&model.SkeletonRef{ResourceID: "ks-1", Property: "value", Locale: "fr"},
				&model.SkeletonText{Text: `}`},
			},
		},
		DisplayHint: &model.DisplayHint{MaxLength: 80, ContentType: "text/plain", Context: "hero", Preview: "Hello…"},
		ContentRef: &model.ContentRef{
			ConnectorID: "conn-1",
			ExternalID:  "ext-9",
			ExternalURL: "https://cms.example/ext-9",
			SyncedAt:    time.Date(2026, 7, 19, 10, 30, 0, 0, time.UTC),
		},
	}

	// Source: one run of every kind (Text, Ph, PcOpen, PcClose, Sub, Plural, Select).
	b.Source = []model.Run{
		{Text: &model.TextRun{Text: "Hello "}},
		{Ph: &model.PlaceholderRun{ID: "v1", Type: "var", Data: "{name}", Equiv: "name", Attrs: map[string]string{"k": "v"}}},
		{PcOpen: &model.PcOpenRun{ID: "b1", Type: "element", Data: "<b>", Equiv: "b"}},
		{Text: &model.TextRun{Text: "world"}},
		{PcClose: &model.PcCloseRun{ID: "b1", Type: "element", Data: "</b>", Equiv: "b"}},
		{Sub: &model.SubRun{ID: "s1", Ref: "ref-1", Equiv: "sub"}},
		{Plural: &model.PluralRun{Pivot: "count", Forms: map[model.PluralForm][]model.Run{
			model.PluralOne:   {{Text: &model.TextRun{Text: "one"}}},
			model.PluralOther: {{Text: &model.TextRun{Text: "many"}}},
		}}},
		{Select: &model.SelectRun{Pivot: "gender", Cases: map[string][]model.Run{
			"female": {{Text: &model.TextRun{Text: "elle"}}},
			"other":  {{Text: &model.TextRun{Text: "iel"}}},
		}}},
	}

	// Targets: two variants; fully-populated provenance on the first.
	b.SetTargetVariant(frVariant, &model.Target{
		Runs:   []model.Run{{Text: &model.TextRun{Text: "Bonjour"}}},
		Status: model.TargetStatusReviewed,
		Score:  0.97,
		Origin: model.Origin{
			Kind:               model.OriginAI,
			Engine:             "sonnet",
			Tool:               "translate",
			Reference:          "batch-3",
			Timestamp:          "2026-07-19T10:00:00Z",
			Confidence:         0.5,
			Profile:            "end-user-help",
			ProfileVersion:     "7",
			ContextFingerprint: "9f2b7c1d4e6a8035",
		},
	})
	b.SetTargetVariant(deFormal, &model.Target{
		Runs:   []model.Run{{Text: &model.TextRun{Text: "Guten Tag"}}},
		Status: model.TargetStatusTranslated,
		Origin: model.Origin{Kind: model.OriginHuman},
	})
	// A memory-origin target: recycled from content memory, carrying the governing
	// context it was filled under (Profile + ContextFingerprint, the same value an
	// AI target under one context carries). Recycled targets are the bulk of a
	// converged corpus, so both wire paths must carry their governance too — the
	// parity round-trip forces it.
	b.SetTargetVariant(esVariant, &model.Target{
		Runs:   []model.Run{{Text: &model.TextRun{Text: "Hola"}}},
		Status: model.TargetStatusDraft,
		Score:  0.88,
		Origin: model.Origin{
			Kind:               model.OriginMemory,
			Tool:               "recycle",
			Profile:            "end-user-help",
			ProfileVersion:     "7",
			ContextFingerprint: "9f2b7c1d4e6a8035",
		},
	})

	// Annotations: block-scoped note (rides on the annotations map, not overlays).
	b.AddNote(&model.NoteAnnotation{Text: "hero copy", From: "dev", Priority: 1, Annotates: "source"})

	// Overlays: every kind, run-anchored.
	b.Overlays = KitchenSinkOverlays()

	return b
}

// KitchenSinkOverlays returns one overlay of every kind (segmentation with an
// ignorable span, entity + term + term-candidate carrying typed Values, a check
// finding with props, and a target-side alignment) — the full OverlayType
// vocabulary the sync wire must carry.
func KitchenSinkOverlays() []model.Overlay {
	frVariant := model.VariantKey{Locale: model.LocaleFrench}
	return []model.Overlay{
		{
			Type:  model.OverlaySegmentation,
			Layer: model.LayerPrimary,
			Spans: []model.Span{
				{ID: "s1", Range: model.SpanAnchor(model.RunPos{Run: 0}, model.RunPos{Run: 4})},
				{
					ID:    "s2",
					Range: model.SpanAnchor(model.RunPos{Run: 4}, model.RunPos{Run: 5}),
					Props: map[string]string{model.SpanPropIgnorable: "true"},
				},
			},
		},
		{
			Type: model.OverlayEntity,
			Spans: []model.Span{{
				ID:    "entity:0",
				Range: model.SpanAnchor(model.RunPos{Run: 0}, model.RunPos{Run: 0, Offset: 5}),
				Value: &model.EntityAnnotation{Text: "Acme", Type: model.EntityOrganization, Locale: model.LocaleEnglish, DNT: true, Source: "ner"},
			}},
		},
		{
			Type: model.OverlayTerm,
			Spans: []model.Span{{
				ID:    "term:0",
				Range: model.SpanAnchor(model.RunPos{Run: 3}, model.RunPos{Run: 4}),
				Props: map[string]string{"strength": "preferred"},
				Value: &model.TermAnnotation{
					SourceTerm:  "world",
					ConceptID:   "c-42",
					Status:      model.TermPreferred,
					Score:       1,
					MatchType:   model.MatchStrategyExact,
					TargetTerms: []model.TermRef{{Text: "monde", Locale: model.LocaleFrench, Status: model.TermPreferred}},
				},
			}},
		},
		{
			Type: model.OverlayTermCandidate,
			Spans: []model.Span{{
				ID:    "term-candidate:0",
				Range: model.SpanAnchor(model.RunPos{Run: 3}, model.RunPos{Run: 4}),
				Value: &model.TermCandidateAnnotation{Text: "world", Locale: model.LocaleEnglish},
			}},
		},
		{
			Type: model.OverlayCheck,
			Spans: []model.Span{{
				ID:    "qa:0",
				Range: model.SpanAnchor(model.RunPos{Run: 0}, model.RunPos{Run: 4}),
				Props: map[string]string{"rule": "length", "severity": "warn"},
			}},
		},
		{
			Type:    model.OverlayAlignment,
			Variant: &frVariant,
			Spans: []model.Span{{
				ID:    "a0",
				Range: model.SpanAnchor(model.RunPos{Run: 0}, model.RunPos{Run: 1}),
				Props: map[string]string{"target": "0:0-0:7"},
			}},
		},
	}
}
