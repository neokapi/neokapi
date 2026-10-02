package xliff2_test

import (
	"bytes"
	"strings"
	"testing"

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
	ctx := t.Context()

	reader := xliff2.NewReader()
	writer := xliff2.NewWriter()
	store, err := format.NewSkeletonStore()
	require.NoError(t, err)
	defer store.Close()
	reader.SetSkeletonStore(store)
	writer.SetSkeletonStore(store)

	require.NoError(t, reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish)))
	parts := testutil.CollectParts(t, reader.Read(ctx))
	require.NoError(t, reader.Close())
	for _, p := range parts {
		if b, ok := p.Resource.(*model.Block); ok && edit != nil {
			edit(b)
		}
	}

	var buf bytes.Buffer
	require.NoError(t, writer.SetOutputWriter(&buf))
	require.NoError(t, writer.Write(ctx, testutil.PartsToChannel(parts)))
	require.NoError(t, writer.Close())
	return buf.String()
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

// TestSkeletonPathWritesEachSegment covers units of several segments. Each
// segment is written from its own runs; the skeleton path used to write the
// whole unit's text into every segment. An edit through edit text carries no
// segment boundaries, so an edited unit whose segmentation no longer describes
// its runs keeps all of its content, written into the first segment.
func TestSkeletonPathWritesEachSegment(t *testing.T) {
	tests := []struct {
		name string
		edit func(*model.Block)
		want string
	}{
		{
			name: "untouched",
			want: multiSegmentDoc,
		},
		{
			name: "a source edit",
			edit: editSource("utilize", "use"),
			want: strings.NewReplacer(
				`<source>First we utilize <pc id="1">this</pc>. </source>`,
				`<source>First we use <pc id="1">this</pc>. Then <ph id="2"/> that.</source>`,
				`<source>Then <ph id="2"/> that.</source>`, `<source></source>`,
			).Replace(multiSegmentDoc),
		},
		{
			name: "a target edit",
			edit: editTarget("fr", "employons", "utilisons"),
			want: strings.NewReplacer(
				`<target>D'abord nous employons <pc id="1">ceci</pc>. </target>`,
				`<target>D'abord nous utilisons <pc id="1">ceci</pc>. Puis <ph id="2"/> cela.</target>`,
				`<target>Puis <ph id="2"/> cela.</target>`, `<target></target>`,
			).Replace(multiSegmentDoc),
		},
		{
			name: "a target replaced segment by segment",
			edit: func(b *model.Block) {
				runs := b.TargetRuns("fr")
				out := make([]model.Run, len(runs))
				copy(out, runs)
				for i := range out {
					if out[i].Text != nil {
						txt := *out[i].Text
						txt.Text = strings.ReplaceAll(txt.Text, "employons", "utilisons")
						out[i].Text = &txt
					}
				}
				b.SetTargetRuns("fr", out)
			},
			want: strings.Replace(multiSegmentDoc, "nous employons", "nous utilisons", 1),
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, skeletonEdit(t, multiSegmentDoc, tc.edit))
		})
	}
}
