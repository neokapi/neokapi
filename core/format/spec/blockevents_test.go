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
	frFR := model.Variant("fr-FR")
	return []*model.Part{
		{Type: model.PartLayerStart, Resource: &model.Layer{
			ID: "doc", Format: "html", Locale: "en", MimeType: "text/html",
		}},
		{Type: model.PartData, Resource: &model.Data{ID: "d1"}},
		{Type: model.PartBlock, Resource: &model.Block{
			ID:           "b1",
			Translatable: true,
			Source: []model.Run{
				{Text: &model.TextRun{Text: "Press "}},
				{PcOpen: &model.PcOpenRun{ID: "1", Type: "fmt:bold", Data: "<b>"}},
				{Text: &model.TextRun{Text: "Start"}},
				{PcClose: &model.PcCloseRun{ID: "1", Type: "fmt:bold", Data: "</b>"}},
			},
			Properties: map[string]string{"resname": "intro"},
		}},
		{Type: model.PartBlock, Resource: &model.Block{
			ID:           "b2",
			Translatable: true,
			Source:       []model.Run{{Text: &model.TextRun{Text: "Hello"}}},
			Targets: map[model.VariantKey]*model.Target{
				frFR: {Runs: []model.Run{{Text: &model.TextRun{Text: "Bonjour"}}}},
			},
			Overlays: []model.Overlay{
				{Type: model.OverlaySegmentation, Spans: []model.Span{
					{ID: "s1", Range: model.RunRange{StartRun: 0, StartOffset: 0, EndRun: 0, EndOffset: 5}},
				}},
			},
		}},
		{Type: model.PartLayerEnd, Resource: &model.Layer{ID: "doc"}},
	}
}

// TestDumpBlockEvents_Shape asserts the documented §4.1 event shape on a
// hand-built stream: one event per part, the §4.1 keys, typed-code runs with
// `semantic`, VariantKey-keyed targets, RunRange-anchored overlays, and no
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
// (matching model.Run.MarshalJSON and the KLF wire form). JSON-mandated
// escaping of `"` inside a string still applies. The expected line is
// asserted exactly so an accidental json.Marshal (which HTML-escapes) is
// caught.
func TestDumpBlockEvents_NoHTMLEscape(t *testing.T) {
	parts := []*model.Part{
		{Type: model.PartBlock, Resource: &model.Block{
			ID:           "b",
			Translatable: true,
			Source: []model.Run{
				{PcOpen: &model.PcOpenRun{ID: "1", Type: "link:hyperlink", Data: `<a href="x?a=1&b=2">`}},
			},
		}},
	}
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
	parts := []*model.Part{
		{Type: model.PartBlock, Resource: &model.Block{
			ID:           "b",
			Translatable: true,
			Source:       []model.Run{{Text: &model.TextRun{Text: "x"}}},
			Properties:   map[string]string{"zeta": "1", "alpha": "2", "mid": "3"},
			Targets: map[model.VariantKey]*model.Target{
				model.Variant("fr"): {Runs: []model.Run{{Text: &model.TextRun{Text: "y"}}}},
				model.Variant("de"): {Runs: []model.Run{{Text: &model.TextRun{Text: "z"}}}},
			},
		}},
	}
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
