// Package jsx implements the DataFormatReader / DataFormatWriter
// pair for the Kapi Localization Format (.klf single-document JSON).
//
// The reader routes an input document through core/klf and emits
// one model.Block per canonical Block. The flattened source text
// lives on the model.Block; the full structured Run[] graph travels
// along in a KLFAnnotation so writers and tools can reconstruct the
// source without going back to disk.
//
// The writer reverses the process: it reads KLFAnnotation off each
// incoming block, reassembles the klf.File, and writes to the
// configured output path. If a block arrives without a KLFAnnotation
// (e.g. inserted by an intermediate tool) the writer synthesizes a
// minimal text-only Run sequence from the block's plain text so the
// file stays well-formed.
package jsx

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/klf"
	"github.com/neokapi/neokapi/core/model"
)

// AnnotationType is the discriminator key the KLFAnnotation
// registers under model.Block.Annotations. Consumers that
// understand KLF look it up by this key to read structured Runs.
const AnnotationType = "neokapi-klf-block"

// FormatName is the canonical registry ID this format registers
// under. It is the user-facing id surfaced by `kapi formats`, the
// `--format klf` flag, and the generated reference page. The legacy
// id "jsx" remains a back-compat alias (see FormatAlias and the
// registration in core/formats/register.go).
const FormatName = "klf"

// FormatAlias is the legacy registry ID this format also resolves
// under. `--format jsx` (and any older recipe / script referencing
// "jsx") keeps working: the alias is a name-only lookup that resolves
// to FormatName. It is NOT a detection signature, so auto-detection
// always reports the canonical "klf" id.
const FormatAlias = "jsx"

// Extensions this reader responds to.
var Extensions = []string{".klf"}

// MimeTypes advertised for this format.
var MimeTypes = []string{
	"application/vnd.neokapi.klf+json",
}

// KLFAnnotation travels alongside a model.Block and carries the
// structured Runs (source + per-locale targets), placeholders,
// properties, preview hints, and enclosing-document metadata the
// Phase-2 Run-first model will eventually absorb into the block
// itself. In Phase 1 this annotation is the handoff between reader,
// tools, and writer.
type KLFAnnotation struct {
	// Source runs copied verbatim from the .klf Block.
	Source []klf.Run
	// Targets maps locale → target runs copied verbatim from the
	// .klf Block (nil-safe for blocks without targets).
	Targets map[klf.LocaleID][]klf.Run
	// Placeholders carries the Block.placeholders list.
	Placeholders []klf.Placeholder
	// Properties carries translator-facing context (file,
	// component, element, jsxPath, line, locNote).
	Properties klf.BlockProperties
	// Preview carries optional preview hints.
	Preview *klf.BlockPreviewHints
	// Type carries the BlockType (jsx:element or jsx:attribute).
	Type klf.BlockType
	// Hash carries the content hash from the .klf Block.
	Hash string
	// DocumentID is the enclosing Document.ID in the .klf file.
	DocumentID string
	// DocumentPath is the enclosing Document.Path in the .klf file.
	DocumentPath string
}

// AnnotationType satisfies any.
func (a *KLFAnnotation) TypeName() string { return AnnotationType }

// Runs returns the source runs. Convenience for tools that want to
// walk a block's structured content without repeating the map
// lookup.
func (a *KLFAnnotation) Runs() []klf.Run { return a.Source }

// FlattenRuns walks a run sequence and returns the plain source
// text. Placeholder runs contribute their `equiv` wrapped in braces
// (matching neokapi's existing "{varname}" plain-text convention),
// paired codes contribute nothing (the text between the pairs is
// what matters), and plural / select forms are flattened using
// their 'other' branch if present.
func FlattenRuns(runs []klf.Run) string {
	var out strings.Builder
	flatten(&out, runs)
	return out.String()
}

func flatten(out *strings.Builder, runs []klf.Run) {
	for _, r := range runs {
		switch {
		case r.Text != nil:
			out.WriteString(r.Text.Text)
		case r.Ph != nil:
			out.WriteString("{")
			out.WriteString(r.Ph.Equiv)
			out.WriteString("}")
		case r.PcOpen != nil, r.PcClose != nil:
			// Paired codes carry structural framing; the text
			// between them is what flattens, so skip them here.
		case r.Sub != nil:
			out.WriteString("[")
			out.WriteString(r.Sub.Equiv)
			out.WriteString("]")
		case r.Plural != nil:
			forms := r.Plural.Forms
			if other, ok := forms[klf.PluralOther]; ok {
				flatten(out, other)
				continue
			}
			for _, form := range forms {
				flatten(out, form)
				break
			}
		case r.Select != nil:
			cases := r.Select.Cases
			if other, ok := cases["other"]; ok {
				flatten(out, other)
				continue
			}
			for _, c := range cases {
				flatten(out, c)
				break
			}
		}
	}
}

// ───────── Reader ─────────

// Reader implements format.DataFormatReader for .klf.
type Reader struct {
	format.BaseFormatReader
}

// NewReader creates a new KLF reader.
func NewReader() *Reader {
	return &Reader{
		BaseFormatReader: format.BaseFormatReader{
			FormatName:        FormatName,
			FormatDisplayName: "Kapi Localization Format (KLF)",
			FormatMimeType:    MimeTypes[0],
			FormatExtensions:  Extensions,
		},
	}
}

// Signature returns detection metadata — .klf files are JSON
// bearing the canonical `kapi-localization-format` kind marker.
func (r *Reader) Signature() format.FormatSignature {
	return format.FormatSignature{
		MIMETypes:  MimeTypes,
		Extensions: Extensions,
		Sniff: func(data []byte) bool {
			return bytes.Contains(data, []byte(`"kapi-localization-format"`))
		},
	}
}

// Open opens a RawDocument for reading.
func (r *Reader) Open(ctx context.Context, doc *model.RawDocument) error {
	if doc == nil || doc.Reader == nil {
		return errors.New("klf: nil document or reader")
	}
	r.Doc = doc
	return nil
}

// Read returns a channel of PartResults.
func (r *Reader) Read(ctx context.Context) <-chan model.PartResult {
	ch := make(chan model.PartResult, 64)
	go func() {
		defer close(ch)
		r.stream(ctx, ch)
	}()
	return ch
}

// Close releases resources.
func (r *Reader) Close() error { return nil }

// Config returns the configuration. This reader has no tunables
// beyond what BaseFormatReader provides.
func (r *Reader) Config() format.DataFormatConfig { return r.Cfg }

func (r *Reader) stream(ctx context.Context, ch chan<- model.PartResult) {
	body, err := io.ReadAll(r.Doc.Reader)
	if err != nil {
		ch <- model.PartResult{Error: fmt.Errorf("klf: read body: %w", err)}
		return
	}

	locale := r.Doc.SourceLocale
	if locale.IsEmpty() {
		locale = model.LocaleEnglish
	}
	layerID := "doc1"
	layer := &model.Layer{
		ID:       layerID,
		Name:     r.Doc.URI,
		Format:   FormatName,
		Locale:   locale,
		Encoding: r.Doc.Encoding,
		MimeType: MimeTypes[0],
	}
	if !r.emit(ctx, ch, &model.Part{Type: model.PartLayerStart, Resource: layer}) {
		return
	}

	r.streamKLF(ctx, ch, body)

	r.emit(ctx, ch, &model.Part{Type: model.PartLayerEnd, Resource: layer})
}

func (r *Reader) emit(ctx context.Context, ch chan<- model.PartResult, part *model.Part) bool {
	select {
	case <-ctx.Done():
		return false
	case ch <- model.PartResult{Part: part}:
		return true
	}
}

func (r *Reader) streamKLF(ctx context.Context, ch chan<- model.PartResult, data []byte) {
	file, err := klf.Unmarshal(data)
	if err != nil {
		ch <- model.PartResult{Error: fmt.Errorf("klf: parse klf: %w", err)}
		return
	}
	for _, doc := range file.Documents {
		for i := range doc.Blocks {
			block := toModelBlock(&doc, &doc.Blocks[i])
			if !r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block}) {
				return
			}
		}
	}
}

// toModelBlock lifts a klf.Block into a model.Block with Runs as
// first-class content. The KLFAnnotation overlay carries extra
// wire-level metadata (Hash, DocumentID, DocumentPath, Placeholders,
// Preview) that the legacy model.Block doesn't have first-class
// fields for; writers read it back to reconstruct the archive.
func toModelBlock(doc *klf.Document, b *klf.Block) *model.Block {
	mb := model.NewRunsBlock(b.ID, cloneRuns(b.Source))
	mb.Translatable = b.Translatable
	mb.Type = string(b.Type)
	if b.Properties.File != "" {
		mb.Properties["file"] = b.Properties.File
	}
	if b.Properties.Component != "" {
		mb.Properties["component"] = b.Properties.Component
	}
	if b.Properties.Element != "" {
		mb.Properties["element"] = b.Properties.Element
	}
	if b.Properties.JSXPath != "" {
		mb.Properties["jsxPath"] = b.Properties.JSXPath
	}
	if b.Properties.LocNote != "" {
		mb.Properties["locNote"] = b.Properties.LocNote
	}
	for locale, runs := range b.Targets {
		mb.SetTargetRuns(model.LocaleID(locale), cloneRuns(runs))
	}
	ann := &KLFAnnotation{
		Source:       cloneRuns(b.Source),
		Targets:      cloneTargets(b.Targets),
		Placeholders: append([]klf.Placeholder(nil), b.Placeholders...),
		Properties:   b.Properties,
		Preview:      b.Preview,
		Type:         b.Type,
		Hash:         b.Hash,
		DocumentID:   doc.ID,
		DocumentPath: doc.Path,
	}
	mb.SetAnno(AnnotationType, ann)
	return mb
}

func cloneRuns(runs []klf.Run) []klf.Run {
	out := make([]klf.Run, len(runs))
	copy(out, runs)
	return out
}

func cloneTargets(in map[klf.LocaleID][]klf.Run) map[klf.LocaleID][]klf.Run {
	if in == nil {
		return nil
	}
	out := make(map[klf.LocaleID][]klf.Run, len(in))
	for k, v := range in {
		out[k] = append([]klf.Run(nil), v...)
	}
	return out
}

// ───────── Writer ─────────

// Writer implements format.DataFormatWriter for .klf.
type Writer struct {
	outPath string
	out     io.Writer
	outFile *os.File
	locale  model.LocaleID

	generator klf.GeneratorInfo
	project   klf.ProjectInfo

	// Accumulated blocks grouped by document id/path.
	pending map[string]*pendingDoc
	order   []string
}

type pendingDoc struct {
	id     string
	path   string
	blocks []klf.Block
}

// NewWriter creates a new KLF writer.
func NewWriter() *Writer {
	return &Writer{
		generator: klf.GeneratorInfo{ID: "neokapi", Version: "1.0"},
		project:   klf.ProjectInfo{ID: "neokapi-output", SourceLocale: "en"},
		pending:   make(map[string]*pendingDoc),
	}
}

// Name returns the format name.
func (w *Writer) Name() string { return FormatName }

// SetOutput configures an output path.
func (w *Writer) SetOutput(path string) error {
	w.outPath = path
	return nil
}

// SetOutputWriter configures an io.Writer as output.
func (w *Writer) SetOutputWriter(out io.Writer) error {
	w.out = out
	return nil
}

// SetLocale sets the target locale.
func (w *Writer) SetLocale(locale model.LocaleID) { w.locale = locale }

// SetEncoding is a no-op — .klf is always UTF-8.
func (w *Writer) SetEncoding(_ string) {}

// Write consumes blocks from the channel and accumulates them.
// Close() flushes them to the configured output.
func (w *Writer) Write(ctx context.Context, parts <-chan *model.Part) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case part, ok := <-parts:
			if !ok {
				return nil
			}
			if err := w.handlePart(part); err != nil {
				return err
			}
		}
	}
}

func (w *Writer) handlePart(part *model.Part) error {
	if part == nil || part.Type != model.PartBlock {
		return nil
	}
	mblock, ok := part.Resource.(*model.Block)
	if !ok || mblock == nil {
		return nil
	}
	block, docID, docPath := w.materializeBlock(mblock)
	pd, ok := w.pending[docID]
	if !ok {
		pd = &pendingDoc{id: docID, path: docPath}
		w.pending[docID] = pd
		w.order = append(w.order, docID)
	}
	pd.blocks = append(pd.blocks, block)
	return nil
}

// materializeBlock reconstructs a klf.Block from a model.Block. If
// the source block carries a KLFAnnotation (the round-trip case)
// the structured Runs are preserved verbatim; a target is written
// in as a text-only run sequence at the writer's locale when the
// annotation already has a target for that locale with updated
// content. If there is no annotation (the synthesized case) the
// writer emits a minimal text-only block so the archive is still
// well-formed.
func (w *Writer) materializeBlock(mb *model.Block) (klf.Block, string, string) {
	if annRaw, ok := mb.Anno(AnnotationType); ok {
		if ann, ok := annRaw.(*KLFAnnotation); ok && ann != nil {
			b := klf.Block{
				ID:           mb.ID,
				Hash:         ann.Hash,
				Translatable: mb.Translatable,
				Type:         ann.Type,
				Source:       cloneRuns(ann.Source),
				Targets:      cloneTargets(ann.Targets),
				Placeholders: append([]klf.Placeholder(nil), ann.Placeholders...),
				Properties:   ann.Properties,
				Preview:      ann.Preview,
			}
			if w.locale != "" && mb.HasTarget(w.locale) {
				if b.Targets == nil {
					b.Targets = make(map[klf.LocaleID][]klf.Run)
				}
				// Update only if the annotation didn't already carry a
				// target for this locale — preserves upstream structure.
				// Carry the model.Block's structured Runs across so
				// placeholders / paired codes survive tools that populate
				// targets via SetTargetRuns (e.g. pseudo-translate).
				if _, hasExisting := b.Targets[klf.LocaleID(w.locale)]; !hasExisting {
					b.Targets[klf.LocaleID(w.locale)] = runsFromModel(mb.TargetRuns(w.locale))
				}
			}
			return b, ann.DocumentID, ann.DocumentPath
		}
	}
	// Synthesized fallback: minimal text-only block from whatever
	// content the model.Block carries.
	b := klf.Block{
		ID:           mb.ID,
		Translatable: mb.Translatable,
		Type:         klf.BlockTypeJSXElement,
		Source:       []klf.Run{{Text: &klf.TextRun{Text: mb.SourceText()}}},
		Properties:   klf.BlockProperties{File: mb.Properties["file"], Component: mb.Properties["component"], Element: mb.Properties["element"]},
	}
	if w.locale != "" && mb.HasTarget(w.locale) {
		b.Targets = map[klf.LocaleID][]klf.Run{
			klf.LocaleID(w.locale): runsFromModel(mb.TargetRuns(w.locale)),
		}
	}
	return b, "synthesized", "synthesized"
}

// runsFromModel is the model.Run → klf.Run adapter used when a
// tool populated block.Targets with structured Runs. Runs are
// preserved verbatim, including placeholders and paired codes.
func runsFromModel(runs []model.Run) []klf.Run {
	if len(runs) == 0 {
		return nil
	}
	// klf.Run is a type alias for model.Run today; the cast is
	// effectively a no-op but keeps the site explicit and lets us
	// insert a deep clone or shape mapping here if the types ever
	// diverge.
	out := make([]klf.Run, len(runs))
	copy(out, runs)
	return out
}

// Close flushes the accumulated blocks to the configured output.
// Emits .klf (JSON) either to the configured io.Writer or the
// configured output path.
func (w *Writer) Close() error {
	file := w.buildKLF()
	if w.outFile != nil {
		defer func() {
			_ = w.outFile.Close()
			w.outFile = nil
		}()
	}
	if w.out != nil {
		return klf.Encode(w.out, file)
	}
	if w.outPath == "" {
		return nil
	}
	return w.writeKLF(file)
}

func (w *Writer) buildKLF() *klf.File {
	file := &klf.File{
		SchemaVersion: klf.SchemaVersion,
		Kind:          klf.Kind,
		Generator:     w.generator,
		Project:       w.project,
	}
	for _, id := range w.order {
		pd := w.pending[id]
		file.Documents = append(file.Documents, klf.Document{
			ID:           pd.id,
			DocumentType: klf.DocumentTypeJSX,
			Path:         pd.path,
			Blocks:       pd.blocks,
		})
	}
	return file
}

func (w *Writer) writeKLF(file *klf.File) error {
	data, err := klf.Marshal(file)
	if err != nil {
		return err
	}
	return os.WriteFile(w.outPath, data, 0o644)
}

// ───────── PreviewBuilder ─────────

// PreviewBuilder produces the <kat-block> preview HTML for a block
// by walking its structured Runs through the KLF reference
// renderer. Consumers that wire this into the framework's existing
// preview pipeline get JSX support with no additional code.
type PreviewBuilder struct {
	vocab klf.VocabularyLookup
}

// NewPreviewBuilder creates a new preview builder. Vocab defaults
// to the built-in JSX vocabulary if nil.
func NewPreviewBuilder() *PreviewBuilder {
	return &PreviewBuilder{vocab: klf.DefaultJSXVocabulary()}
}

// BuildBlockPreview returns the <kat-block> HTML preview for a
// model.Block. Requires the block to carry a KLFAnnotation; falls
// back to the flattened text wrapped in a <kat-block> envelope
// otherwise so downstream tools never see an empty preview.
func (p *PreviewBuilder) BuildBlockPreview(mb *model.Block) string {
	if mb == nil {
		return ""
	}
	av, _ := mb.Anno(AnnotationType)
	ann, ok := av.(*KLFAnnotation)
	if !ok || ann == nil {
		escaped := jsonEscapeAttr(mb.ID)
		return fmt.Sprintf(`<kat-block id=%s data-type="text">%s</kat-block>`,
			escaped, htmlEscape(mb.SourceText()))
	}
	b := &klf.Block{
		ID:     mb.ID,
		Type:   ann.Type,
		Source: ann.Source,
	}
	return klf.RenderBlockHTML(b, p.vocab)
}

func jsonEscapeAttr(s string) string {
	buf, _ := json.Marshal(s)
	return string(buf)
}

func htmlEscape(s string) string {
	var out strings.Builder
	for _, r := range s {
		switch r {
		case '&':
			out.WriteString("&amp;")
		case '<':
			out.WriteString("&lt;")
		case '>':
			out.WriteString("&gt;")
		case '"':
			out.WriteString("&quot;")
		default:
			out.WriteRune(r)
		}
	}
	return out.String()
}
