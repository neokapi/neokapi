package spec

import (
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/model"
)

// handBuiltStream constructs the part stream documented in
// format-spec-cases.md §4.1: a layer, a data part, a block with typed
// inline codes, a block with a target and a segmentation overlay, and the
// closing layer. It exercises every event kind, run kind, target keying,
// and overlay anchoring the dump must encode.
func handBuiltStream() []*model.Part {
	b1 := &model.Block{
		ID:           "b1",
		Translatable: true,
		Properties:   map[string]string{"resname": "intro"},
	}
	b1.SetSourceRuns([]model.Run{
		{Text: &model.TextRun{Text: "Press "}},
		{PcOpen: &model.PcOpenRun{ID: "1", Type: "fmt:bold", Data: "<b>"}},
		{Text: &model.TextRun{Text: "Start"}},
		{PcClose: &model.PcCloseRun{ID: "1", Type: "fmt:bold", Data: "</b>"}},
	})
	b2 := &model.Block{
		ID:           "b2",
		Translatable: true,
		Overlays: []model.Overlay{
			{Type: model.OverlaySegmentation, Spans: []model.Span{
				{ID: "s1", Range: model.SpanAnchor(model.RunPos{Run: 0}, model.RunPos{Run: 0, Offset: 5})},
			}},
		},
	}
	b2.SetSourceText("Hello")
	b2.SetTargetText("fr-FR", "Bonjour")
	return []*model.Part{
		{Type: model.PartLayerStart, Resource: &model.Layer{
			ID: "doc", Format: "html", Locale: "en", MimeType: "text/html",
		}},
		{Type: model.PartData, Resource: &model.Data{ID: "d1"}},
		{Type: model.PartBlock, Resource: b1},
		{Type: model.PartBlock, Resource: b2},
		{Type: model.PartLayerEnd, Resource: &model.Layer{ID: "doc"}},
	}
}

// TestDumpBlockEvents_Shape asserts the documented §4.1 event shape on a
// hand-built stream: one event per part, the §4.1 keys, typed-code runs with
// `semantic`, EditionKey-keyed targets, Anchor-anchored overlays, and no
// HTML escaping of run data.
func TestDumpBlockEvents_Shape(t *testing.T) {
	got, err := DumpBlockEvents(handBuiltStream())
	if err != nil {
		t.Fatalf("DumpBlockEvents: %v", err)
	}
	want := strings.Join([]string{
		`{"layer_start":{"id":"doc","format":"html","locale":"en","mime_type":"text/html"}}`,
		`{"data":{"id":"d1"}}`,
		`{"block":{"id":"b1","translatable":true,"source":[{"type":"text","text":"Press "},{"type":"pcOpen","id":"1","semantic":"fmt:bold","data":"<b>"},{"type":"text","text":"Start"},{"type":"pcClose","id":"1","semantic":"fmt:bold","data":"</b>"}],"properties":{"resname":"intro"}}}`,
		`{"block":{"id":"b2","translatable":true,"source":[{"type":"text","text":"Hello"}],"targets":{"fr-FR":[{"type":"text","text":"Bonjour"}]},"overlays":[{"type":"segmentation","spans":[{"id":"s1","range":[0,0,0,5]}]}]}}`,
		`{"layer_end":{"id":"doc"}}`,
		"",
	}, "\n")
	if string(got) != want {
		t.Errorf("dump mismatch\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

// TestDumpBlockEvents_Deterministic proves the dump is byte-identical across
// repeated runs — the property that makes it usable as a stored oracle.
func TestDumpBlockEvents_Deterministic(t *testing.T) {
	parts := handBuiltStream()
	first, err := DumpBlockEvents(parts)
	if err != nil {
		t.Fatalf("DumpBlockEvents (1): %v", err)
	}
	for i := range 5 {
		again, err := DumpBlockEvents(parts)
		if err != nil {
			t.Fatalf("DumpBlockEvents (%d): %v", i+2, err)
		}
		if string(again) != string(first) {
			t.Fatalf("dump not deterministic on run %d", i+2)
		}
	}
}

// TestDumpBlockEvents_NoHTMLEscape confirms `<`, `>`, `&` survive literally
// (matching model.Run.MarshalJSON and the KBF wire form). JSON-mandated
// escaping of `"` inside a string still applies. The expected line is
// asserted exactly so an accidental json.Marshal (which HTML-escapes) is
// caught.
func TestDumpBlockEvents_NoHTMLEscape(t *testing.T) {
	b := &model.Block{ID: "b", Translatable: true}
	b.SetSourceRuns([]model.Run{
		{PcOpen: &model.PcOpenRun{ID: "1", Type: "link:hyperlink", Data: `<a href="x?a=1&b=2">`}},
	})
	parts := []*model.Part{{Type: model.PartBlock, Resource: b}}
	got, err := DumpBlockEvents(parts)
	if err != nil {
		t.Fatalf("DumpBlockEvents: %v", err)
	}
	want := `{"block":{"id":"b","translatable":true,"source":[{"type":"pcOpen","id":"1","semantic":"link:hyperlink","data":"<a href=\"x?a=1&b=2\">"}]}}` + "\n"
	if string(got) != want {
		t.Errorf("HTML escaping not disabled\n got: %s\nwant: %s", got, want)
	}
	// Belt-and-braces: the HTML entity forms must not appear at all.
	htmlEntities := []string{"&" + "amp;", "&" + "lt;", "&" + "gt;"}
	for _, ent := range htmlEntities {
		if strings.Contains(string(got), ent) {
			t.Errorf("found HTML entity %q in dump: %s", ent, got)
		}
	}
}

// TestDumpBlockEvents_SortedMaps confirms map-valued fields (properties,
// targets) emit with sorted keys regardless of insertion order.
func TestDumpBlockEvents_SortedMaps(t *testing.T) {
	b := model.NewBlock("b", "x")
	b.Properties = map[string]string{"zeta": "1", "alpha": "2", "mid": "3"}
	b.SetTargetText("fr", "y")
	b.SetTargetText("de", "z")
	parts := []*model.Part{{Type: model.PartBlock, Resource: b}}
	got, err := DumpBlockEvents(parts)
	if err != nil {
		t.Fatalf("DumpBlockEvents: %v", err)
	}
	s := string(got)
	if strings.Index(s, "alpha") > strings.Index(s, "mid") || strings.Index(s, "mid") > strings.Index(s, "zeta") {
		t.Errorf("properties not sorted: %s", s)
	}
	if strings.Index(s, `"de"`) > strings.Index(s, `"fr"`) {
		t.Errorf("targets not sorted: %s", s)
	}
}

// TestDumpBlockEvents_ZeroKeyTarget confirms the dump keeps a translation
// filed under the empty locale. The Qt TS reader files one for a file with no
// language attribute read with no source locale, and the TS writer writes it.
func TestDumpBlockEvents_ZeroKeyTarget(t *testing.T) {
	b := model.NewBlock("b", "Hello")
	b.SetTargetText("", "Hallo")
	b.SetTargetText("fr", "Bonjour")
	empty := model.NewBlock("e", "Hello")
	empty.SetTargetRuns("", nil)
	got, err := DumpBlockEvents([]*model.Part{
		{Type: model.PartBlock, Resource: b},
		{Type: model.PartBlock, Resource: empty},
	})
	if err != nil {
		t.Fatalf("DumpBlockEvents: %v", err)
	}
	want := `{"block":{"id":"b","translatable":true,"source":[{"type":"text","text":"Hello"}],"targets":{"":[{"type":"text","text":"Hallo"}],"fr":[{"type":"text","text":"Bonjour"}]}}}` + "\n" +
		`{"block":{"id":"e","translatable":true,"source":[{"type":"text","text":"Hello"}],"targets":{"":null}}}` + "\n"
	if string(got) != want {
		t.Errorf("zero-key target dump\n got: %s\nwant: %s", got, want)
	}
}
