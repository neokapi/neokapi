package xliff

import (
	"reflect"
	"testing"

	"github.com/neokapi/neokapi/core/model"
)

func TestParseNativeContent_BptEpt(t *testing.T) {
	src := `The quick brown <bpt id="1" ctype="bold">&lt;b></bpt>fox<ept id="1">&lt;/b></ept> jumped.`
	nc := parseNativeContent(src)
	if got, want := len(nc.Inlines), 5; got != want {
		t.Fatalf("inline count: got %d want %d (%+v)", got, want, nc.Inlines)
	}
	if nc.Inlines[1].Bpt == nil {
		t.Fatalf("expected [1] to be Bpt, got %+v", nc.Inlines[1])
	}
	bpt := nc.Inlines[1].Bpt
	if got := AttrLookup(bpt.Attrs, "id"); got != "1" {
		t.Errorf("bpt id: got %q want %q", got, "1")
	}
	if got := AttrLookup(bpt.Attrs, "ctype"); got != "bold" {
		t.Errorf("bpt ctype: got %q want %q", got, "bold")
	}
	if len(bpt.Inner) != 1 || bpt.Inner[0].Text == nil {
		t.Fatalf("bpt inner: %+v", bpt.Inner)
	}
	if got, want := bpt.Inner[0].Text.Content, "<b>"; got != want {
		t.Errorf("bpt inner text: got %q want %q", got, want)
	}
	if nc.Inlines[3].Ept == nil {
		t.Fatalf("expected [3] to be Ept, got %+v", nc.Inlines[3])
	}
	if got, want := nc.Inlines[3].Ept.Inner[0].Text.Content, "</b>"; got != want {
		t.Errorf("ept inner text: got %q want %q", got, want)
	}
}

func TestRenderNativeWithRuns_Roundtrip(t *testing.T) {
	src := `The quick brown <bpt id="1" ctype="bold">&lt;b></bpt>fox<ept id="1">&lt;/b></ept> jumped.`
	nc := parseNativeContent(src)
	got := renderNativeWithRuns(nc, nil)
	want := `The quick brown <bpt id="1" ctype="bold">&lt;b></bpt>fox<ept id="1">&lt;/b></ept> jumped.`
	if got != want {
		t.Errorf("\ngot  %q\nwant %q", got, want)
	}
}

func TestParseNativeContent_Mrk(t *testing.T) {
	src := `<mrk mid="0" mtype="seg">Segment one.</mrk> <mrk mid="1" mtype="seg">Segment two.</mrk>`
	nc := parseNativeContent(src)
	if got, want := len(nc.Inlines), 3; got != want {
		t.Fatalf("inline count: got %d want %d", got, want)
	}
	if nc.Inlines[0].Mrk == nil || AttrLookup(nc.Inlines[0].Mrk.Attrs, "mid") != "0" || AttrLookup(nc.Inlines[0].Mrk.Attrs, "mtype") != "seg" {
		t.Errorf("[0] mrk: %+v", nc.Inlines[0].Mrk)
	}
	if nc.Inlines[1].Text == nil || nc.Inlines[1].Text.Content != " " {
		t.Errorf("[1] expected space text, got %+v", nc.Inlines[1])
	}
	if nc.Inlines[2].Mrk == nil || AttrLookup(nc.Inlines[2].Mrk.Attrs, "mid") != "1" {
		t.Errorf("[2] mrk: %+v", nc.Inlines[2].Mrk)
	}
}

func TestRenderBodyWithSegments_PreservesBetweenMrkText(t *testing.T) {
	src := `<mrk mid="0" mtype="seg">Segment one.</mrk> <mrk mid="1" mtype="seg">Segment two.</mrk>`
	nc := parseNativeContent(src)
	segs := []*model.Segment{
		model.NewRunsSegment("0", []model.Run{{Text: &model.TextRun{Text: "Segment one."}}}),
		model.NewRunsSegment("1", []model.Run{{Text: &model.TextRun{Text: "Segment two."}}}),
	}
	got := renderBodyWithSegments(nc, segs)
	want := `<mrk mid="0" mtype="seg">Segment one.</mrk> <mrk mid="1" mtype="seg">Segment two.</mrk>`
	if got != want {
		t.Errorf("\ngot  %q\nwant %q", got, want)
	}
}

func TestRenderBodyWithSegments_PseudoSubstitution(t *testing.T) {
	// Native body has the original German text; segments carry pseudo'd
	// content. Body walker substitutes from runs while keeping bpt
	// attributes from native.
	src := `<bpt id="1" ctype="bold">&lt;b></bpt>fox<ept id="1">&lt;/b></ept>`
	nc := parseNativeContent(src)
	segs := []*model.Segment{
		model.NewRunsSegment("s1", []model.Run{
			{PcOpen: &model.PcOpenRun{ID: "1", Type: "fmt:bold", Data: "<b>"}},
			{Text: &model.TextRun{Text: "ƒõẋ"}}, // pseudo-translated
			{PcClose: &model.PcCloseRun{ID: "1", Type: "fmt:bold", Data: "</b>"}},
		}),
	}
	got := renderBodyWithSegments(nc, segs)
	want := `<bpt id="1" ctype="bold">&lt;b></bpt>ƒõẋ<ept id="1">&lt;/b></ept>`
	if got != want {
		t.Errorf("\ngot  %q\nwant %q", got, want)
	}
}

// TestNativeToRuns_EquivalentToParseInlineContent proves the X1 perf fix
// is byte-neutral: deriving the generic Runs from the already-parsed
// native IR (nativeToRuns) yields exactly the same []model.Run that the
// old direct second-decoder path (parseInlineContent) produced. If this
// drifts, the reader would change its emitted Runs — so we lock it down
// across the full inline vocabulary.
func TestNativeToRuns_EquivalentToParseInlineContent(t *testing.T) {
	cases := []string{
		"",
		"plain text only",
		"leading <g id=\"1\">grouped</g> trailing",
		`The quick brown <bpt id="1" ctype="bold">&lt;b></bpt>fox<ept id="1">&lt;/b></ept> jumped.`,
		`<ph id="1">{0}</ph> and <x id="2" equiv-text="[X]"/> done`,
		`a<bx id="1" ctype="link"/>b<ex id="1"/>c`,
		`<it id="1" pos="open" ctype="bold">&lt;b></it>x<it id="2" pos="close">&lt;/b></it>`,
		`<it id="3" ctype="italic">&lt;i></it> standalone-it`,
		`<mrk mid="0" mtype="seg">Segment one.</mrk> <mrk mid="1" mtype="seg">Segment two.</mrk>`,
		`<mrk mtype="x-comment">noted</mrk> tail`,
		`pre <ph id="1">code<sub>nested<ph id="9">x</ph>still in sub</sub>more</ph> post`,
		`<ph id="1">a<sub>one</sub>b<sub>two</sub>c</ph>`,
		`<bpt id="1">open<sub>subtext</sub></bpt>mid<ept id="1">close</ept>`,
		`nested <g id="1"><g id="2">deep</g></g> end`,
		`text with <unknown attr="z">kept text</unknown> after`,
		`amp &amp; lt &lt; gt &gt; quote &quot; entities`,
		`&#x20AC; numeric ref and literal € euro`,
		`<ph id="1" ctype="x-custom" equiv-text="EQ">DATA</ph>`,
		`<g id="1" ctype="link" equiv-text="L">link text</g>`,
		`mixed <g id="1">a<ph id="2">P</ph>b<bpt id="3">&lt;b></bpt>c<ept id="3">&lt;/b></ept>d</g> tail`,
	}
	for _, src := range cases {
		want := parseInlineContent(src)
		got := nativeToRuns(parseNativeContent(src))
		if !reflect.DeepEqual(want, got) {
			t.Errorf("nativeToRuns mismatch for %q:\nwant %#v\ngot  %#v", src, want, got)
		}
	}
}
