// Canonical-JSON golden suite (AD-034).
//
// The checked-in files under testdata/ ARE the JSON contract for the content
// model: protojson of neokapi.content.v1 with the fixed options in json.go.
// A failure here means the canonical JSON encoding changed — field names,
// casing, value encodings, or structure. Treat it as a wire-compatibility
// break, not a test to update casually. Additive schema changes (new fields
// populated by the fixtures) regenerate legitimately with:
//
//	go test ./core/proto/content/v1/ -run TestCanonicalJSONGolden -update
//
// The reverse leg (golden → message) must always pass unmodified: yesterday's
// canonical JSON must keep decoding to the same messages.
package contentv1_test

import (
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/plugin/protoconvert"
	contentv1 "github.com/neokapi/neokapi/core/proto/content/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

var update = flag.Bool("update", false, "rewrite the canonical-JSON golden files")

// runFixture returns one run of every kind with every field populated,
// wrapped in a SegmentMessage so the whole corpus is a single message.
func runFixture() *contentv1.SegmentMessage {
	runs := []model.Run{
		{Text: &model.TextRun{Text: "plain text — ünïcode 日本語 <b> & 'quotes'"}},
		{Ph: &model.PlaceholderRun{
			ID: "ph1", Type: "jsx:var", SubType: "expr", Data: "{name}",
			Equiv: "name", Disp: "Name",
			Constraints: &model.RunConstraints{Deletable: true, Cloneable: true, Reorderable: true},
		}},
		{PcOpen: &model.PcOpenRun{
			ID: "pc1", Type: "html:element", SubType: "inline", Data: `<span class="muted">`,
			Equiv: "span", Disp: "Span",
			Constraints: &model.RunConstraints{Cloneable: true},
		}},
		{Text: &model.TextRun{Text: "wrapped"}},
		{PcClose: &model.PcCloseRun{
			ID: "pc1", Type: "html:element", SubType: "inline", Data: "</span>", Equiv: "span",
		}},
		{Sub: &model.SubRun{ID: "sub1", Ref: "block-42", Equiv: "sub"}},
		// Attrs-bearing runs: a hyperlink pair (href/title on the opening
		// half) and a self-closing image placeholder (src/alt). Added when the
		// attrs field was appended to the schema — a compatible, additive
		// golden change (the fixtures gained the new field).
		{PcOpen: &model.PcOpenRun{
			ID: "lnk1", Type: "link:hyperlink", Data: `<a href="https://example.com" title="Docs">`,
			Equiv: "a", Disp: "Link",
			Attrs: map[string]string{model.AttrHref: "https://example.com", model.AttrTitle: "Docs"},
		}},
		{Text: &model.TextRun{Text: "docs"}},
		{PcClose: &model.PcCloseRun{ID: "lnk1", Type: "link:hyperlink", Data: "</a>", Equiv: "a"}},
		{Ph: &model.PlaceholderRun{
			ID: "img1", Type: "link:image", Data: `![Logo](img/logo.png)`, Equiv: "img", Disp: "Image",
			Attrs: map[string]string{model.AttrSrc: "img/logo.png", model.AttrAlt: "Logo"},
		}},
		{Plural: &model.PluralRun{
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
		{Select: &model.SelectRun{
			Pivot: "gender",
			Cases: map[string][]model.Run{
				"female": {{Text: &model.TextRun{Text: "She"}}},
				"other":  {{Text: &model.TextRun{Text: "They"}}},
			},
		}},
	}
	return &contentv1.SegmentMessage{
		Id:         "all-run-kinds",
		Runs:       protoconvert.RunsToProto(runs),
		Properties: map[string]string{"kapi:target-status": "reviewed"},
	}
}

// blockFixture returns a fully-populated BlockMessage: rich source runs,
// multi-locale targets, segmentation, stand-off overlays, skeleton, display
// hint, and typed annotations.
func blockFixture(t *testing.T) *contentv1.BlockMessage {
	t.Helper()
	b := model.NewRunsBlock("golden-1", []model.Run{
		{Text: &model.TextRun{Text: "Hello "}},
		{Ph: &model.PlaceholderRun{ID: "ph1", Type: "jsx:var", Data: "{name}", Equiv: "name"}},
		{Text: &model.TextRun{Text: ". Welcome."}},
	})
	b.Name = "greeting.title"
	b.Type = "paragraph"
	b.MimeType = "text/html"
	b.PreserveWhitespace = true
	b.IsReferent = true
	b.Properties["context"] = "landing page"

	b.SetTargetRuns(model.LocaleID("fr-FR"), []model.Run{
		{Text: &model.TextRun{Text: "Bonjour. "}},
		{Text: &model.TextRun{Text: "Bienvenue."}},
	})
	frKey := model.Variant(model.LocaleID("fr-FR"))
	b.SetSegmentation(&frKey, []model.Span{
		{ID: "s1", Range: model.SpanAnchor(model.RunPos{Run: 0}, model.RunPos{Run: 1})},
		{ID: "s2", Range: model.SpanAnchor(model.RunPos{Run: 1}, model.RunPos{Run: 2})},
	})
	b.SetTargetText(model.LocaleID("de-DE"), "Hallo. Willkommen.")

	b.SetSegmentation(nil, []model.Span{
		{ID: "s1", Range: model.SpanAnchor(model.RunPos{Run: 0}, model.RunPos{Run: 3})},
	})

	b.Overlays = append(b.Overlays, model.Overlay{
		Type: model.OverlayTerm,
		Spans: []model.Span{{
			ID:    "t1",
			Range: model.SpanAnchor(model.RunPos{Run: 0}, model.RunPos{Run: 1, Offset: 5}),
			Props: map[string]string{"concept": "c-42"},
			Value: &model.TermAnnotation{SourceTerm: "Hello", ConceptID: "c-42", Score: 1.0},
		}},
	})

	b.Skeleton = &model.Skeleton{
		Strategy:  model.SkeletonStrategy(0),
		SourceURI: "source://file.html",
		Parts: []model.SkeletonPart{
			&model.SkeletonText{Text: "<title>"},
			&model.SkeletonRef{ResourceID: "golden-1", Property: "target", Locale: "fr-FR"},
			&model.SkeletonText{Text: "</title>"},
		},
	}
	b.DisplayHint = &model.DisplayHint{
		MaxLength: 120, ContentType: "heading", Context: "page header", Preview: "Greeting…",
	}
	b.SetAnno("note", &model.Notes{Items: []*model.NoteAnnotation{
		{Text: "keep the greeting informal", From: "reviewer", Priority: 2},
	}})
	b.SetAnno("alt-translation", &model.AltTranslations{Items: []*model.AltTranslation{{
		Target:    []model.Run{{Text: &model.TextRun{Text: "Salut. Bienvenue."}}},
		Locale:    model.LocaleID("fr-FR"),
		Origin:    "tm",
		Score:     0.92,
		MatchType: model.MatchFuzzy,
		ToolID:    "tm-leverage",
	}}})

	msg := protoconvert.BlockToProto(b)
	// model.Block targets are a Go map, so TargetEntry order is unspecified;
	// sort by locale for a stable golden. (Entry order is not part of the
	// contract — the locale keys are.)
	sort.Slice(msg.Targets, func(i, j int) bool { return msg.Targets[i].Locale < msg.Targets[j].Locale })
	return msg
}

// partFixtures returns one PartMessage per part kind the wire carries.
func partFixtures() []*contentv1.PartMessage {
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
	return protoconvert.PartsToProto(parts)
}

// goldenFixtures enumerates the golden corpus: file name → message.
func goldenFixtures(t *testing.T) map[string]proto.Message {
	t.Helper()
	fixtures := map[string]proto.Message{
		"runs.json":  runFixture(),
		"block.json": blockFixture(t),
	}
	for i, p := range partFixtures() {
		fixtures[partGoldenName(i, p)] = p
	}
	return fixtures
}

func partGoldenName(i int, p *contentv1.PartMessage) string {
	// model.PartType values (core/model/part.go).
	kinds := map[int32]string{
		1: "layer-start", 2: "layer-end", 3: "group-start", 4: "group-end",
		5: "block", 6: "data", 7: "media",
	}
	kind, ok := kinds[p.PartType]
	if !ok {
		kind = "unknown"
	}
	return filepath.Join("parts", "part-"+string(rune('0'+i))+"-"+kind+".json")
}

// TestCanonicalJSONGolden locks the canonical JSON encoding: marshal must
// reproduce the checked-in bytes, and the checked-in bytes must decode back to
// the same messages (backward compatibility of previously-emitted JSON).
func TestCanonicalJSONGolden(t *testing.T) {
	for name, msg := range goldenFixtures(t) {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join("testdata", name)
			got, err := contentv1.MarshalJSONIndent(msg)
			require.NoError(t, err)
			got = append(got, '\n')

			if *update {
				require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
				require.NoError(t, os.WriteFile(path, got, 0o644))
			}

			want, err := os.ReadFile(path)
			require.NoError(t, err, "missing golden — run with -update to create it")
			require.Equal(t, string(want), string(got),
				"canonical JSON changed; this is a wire-compatibility break (see file header)")

			// Backward-compat leg: the golden JSON must decode to the same message.
			back := msg.ProtoReflect().New().Interface()
			require.NoError(t, contentv1.UnmarshalJSON(want, back))
			require.True(t, proto.Equal(msg, back),
				"golden JSON no longer decodes to the original message")
		})
	}
}

// TestMarshalJSONDeterministic guards against protojson's deliberate output
// instability leaking through the compaction/indent normalization.
func TestMarshalJSONDeterministic(t *testing.T) {
	msg := blockFixture(t)
	first, err := contentv1.MarshalJSON(msg)
	require.NoError(t, err)
	for range 50 {
		again, err := contentv1.MarshalJSON(msg)
		require.NoError(t, err)
		require.Equal(t, string(first), string(again))
	}
}

// TestUnmarshalAcceptsProtoNames pins that original proto field names decode
// too (protojson always accepts both spellings; canonical output is camel).
func TestUnmarshalAcceptsProtoNames(t *testing.T) {
	protoNames := `{"pc_open": {"id": "pc1", "type": "html:element", "sub_type": "inline", "data": "<b>", "equiv": "b"}}`
	camel := `{"pcOpen": {"id": "pc1", "type": "html:element", "subType": "inline", "data": "<b>", "equiv": "b"}}`

	var a, b contentv1.RunMessage
	require.NoError(t, contentv1.UnmarshalJSON([]byte(protoNames), &a))
	require.NoError(t, contentv1.UnmarshalJSON([]byte(camel), &b))
	assert.True(t, proto.Equal(&a, &b))

	// Canonical output is lowerCamel regardless of input spelling.
	out, err := contentv1.MarshalJSON(&a)
	require.NoError(t, err)
	var keys map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(out, &keys))
	assert.Contains(t, keys, "pcOpen")
	assert.NotContains(t, keys, "pc_open")
}

// TestUnmarshalIgnoresUnknownFields pins forward compatibility: a newer
// producer's appended field must not fail an older consumer.
func TestUnmarshalIgnoresUnknownFields(t *testing.T) {
	var m contentv1.BlockMessage
	err := contentv1.UnmarshalJSON([]byte(`{"id": "b1", "someFutureField": {"x": 1}}`), &m)
	require.NoError(t, err)
	assert.Equal(t, "b1", m.Id)
}
