package protoconvert_test

import (
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/plugin/protoconvert"
	"github.com/stretchr/testify/require"
)

// ─────────────────────────────────────────────────────────────────────────────
// Helpers
// ─────────────────────────────────────────────────────────────────────────────

func runRoundTrip(t *testing.T, r model.Run) model.Run {
	t.Helper()
	proto := protoconvert.RunToProto(r)
	require.NotNil(t, proto, "RunToProto must not return nil for a valid run")
	got := protoconvert.ProtoToRun(proto)
	return got
}

// ─────────────────────────────────────────────────────────────────────────────
// Task 1a: Round-trip tests for every Run variant
// ─────────────────────────────────────────────────────────────────────────────

func TestRunRoundTrip(t *testing.T) {
	tests := []struct {
		name string
		run  model.Run
	}{
		{
			name: "Text",
			run:  model.Run{Text: &model.TextRun{Text: "hello world"}},
		},
		{
			name: "TextEmpty",
			run:  model.Run{Text: &model.TextRun{Text: ""}},
		},
		{
			name: "Ph",
			run: model.Run{Ph: &model.PlaceholderRun{
				ID:      "ph1",
				Type:    "variable",
				SubType: "bold",
				Data:    "{name}",
				Equiv:   "name",
				Disp:    "Name",
			}},
		},
		{
			name: "PhWithConstraints",
			run: model.Run{Ph: &model.PlaceholderRun{
				ID:    "ph2",
				Type:  "element",
				Data:  "<br/>",
				Equiv: "br",
				Constraints: &model.RunConstraints{
					Deletable:   true,
					Cloneable:   false,
					Reorderable: true,
				},
			}},
		},
		{
			name: "PcOpen",
			run: model.Run{PcOpen: &model.PcOpenRun{
				ID:      "pc1",
				Type:    "element",
				SubType: "inline",
				Data:    "<b>",
				Equiv:   "b",
				Disp:    "Bold",
			}},
		},
		{
			name: "PcOpenWithConstraints",
			run: model.Run{PcOpen: &model.PcOpenRun{
				ID:    "pc2",
				Type:  "element",
				Data:  "<span>",
				Equiv: "span",
				Constraints: &model.RunConstraints{
					Deletable:   false,
					Cloneable:   true,
					Reorderable: false,
				},
			}},
		},
		{
			name: "PcClose",
			run: model.Run{PcClose: &model.PcCloseRun{
				ID:      "pc1",
				Type:    "element",
				SubType: "inline",
				Data:    "</b>",
				Equiv:   "b",
			}},
		},
		{
			name: "PcCloseMinimal",
			run: model.Run{PcClose: &model.PcCloseRun{
				ID:   "pc2",
				Type: "element",
				Data: "</span>",
			}},
		},
		{
			name: "Sub",
			run: model.Run{Sub: &model.SubRun{
				ID:    "sub1",
				Ref:   "block-ref-42",
				Equiv: "sub",
			}},
		},
		{
			name: "PluralTwoForms",
			run: model.Run{Plural: &model.PluralRun{
				Pivot: "count",
				Forms: map[model.PluralForm][]model.Run{
					model.PluralOne:   {{Text: &model.TextRun{Text: "one item"}}},
					model.PluralOther: {{Text: &model.TextRun{Text: "{count} items"}}},
				},
			}},
		},
		{
			name: "PluralAllForms",
			run: model.Run{Plural: &model.PluralRun{
				Pivot: "n",
				Forms: map[model.PluralForm][]model.Run{
					model.PluralZero:  {{Text: &model.TextRun{Text: "zero"}}},
					model.PluralOne:   {{Text: &model.TextRun{Text: "one"}}},
					model.PluralTwo:   {{Text: &model.TextRun{Text: "two"}}},
					model.PluralFew:   {{Text: &model.TextRun{Text: "few"}}},
					model.PluralMany:  {{Text: &model.TextRun{Text: "many"}}},
					model.PluralOther: {{Text: &model.TextRun{Text: "other"}}},
				},
			}},
		},
		{
			name: "PluralNestedRuns",
			run: model.Run{Plural: &model.PluralRun{
				Pivot: "count",
				Forms: map[model.PluralForm][]model.Run{
					model.PluralOne: {
						{Ph: &model.PlaceholderRun{ID: "p1", Type: "var", Data: "{n}", Equiv: "n"}},
						{Text: &model.TextRun{Text: " item"}},
					},
					model.PluralOther: {
						{Ph: &model.PlaceholderRun{ID: "p1", Type: "var", Data: "{n}", Equiv: "n"}},
						{Text: &model.TextRun{Text: " items"}},
					},
				},
			}},
		},
		{
			name: "SelectTwoCases",
			run: model.Run{Select: &model.SelectRun{
				Pivot: "gender",
				Cases: map[string][]model.Run{
					"male":   {{Text: &model.TextRun{Text: "He went"}}},
					"female": {{Text: &model.TextRun{Text: "She went"}}},
					"other":  {{Text: &model.TextRun{Text: "They went"}}},
				},
			}},
		},
		{
			name: "SelectNestedRuns",
			run: model.Run{Select: &model.SelectRun{
				Pivot: "type",
				Cases: map[string][]model.Run{
					"button": {
						{PcOpen: &model.PcOpenRun{ID: "b1", Type: "element", Data: "<button>", Equiv: "button"}},
						{Text: &model.TextRun{Text: "Click me"}},
						{PcClose: &model.PcCloseRun{ID: "b1", Type: "element", Data: "</button>"}},
					},
					"link": {
						{Text: &model.TextRun{Text: "Click here"}},
					},
				},
			}},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := runRoundTrip(t, tc.run)
			require.Equal(t, tc.run, got,
				"ProtoToRun(RunToProto(r)) must deep-equal original run")
		})
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Task 1b: RunsToProto / ProtoToRuns slice helpers
// ─────────────────────────────────────────────────────────────────────────────

func TestRunsSliceRoundTrip(t *testing.T) {
	runs := []model.Run{
		{Text: &model.TextRun{Text: "Hello, "}},
		{Ph: &model.PlaceholderRun{ID: "ph1", Type: "variable", Data: "{name}", Equiv: "name"}},
		{Text: &model.TextRun{Text: "!"}},
	}

	protos := protoconvert.RunsToProto(runs)
	require.Len(t, protos, len(runs))

	got := protoconvert.ProtoToRuns(protos)
	require.Equal(t, runs, got, "ProtoToRuns(RunsToProto(runs)) must equal original slice")
}

func TestRunsSliceEmpty(t *testing.T) {
	require.Nil(t, protoconvert.RunsToProto(nil))
	require.Nil(t, protoconvert.ProtoToRuns(nil))
}

// ─────────────────────────────────────────────────────────────────────────────
// Task 1c: Block round-trip — multiple targets, overlays, skeleton
// ─────────────────────────────────────────────────────────────────────────────

func TestBlockRoundTrip(t *testing.T) {
	tests := []struct {
		name string
		b    *model.Block
	}{
		{
			name: "SimpleBlock",
			b:    model.NewBlock("id1", "Hello world"),
		},
		{
			name: "BlockWithMultipleTargets",
			b: func() *model.Block {
				b := model.NewBlock("id2", "Source text")
				b.SetTargetText(model.LocaleFrench, "Texte source")
				b.SetTargetText(model.LocaleGerman, "Quelltext")
				b.SetTargetText(model.LocaleSpanish, "Texto fuente")
				return b
			}(),
		},
		{
			name: "BlockWithRichSource",
			b: func() *model.Block {
				b := model.NewRunsBlock("id3", []model.Run{
					{PcOpen: &model.PcOpenRun{ID: "b1", Type: "element", Data: "<b>", Equiv: "b"}},
					{Text: &model.TextRun{Text: "Bold text"}},
					{PcClose: &model.PcCloseRun{ID: "b1", Type: "element", Data: "</b>"}},
				})
				b.SetTargetRuns(model.LocaleFrench, []model.Run{
					{PcOpen: &model.PcOpenRun{ID: "b1", Type: "element", Data: "<b>", Equiv: "b"}},
					{Text: &model.TextRun{Text: "Texte gras"}},
					{PcClose: &model.PcCloseRun{ID: "b1", Type: "element", Data: "</b>"}},
				})
				return b
			}(),
		},
		{
			name: "BlockWithProperties",
			b: func() *model.Block {
				b := model.NewBlock("id4", "Prop text")
				b.Properties["note"] = "translator note"
				b.Properties["context"] = "UI button"
				b.Name = "button.label"
				b.Type = "text"
				b.MimeType = "text/plain"
				return b
			}(),
		},
		{
			name: "BlockWithSkeleton",
			b: func() *model.Block {
				b := model.NewBlock("id5", "Skeleton text")
				b.Skeleton = &model.Skeleton{
					Strategy:  model.SkeletonStrategy(1),
					SourceURI: "source://file.json",
					Parts: []model.SkeletonPart{
						&model.SkeletonText{Text: `{"key":`},
						&model.SkeletonRef{ResourceID: "id5", Property: "value", Locale: "fr"},
						&model.SkeletonText{Text: `}`},
					},
				}
				return b
			}(),
		},
		{
			name: "BlockWithDisplayHint",
			b: func() *model.Block {
				b := model.NewBlock("id6", "Hint text")
				b.DisplayHint = &model.DisplayHint{
					MaxLength:   80,
					ContentType: "text/plain",
					Context:     "button label",
					Preview:     "preview text",
				}
				return b
			}(),
		},
		{
			name: "BlockWithSegmentation",
			b: func() *model.Block {
				b := model.NewRunsBlock("id7", []model.Run{
					{Text: &model.TextRun{Text: "First sentence. "}},
					{Text: &model.TextRun{Text: "Second sentence."}},
				})
				spans := []model.Span{
					{ID: "s1", Range: model.RunRange{StartRun: 0, EndRun: 1}},
					{ID: "s2", Range: model.RunRange{StartRun: 1, EndRun: 2}},
				}
				b.SetSegmentation(nil, spans)
				return b
			}(),
		},
		{
			name: "BlockNonTranslatable",
			b: func() *model.Block {
				return &model.Block{
					ID:           "id8",
					Translatable: false,
					Source:       []model.Run{{Text: &model.TextRun{Text: "non-translatable"}}},
					Targets:      make(map[model.VariantKey]*model.Target),
					Properties:   make(map[string]string),
					Annotations:  make(map[string]model.Annotation),
				}
			}(),
		},
		{
			name: "BlockPreserveWhitespace",
			b: func() *model.Block {
				b := model.NewBlock("id9", "  indented text  ")
				b.PreserveWhitespace = true
				b.IsReferent = true
				return b
			}(),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			proto := protoconvert.BlockToProto(tc.b)
			require.NotNil(t, proto)
			got := protoconvert.ProtoToBlock(proto)
			require.NotNil(t, got)

			// Compare scalar fields directly.
			require.Equal(t, tc.b.ID, got.ID, "ID")
			require.Equal(t, tc.b.Name, got.Name, "Name")
			require.Equal(t, tc.b.Type, got.Type, "Type")
			require.Equal(t, tc.b.MimeType, got.MimeType, "MimeType")
			require.Equal(t, tc.b.Translatable, got.Translatable, "Translatable")
			require.Equal(t, tc.b.PreserveWhitespace, got.PreserveWhitespace, "PreserveWhitespace")
			require.Equal(t, tc.b.IsReferent, got.IsReferent, "IsReferent")
			require.Equal(t, tc.b.Properties, got.Properties, "Properties")

			// Source runs round-trip.
			require.Equal(t, tc.b.Source, got.Source, "Source runs")

			// Targets round-trip: same set of locales and run content.
			require.Len(t, got.TargetLocales(), len(tc.b.TargetLocales()),
				"target locale count")
			for _, loc := range tc.b.TargetLocales() {
				require.Equal(t, tc.b.TargetRuns(loc), got.TargetRuns(loc),
					"target runs for locale %s", loc)
			}

			// Skeleton.
			require.Equal(t, tc.b.Skeleton, got.Skeleton, "Skeleton")

			// DisplayHint.
			require.Equal(t, tc.b.DisplayHint, got.DisplayHint, "DisplayHint")
		})
	}
}

func TestBlockNilRoundTrip(t *testing.T) {
	require.Nil(t, protoconvert.BlockToProto(nil))
	require.Nil(t, protoconvert.ProtoToBlock(nil))
}

// ─────────────────────────────────────────────────────────────────────────────
// Task 1d: Skeleton round-trip
// ─────────────────────────────────────────────────────────────────────────────

func TestSkeletonRoundTrip(t *testing.T) {
	tests := []struct {
		name string
		s    *model.Skeleton
	}{
		{
			name: "Nil",
			s:    nil,
		},
		{
			name: "TextPartsOnly",
			s: &model.Skeleton{
				Strategy:  model.SkeletonStrategy(0),
				SourceURI: "uri://file.json",
				Parts: []model.SkeletonPart{
					&model.SkeletonText{Text: `{"key":`},
					&model.SkeletonText{Text: `}`},
				},
			},
		},
		{
			name: "RefPartsOnly",
			s: &model.Skeleton{
				Strategy:  model.SkeletonStrategy(2),
				SourceURI: "uri://file.po",
				Parts: []model.SkeletonPart{
					&model.SkeletonRef{ResourceID: "id1", Property: "msgstr", Locale: "fr"},
					&model.SkeletonRef{ResourceID: "id2", Property: "msgstr", Locale: "de"},
				},
			},
		},
		{
			name: "MixedParts",
			s: &model.Skeleton{
				SourceURI: "uri://file.html",
				Parts: []model.SkeletonPart{
					&model.SkeletonText{Text: "<html><body>"},
					&model.SkeletonRef{ResourceID: "id1", Property: "content", Locale: "fr"},
					&model.SkeletonText{Text: "</body></html>"},
				},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			proto := protoconvert.SkeletonToProto(tc.s)
			got := protoconvert.ProtoToSkeleton(proto)
			require.Equal(t, tc.s, got, "Skeleton round-trip")
		})
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Task 1e: Layer, Data, GroupStart, GroupEnd, Media round-trips
// ─────────────────────────────────────────────────────────────────────────────

func TestLayerRoundTrip(t *testing.T) {
	l := &model.Layer{
		ID:             "layer1",
		Name:           "Document",
		Format:         "html",
		Locale:         model.LocaleEnglish,
		Encoding:       "UTF-8",
		MimeType:       "text/html",
		LineBreak:      "\n",
		IsMultilingual: true,
		ParentID:       "parent-layer",
		Properties:     map[string]string{"key": "val"},
		HasBOM:         true,
	}
	got := protoconvert.ProtoToLayer(protoconvert.LayerToProto(l))
	require.Equal(t, l, got)
}

func TestDataRoundTrip(t *testing.T) {
	d := &model.Data{
		ID:         "data1",
		Name:       "meta",
		Properties: map[string]string{"mime": "text/plain"},
		IsReferent: true,
	}
	got := protoconvert.ProtoToData(protoconvert.DataToProto(d))
	require.Equal(t, d, got)
}

func TestGroupStartRoundTrip(t *testing.T) {
	g := &model.GroupStart{
		ID:         "grp1",
		Name:       "section",
		Type:       "group",
		Properties: map[string]string{"order": "1"},
	}
	got := protoconvert.ProtoToGroupStart(protoconvert.GroupStartToProto(g))
	require.Equal(t, g, got)
}

func TestGroupEndRoundTrip(t *testing.T) {
	g := &model.GroupEnd{ID: "grp1"}
	got := protoconvert.ProtoToGroupEnd(protoconvert.GroupEndToProto(g))
	require.Equal(t, g, got)
}

func TestMediaRoundTrip(t *testing.T) {
	m := &model.Media{
		ID:         "media1",
		MimeType:   "image/png",
		Data:       []byte{0x89, 0x50, 0x4E, 0x47},
		URI:        "https://example.com/image.png",
		AltText:    "A test image",
		Properties: map[string]string{"width": "100"},
	}
	got := protoconvert.ProtoToMedia(protoconvert.MediaToProto(m))
	require.Equal(t, m, got)
}

// ─────────────────────────────────────────────────────────────────────────────
// Task 1f: Part round-trip (each part type)
// ─────────────────────────────────────────────────────────────────────────────

func TestPartRoundTrip(t *testing.T) {
	tests := []struct {
		name string
		p    *model.Part
	}{
		{
			name: "Block",
			p: &model.Part{
				Type:     model.PartBlock,
				Resource: model.NewBlock("block1", "test text"),
			},
		},
		{
			name: "LayerStart",
			p: &model.Part{
				Type:     model.PartLayerStart,
				Resource: &model.Layer{ID: "l1", Format: "html"},
			},
		},
		{
			name: "LayerEnd",
			p: &model.Part{
				Type:     model.PartLayerEnd,
				Resource: &model.Layer{ID: "l1", Format: "html"},
			},
		},
		{
			name: "Data",
			p: &model.Part{
				Type:     model.PartData,
				Resource: &model.Data{ID: "d1"},
			},
		},
		{
			name: "GroupStart",
			p: &model.Part{
				Type:     model.PartGroupStart,
				Resource: &model.GroupStart{ID: "g1", Name: "section"},
			},
		},
		{
			name: "GroupEnd",
			p: &model.Part{
				Type:     model.PartGroupEnd,
				Resource: &model.GroupEnd{ID: "g1"},
			},
		},
		{
			name: "Media",
			p: &model.Part{
				Type:     model.PartMedia,
				Resource: &model.Media{ID: "m1", MimeType: "image/png"},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			proto := protoconvert.PartToProto(tc.p)
			require.NotNil(t, proto)
			got := protoconvert.ProtoToPart(proto)
			require.NotNil(t, got)
			require.Equal(t, tc.p.Type, got.Type, "PartType")
		})
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Task 1g: DisplayHint round-trip
// ─────────────────────────────────────────────────────────────────────────────

func TestDisplayHintRoundTrip(t *testing.T) {
	h := &model.DisplayHint{
		MaxLength:   120,
		ContentType: "text/html",
		Context:     "header",
		Preview:     "A sample header text",
	}
	got := protoconvert.ProtoToDisplayHint(protoconvert.DisplayHintToProto(h))
	require.Equal(t, h, got)

	// nil round-trip
	require.Nil(t, protoconvert.DisplayHintToProto(nil))
	require.Nil(t, protoconvert.ProtoToDisplayHint(nil))
}

// ─────────────────────────────────────────────────────────────────────────────
// Fuzz functions
// ─────────────────────────────────────────────────────────────────────────────

// FuzzRunRoundTrip fuzz-tests the TextRun round-trip (the most common hot path).
// The fuzzer supplies arbitrary text bytes and verifies text is preserved.
func FuzzRunRoundTrip(f *testing.F) {
	f.Add("hello world")
	f.Add("")
	f.Add("text with <html> & entities")
	f.Add("unicode: 日本語テスト")
	f.Add("\x00\x01\x02")

	f.Fuzz(func(t *testing.T, text string) {
		r := model.Run{Text: &model.TextRun{Text: text}}
		proto := protoconvert.RunToProto(r)
		if proto == nil {
			t.Fatal("RunToProto returned nil for TextRun")
		}
		got := protoconvert.ProtoToRun(proto)
		if got.Text == nil {
			t.Fatal("ProtoToRun returned non-Text run for TextRun")
		}
		if got.Text.Text != text {
			t.Errorf("text mismatch: got %q, want %q", got.Text.Text, text)
		}
	})
}

// FuzzBlockRoundTrip fuzz-tests block source text preservation across the
// BlockToProto / ProtoToBlock round-trip.
func FuzzBlockRoundTrip(f *testing.F) {
	f.Add("hello", "fr", "bonjour")
	f.Add("test", "de", "test")
	f.Add("", "fr", "")

	f.Fuzz(func(t *testing.T, srcText, locale, targetText string) {
		b := model.NewBlock("fuzz-id", srcText)
		if locale != "" {
			b.SetTargetText(model.LocaleID(locale), targetText)
		}
		proto := protoconvert.BlockToProto(b)
		if proto == nil {
			t.Fatal("BlockToProto returned nil")
		}
		got := protoconvert.ProtoToBlock(proto)
		if got == nil {
			t.Fatal("ProtoToBlock returned nil")
		}
		if got.SourceText() != srcText {
			t.Errorf("source text mismatch: got %q, want %q", got.SourceText(), srcText)
		}
		if locale != "" {
			if got.TargetText(model.LocaleID(locale)) != targetText {
				t.Errorf("target text mismatch for locale %q: got %q, want %q",
					locale, got.TargetText(model.LocaleID(locale)), targetText)
			}
		}
	})
}
