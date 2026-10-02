package xliff2_test

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/formats/xliff2"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// skeletonEdit reads input through the skeleton path, hands every block to
// edit, and writes it back through the same skeleton, the way `kapi apply`
// and `ksed` drive a same-format round trip.
func skeletonEdit(t *testing.T, input string, edit func(*model.Block)) string {
	t.Helper()
	out, err := skeletonWrite(t, input, edit, nil)
	require.NoError(t, err)
	return out
}

// skeletonWrite is skeletonEdit returning the writer's error, with the
// skeleton the reader recorded passed through replay before the writer reads
// it, when replay is set.
func skeletonWrite(t *testing.T, input string, edit func(*model.Block), replay func(*testing.T, *format.SkeletonStore) *format.SkeletonStore) (string, error) {
	t.Helper()
	ctx := t.Context()

	reader := xliff2.NewReader()
	writer := xliff2.NewWriter()
	store, err := format.NewSkeletonStore()
	require.NoError(t, err)
	defer store.Close()
	reader.SetSkeletonStore(store)

	require.NoError(t, reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish)))
	parts := testutil.CollectParts(t, reader.Read(ctx))
	require.NoError(t, reader.Close())
	for _, p := range parts {
		if b, ok := p.Resource.(*model.Block); ok && edit != nil {
			edit(b)
		}
	}

	if replay != nil {
		writer.SetSkeletonStore(replay(t, store))
	} else {
		writer.SetSkeletonStore(store)
	}
	var buf bytes.Buffer
	require.NoError(t, writer.SetOutputWriter(&buf))
	werr := writer.Write(ctx, testutil.PartsToChannel(parts))
	require.NoError(t, writer.Close())
	return buf.String(), werr
}

// replaceInEditText rewrites a word in an edition's edit text and parses it
// back against the edition's runs, as `kapi apply` does.
func replaceInEditText(runs []model.Run, from, to string) []model.Run {
	text := model.RunsEditText(runs)
	return model.ParseRunsEditText(strings.ReplaceAll(text, from, to), runs)
}

func editSource(from, to string) func(*model.Block) {
	return func(b *model.Block) {
		b.EditSourceRuns(replaceInEditText(b.Source, from, to))
	}
}

func editTarget(loc model.LocaleID, from, to string) func(*model.Block) {
	return func(b *model.Block) {
		if runs := b.TargetRuns(loc); runs != nil {
			b.SetTargetRuns(loc, replaceInEditText(runs, from, to))
		}
	}
}

const inlineCodesDoc = `<?xml version="1.0" encoding="UTF-8"?>
<xliff xmlns="urn:oasis:names:tc:xliff:document:2.0" version="2.0" srcLang="en" trgLang="fr">
  <file id="f1">
    <unit id="u1">
      <originalData>
        <data id="d1">&lt;br/&gt;</data>
      </originalData>
      <segment>
        <source>We utilize the <pc id="1" canCopy="no">finest</pc> ingredients<ph id="2" dataRef="d1"/></source>
        <target>Nous employons les <pc id="1" canCopy="no">meilleurs</pc> ingrédients<ph id="2" dataRef="d1"/></target>
      </segment>
    </unit>
    <unit id="u2">
      <segment id="s1">
        <source>Spices we <sc id="3"/>utilize<ec startRef="3"/> &amp; love</source>
      </segment>
    </unit>
    <unit id="u3">
      <segment>
        <source>Untouched <pc id="1">bold</pc> &amp; <ph id="2"/> text</source>
        <target>Intact <pc id="1">gras</pc> &amp; <ph id="2"/> texte</target>
      </segment>
    </unit>
  </file>
</xliff>`

// TestSkeletonPathKeepsInlineCodes is the regression for D1: the skeleton path
// read a <source>'s inline codes as literal text and wrote them back escaped,
// so `<pc id="1">` became `&lt;pc id="1"&gt;` in every segment the write
// reached, edited or not.
func TestSkeletonPathKeepsInlineCodes(t *testing.T) {
	tests := []struct {
		name string
		edit func(*model.Block)
		want string
	}{
		{
			name: "untouched document is written as read",
			want: inlineCodesDoc,
		},
		{
			name: "a source edit keeps the codes and their attributes",
			edit: editSource("utilize", "use"),
			want: strings.NewReplacer(
				`We utilize the`, `We use the`,
				`<sc id="3"/>utilize<ec startRef="3"/>`, `<sc id="3"/>use<ec startRef="3"/>`,
			).Replace(inlineCodesDoc),
		},
		{
			name: "a target edit keeps the codes and their attributes",
			edit: editTarget("fr", "employons", "utilisons"),
			want: strings.Replace(inlineCodesDoc, "Nous employons", "Nous utilisons", 1),
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, skeletonEdit(t, inlineCodesDoc, tc.edit))
		})
	}
}

// TestSkeletonPathReadsInlineCodesAsRuns pins the read side of D1: the codes
// arrive as runs, so the edit text an agent reads names each one as a code,
// and an <sc>/<ec> pair as the pair it is.
func TestSkeletonPathReadsInlineCodesAsRuns(t *testing.T) {
	var texts []string
	skeletonEdit(t, inlineCodesDoc, func(b *model.Block) {
		texts = append(texts, model.RunsEditText(b.Source))
	})
	assert.Equal(t, []string{
		`We utilize the <x id="1"/>finest<x id="/1"/> ingredients<x id="2/"/>`,
		`Spices we <x id="3"/>utilize<x id="/3"/> & love`,
		`Untouched <x id="1"/>bold<x id="/1"/> & <x id="2/"/> text`,
	}, texts)
}

const multiSegmentDoc = `<?xml version="1.0" encoding="UTF-8"?>
<xliff xmlns="urn:oasis:names:tc:xliff:document:2.0" version="2.0" srcLang="en" trgLang="fr">
  <file id="f1">
    <unit id="u1">
      <segment id="a">
        <source>First we utilize <pc id="1">this</pc>. </source>
        <target>D'abord nous employons <pc id="1">ceci</pc>. </target>
      </segment>
      <segment id="b">
        <source>Then <ph id="2"/> that.</source>
        <target>Puis <ph id="2"/> cela.</target>
      </segment>
    </unit>
  </file>
</xliff>`

const untranslatedSegmentsDoc = `<?xml version="1.0" encoding="UTF-8"?>
<xliff xmlns="urn:oasis:names:tc:xliff:document:2.0" version="2.0" srcLang="en" trgLang="fr">
  <file id="f1">
    <unit id="u1">
      <segment id="a">
        <source>First we use this. </source>
      </segment>
      <segment id="b">
        <source>Then that.</source>
      </segment>
    </unit>
  </file>
</xliff>`

// translateUnit writes text as the fr translation of the whole unit, with no
// segmentation, as a tool that translates a unit at once leaves it.
func translateUnit(text string) func(*model.Block) {
	return func(b *model.Block) {
		b.SetTargetText("fr", text)
		key := model.Variant("fr")
		b.SetSegmentation(&key, nil)
	}
}

// replaceInRuns rewrites a word inside each text run, keeping the run
// structure, and with it the segment boundaries.
func replaceInRuns(runs []model.Run, from, to string) []model.Run {
	out := make([]model.Run, len(runs))
	copy(out, runs)
	for i := range out {
		if out[i].Text != nil {
			txt := *out[i].Text
			txt.Text = strings.ReplaceAll(txt.Text, from, to)
			out[i].Text = &txt
		}
	}
	return out
}

// TestSkeletonPathWritesEachSegment covers units of several segments. Each
// segment is written from its own runs; the skeleton path used to write the
// whole unit's text into every segment.
//
// An edit through edit text carries no segment boundaries. When the runs it
// rebuilds merge two segments, the segmentation no longer describes them, and
// writing the unit would move words between segments or leave a source with
// no translation beside it. The writer refuses that document before it writes
// a byte, and refuses a translation with no segmentation over one read segment
// by segment, which looks the same. A unit read with no translation has no
// boundaries to lose, and its first segment takes a translation of the unit.
func TestSkeletonPathWritesEachSegment(t *testing.T) {
	tests := []struct {
		name    string
		doc     string // multiSegmentDoc when empty
		edit    func(*model.Block)
		want    string
		refused error
	}{
		{
			name: "untouched",
			want: multiSegmentDoc,
		},
		{
			name:    "a source edit that merges the segments",
			edit:    editSource("utilize", "use"),
			refused: xliff2.ErrSegmentsLost,
		},
		{
			name: "a source edit that merges the segments, with the overlays rebased",
			edit: func(b *model.Block) {
				old := b.Source
				b.EditSourceRuns(replaceInEditText(b.Source, "utilize", "use"))
				model.RemapOverlays(b, nil, old, b.Source, []model.RunEdit{{
					Start: 0, End: len([]rune(model.RunsText(old))), NewLen: len([]rune(model.RunsText(b.Source))),
				}})
			},
			refused: xliff2.ErrSegmentsLost,
		},
		{
			name:    "a target edit that merges the segments",
			edit:    editTarget("fr", "employons", "utilisons"),
			refused: xliff2.ErrSegmentsLost,
		},
		{
			name: "a source replaced segment by segment",
			edit: func(b *model.Block) { b.EditSourceRuns(replaceInRuns(b.Source, "utilize", "use")) },
			want: strings.Replace(multiSegmentDoc, "we utilize", "we use", 1),
		},
		{
			name: "a target replaced segment by segment",
			edit: func(b *model.Block) {
				b.SetTargetRuns("fr", replaceInRuns(b.TargetRuns("fr"), "employons", "utilisons"))
			},
			want: strings.Replace(multiSegmentDoc, "nous employons", "nous utilisons", 1),
		},
		{
			// The same shape as an edit whose rebase dropped the segmentation.
			name:    "a translation with no segmentation over one read segment by segment",
			edit:    translateUnit("Tout en un."),
			refused: xliff2.ErrSegmentsLost,
		},
		{
			name: "a translation of a unit read with no translation",
			doc:  untranslatedSegmentsDoc,
			edit: translateUnit("Tout en un."),
			want: strings.Replace(untranslatedSegmentsDoc,
				"<source>First we use this. </source>",
				"<source>First we use this. </source>\n        <target>Tout en un.</target>", 1),
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			doc := tc.doc
			if doc == "" {
				doc = multiSegmentDoc
			}
			out, err := skeletonWrite(t, doc, tc.edit, nil)
			if tc.refused != nil {
				require.ErrorIs(t, err, tc.refused)
				assert.Empty(t, out, "a refused write writes nothing")
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, out)
		})
	}
}

// legacySkeleton copies a skeleton the way a build before segment ids spelled
// it: each reference as block:segment:element:indent, and no pairing of an
// element with its bytes.
func legacySkeleton(t *testing.T, store *format.SkeletonStore) *format.SkeletonStore {
	t.Helper()
	require.NoError(t, store.Flush())
	out := format.NewMemorySkeletonStore()
	for {
		entry, err := store.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		require.NoError(t, err)
		switch entry.Type {
		case format.SkeletonText:
			out.WriteText(entry.Data)
		case format.SkeletonRef:
			f := strings.SplitN(string(entry.Data), ":", 5)
			require.Len(t, f, 5, "ref %q", entry.Data)
			out.WriteRef(f[0] + ":" + f[1] + ":" + f[2] + ":" + f[4])
		case format.SkeletonOriginal:
		default:
			t.Fatalf("unexpected skeleton entry type %d", entry.Type)
		}
	}
	return out
}

// TestSkeletonPathReadsReferencesWithoutSegmentIDs is the regression for
// skeletons persisted by an earlier build. Their references carry no segment
// id; the writer skipped every one, and each <source> and <target> was written
// empty with no error. They name their segment by position.
func TestSkeletonPathReadsReferencesWithoutSegmentIDs(t *testing.T) {
	for _, doc := range []struct{ name, input string }{
		{"one segment per unit", inlineCodesDoc},
		{"several segments in a unit", multiSegmentDoc},
	} {
		t.Run(doc.name, func(t *testing.T) {
			out, err := skeletonWrite(t, doc.input, nil, legacySkeleton)
			require.NoError(t, err)
			assert.Equal(t, doc.input, out)
		})
	}
}

// TestSkeletonPathRefusesCodesItCannotWrite pins the writer's refusal of runs
// no XLIFF 2 markup expresses, such as two code pairs that cross. Writing such
// a segment as its text would drop both codes.
func TestSkeletonPathRefusesCodesItCannotWrite(t *testing.T) {
	crossing := func(b *model.Block) {
		if b.ID != "u3" {
			return
		}
		b.EditSourceRuns([]model.Run{
			{Text: &model.TextRun{Text: "See "}},
			{PcOpen: &model.PcOpenRun{ID: "1"}},
			{Text: &model.TextRun{Text: "alpha "}},
			{PcOpen: &model.PcOpenRun{ID: "9"}},
			{Text: &model.TextRun{Text: "and"}},
			{PcClose: &model.PcCloseRun{ID: "1"}},
			{Text: &model.TextRun{Text: " beta"}},
			{PcClose: &model.PcCloseRun{ID: "9"}},
			{Text: &model.TextRun{Text: " here"}},
		})
	}
	out, err := skeletonWrite(t, inlineCodesDoc, crossing, nil)
	require.ErrorIs(t, err, xliff2.ErrCodesUnwritable)
	assert.Empty(t, out, "a refused write writes nothing")
}

// The skeleton path draws term marks by position too. A precise edit to the
// first segment joins its text to the second's, and the second segment's text
// to the third's, in one run each; both marks still land around their terms.
func TestSkeletonPathDrawsTermMarksAfterAnEdit(t *testing.T) {
	doc := `<?xml version="1.0"?>
<xliff xmlns="urn:oasis:names:tc:xliff:document:2.0" version="2.0" srcLang="en">
  <file id="f1"><unit id="u1">
    <segment id="s1"><source>First.</source></segment>
    <segment id="s2"><source>Run <ph id="1"/>kapi<ph id="2"/> now.</source></segment>
    <segment id="s3"><source>Install the kapi CLI.</source></segment>
  </unit></file>
</xliff>`
	out := skeletonEdit(t, doc, func(block *model.Block) {
		// Runs: 0 "First.", 1 "Run ", 2 ph, 3 "kapi", 4 ph, 5 " now.",
		// 6 "Install the kapi CLI.".
		block.AddOverlaySpan(model.OverlayTerm, model.Span{ID: "t0", Range: model.SpanAnchor(
			model.RunPos{Run: 3}, model.RunPos{Run: 4})})
		block.AddOverlaySpan(model.OverlayTerm, model.Span{ID: "t1", Range: model.SpanAnchor(
			model.RunPos{Run: 6, Offset: 12}, model.RunPos{Run: 6, Offset: 16})})
		find := "First"
		res := change.ApplyBlock(block, []change.Op{{
			Kind: change.KindReplaceText, At: change.Ref{Block: block.ID}, IfMatch: change.AnyRevision,
			Body: &change.ReplaceText{Edits: []change.TextEdit{{Find: &find, Text: "Begin"}}},
		}}, change.BlockEnv{Actor: change.Actor{Kind: change.ActorTool, Name: "test"}})
		require.Equal(t, change.OpApplied, res[0].Status)
	})

	assert.Contains(t, out, `<segment id="s1"><source>Begin.</source></segment>`)
	assert.Contains(t, out, `<source>Run <ph id="1"/><sm id="t0" type="term"/>kapi<em startRef="t0"/><ph id="2"/> now.</source>`)
	assert.Contains(t, out, `<source>Install the <sm id="t1" type="term"/>kapi<em startRef="t1"/> CLI.</source>`)
}
