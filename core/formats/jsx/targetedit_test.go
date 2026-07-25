package jsx

import (
	"bytes"
	"context"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/kbf"
	"github.com/neokapi/neokapi/core/model"
)

// The writer seeded each output block's targets from the KBFAnnotation — the
// block *as read* — and then filled in the model's target only for locales the
// annotation lacked. So whenever the input .kbf already carried a target for
// the write locale, whatever the pipeline produced was thrown away and the
// original re-emitted, byte for byte. The tool reported success; the edit was
// simply gone. Any tool that edits an existing target was a silent no-op
// through .kbf: a whitespace correction, a post-edit pass, a re-translation, an
// unredact, a removal.
//
// These tests drive the real reader → edit → writer path, because the reader is
// what attaches the annotation and the writer is what consulted it.

// nbTgt is the locale these tests edit.
const nbTgt = model.LocaleID("nb")

// translatedKBF builds a .kbf that already carries an `nb` target, with inline
// codes on both sides — the shape a catalog has after one translation pass, and
// the shape a second pass has to be able to change.
func translatedKBF() *kbf.File {
	return &kbf.File{
		SchemaVersion: kbf.SchemaVersion,
		Kind:          kbf.Kind,
		Generator:     kbf.GeneratorInfo{ID: "@neokapi/i18n-react", Version: "1.2.3"},
		Project:       kbf.ProjectInfo{ID: "app", SourceLocale: "en"},
		Documents: []kbf.Document{{
			ID:           "src/Checkout.tsx",
			DocumentType: kbf.DocumentTypeJSX,
			Path:         "src/Checkout.tsx",
			Blocks: []kbf.Block{{
				ID: "src/Checkout.tsx:12:0", Hash: "k53pbACm9yC", Translatable: true,
				Type: kbf.BlockTypeJSXElement,
				Source: []kbf.Run{
					{Text: &kbf.TextRun{Text: "Save "}},
					{PcOpen: &kbf.PcOpenRun{ID: "1", Type: "jsx:element", SubType: "strong", Data: "<strong>", Equiv: "=m0"}},
					{Text: &kbf.TextRun{Text: "now"}},
					{PcClose: &kbf.PcCloseRun{ID: "1", Type: "jsx:element", SubType: "strong", Data: "</strong>", Equiv: "=m1"}},
				},
				Targets: map[kbf.LocaleID][]kbf.Run{
					"nb": {
						{Text: &kbf.TextRun{Text: "Lagre  "}},
						{PcOpen: &kbf.PcOpenRun{ID: "1", Type: "jsx:element", SubType: "strong", Data: "<strong>", Equiv: "=m0"}},
						{Text: &kbf.TextRun{Text: "nå"}},
						{PcClose: &kbf.PcCloseRun{ID: "1", Type: "jsx:element", SubType: "strong", Data: "</strong>", Equiv: "=m1"}},
					},
				},
				Properties: kbf.BlockProperties{File: "src/Checkout.tsx", Line: 12, Component: "Checkout", Element: "p"},
			}},
		}},
	}
}

// editThroughKBF runs the real reader over file, hands the model blocks to
// edit (standing in for a tool), writes at locale, and returns the parsed
// output. locale may be empty — the writer is not always pointed at a locale.
func editThroughKBF(t *testing.T, file *kbf.File, locale model.LocaleID, edit func([]*model.Block)) *kbf.File {
	t.Helper()
	in, err := kbf.Marshal(file)
	require.NoError(t, err)

	r := NewReader()
	require.NoError(t, r.Open(context.Background(), &model.RawDocument{
		URI: "in.kbf", Reader: io.NopCloser(bytes.NewReader(in)),
	}))
	blocks := collectBlocks(t, r)
	require.NotEmpty(t, blocks)

	edit(blocks)

	w := NewWriter()
	if locale != "" {
		w.SetLocale(locale)
	}
	var sink bytes.Buffer
	require.NoError(t, w.SetOutputWriter(&sink))
	ch := make(chan *model.Part, len(blocks))
	for _, b := range blocks {
		ch <- &model.Part{Type: model.PartBlock, Resource: b}
	}
	close(ch)
	require.NoError(t, w.Write(context.Background(), ch))
	require.NoError(t, w.Close())

	out, err := kbf.Unmarshal(sink.Bytes())
	require.NoError(t, err)
	return out
}

// kbfRunShapes renders a run sequence one string per run, so a failure names
// exactly which run changed.
func kbfRunShapes(runs []kbf.Run) []string {
	out := make([]string, 0, len(runs))
	for _, r := range runs {
		switch {
		case r.Text != nil:
			out = append(out, "text:"+r.Text.Text)
		case r.Ph != nil:
			out = append(out, "ph:"+r.Ph.Equiv)
		case r.PcOpen != nil:
			out = append(out, "pcOpen:"+r.PcOpen.ID)
		case r.PcClose != nil:
			out = append(out, "pcClose:"+r.PcClose.ID)
		default:
			out = append(out, "other")
		}
	}
	return out
}

// The headline: a tool edits a target the input already had, and the edit
// reaches the file. The double space is exactly what whitespace-correct
// collapses, and the inline codes on either side of it must survive.
func TestWriterEmitsToolEditToAnExistingTarget(t *testing.T) {
	out := editThroughKBF(t, translatedKBF(), nbTgt, func(blocks []*model.Block) {
		runs := blocks[0].TargetRuns(nbTgt)
		require.Len(t, runs, 4)
		edited := append([]model.Run(nil), runs...)
		edited[0] = model.Run{Text: &model.TextRun{Text: "Lagre "}}
		blocks[0].SetTargetRuns(nbTgt, edited)
	})

	blocks := allBlocks(out)
	require.Len(t, blocks, 1)
	assert.Equal(t,
		[]string{"text:Lagre ", "pcOpen:1", "text:nå", "pcClose:1"},
		kbfRunShapes(blocks[0].Targets["nb"]),
		"the written target must be the one the pipeline produced, not the one the file arrived with")
}

// A tool that populates a target from scratch — pseudo-translate, recycle, a
// re-translation — must also land, even though the file already had one.
func TestWriterEmitsAReplacedTarget(t *testing.T) {
	out := editThroughKBF(t, translatedKBF(), nbTgt, func(blocks []*model.Block) {
		blocks[0].SetTargetRuns(nbTgt, []model.Run{
			{Text: &model.TextRun{Text: "Kjøp "}},
			{Ph: &model.PlaceholderRun{ID: "9", Type: "jsx:var", Data: "{price}", Equiv: "price"}},
		})
	})

	blocks := allBlocks(out)
	require.Len(t, blocks, 1)
	assert.Equal(t,
		[]string{"text:Kjøp ", "ph:price"},
		kbfRunShapes(blocks[0].Targets["nb"]),
		"a wholesale replacement is an edit too")
}

// Removal is an edit the writer has to be able to express: remove-target
// deletes the locale from the model block, and the file must lose it.
func TestWriterEmitsTargetRemoval(t *testing.T) {
	out := editThroughKBF(t, translatedKBF(), nbTgt, func(blocks []*model.Block) {
		delete(blocks[0].Targets, model.Variant(nbTgt))
	})

	blocks := allBlocks(out)
	require.Len(t, blocks, 1)
	assert.NotContains(t, blocks[0].Targets, kbf.LocaleID("nb"),
		"a target the pipeline removed must not be re-emitted from the annotation")
}

// The writer is not always pointed at the locale a tool edited — a flow can
// write a multi-locale catalog with no writer locale at all. The model's
// targets are the writer's authority either way.
func TestWriterEmitsEditsWhenNotPointedAtTheLocale(t *testing.T) {
	for name, locale := range map[string]model.LocaleID{
		"no writer locale":         "",
		"a different write locale": model.LocaleID("de"),
	} {
		t.Run(name, func(t *testing.T) {
			out := editThroughKBF(t, translatedKBF(), locale, func(blocks []*model.Block) {
				blocks[0].SetTargetRuns(nbTgt, []model.Run{{Text: &model.TextRun{Text: "Lagre nå"}}})
			})

			blocks := allBlocks(out)
			require.Len(t, blocks, 1)
			assert.Equal(t, []string{"text:Lagre nå"},
				kbfRunShapes(blocks[0].Targets["nb"]),
				"an edit to a locale the writer was not pointed at is still an edit")
		})
	}
}

// A newly added locale must land alongside the one already in the file.
func TestWriterEmitsAnAddedLocaleAlongsideTheExistingOne(t *testing.T) {
	out := editThroughKBF(t, translatedKBF(), model.LocaleID("de"), func(blocks []*model.Block) {
		blocks[0].SetTargetRuns(model.LocaleID("de"), []model.Run{{Text: &model.TextRun{Text: "Jetzt sparen"}}})
	})

	blocks := allBlocks(out)
	require.Len(t, blocks, 1)
	assert.Equal(t, []string{"text:Jetzt sparen"}, kbfRunShapes(blocks[0].Targets["de"]))
	assert.Equal(t,
		[]string{"text:Lagre  ", "pcOpen:1", "text:nå", "pcClose:1"},
		kbfRunShapes(blocks[0].Targets["nb"]),
		"the locale nothing touched is unchanged")
}

// A tone- or channel-qualified variant has no slot in the .kbf wire shape,
// which keys targets by bare locale. It must not silently overwrite the plain
// target for that locale.
func TestWriterIgnoresToneQualifiedVariants(t *testing.T) {
	out := editThroughKBF(t, translatedKBF(), nbTgt, func(blocks []*model.Block) {
		blocks[0].Targets[model.VariantKey{Locale: nbTgt, Tone: "formal"}] = &model.Target{
			Runs: []model.Run{{Text: &model.TextRun{Text: "De lagrer nå"}}},
		}
	})

	blocks := allBlocks(out)
	require.Len(t, blocks, 1)
	assert.Equal(t,
		[]string{"text:Lagre  ", "pcOpen:1", "text:nå", "pcClose:1"},
		kbfRunShapes(blocks[0].Targets["nb"]),
		"the locale-only target keeps the slot; a toned variant does not claim it")
}
