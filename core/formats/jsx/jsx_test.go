package jsx

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/kbf"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFlattenRuns(t *testing.T) {
	runs := []kbf.Run{
		{Text: &kbf.TextRun{Text: "Files "}},
		{PcOpen: &kbf.PcOpenRun{ID: "1", Type: "jsx:element", SubType: "span", Equiv: "muted"}},
		{Text: &kbf.TextRun{Text: "("}},
		{Ph: &kbf.PlaceholderRun{ID: "2", Type: "jsx:var", Equiv: "count"}},
		{Text: &kbf.TextRun{Text: " matched)"}},
		{PcClose: &kbf.PcCloseRun{ID: "1", Type: "jsx:element", SubType: "span"}},
	}
	got := FlattenRuns(runs)
	assert.Equal(t, "Files ({count} matched)", got)
}

func TestFlattenRunsPlural(t *testing.T) {
	runs := []kbf.Run{
		{Plural: &kbf.PluralRun{
			Pivot: "count",
			Forms: map[kbf.PluralForm][]kbf.Run{
				kbf.PluralOne:   {{Text: &kbf.TextRun{Text: "1 item"}}},
				kbf.PluralOther: {{Text: &kbf.TextRun{Text: "many items"}}},
			},
		}},
	}
	got := FlattenRuns(runs)
	assert.Equal(t, "many items", got, "plural flattens to the 'other' branch")
}

func TestReaderReadsKBF(t *testing.T) {
	// Build a .kbf.json in memory and feed it through the reader.
	doc := makeKBFFile()
	buf, err := kbf.Marshal(doc)
	require.NoError(t, err)

	r := NewReader()
	raw := &model.RawDocument{
		URI:    "inline.kbf.json",
		Reader: io.NopCloser(bytes.NewReader(buf)),
	}
	require.NoError(t, r.Open(context.Background(), raw))

	blocks := collectBlocks(t, r)
	require.Len(t, blocks, 3)

	// The first block has a KBFAnnotation carrying structured runs.
	ann, ok := model.AnnoAs[*KBFAnnotation](blocks[0], AnnotationType)
	require.True(t, ok)
	assert.NotEmpty(t, ann.Source)
	assert.Equal(t, "files-heading", blocks[0].ID)
}

func TestWriterRoundTripKBF(t *testing.T) {
	// Build → read → write → read again; all three blocks must be
	// present with their KBFAnnotations intact after the round trip.
	inDoc := makeKBFFile()
	inBuf, err := kbf.Marshal(inDoc)
	require.NoError(t, err)

	r := NewReader()
	require.NoError(t, r.Open(context.Background(), &model.RawDocument{URI: "in.kbf.json", Reader: io.NopCloser(bytes.NewReader(inBuf))}))
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
	roundTrip, err := kbf.Unmarshal(sink.Bytes())
	require.NoError(t, err)
	require.Len(t, roundTrip.Documents, 1)
	require.Len(t, roundTrip.Documents[0].Blocks, 3)

	// Structured content preserved.
	assert.Equal(t, "files-heading", roundTrip.Documents[0].Blocks[0].ID)
	assert.NotEmpty(t, roundTrip.Documents[0].Blocks[0].Source)
	assert.Equal(t, kbf.BlockTypeJSXElement, roundTrip.Documents[0].Blocks[0].Type)
}

func TestWriterPreservesStructuredTargetRuns(t *testing.T) {
	// Regression for #376: `materializeBlock` used to flatten the
	// target to a single TextRun via `mb.TargetText()`, dropping
	// every Ph / PcOpen / PcClose run a tool had placed via
	// `SetTargetRuns`. A tag-chip target with three Ph runs plus
	// two Text separators must round-trip with all five runs intact.
	inDoc := makeKBFFile()
	inBuf, err := kbf.Marshal(inDoc)
	require.NoError(t, err)

	r := NewReader()
	require.NoError(t, r.Open(context.Background(), &model.RawDocument{
		URI:    "in.kbf.json",
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

	outPath := filepath.Join(t.TempDir(), "with-targets.kbf.json")
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
	file, err := kbf.Unmarshal(data)
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
	doc := makeKBFFile()
	buf, err := kbf.Marshal(doc)
	require.NoError(t, err)

	r := NewReader()
	require.NoError(t, r.Open(context.Background(), &model.RawDocument{URI: "inline.kbf.json", Reader: io.NopCloser(bytes.NewReader(buf))}))
	blocks := collectBlocks(t, r)

	pb := NewPreviewBuilder()
	preview := pb.BuildBlockPreview(blocks[0])
	assert.Contains(t, preview, `<kat-block id="files-heading"`)
	assert.Contains(t, preview, "Files ")
}

// A bundle is a catalog of a React tree, and the document preview says so: the
// blocks are grouped by the component they came from, each one labelled with
// the element it sits in, and every run rendered — a variable is a chip, not a
// gap. Falling through to the generic listing dropped all three: the component,
// the element, and (because a placeholder run carries no text) the variable.
func TestBuildPreviewRendersTheComponentTree(t *testing.T) {
	doc := makeKBFFile()
	buf, err := kbf.Marshal(doc)
	require.NoError(t, err)

	r := NewReader()
	require.NoError(t, r.Open(context.Background(), &model.RawDocument{URI: "inline.kbf.json", Reader: io.NopCloser(bytes.NewReader(buf))}))

	parts := collectParts(t, r)

	preview := r.BuildPreview(parts)

	// Every block is addressable, so a click selects it and a target swap can
	// replace it — the contract the preview host works to.
	assert.Contains(t, preview, `<kat-block id="files-heading"`)
	assert.Contains(t, preview, `<kat-block id="tag-chip"`)

	// The component each block was extracted from, stated once above its blocks.
	assert.Contains(t, preview, "FilesHeading")
	assert.Contains(t, preview, "TagChip")

	// The element it sits in, and where it came from.
	assert.Contains(t, preview, "&lt;h2&gt;")
	assert.Contains(t, preview, "src/FilesHeading.tsx:4")

	// The variable renders as itself, not as the hole a text listing leaves.
	assert.Contains(t, preview, `class="neokapi-var"`)
	assert.Contains(t, preview, "count")
	assert.Contains(t, preview, "Files ")
}

// A block the reader could not annotate still appears: the document preview
// leans on the block-level builder, which falls back to the flattened text
// rather than dropping the block.
func TestBuildPreviewIsTheSameBlockRenderingAsTheBlockBuilder(t *testing.T) {
	doc := makeKBFFile()
	buf, err := kbf.Marshal(doc)
	require.NoError(t, err)

	r := NewReader()
	require.NoError(t, r.Open(context.Background(), &model.RawDocument{URI: "inline.kbf.json", Reader: io.NopCloser(bytes.NewReader(buf))}))
	blocks := collectBlocks(t, r)

	one := NewPreviewBuilder().BuildBlockPreview(blocks[0])
	require.NotEmpty(t, one)

	r2 := NewReader()
	require.NoError(t, r2.Open(context.Background(), &model.RawDocument{URI: "inline.kbf.json", Reader: io.NopCloser(bytes.NewReader(buf))}))
	parts := collectParts(t, r2)
	assert.Contains(t, r2.BuildPreview(parts), one,
		"a block reads identically whether the host asked for it or for the document")
}

func TestReaderSniffsKBFEnvelope(t *testing.T) {
	r := NewReader()
	sig := r.Signature()
	require.NotNil(t, sig.Sniff)
	// A .kbf.json envelope.
	assert.True(t, sig.Sniff([]byte(`{"schemaVersion":"1.0","kind":"kapi-bundle"}`)))
	// Random JSON isn't a match.
	assert.False(t, sig.Sniff([]byte(`{"foo":1}`)))
}

// The sibling envelopes are JSON with a `kind` marker too, so detection has to
// key on this format's own marker rather than on "looks like a kapi envelope".
func TestReaderDoesNotSniffSiblingEnvelopes(t *testing.T) {
	r := NewReader()
	sig := r.Signature()
	require.NotNil(t, sig.Sniff)
	assert.False(t, sig.Sniff([]byte(`{"schemaVersion":"1.0","kind":"kapi-memory"}`)),
		"a content-memory bundle is not a document bundle")
	assert.False(t, sig.Sniff([]byte(`{"schemaVersion":"1.0","kind":"kapi-terms"}`)),
		"a terms bundle is not a document bundle")
}

// ───────── helpers ─────────

// collectParts drains a reader into the part slice a PreviewBuilder is given.
func collectParts(t *testing.T, r *Reader) []*model.Part {
	t.Helper()
	var parts []*model.Part
	for res := range r.Read(context.Background()) {
		require.NoError(t, res.Error)
		if res.Part != nil {
			parts = append(parts, res.Part)
		}
	}
	return parts
}

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

// makeKBFFile builds an in-memory .kbf.json with the three canonical
// example blocks. This is the Go-side mirror of the TS fixtures in
// packages/kapi-format/examples.
func makeKBFFile() *kbf.File {
	return &kbf.File{
		SchemaVersion: kbf.SchemaVersion,
		Kind:          kbf.Kind,
		Generator:     kbf.GeneratorInfo{ID: "@neokapi/kapi-format-examples", Version: "0.0.1"},
		Project:       kbf.ProjectInfo{ID: "neokapi-kapi-format-examples", SourceLocale: "en"},
		Documents: []kbf.Document{
			{
				ID:           "examples",
				DocumentType: kbf.DocumentTypeJSX,
				Path:         "examples/all.tsx",
				Blocks: []kbf.Block{
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

func filesHeading() *kbf.Block {
	return &kbf.Block{
		ID: "files-heading", Hash: "2xykvb", Translatable: true,
		Type: kbf.BlockTypeJSXElement,
		Source: []kbf.Run{
			{Text: &kbf.TextRun{Text: "Files "}},
			{PcOpen: &kbf.PcOpenRun{
				ID: "1", Type: "jsx:element", SubType: "span",
				Data:  `<span className="muted">`,
				Equiv: "muted", Disp: "span",
			}},
			{Text: &kbf.TextRun{Text: "("}},
			{Ph: &kbf.PlaceholderRun{
				ID: "2", Type: "jsx:var", SubType: "number",
				Data: "{count}", Equiv: "count", Disp: "count",
			}},
			{Text: &kbf.TextRun{Text: " matched)"}},
			{PcClose: &kbf.PcCloseRun{
				ID: "1", Type: "jsx:element", SubType: "span",
				Data: "</span>", Equiv: "muted",
			}},
		},
		Placeholders: []kbf.Placeholder{
			{Name: "muted", Kind: kbf.PlaceholderElement,
				SourceExpr: `<span className="muted">...</span>`, JSType: "ReactNode"},
			{Name: "count", Kind: kbf.PlaceholderVariable,
				SourceExpr: "count", JSType: "number"},
		},
		Properties: kbf.BlockProperties{
			File: "src/FilesHeading.tsx", Line: 4,
			Component: "FilesHeading", JSXPath: "FilesHeading > h2", Element: "h2",
		},
	}
}

func tagChip() *kbf.Block {
	return &kbf.Block{
		ID: "tag-chip", Hash: "2GcSuQ", Translatable: true,
		Type: kbf.BlockTypeJSXElement,
		Source: []kbf.Run{
			{Ph: &kbf.PlaceholderRun{
				ID: "1", Type: "jsx:node", SubType: "logical-and",
				Data:  `index !== undefined && <span className="badge">{index}</span>`,
				Equiv: "badge", Disp: "⟨badge⟩",
			}},
			{Text: &kbf.TextRun{Text: " "}},
			{Ph: &kbf.PlaceholderRun{
				ID: "2", Type: "jsx:var", SubType: "string",
				Data: "{label}", Equiv: "label", Disp: "label",
			}},
			{Text: &kbf.TextRun{Text: " "}},
			{Ph: &kbf.PlaceholderRun{
				ID: "3", Type: "jsx:node", SubType: "logical-and",
				Data:  `!deletable && <span className="required">*</span>`,
				Equiv: "required", Disp: "⟨required⟩",
			}},
		},
		Placeholders: []kbf.Placeholder{
			{Name: "badge", Kind: kbf.PlaceholderNode,
				SourceExpr: `index !== undefined && <span className="badge">{index}</span>`,
				JSType:     "ReactNode", Optional: true},
			{Name: "label", Kind: kbf.PlaceholderVariable, SourceExpr: "label", JSType: "string"},
			{Name: "required", Kind: kbf.PlaceholderNode,
				SourceExpr: `!deletable && <span className="required">*</span>`,
				JSType:     "ReactNode", Optional: true},
		},
		Properties: kbf.BlockProperties{
			File: "src/TagChip.tsx", Line: 3,
			Component: "TagChip", JSXPath: "TagChip > span[data-tag-chip]", Element: "span",
			LocNote: "Tag chip shown in the sidebar list of filters.",
		},
	}
}

func shoppingCart() *kbf.Block {
	return &kbf.Block{
		ID: "shopping-cart-plural", Hash: "9QpZ11", Translatable: true,
		Type: kbf.BlockTypeJSXElement,
		Source: []kbf.Run{
			{Plural: &kbf.PluralRun{
				Pivot: "count",
				Forms: map[kbf.PluralForm][]kbf.Run{
					kbf.PluralZero: {{Text: &kbf.TextRun{Text: "Your cart is empty"}}},
					kbf.PluralOne:  {{Text: &kbf.TextRun{Text: "1 item in your cart"}}},
					kbf.PluralOther: {
						{Ph: &kbf.PlaceholderRun{
							ID: "1", Type: "jsx:var", SubType: "number",
							Data: "{count}", Equiv: "count", Disp: "count",
						}},
						{Text: &kbf.TextRun{Text: " items in your cart"}},
					},
				},
			}},
		},
		Placeholders: []kbf.Placeholder{
			{Name: "count", Kind: kbf.PlaceholderICUPivot,
				SourceExpr: "items", JSType: "number"},
		},
		Properties: kbf.BlockProperties{
			File: "src/ShoppingCart.tsx", Line: 4,
			Component: "ShoppingCart", JSXPath: "ShoppingCart > p > Plural", Element: "Plural",
		},
	}
}
