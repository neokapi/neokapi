// Package jsx implements the DataFormatReader / DataFormatWriter
// pair for the Kapi Bundle Format (.kbf.json single-document JSON).
//
// The reader routes an input document through core/kbf and emits
// one model.Block per canonical Block, with every edition the bundle
// holds filed on it: the source as the edition the block was read in,
// each other edition under its key, the unlabelled one as the block's
// translation under no language. The source runs as read travel
// along in a KBFAnnotation, with the wire-level metadata the model has
// no field for, so a writer reproduces an untouched block verbatim.
//
// The writer reverses the process: it reads KBFAnnotation off each
// incoming block, reassembles the kbf.File, and writes to the
// configured output path. If a block arrives without a KBFAnnotation
// (e.g. inserted by an intermediate tool) the writer synthesizes a
// minimal text-only source from the block's plain text so the file
// stays well-formed.
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
	"github.com/neokapi/neokapi/core/kbf"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/safeio"
)

// AnnotationType is the discriminator key the KBFAnnotation
// registers under model.Block.Annotations. Consumers that
// understand KBF look it up by this key to read structured Runs.
const AnnotationType = "neokapi-kbf-block"

// FormatName is the registry ID this format registers under. It is
// the user-facing id surfaced by `kapi formats`, the `--format kbf`
// flag, and the generated reference page.
const FormatName = "kbf"

// Extensions this reader responds to, the current one first so it is the
// default an output name is derived from.
//
// [format.KBFExt] is a compound suffix (".kbf.json"), so the registry has to
// resolve it with [format.Ext] rather than path/filepath.Ext — the latter sees
// only ".json" and hands every bundle to the plain JSON reader.
// [format.KBFExtI18nReact] is the plain ".klf" suffix @neokapi/i18n-react
// 1.2.3 writes; the payload under it is a bundle, so this reader takes it.
var Extensions = []string{format.KBFExt, format.KBFExtI18nReact}

// MimeTypes advertised for this format.
var MimeTypes = []string{
	"application/vnd.neokapi.kbf+json",
}

// KBFAnnotation travels alongside a model.Block and carries what the
// bundle says about the block that the model has no field for: the
// source runs as read, placeholders, properties, preview hints, and
// the enclosing document. Every edition lives on the model.Block
// itself; this annotation is the handoff between reader, tools, and
// writer for the rest.
type KBFAnnotation struct {
	// Source is the runs of the edition the block was read in, copied
	// verbatim from the .kbf.json Block. The writer emits them while they
	// still carry the block's current source, so an untouched block keeps
	// its structure byte for byte.
	Source []kbf.Run
	// Placeholders carries the Block.placeholders list.
	Placeholders []kbf.Placeholder
	// Properties carries translator-facing context (file,
	// component, element, jsxPath, line, locNote).
	Properties kbf.BlockProperties
	// Preview carries optional preview hints.
	Preview *kbf.BlockPreviewHints
	// Type carries the BlockType (jsx:element or jsx:attribute).
	Type kbf.BlockType
	// Hash carries the content hash from the .kbf.json Block.
	Hash string
	// DocumentID is the enclosing Document.ID in the .kbf.json file.
	DocumentID string
	// DocumentPath is the enclosing Document.Path in the .kbf.json file.
	DocumentPath string
}

// AnnotationType satisfies any.
func (a *KBFAnnotation) TypeName() string { return AnnotationType }

func init() {
	// Register KBFAnnotation so the wire and store layers can rehydrate it from
	// its type name — most importantly the streaming document cache (host
	// docCache), which serializes each block's annotations and drops any whose
	// type is unregistered on replay. Unregistered, a block served from cache
	// silently loses its KBFAnnotation, and with it the block's content hash,
	// placeholders, and preview — so a reconstructed .kbf.json target comes out with
	// empty hashes (breaking downstream identity, e.g. neokapi-i18n compile). This
	// surfaced non-deterministically under `kapi up`, whose concurrent per-locale
	// converge workers share the cache and race on the same cached source.
	model.RegisterPayload(AnnotationType, func() model.Payload { return &KBFAnnotation{} })
}

// Runs returns the source runs. Convenience for tools that want to
// walk a block's structured content without repeating the map
// lookup.
func (a *KBFAnnotation) Runs() []kbf.Run { return a.Source }

// FlattenRuns walks a run sequence and returns the plain source
// text. Placeholder runs contribute their `equiv` wrapped in braces
// (matching neokapi's existing "{varname}" plain-text convention),
// paired codes contribute nothing (the text between the pairs is
// what matters), and plural / select forms are flattened using
// their 'other' branch if present.
func FlattenRuns(runs []kbf.Run) string {
	var out strings.Builder
	flatten(&out, runs)
	return out.String()
}

func flatten(out *strings.Builder, runs []kbf.Run) {
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
			if other, ok := forms[kbf.PluralOther]; ok {
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

// Reader implements format.DataFormatReader for .kbf.json.
type Reader struct {
	format.BaseFormatReader
}

// NewReader creates a new KBF reader.
func NewReader() *Reader {
	return &Reader{
		FormatName:        FormatName,
		FormatDisplayName: "Kapi Bundle Format (KBF)",
		FormatMimeType:    MimeTypes[0],
		FormatExtensions:  Extensions,
	}
}

// Signature returns detection metadata. A bundle is JSON bearing one of the
// root kind markers [kbf.ReadableKinds] names.
func (r *Reader) Signature() format.FormatSignature {
	return format.FormatSignature{
		MIMETypes:  MimeTypes,
		Extensions: Extensions,
		Sniff:      Sniff,
	}
}

// Sniff reports whether data carries a bundle's root kind marker. It is the
// one detector both this reader's signature and the registry entry use, so a
// kind the reader accepts can never be a kind detection rejects.
func Sniff(data []byte) bool {
	for _, kind := range kbf.ReadableKinds {
		if bytes.Contains(data, []byte(`"`+kind+`"`)) {
			return true
		}
	}
	return false
}

// Open opens a RawDocument for reading.
func (r *Reader) Open(ctx context.Context, doc *model.RawDocument) error {
	if doc == nil || doc.Reader == nil {
		return errors.New("kbf: nil document or reader")
	}
	r.Doc = doc
	return nil
}

// Read returns a channel of PartResults.
func (r *Reader) Read(ctx context.Context) <-chan model.PartResult {
	return format.StreamParts(ctx, func(ctx context.Context, ch chan<- model.PartResult) error {
		r.stream(ctx, ch)
		return nil
	})
}

// Close releases resources.
func (r *Reader) Close() error { return nil }

// Config returns the configuration. This reader has no tunables
// beyond what BaseFormatReader provides.
func (r *Reader) Config() format.DataFormatConfig { return r.Cfg }

func (r *Reader) stream(ctx context.Context, ch chan<- model.PartResult) {
	// Bound the whole-input read with the shared safeio byte budget so an
	// unbounded/oversized stream fails with a typed error (identical limit
	// across CLI/server/WASM — see core/safeio).
	body, err := io.ReadAll(safeio.DefaultBudget().Reader(r.Doc.Reader))
	if err != nil {
		ch <- model.PartResult{Error: fmt.Errorf("kbf: read body: %w", err)}
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

	r.streamKBF(ctx, ch, body)

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

func (r *Reader) streamKBF(ctx context.Context, ch chan<- model.PartResult, data []byte) {
	file, err := kbf.Unmarshal(data)
	if err != nil {
		ch <- model.PartResult{Error: fmt.Errorf("kbf: parse kbf: %w", err)}
		return
	}
	// ids separates the block ids this bundle repeats. A `.kbf.json` supplies
	// every block's id and bundles many documents into one file, so uniqueness
	// rests entirely on whoever generated it — and the store keys on the bundle.
	// See core/model/blockid.go.
	var ids model.IDBuilder
	for _, doc := range file.Documents {
		for i := range doc.Blocks {
			block := toModelBlock(&doc, &doc.Blocks[i])
			ids.Assign(block)
			if !r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block}) {
				return
			}
		}
	}
}

// toModelBlock lifts a kbf.Block into a model.Block with every edition
// the bundle holds filed on it (kbf.Block.FileEditions): the source as
// the edition the block was read in, each other edition under its key
// and marked native, with the status, provenance and derivation the
// bundle recorded for it. Without the provenance an answer re-seeded
// from a bundle would arrive with a zero Origin and read as produced
// under no governance. The KBFAnnotation overlay carries extra
// wire-level metadata (Hash, DocumentID, DocumentPath, Placeholders,
// Preview) that the model.Block doesn't have first-class fields for;
// writers read it back to reconstruct the archive.
func toModelBlock(doc *kbf.Document, b *kbf.Block) *model.Block {
	source := b.SourceRuns()
	mb := model.NewRunsBlock(b.ID, cloneRuns(source))
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
	// The bundle's block hash is the key the i18n runtime looks a string up by
	// at render time (both are `hashKey(text, descriptor)`), so it is the join
	// between a block here and the string a running component displays. A
	// surface that renders the component — a story, a live preview — can only
	// show *this* block's translation if it can name it, and the annotation
	// carrying the hash does not survive into a REST block payload.
	if b.Hash != "" {
		mb.Properties["hash"] = b.Hash
	}
	b.FileEditions(mb)
	ann := &KBFAnnotation{
		Source:       cloneRuns(source),
		Placeholders: append([]kbf.Placeholder(nil), b.Placeholders...),
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

func cloneRuns(runs []kbf.Run) []kbf.Run {
	out := make([]kbf.Run, len(runs))
	copy(out, runs)
	return out
}

// ───────── Writer ─────────

// Writer implements format.DataFormatWriter for .kbf.json.
type Writer struct {
	outPath string
	out     io.Writer
	locale  model.LocaleID

	generator kbf.GeneratorInfo
	project   kbf.ProjectInfo

	// Accumulated blocks grouped by document id/path.
	pending map[string]*pendingDoc
	order   []string
}

type pendingDoc struct {
	id     string
	path   string
	blocks []kbf.Block
}

// NewWriter creates a new KBF writer.
func NewWriter() *Writer {
	return &Writer{
		generator: kbf.GeneratorInfo{ID: "neokapi", Version: "1.0"},
		project:   kbf.ProjectInfo{ID: "neokapi-output", SourceLocale: "en"},
		pending:   make(map[string]*pendingDoc),
	}
}

// Name returns the format name.
func (w *Writer) Name() string { return FormatName }

// Generative reports that KBF writes a complete, self-contained document from
// the content model alone (no source skeleton).
func (w *Writer) Generative() bool { return true }

// IsInterchange reports that KBF is neokapi's native bilingual interchange
// format — the `kapi extract --format kpz` / `kapi merge` loop — so it is not
// offered as a `convert` target. See AD-005 "Writer output modes".
func (w *Writer) IsInterchange() bool { return true }

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

// SetEncoding is a no-op — .kbf.json is always UTF-8.
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

// materializeBlock reconstructs a kbf.Block from a model.Block. The
// editions come from the model.Block, every one it holds: the source,
// each translation, every tone and channel edition, and a translation
// filed under no language, with the status, provenance and derivation
// the pipeline recorded (kbf.EditionsOf). Every edition is written, not
// just the writer's own locale: a flow that produced an edition the
// writer was not pointed at still produced content the file has to
// carry, and the reader put every edition in the file on the block to
// begin with.
//
// If the block carries a KBFAnnotation (the round-trip case) the
// annotation supplies the wire-level metadata the model has no
// first-class field for (hash, placeholders, properties, preview,
// document identity), and the source runs as read are written while
// they still carry the block's current source. If there is no
// annotation (the synthesized case) the writer emits a minimal
// text-only source so the archive is still well-formed.
func (w *Writer) materializeBlock(mb *model.Block) (kbf.Block, string, string) {
	editions, unlabelled := kbf.EditionsOf(mb)
	source := editions[kbf.SourceEdition]
	if annRaw, ok := mb.Anno(AnnotationType); ok {
		if ann, ok := annRaw.(*KBFAnnotation); ok && ann != nil {
			// The annotation's source runs are the ones read from the input
			// bundle, kept so a block nothing touched round-trips with its
			// structure (placeholders, paired codes) verbatim. They are only
			// usable while they still carry the block's current text: a source
			// edit (`kapi ksed -i`, `kapi apply`, the desktop's "apply fix")
			// is written as the edited runs (#1473), as an edit to any other
			// edition is (#1471).
			if format.VerbatimRunsCurrent(ann.Source, source.Runs) {
				source.Runs = cloneRuns(ann.Source)
			}
			editions[kbf.SourceEdition] = source
			b := kbf.Block{
				// The id is what the bundle says; a block's ID is an identity
				// for the store, and the two are the same for every bundle whose
				// blocks already identify themselves.
				ID:           model.DocumentID(mb),
				Hash:         ann.Hash,
				Translatable: mb.Translatable,
				Type:         ann.Type,
				Editions:     editions,
				Unlabelled:   unlabelled,
				Placeholders: append([]kbf.Placeholder(nil), ann.Placeholders...),
				Properties:   ann.Properties,
				Preview:      ann.Preview,
			}
			return b, ann.DocumentID, ann.DocumentPath
		}
	}
	// Synthesized fallback: a minimal text-only source from whatever
	// content the model.Block carries, so the source declares no
	// placeholder the block has no record of.
	source.Runs = []kbf.Run{{Text: &kbf.TextRun{Text: mb.SourceText()}}}
	editions[kbf.SourceEdition] = source
	b := kbf.Block{
		ID:           model.DocumentID(mb),
		Hash:         mb.Properties["hash"],
		Translatable: mb.Translatable,
		Type:         kbf.BlockTypeJSXElement,
		Editions:     editions,
		Unlabelled:   unlabelled,
		Properties:   kbf.BlockProperties{File: mb.Properties["file"], Component: mb.Properties["component"], Element: mb.Properties["element"]},
	}
	return b, "synthesized", "synthesized"
}

// Close flushes the accumulated blocks to the configured output.
// Emits .kbf.json (JSON) either to the configured io.Writer or the
// configured output path.
// The writer owns no file handle: SetOutput records a path that writeKBF hands
// to os.WriteFile, and SetOutputWriter records an io.Writer the caller closes.
// A `outFile *os.File` field and a deferred close for it lived here, flagged as
// a discarded Close error — but nothing ever assigned the field, so the branch
// could not run. Removed rather than error-checked: an unreachable close is a
// worse defect than an unchecked one, because it reads as coverage that is not
// there.
func (w *Writer) Close() error {
	file := w.buildKBF()
	if w.out != nil {
		return kbf.Encode(w.out, file)
	}
	if w.outPath == "" {
		return nil
	}
	return w.writeKBF(file)
}

func (w *Writer) buildKBF() *kbf.File {
	file := &kbf.File{
		SchemaVersion: kbf.SchemaVersion,
		Kind:          kbf.Kind,
		Generator:     w.generator,
		Project:       w.project,
	}
	for _, id := range w.order {
		pd := w.pending[id]
		file.Documents = append(file.Documents, kbf.Document{
			ID:           pd.id,
			DocumentType: kbf.DocumentTypeJSX,
			Path:         pd.path,
			Blocks:       pd.blocks,
		})
	}
	return file
}

func (w *Writer) writeKBF(file *kbf.File) error {
	data, err := kbf.Marshal(file)
	if err != nil {
		return err
	}
	return os.WriteFile(w.outPath, data, 0o644)
}

// ───────── PreviewBuilder ─────────

// PreviewBuilder produces the <kat-block> preview HTML for a block
// by walking its structured Runs through the KBF reference
// renderer. Consumers that wire this into the framework's existing
// preview pipeline get JSX support with no additional code.
type PreviewBuilder struct {
	vocab kbf.VocabularyLookup
}

// NewPreviewBuilder creates a new preview builder. Vocab defaults
// to the built-in JSX vocabulary if nil.
func NewPreviewBuilder() *PreviewBuilder {
	return &PreviewBuilder{vocab: kbf.DefaultJSXVocabulary()}
}

// BuildBlockPreview returns the <kat-block> HTML preview for a
// model.Block. Requires the block to carry a KBFAnnotation; falls
// back to the flattened text wrapped in a <kat-block> envelope
// otherwise so downstream tools never see an empty preview.
func (p *PreviewBuilder) BuildBlockPreview(mb *model.Block) string {
	if mb == nil {
		return ""
	}
	av, _ := mb.Anno(AnnotationType)
	ann, ok := av.(*KBFAnnotation)
	if !ok || ann == nil {
		escaped := jsonEscapeAttr(mb.ID)
		return fmt.Sprintf(`<kat-block id=%s data-type="text">%s</kat-block>`,
			escaped, htmlEscape(mb.SourceText()))
	}
	b := &kbf.Block{
		ID:       mb.ID,
		Type:     ann.Type,
		Editions: kbf.SourceEditions(ann.Source),
	}
	return kbf.RenderBlockHTML(b, p.vocab)
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
