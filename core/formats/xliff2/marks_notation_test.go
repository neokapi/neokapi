package xliff2_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/formats/xliff2"
	"github.com/neokapi/neokapi/core/model"
)

// unitDoc wraps the units of one <file> in an XLIFF 2 document.
func unitDoc(units string) string {
	return `<?xml version="1.0"?>
<xliff xmlns="urn:oasis:names:tc:xliff:document:2.0" version="2.0" srcLang="en" trgLang="fr">
  <file id="f1">` + units + `</file>
</xliff>`
}

// writePaths writes input after edit through each of the writer's three
// paths: the skeleton the streaming reader recorded, the document the reader
// parsed, and a document built from the blocks alone.
func writePaths(t *testing.T, input string, edit func(*model.Block)) map[string]string {
	t.Helper()
	blocks := readBlocks(t, input)
	for _, b := range blocks {
		edit(b)
	}
	return map[string]string{
		"skeleton": skeletonEdit(t, input, edit),
		"document": domWrite(t, input, edit),
		"scratch":  writeBlocksIn(t, model.LocaleFrench, blocks...),
	}
}

// replaceIn replaces find with text in one edition of b, as an edit through
// core/change does.
func replaceIn(t *testing.T, b *model.Block, key model.EditionKey, find, text string) {
	t.Helper()
	res := change.ApplyBlock(b, []change.Op{{
		Kind: change.KindReplaceText, At: change.Ref{Block: b.ID, Edition: key}, IfMatch: change.AnyRevision,
		Body: &change.ReplaceText{Edits: []change.TextEdit{{Find: &find, Text: text}}},
	}}, change.BlockEnv{Actor: change.Actor{Kind: change.ActorTool, Name: "test"}})
	require.Equal(t, change.OpApplied, res[0].Status, "%+v", res[0].Error)
}

// A marker the file wrote as <mrk> is written as <mrk> after an edit to its
// segment, on every write path and on both sides, while its ends still sit in
// one inline list. Each was rewritten as an <sm>/<em> pair, or dropped.
func TestAnEditKeepsTheMarkerNotation(t *testing.T) {
	t.Parallel()
	fr := model.Variant(model.LocaleFrench)
	cases := []struct {
		name       string
		units      string
		key        model.EditionKey
		find, with string
		want       []string
	}{
		{"a term with its ref, in the source", `<unit id="u1"><segment>
    <source>Grind the <mrk id="m1" type="term" ref="#c1">coffee</mrk> now</source>
    <target>Moulez le <mrk id="m1" type="term" ref="#c1">café</mrk> maintenant</target>
  </segment></unit>`, model.EditionKey{}, "Grind", "Brew",
			[]string{`Brew the <mrk id="m1" type="term" ref="#c1">coffee</mrk> now`, `Moulez le <mrk id="m1" type="term" ref="#c1">café</mrk> maintenant`}},
		{"a term in the target", `<unit id="u1"><segment>
    <source>Grind the <mrk id="m1" type="term" ref="#c1">coffee</mrk> now</source>
    <target>Moulez le <mrk id="m1" type="term" ref="#c1">café</mrk> maintenant</target>
  </segment></unit>`, fr, "Moulez", "Broyez",
			[]string{`Broyez le <mrk id="m1" type="term" ref="#c1">café</mrk> maintenant`}},
		{"translate no, in the target", `<unit id="u1"><segment>
    <source>Grind <mrk id="n1" translate="no">Kapi</mrk> coffee</source>
    <target>Moulez <mrk id="n1" translate="no">Kapi</mrk> café</target>
  </segment></unit>`, fr, "Moulez", "Broyez",
			[]string{`Grind <mrk id="n1" translate="no">Kapi</mrk> coffee`, `Broyez <mrk id="n1" translate="no">Kapi</mrk> café`}},
		{"a term around a paired code", `<unit id="u1"><originalData><data id="d1">&lt;b&gt;</data><data id="d2">&lt;/b&gt;</data></originalData><segment>
    <source>Grind the <mrk id="m1" type="term"><pc id="1" dataRefStart="d1" dataRefEnd="d2">coffee</pc></mrk> now</source>
  </segment></unit>`, model.EditionKey{}, "Grind", "Brew",
			[]string{`Brew the <mrk id="m1" type="term"><pc id="1" dataRefStart="d1" dataRefEnd="d2">coffee</pc></mrk> now`}},
		{"a term inside a paired code", `<unit id="u1"><originalData><data id="d1">&lt;b&gt;</data><data id="d2">&lt;/b&gt;</data></originalData><segment>
    <source>Grind the <pc id="1" dataRefStart="d1" dataRefEnd="d2"><mrk id="m1" type="term">coffee</mrk></pc> now</source>
  </segment></unit>`, model.EditionKey{}, "Grind", "Brew",
			[]string{`Brew the <pc id="1" dataRefStart="d1" dataRefEnd="d2"><mrk id="m1" type="term">coffee</mrk></pc> now`}},
		{"a term inside a comment", `<unit id="u1"><segment>
    <source>Grind <mrk id="c1" type="comment" value="note">the <mrk id="m1" type="term">coffee</mrk></mrk> now</source>
  </segment></unit>`, model.EditionKey{}, "Grind", "Brew",
			[]string{`Brew <mrk id="c1" type="comment" value="note">the <mrk id="m1" type="term">coffee</mrk></mrk> now`}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			for path, out := range writePaths(t, unitDoc(tc.units), func(b *model.Block) { replaceIn(t, b, tc.key, tc.find, tc.with) }) {
				for _, want := range tc.want {
					assert.Contains(t, out, want, "%s path", path)
				}
				assert.NotContains(t, out, "<sm", "%s path", path)
			}
		})
	}
}

// A segment nothing changed is written with the markers it was read with on
// every path, the scratch path included, which drew a term as an <sm>/<em>
// pair whatever the file wrote.
func TestAnUntouchedSegmentKeepsItsMarkers(t *testing.T) {
	t.Parallel()
	source := `Grind <mrk id="c1" type="comment" value="note">the <mrk id="m1" type="term" ref="#c1">coffee</mrk></mrk> and <sm id="m2" type="term"/>tea<em startRef="m2"/> now`
	in := unitDoc(`<unit id="u1"><segment>
    <source>` + source + `</source>
  </segment></unit>`)
	for path, out := range writePaths(t, in, func(*model.Block) {}) {
		assert.Contains(t, out, source, "%s path", path)
	}
}

// A marker over text an edit changes inside it, or over text a case
// conversion changes, stays and covers the new text, on every path.
func TestAMarkerFollowsAnEditToItsText(t *testing.T) {
	t.Parallel()
	in := unitDoc(`<unit id="u1"><segment>
    <source>Grind <mrk id="c1" type="comment" value="check">the coffee</mrk> and <mrk id="n1" translate="no">Kapi Pro</mrk> with <mrk id="m1" type="term" ref="#c1">fresh beans</mrk></source>
  </segment></unit>`)
	cases := []struct {
		name string
		edit func(t *testing.T, b *model.Block)
		want string
	}{
		{"an edit inside a comment", func(t *testing.T, b *model.Block) { replaceIn(t, b, model.EditionKey{}, "coffee", "beans") },
			`Grind <mrk id="c1" type="comment" value="check">the beans</mrk> and <mrk id="n1" translate="no">Kapi Pro</mrk> with <mrk id="m1" type="term" ref="#c1">fresh beans</mrk>`},
		{"an edit inside translate no", func(t *testing.T, b *model.Block) { replaceIn(t, b, model.EditionKey{}, "Pro", "Plus") },
			`Grind <mrk id="c1" type="comment" value="check">the coffee</mrk> and <mrk id="n1" translate="no">Kapi Plus</mrk> with <mrk id="m1" type="term" ref="#c1">fresh beans</mrk>`},
		{"an edit inside a term", func(t *testing.T, b *model.Block) { replaceIn(t, b, model.EditionKey{}, "fresh", "ground") },
			`Grind <mrk id="c1" type="comment" value="check">the coffee</mrk> and <mrk id="n1" translate="no">Kapi Pro</mrk> with <mrk id="m1" type="term" ref="#c1">ground beans</mrk>`},
		{"a case conversion", func(t *testing.T, b *model.Block) {
			var edits []change.TextEdit
			text := []rune(model.SequenceText(b.SourceRuns()))
			for i := 0; i < len(text); {
				if strings.ToUpper(string(text[i])) == string(text[i]) {
					i++
					continue
				}
				j := i
				for j < len(text) && strings.ToUpper(string(text[j])) != string(text[j]) {
					j++
				}
				start, end := i, j
				edits = append(edits, change.TextEdit{Start: &start, End: &end, Text: strings.ToUpper(string(text[i:j]))})
				i = j
			}
			res := change.ApplyBlock(b, []change.Op{{Kind: change.KindReplaceText, At: change.Ref{Block: b.ID}, IfMatch: change.AnyRevision,
				Body: &change.ReplaceText{Edits: edits}}}, change.BlockEnv{Actor: change.Actor{Kind: change.ActorTool, Name: "test"}})
			require.Equal(t, change.OpApplied, res[0].Status)
		}, `GRIND <mrk id="c1" type="comment" value="check">THE COFFEE</mrk> AND <mrk id="n1" translate="no">KAPI PRO</mrk> WITH <mrk id="m1" type="term" ref="#c1">FRESH BEANS</mrk>`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			for path, out := range writePaths(t, in, func(b *model.Block) { tc.edit(t, b) }) {
				assert.Contains(t, out, tc.want, "%s path", path)
			}
		})
	}
}

// Once the overlay moves a marker the file wrote as <mrk> so that a code
// boundary falls between its ends, no <mrk> can hold it, and it is written as
// an <sm>/<em> pair.
func TestAMarkerAcrossACodeBoundaryFallsBackToAPair(t *testing.T) {
	t.Parallel()
	in := unitDoc(`<unit id="u1"><originalData><data id="d1">&lt;b&gt;</data><data id="d2">&lt;/b&gt;</data></originalData><segment>
    <source>Grind <mrk id="c1" type="comment">the</mrk> <pc id="1" dataRefStart="d1" dataRefEnd="d2">coffee</pc> now</source>
  </segment></unit>`)
	// Runs: 0 "Grind ", 1 "the", 2 " ", 3 pc open, 4 "coffee", 5 pc close, 6 " now".
	stretch := func(b *model.Block) {
		o := b.OverlayOf(xliff2.OverlayMrk)
		require.NotNil(t, o)
		o.Spans[0].Range.End = model.RunPos{Run: 4, Offset: 3}
	}
	for path, out := range writePaths(t, in, stretch) {
		assert.Contains(t, out, `Grind <sm id="c1" type="comment"/>the <pc id="1" dataRefStart="d1" dataRefEnd="d2">cof<em startRef="c1"/>fee</pc> now`, "%s path", path)
	}
}

// A marker pair split across segments is written as the file wrote it: the
// <sm> in its segment and the <em> in the next, after an edit to either. The
// writer stripped the <sm> of a term pair and left the <em> alone, which is
// not XLIFF 2. When the pair's span is gone and a segment holding a half is
// rebuilt, neither half is written.
func TestAMarkerPairAcrossSegmentsKeepsBothHalves(t *testing.T) {
	t.Parallel()
	for _, typ := range []string{`type="term"`, `type="comment" value="n"`} {
		in := unitDoc(`<unit id="u1"><segment id="s1">
    <source>Grind <sm id="m1" ` + typ + `/>coffee </source>
  </segment><segment id="s2">
    <source>beans<em startRef="m1"/> now</source>
  </segment></unit>`)
		cases := []struct {
			name, find, with string
			want             []string
		}{
			{"untouched", "", "", []string{`Grind <sm id="m1" ` + typ + `/>coffee </source>`, `<source>beans<em startRef="m1"/> now</source>`}},
			{"an edit to the first segment", "Grind", "Brew", []string{`Brew <sm id="m1" ` + typ + `/>coffee </source>`, `<source>beans<em startRef="m1"/> now</source>`}},
			{"an edit to the second segment", "now", "today", []string{`Grind <sm id="m1" ` + typ + `/>coffee </source>`, `<source>beans<em startRef="m1"/> today</source>`}},
		}
		for _, tc := range cases {
			t.Run(typ+" "+tc.name, func(t *testing.T) {
				t.Parallel()
				edit := func(b *model.Block) {
					if tc.find != "" {
						replaceIn(t, b, model.EditionKey{}, tc.find, tc.with)
					}
				}
				for path, out := range writePaths(t, in, edit) {
					for _, want := range tc.want {
						assert.Contains(t, out, want, "%s path", path)
					}
				}
			})
		}
	}

	in := unitDoc(`<unit id="u1"><segment id="s1">
    <source>Grind <sm id="m1" type="term"/>coffee </source>
  </segment><segment id="s2">
    <source>beans<em startRef="m1"/> now</source>
  </segment></unit>`)
	b := readOneBlock(t, in)
	term := b.OverlayOf(model.OverlayTerm)
	require.NotNil(t, term, "the reader records the pair as one span")
	require.Len(t, term.Spans, 1)
	assert.Equal(t, "coffee beans", model.RunsText(term.Spans[0].Range.ExtractRuns(b.SourceRuns())))

	dropped := func(b *model.Block) {
		b.RemoveOverlay(model.OverlayTerm)
		replaceIn(t, b, model.EditionKey{}, "Grind", "Brew")
	}
	for path, out := range writePaths(t, in, dropped) {
		assert.NotContains(t, out, "<sm", "%s path", path)
		assert.NotContains(t, out, "<em", "%s path", path)
		assert.Contains(t, out, `beans now</source>`, "%s path", path)
	}
}

// When only the overlays change, every write path writes what they hold: a
// term span removed is no longer marked, and one added to an untouched target
// is drawn. The path that patches the parsed document compared the IR without
// its marks to the document and so changed nothing.
func TestTheWritePathsAgreeWhenOnlyTheOverlaysChange(t *testing.T) {
	t.Parallel()
	in := unitDoc(`<unit id="u1"><segment>
    <source>Grind the <mrk id="m1" type="term" ref="#c1">coffee</mrk> now</source>
    <target>Moulez le café maintenant</target>
  </segment></unit>`)
	removed := func(b *model.Block) { b.RemoveOverlay(model.OverlayTerm) }
	for path, out := range writePaths(t, in, removed) {
		assert.Contains(t, out, `<source>Grind the coffee now</source>`, "%s path", path)
	}
	added := func(b *model.Block) {
		fr := model.Variant(model.LocaleFrench)
		b.Overlays = append(b.Overlays, model.Overlay{Type: model.OverlayTerm, Variant: &fr, Spans: []model.Span{{ID: "t1",
			Range: model.SpanAnchor(model.RunPos{Run: 0, Offset: 10}, model.RunPos{Run: 0, Offset: 14})}}})
	}
	for path, out := range writePaths(t, in, added) {
		assert.Contains(t, out, `Moulez le <sm id="t1" type="term"/>café<em startRef="t1"/> maintenant`, "%s path", path)
	}
}

// A target a tool writes into a unit has no inline IR of its own. Its codes
// take the attributes the document gave them in the source, on the path that
// patches the parsed document as on the skeleton path, which wrote its text
// alone, and the scratch path writes it beside the first segment's source,
// where it wrote nothing.
func TestATargetAToolCreatedKeepsItsCodes(t *testing.T) {
	t.Parallel()
	in := unitDoc(`<unit id="u1"><originalData><data id="d1">&lt;b&gt;</data><data id="d2">&lt;/b&gt;</data></originalData><segment>
    <source>Click <pc id="1" dataRefStart="d1" dataRefEnd="d2">Save</pc> now</source>
  </segment></unit>`)
	translate := func(b *model.Block) {
		tgt := []model.Run{model.TextR("Cliquez ")}
		tgt = append(tgt, b.SourceRuns()[1], model.TextR("Enregistrer"), b.SourceRuns()[3], model.TextR(" maintenant"))
		b.SetTargetRuns(model.LocaleFrench, tgt)
	}
	for path, out := range writePaths(t, in, translate) {
		assert.Contains(t, out, `<target>Cliquez <pc id="1" dataRefStart="d1" dataRefEnd="d2">Enregistrer</pc> maintenant</target>`, "%s path", path)
	}
}
