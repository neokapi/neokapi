package ts_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/formats/ts"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/tools"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const numerusDoc = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE TS []>
<TS version="2.1" language="fr" sourcelanguage="en">
<context>
    <name>MainWindow</name>
    <message numerus="yes">
        <source>We use %n item(s)</source>
        <translation>
            <numerusform>Nous employons %n article</numerusform>
            <numerusform>Nous employons %n articles</numerusform>
        </translation>
    </message>
</context>
</TS>
`

// lupdateDoc is a file as lupdate writes it before anyone translates it: every
// translation empty and unfinished, a numerus message with one empty
// `<numerusform>` per plural form of the language.
const lupdateDoc = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE TS []>
<TS version="2.1" language="fr" sourcelanguage="en">
<context>
    <name>MainWindow</name>
    <message>
        <source>Open file</source>
        <translation type="unfinished"></translation>
    </message>
    <message numerus="yes">
        <source>We use %n item(s)</source>
        <translation type="unfinished">
            <numerusform></numerusform>
            <numerusform></numerusform>
        </translation>
    </message>
</context>
</TS>
`

// singleFormDoc is a numerus message in a language with one plural form, whose
// translation was read as one text run.
const singleFormDoc = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE TS []>
<TS version="2.1" language="fr" sourcelanguage="en">
<context>
    <name>MainWindow</name>
    <message numerus="yes">
        <source>We use %n item(s)</source>
        <translation>
            <numerusform>Des articles</numerusform>
        </translation>
    </message>
</context>
</TS>
`

// writeNumerus reads doc through the skeleton path, hands every message to
// edit, and writes the document back for the locale fr. It returns what was
// written and the writer's error.
func writeNumerus(t *testing.T, doc string, edit func(*model.Block)) (string, error) {
	t.Helper()
	ctx := t.Context()
	reader, writer := ts.NewReader(), ts.NewWriter()
	store, err := format.NewSkeletonStore()
	require.NoError(t, err)
	defer store.Close()
	reader.SetSkeletonStore(store)
	writer.SetSkeletonStore(store)

	raw := testutil.RawDocFromString(doc, model.LocaleEnglish)
	raw.TargetLocale = "fr"
	require.NoError(t, reader.Open(ctx, raw))
	parts := testutil.CollectParts(t, reader.Read(ctx))
	require.NoError(t, reader.Close())
	for _, p := range parts {
		if b, ok := p.Resource.(*model.Block); ok && edit != nil {
			edit(b)
		}
	}

	var buf bytes.Buffer
	require.NoError(t, writer.SetOutputWriter(&buf))
	writer.SetLocale("fr")
	werr := writer.Write(ctx, testutil.PartsToChannel(parts))
	require.NoError(t, writer.Close())
	return buf.String(), werr
}

// pseudoTranslate runs the pseudo-translate tool over a message, as
// `kapi pseudo-translate` does: the translation is written from the source.
func pseudoTranslate(t *testing.T) func(*model.Block) {
	cfg := &tools.PseudoConfig{}
	cfg.Reset()
	cfg.TargetLocale = "fr"
	pt := tools.NewPseudoTranslateTool(cfg)
	return func(b *model.Block) {
		_, err := pt.ApplyContext(context.Background(), &model.Part{Type: model.PartBlock, Resource: b})
		require.NoError(t, err)
	}
}

// A numerus message holds its plural forms as spans over one translation: one
// target segmentation span per `<numerusform>`. The writer writes each form
// from its span while the spans tile the translation's runs, and a one-form
// message from its whole translation.
//
// When a message of several forms has no span per form tiling its runs,
// nothing says which words belong to which form: a tool's translation of the
// whole message and an edit whose spans were dropped look the same. Cutting
// the new runs at the old spans moved words between forms ("Nous utilisons %n
// articleNous utilisons " in the first form), and writing the whole
// translation into every form would repeat an edit's forms in each, so the
// writer refuses that message before it writes any byte. It refuses only when
// the translation would be written: a message whose forms were all empty when
// read, as lupdate writes them, is written empty whatever a tool put in its
// translation, so translating or pseudo-translating such a file writes the
// file.
func TestNumerusFormsAreWrittenOrRefused(t *testing.T) {
	replaceInRuns := func(b *model.Block) {
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
	}
	replaceInEditText := func(b *model.Block) {
		runs := b.TargetRuns("fr")
		text := strings.ReplaceAll(model.RunsEditText(runs), "employons", "utilisons")
		b.SetTargetRuns("fr", model.ParseRunsEditText(text, runs))
	}
	// translateMT replaces the translation as an MT tool does
	// (core/mt/tools/translate.go), leaving the reader's segmentation in place.
	translateMT := func(b *model.Block) {
		if b.Properties["numerus"] == "yes" {
			b.SetTargetText("fr", "Nous utilisons %n article(s)")
		}
	}
	// translateWhole replaces the translation and drops the segmentation that
	// described the old one, as a write that rebases target overlays leaves it.
	translateWhole := func(b *model.Block) {
		translateMT(b)
		key := model.Variant("fr")
		b.SetSegmentation(key, nil)
	}

	tests := []struct {
		name    string
		doc     string
		edit    func(*model.Block)
		want    string
		refused bool
	}{
		{name: "untouched", doc: numerusDoc, want: numerusDoc},
		{
			name: "each form edited in its own runs",
			doc:  numerusDoc,
			edit: replaceInRuns,
			want: strings.ReplaceAll(numerusDoc, "Nous employons", "Nous utilisons"),
		},
		{name: "the forms edited as one text", doc: numerusDoc, edit: replaceInEditText, refused: true},
		{
			name:    "a tool's translation over filled forms whose spans no longer line up",
			doc:     numerusDoc,
			edit:    translateMT,
			refused: true,
		},
		{
			// A rebase that drops the span an edit overlapped and keeps the
			// other leaves one span over the whole translation.
			name: "one span left over a two-form message",
			doc:  numerusDoc,
			edit: func(b *model.Block) {
				b.SetTargetText("fr", "Nous utilisons %n article(s)")
				key := model.Variant("fr")
				b.SetSegmentation(key, []model.Span{{
					ID:    "n1",
					Range: model.SpanAnchor(model.RunPos{Run: 0}, model.RunPos{Run: 1}),
				}})
			},
			refused: true,
		},
		{
			// The same shape as an edit whose rebase dropped the spans, which
			// would repeat the edited forms in every form.
			name:    "a translation with no form boundaries over filled forms",
			doc:     numerusDoc,
			edit:    translateWhole,
			refused: true,
		},
		{
			name: "a translation with no form boundaries over forms read empty",
			doc:  lupdateDoc,
			edit: translateWhole,
			want: lupdateDoc,
		},
		{
			name: "a tool's translation over forms read empty",
			doc:  lupdateDoc,
			edit: pseudoTranslate(t),
			want: strings.Replace(lupdateDoc,
				`<translation type="unfinished"></translation>`,
				`<translation type="unfinished">▒ Öþéñ ƒîļé ▒</translation>`, 1),
		},
		{
			name: "an MT translation over forms read empty",
			doc:  lupdateDoc,
			edit: translateMT,
			want: lupdateDoc,
		},
		{
			name: "a tool's translation of a one-form message is written whole",
			doc:  singleFormDoc,
			edit: pseudoTranslate(t),
			want: strings.Replace(singleFormDoc, "Des articles", "▒ Ŵé üšé %n îţéḿ(š) ▒", 1),
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			out, err := writeNumerus(t, tc.doc, tc.edit)
			if tc.refused {
				require.ErrorIs(t, err, ts.ErrNumerusFormsLost)
				assert.Empty(t, out, "a refused write writes nothing")
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, out)
		})
	}
}
