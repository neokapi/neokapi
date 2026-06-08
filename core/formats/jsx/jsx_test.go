package jsx

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/klf"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFlattenRuns(t *testing.T) {
	runs := []klf.Run{
		{Text: &klf.TextRun{Text: "Files "}},
		{PcOpen: &klf.PcOpenRun{ID: "1", Type: "jsx:element", SubType: "span", Equiv: "muted"}},
		{Text: &klf.TextRun{Text: "("}},
		{Ph: &klf.PlaceholderRun{ID: "2", Type: "jsx:var", Equiv: "count"}},
		{Text: &klf.TextRun{Text: " matched)"}},
		{PcClose: &klf.PcCloseRun{ID: "1", Type: "jsx:element", SubType: "span"}},
	}
	got := FlattenRuns(runs)
	assert.Equal(t, "Files ({count} matched)", got)
}

func TestFlattenRunsPlural(t *testing.T) {
	runs := []klf.Run{
		{Plural: &klf.PluralRun{
			Pivot: "count",
			Forms: map[klf.PluralForm][]klf.Run{
				klf.PluralOne:   {{Text: &klf.TextRun{Text: "1 item"}}},
				klf.PluralOther: {{Text: &klf.TextRun{Text: "many items"}}},
			},
		}},
	}
	got := FlattenRuns(runs)
	assert.Equal(t, "many items", got, "plural flattens to the 'other' branch")
}

func TestReaderReadsKLF(t *testing.T) {
	// Build a .klf in memory and feed it through the reader.
	doc := makeKLFFile()
	buf, err := klf.Marshal(doc)
	require.NoError(t, err)

	r := NewReader()
	raw := &model.RawDocument{
		URI:    "inline.klf",
		Reader: io.NopCloser(bytes.NewReader(buf)),
	}
	require.NoError(t, r.Open(context.Background(), raw))

	blocks := collectBlocks(t, r)
	require.Len(t, blocks, 3)

	// The first block has a KLFAnnotation carrying structured runs.
	ann, ok := model.AnnoAs[*KLFAnnotation](blocks[0], AnnotationType)
	require.True(t, ok)
	assert.NotEmpty(t, ann.Source)
	assert.Equal(t, "files-heading", blocks[0].ID)
}

func TestWriterRoundTripKLF(t *testing.T) {
	// Build → read → write → read again; all three blocks must be
	// present with their KLFAnnotations intact after the round trip.
	inDoc := makeKLFFile()
	inBuf, err := klf.Marshal(inDoc)
	require.NoError(t, err)

	r := NewReader()
	require.NoError(t, r.Open(context.Background(), &model.RawDocument{URI: "in.klf", Reader: io.NopCloser(bytes.NewReader(inBuf))}))
	blocks := collectBlocks(t, r)
	require.Len(t, blocks, 3)

	// Drive a writer with the blocks.
	w := NewWriter()
	var sink bytes.Buffer
	require.NoError(t, w.SetOutputWriter(&sink))

	ch := make(chan *model.Part, len(blocks)+2)
	for _, b := range blocks {
		ch <- &model.Part{Type: model.PartBlock, Resource: b}
	}
	close(ch)
	require.NoError(t, w.Write(context.Background(), ch))
	require.NoError(t, w.Close())

	// Re-parse the emitted JSON.
	roundTrip, err := klf.Unmarshal(sink.Bytes())
	require.NoError(t, err)
	require.Len(t, roundTrip.Documents, 1)
	require.Len(t, roundTrip.Documents[0].Blocks, 3)

	// Structured content preserved.
	assert.Equal(t, "files-heading", roundTrip.Documents[0].Blocks[0].ID)
	assert.NotEmpty(t, roundTrip.Documents[0].Blocks[0].Source)
	assert.Equal(t, klf.BlockTypeJSXElement, roundTrip.Documents[0].Blocks[0].Type)
}

func TestWriterPreservesStructuredTargetRuns(t *testing.T) {
	// Regression for #376: `materializeBlock` used to flatten the
	// target to a single TextRun via `mb.TargetText()`, dropping
	// every Ph / PcOpen / PcClose run a tool had placed via
	// `SetTargetRuns`. A tag-chip target with three Ph runs plus
	// two Text separators must round-trip with all five runs intact.
	inDoc := makeKLFFile()
	inBuf, err := klf.Marshal(inDoc)
	require.NoError(t, err)

	r := NewReader()
	require.NoError(t, r.Open(context.Background(), &model.RawDocument{
		URI:    "in.klf",
		Reader: io.NopCloser(bytes.NewReader(inBuf)),
	}))
	blocks := collectBlocks(t, r)
	require.NotEmpty(t, blocks)

	// Inject a structured target on every block: text + ph + text.
	for _, b := range blocks {
		b.SetTargetRuns("qps", []model.Run{
			{Text: &model.TextRun{Text: "[accented] "}},
			{Ph: &model.PlaceholderRun{ID: "1", Type: "jsx:var", Data: "{x}", Equiv: "x"}},
			{Text: &model.TextRun{Text: " tail"}},
		})
	}

	outPath := filepath.Join(t.TempDir(), "with-targets.klf")
	w := NewWriter()
	w.SetLocale("qps")
	require.NoError(t, w.SetOutput(outPath))
	ch := make(chan *model.Part, len(blocks)+2)
	for _, b := range blocks {
		ch <- &model.Part{Type: model.PartBlock, Resource: b}
	}
	close(ch)
	require.NoError(t, w.Write(context.Background(), ch))
	require.NoError(t, w.Close())

	data, err := os.ReadFile(outPath)
	require.NoError(t, err)
	file, err := klf.Unmarshal(data)
	require.NoError(t, err)
	require.NotEmpty(t, file.Documents)
	for _, d := range file.Documents {
		for _, block := range d.Blocks {
			target, ok := block.Targets["qps"]
			require.True(t, ok, "block %q missing qps target", block.ID)
			require.Len(t, target, 3, "block %q target runs flattened — should be text+ph+text", block.ID)
			require.NotNil(t, target[1].Ph, "block %q second run should be Ph", block.ID)
			assert.Equal(t, "x", target[1].Ph.Equiv)
		}
	}
}

func TestPreviewBuilder(t *testing.T) {
	doc := makeKLFFile()
	buf, err := klf.Marshal(doc)
	require.NoError(t, err)

	r := NewReader()
	require.NoError(t, r.Open(context.Background(), &model.RawDocument{URI: "inline.klf", Reader: io.NopCloser(bytes.NewReader(buf))}))
	blocks := collectBlocks(t, r)

	pb := NewPreviewBuilder()
	preview := pb.BuildBlockPreview(blocks[0])
	assert.Contains(t, preview, `<kat-block id="files-heading"`)
	assert.Contains(t, preview, "Files ")
}

func TestReaderSniffsKLFEnvelope(t *testing.T) {
	r := NewReader()
	sig := r.Signature()
	require.NotNil(t, sig.Sniff)
	// A .klf envelope.
	assert.True(t, sig.Sniff([]byte(`{"schemaVersion":"1.0","kind":"kapi-localization-format"}`)))
	// Random JSON isn't a match.
	assert.False(t, sig.Sniff([]byte(`{"foo":1}`)))
}

// ───────── helpers ─────────

func collectBlocks(t *testing.T, r *Reader) []*model.Block {
	t.Helper()
	var blocks []*model.Block
	ch := r.Read(context.Background())
	for res := range ch {
		require.NoError(t, res.Error)
		if res.Part == nil || res.Part.Type != model.PartBlock {
			continue
		}
		b, ok := res.Part.Resource.(*model.Block)
		require.True(t, ok)
		blocks = append(blocks, b)
	}
	return blocks
}

// makeKLFFile builds an in-memory .klf with the three canonical
// example blocks. This is the Go-side mirror of the TS fixtures in
// packages/kapi-format/examples.
func makeKLFFile() *klf.File {
	return &klf.File{
		SchemaVersion: klf.SchemaVersion,
		Kind:          klf.Kind,
		Generator:     klf.GeneratorInfo{ID: "@neokapi/kapi-format-examples", Version: "0.0.1"},
		Project:       klf.ProjectInfo{ID: "neokapi-kapi-format-examples", SourceLocale: "en"},
		Documents: []klf.Document{
			{
				ID:           "examples",
				DocumentType: klf.DocumentTypeJSX,
				Path:         "examples/all.tsx",
				Blocks: []klf.Block{
					*filesHeading(),
					*tagChip(),
					*shoppingCart(),
				},
			},
		},
	}
}

// ───────── fixture blocks (local copies — kept close to the test
// file so every package can assert against the same canonical
// shape without cross-package dependencies) ─────────

func filesHeading() *klf.Block {
	return &klf.Block{
		ID: "files-heading", Hash: "2xykvb", Translatable: true,
		Type: klf.BlockTypeJSXElement,
		Source: []klf.Run{
			{Text: &klf.TextRun{Text: "Files "}},
			{PcOpen: &klf.PcOpenRun{
				ID: "1", Type: "jsx:element", SubType: "span",
				Data:  `<span className="muted">`,
				Equiv: "muted", Disp: "span",
			}},
			{Text: &klf.TextRun{Text: "("}},
			{Ph: &klf.PlaceholderRun{
				ID: "2", Type: "jsx:var", SubType: "number",
				Data: "{count}", Equiv: "count", Disp: "count",
			}},
			{Text: &klf.TextRun{Text: " matched)"}},
			{PcClose: &klf.PcCloseRun{
				ID: "1", Type: "jsx:element", SubType: "span",
				Data: "</span>", Equiv: "muted",
			}},
		},
		Placeholders: []klf.Placeholder{
			{Name: "muted", Kind: klf.PlaceholderElement,
				SourceExpr: `<span className="muted">...</span>`, JSType: "ReactNode"},
			{Name: "count", Kind: klf.PlaceholderVariable,
				SourceExpr: "count", JSType: "number"},
		},
		Properties: klf.BlockProperties{
			File: "src/FilesHeading.tsx", Line: 4,
			Component: "FilesHeading", JSXPath: "FilesHeading > h2", Element: "h2",
		},
	}
}

func tagChip() *klf.Block {
	return &klf.Block{
		ID: "tag-chip", Hash: "2GcSuQ", Translatable: true,
		Type: klf.BlockTypeJSXElement,
		Source: []klf.Run{
			{Ph: &klf.PlaceholderRun{
				ID: "1", Type: "jsx:node", SubType: "logical-and",
				Data:  `index !== undefined && <span className="badge">{index}</span>`,
				Equiv: "badge", Disp: "⟨badge⟩",
			}},
			{Text: &klf.TextRun{Text: " "}},
			{Ph: &klf.PlaceholderRun{
				ID: "2", Type: "jsx:var", SubType: "string",
				Data: "{label}", Equiv: "label", Disp: "label",
			}},
			{Text: &klf.TextRun{Text: " "}},
			{Ph: &klf.PlaceholderRun{
				ID: "3", Type: "jsx:node", SubType: "logical-and",
				Data:  `!deletable && <span className="required">*</span>`,
				Equiv: "required", Disp: "⟨required⟩",
			}},
		},
		Placeholders: []klf.Placeholder{
			{Name: "badge", Kind: klf.PlaceholderNode,
				SourceExpr: `index !== undefined && <span className="badge">{index}</span>`,
				JSType:     "ReactNode", Optional: true},
			{Name: "label", Kind: klf.PlaceholderVariable, SourceExpr: "label", JSType: "string"},
			{Name: "required", Kind: klf.PlaceholderNode,
				SourceExpr: `!deletable && <span className="required">*</span>`,
				JSType:     "ReactNode", Optional: true},
		},
		Properties: klf.BlockProperties{
			File: "src/TagChip.tsx", Line: 3,
			Component: "TagChip", JSXPath: "TagChip > span[data-tag-chip]", Element: "span",
			LocNote: "Tag chip shown in the sidebar list of filters.",
		},
	}
}

func shoppingCart() *klf.Block {
	return &klf.Block{
		ID: "shopping-cart-plural", Hash: "9QpZ11", Translatable: true,
		Type: klf.BlockTypeJSXElement,
		Source: []klf.Run{
			{Plural: &klf.PluralRun{
				Pivot: "count",
				Forms: map[klf.PluralForm][]klf.Run{
					klf.PluralZero: {{Text: &klf.TextRun{Text: "Your cart is empty"}}},
					klf.PluralOne:  {{Text: &klf.TextRun{Text: "1 item in your cart"}}},
					klf.PluralOther: {
						{Ph: &klf.PlaceholderRun{
							ID: "1", Type: "jsx:var", SubType: "number",
							Data: "{count}", Equiv: "count", Disp: "count",
						}},
						{Text: &klf.TextRun{Text: " items in your cart"}},
					},
				},
			}},
		},
		Placeholders: []klf.Placeholder{
			{Name: "count", Kind: klf.PlaceholderICUPivot,
				SourceExpr: "items", JSType: "number"},
		},
		Properties: klf.BlockProperties{
			File: "src/ShoppingCart.tsx", Line: 4,
			Component: "ShoppingCart", JSXPath: "ShoppingCart > p > Plural", Element: "Plural",
		},
	}
}
