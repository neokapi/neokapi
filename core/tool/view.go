package tool

import (
	"context"
	"fmt"
	"iter"
	"maps"

	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/model"
)

// BlockView is the surface a tool sees for a Block (AD-002 / AD-006
// immutability model). Source and target content are READ-ONLY through this
// view; the writable output surface is overlays, annotations, and properties.
//
// A tool's capability is expressed by which handler field it sets on BaseTool:
//   - Annotate(BlockView)   — analysis / annotation (the default; no content
//     writes possible — the methods simply don't exist here)
//   - Produce(VariantView) — writes target content
//   - Transform(BlockView)  — a read-only edit producer returning an EditPlan;
//     the framework applier is the sole source mutator (AD-006)
//
// Because the read methods hand back the Block's live run slices, tools must
// treat returned runs as read-only; the dispatcher's backstop check
// (EnforceImmutability) catches accidental in-place edits in dev/test.
//
// BlockView is composed from named facets grouped by concern — BlockReader
// (all reads), OverlayWriter, AnnotationWriter, and PropertyWriter (the three
// writable output surfaces) — so tests can mock a facet and readers of the
// code can see at a glance what a tool may touch.
type BlockView interface {
	BlockReader
	OverlayWriter
	AnnotationWriter
	PropertyWriter

	// Drop removes this block from the stream (e.g. remove-target with
	// RemoveBlockIfEmpty). The dispatcher emits nothing for a dropped block.
	Drop()
}

// BlockReader is the read-only facet of a block view: identity and metadata,
// source and target content, and reads over overlays, annotations, and
// properties.
type BlockReader interface {
	// Context returns the dispatch context, so a handler can honour
	// cancellation/deadlines on any work it does (LLM/MT/NER calls, subprocess
	// execution, DB lookups). It is never nil: views built without a context
	// (in-memory Apply, tests) report context.Background().
	Context() context.Context

	ID() string
	Name() string
	Type() string
	MimeType() string
	Translatable() bool
	SourceLocale() model.LocaleID
	Identity() *model.BlockIdentity
	// ChainUnit is the block's durable identity across edits: what links its
	// successive approved translations into one chain. See model.Block.ChainUnit.
	//
	// A tool that needs to ask what this block said before needs this and not
	// the ID, which is assigned per read. Exposed here rather than by handing
	// out the block, so a view stays a view.
	ChainUnit() string
	PreserveWhitespace() bool

	// Source (read-only).
	SourceRuns() []model.Run
	SourceText() string
	WordCount() int
	SourceSegmentation() *model.Overlay
	SourceSegmentCount() int
	SourceSegmentRuns(i int) []model.Run

	// SourceUnits yields the source processing units of the given segmentation
	// layer (model.LayerPrimary = primary): one per segment span, or a single
	// whole-block unit when the layer has no segmentation overlay. It is the
	// uniform replacement for hand-rolled SourceSegmentCount / SourceSegmentRuns
	// loops.
	SourceUnits(layer string) iter.Seq[Unit]

	// Targets (read-only).
	HasTarget(loc model.LocaleID) bool
	TargetLocales() []model.LocaleID
	TargetRuns(loc model.LocaleID) []model.Run
	TargetText(loc model.LocaleID) string
	// Target returns the target edition for loc, its runs with its status,
	// origin and score, or nil when the block holds no target in loc: the
	// target TargetRuns and TargetText read (model.Block.TargetEdition). The
	// edition is a copy, so a status written to it stays there; its runs are
	// the block's own and read-only. The edition the block was read in is
	// never a target, so loc names a target in the source language only when
	// the block holds one.
	Target(loc model.LocaleID) *model.Edition

	// Overlays / annotations / properties (read side).
	Overlays() []model.Overlay
	// SegmentationFor and SegmentationLayerFor read the segmentation on one
	// edition, the zero key naming the edition the block was read in.
	SegmentationFor(k model.EditionKey) *model.Overlay
	SegmentationLayerFor(k model.EditionKey, layer string) *model.Overlay
	// OverlaySpans returns the spans of the source-side overlay of the
	// given type (term, entity, term-candidate, …), or nil. Read-only.
	OverlaySpans(t model.OverlayType) []model.Span
	// Annotations returns a snapshot of the block annotations (the former
	// annotation map). Use Annotate to write; writing to the returned map has
	// no effect.
	Annotations() map[string]model.Payload
	// Properties returns a snapshot of the block properties. Use SetProperty
	// to write; writing to the returned map has no effect.
	Properties() map[string]string
	Property(key string) string

	// SourceStatus reports the block's authoring lifecycle state (written →
	// established); "" means no committed status yet.
	SourceStatus() model.SourceStatus
}

// OverlayWriter is the overlay-writing facet of a block view: positional,
// run-anchored stand-off layers (segmentation, term, entity, qa, alignment).
type OverlayWriter interface {
	// SetSegmentation and SetSegmentationLayer write the segmentation on one
	// edition, the zero key naming the edition the block was read in.
	SetSegmentation(k model.EditionKey, spans []model.Span)
	SetSegmentationLayer(k model.EditionKey, layer string, spans []model.Span)
	AddOverlay(o model.Overlay)
	// AddOverlaySpan appends an overlay span (term, entity, …) to the
	// source-side overlay of the given type, merging into the existing overlay. The
	// span's Range is the position and its ID the stable identity.
	AddOverlaySpan(t model.OverlayType, s model.Span)
	// RemoveOverlay drops the source-side overlay of the given type. A
	// source-transform tool that consumes an overlay and then rewrites
	// the source uses this to drop the now-stale run-anchored spans.
	RemoveOverlay(t model.OverlayType)
}

// AnnotationWriter is the annotation-writing facet of a block view:
// block-scoped typed metadata (notes, alt-translations, analysis results).
type AnnotationWriter interface {
	// AddAltTranslation appends an alternative-translation candidate to the
	// block's AnnoAltTranslation collection (multiplicity lives in the
	// collection, never in numbered keys).
	AddAltTranslation(a *model.AltTranslation)
	// AppendAltUnder appends an alt-translation to the collection under an
	// arbitrary key (e.g. the per-segment content memory-match set).
	AppendAltUnder(key string, a *model.AltTranslation)
	// AddNote appends a note to the block's note collection (multiplicity lives
	// in the collection, never in numbered keys).
	AddNote(n *model.NoteAnnotation)
	// Annotate stores a block annotation payload under key.
	Annotate(key string, a model.Payload)
	// RemoveAnnotation deletes the block annotation stored under key.
	RemoveAnnotation(key string)
}

// PropertyWriter is the property-writing facet of a block view: string
// properties plus the source lifecycle stamp, both metadata about the block
// rather than content rewrites, so they are available at the read-only
// Annotate tier.
type PropertyWriter interface {
	SetProperty(key, value string)
	// SetSourceStatus stamps the block's authoring lifecycle state. This is
	// metadata about the source, not a rewrite of its runs, so it is available
	// at the read-only Annotate tier (like SetProperty) — a check tool that
	// clears a block stamps `written` without touching the source content.
	SetSourceStatus(s model.SourceStatus)
}

// VariantView adds target-write access (TargetWriter). Tools that translate
// or edit targets receive this via the Produce handler.
type VariantView interface {
	BlockView
	TargetWriter
}

// TargetWriter is the target-writing facet of a block view: committing,
// stamping, and removing per-variant target content.
type TargetWriter interface {
	// SetEdition writes the derived edition key names, its runs, status,
	// origin and score, creating it when the block does not hold it. The
	// edition the block was read in is the source, which a Transform handler
	// rewrites through its EditPlan: a write to it here is refused.
	SetEdition(key model.EditionKey, e model.Edition)
	SetTargetRuns(loc model.LocaleID, runs []model.Run)
	SetTargetText(loc model.LocaleID, text string)
	// StampTargetProvenance records how the locale's target was produced
	// (lifecycle status + origin) without touching its runs; no-op if absent.
	StampTargetProvenance(loc model.LocaleID, status model.TargetStatus, origin model.Origin)
	// RemoveEdition removes the derived edition key names; the source has none
	// to remove. RemoveTarget is RemoveEdition for a locale.
	RemoveEdition(key model.EditionKey)
	RemoveTarget(loc model.LocaleID)
	ClearTargets()

	// TargetUnits yields writable per-unit target production over the source
	// segmentation of the given layer (model.LayerPrimary = primary), splicing
	// each unit's runs back into the block target for loc when iteration
	// completes. Commit is all-or-nothing across non-ignorable units; see
	// WritableUnit.
	TargetUnits(loc model.LocaleID, layer string) iter.Seq[WritableUnit]
}

// blockView is the single concrete view; the handler field's parameter type
// (BlockView / VariantView) narrows which methods a tool can call. There is no
// source-write view: a Transform handler is a read-only producer and the
// framework applier rewrites the source (AD-006).
//
// Every content write a view makes, a target and its provenance, and every
// overlay write, is an operation applied at once through change.ApplyBlock,
// the one function that changes a block's content. Applying at once keeps
// read-your-writes: a handler that sets a target and then stamps it sees the
// target it set. The view applies as the tool, with guard violations landing
// as findings (a tool's draft meets the ship gates later). An operation the
// applier refuses is a defect in the tool: the view keeps the first refusal,
// and the dispatcher returns it as the handler's error (see Refusal).
type blockView struct {
	ctx     context.Context
	b       *model.Block
	tool    string
	dropped bool
	err     error
}

func newBlockView(ctx context.Context, b *model.Block) *blockView {
	return &blockView{ctx: ctx, b: b}
}

// newToolView is a view a tool's dispatched handler writes through as the
// named tool.
func newToolView(ctx context.Context, b *model.Block, toolName string) *blockView {
	return &blockView{ctx: ctx, b: b, tool: toolName}
}

// Refusal returns the first write the view's applier refused, or nil. The
// dispatcher returns it for a handler it runs; a tool that overrides Process
// and writes through a view it built checks it itself, or uses WriteAs.
func Refusal(v BlockView) error {
	if bv, ok := v.(*blockView); ok {
		return bv.err
	}
	return nil
}

// WriteAs runs fn over a view of b that writes as the named tool, and returns
// fn's error or else the first write the view's applier refused. A tool that
// overrides Process writes a block through it, so its writes take the path a
// dispatched handler's take.
func WriteAs(ctx context.Context, b *model.Block, toolName string, fn func(VariantView) error) error {
	v := newToolView(ctx, b, toolName)
	if err := fn(v); err != nil {
		return err
	}
	if v.err != nil {
		return fmt.Errorf("tool %q: %w", toolName, v.err)
	}
	return nil
}

// apply applies operations to the view's block as the view's tool.
func (v *blockView) apply(ops ...change.Op) {
	if v.err != nil {
		return
	}
	env := change.BlockEnv{Actor: change.Actor{Kind: change.ActorTool, Name: v.tool}, Guards: change.Report}
	for _, r := range change.ApplyBlock(v.b, ops, env) {
		if r.Status == change.OpRefused {
			v.err = fmt.Errorf("block %q: %s %s refused: %w", v.b.ID, r.Op, editionText(r.At), r.Error)
			return
		}
	}
}

// editionText names the edition an operation result addresses, for messages.
func editionText(at *change.Ref) string {
	if at == nil || at.Edition.IsZero() {
		return "on the source"
	}
	return "on " + at.EditionText()
}

// ref addresses one edition of the view's block.
func (v *blockView) ref(key model.EditionKey) change.Ref {
	return change.Ref{Block: v.b.ID, Edition: key}
}

// setRuns replaces a target's runs, creating the target when absent.
func (v *blockView) setRuns(key model.EditionKey, runs []model.Run, more ...change.Op) {
	if v.err != nil {
		return
	}
	if v.b.IsSourceEdition(key) {
		v.err = sourceLanguageTarget(v.b, key)
		return
	}
	if runs == nil {
		runs = []model.Run{}
	}
	op := change.Op{Kind: change.KindSetContent, At: v.ref(key), IfMatch: change.AnyRevision, Body: &change.SetContent{Runs: runs}}
	v.apply(append([]change.Op{op}, more...)...)
}

// sourceLanguageTarget is the refusal of a target write whose key reaches the
// edition the block was read in: a target in the source language on a block
// that holds none. A block holds such a target when a bilingual file names one
// language twice, and a tool writes it then; creating one would replace the
// source.
func sourceLanguageTarget(b *model.Block, key model.EditionKey) error {
	text, _ := key.MarshalText()
	return fmt.Errorf("block %q: a target in %s, the block's source language, would replace the source; the block holds no target in that language", b.ID, text)
}

// provenance is the operation that records how the tool produced an edition.
func (v *blockView) provenance(key model.EditionKey, status model.Status, origin model.Origin, score *float64) change.Op {
	return change.Op{Kind: change.KindProvenance, At: v.ref(key),
		Body: &change.Provenance{Status: status, Origin: origin, Score: score}}
}

// NewBlockView and NewVariantView build an explicit view over a Block at the
// matching capability tier. Dispatched handlers receive a view automatically;
// these constructors are for Process-override tools (batched or session-aware
// translators, stream operators) that hold a *model.Block directly and want to
// reuse the same capability-scoped surface. The view's Context() reports
// context.Background(); use the WithContext variants from a Process override
// to propagate cancellation into provider/network calls.
func NewBlockView(b *model.Block) BlockView     { return newBlockView(context.Background(), b) }
func NewVariantView(b *model.Block) VariantView { return newBlockView(context.Background(), b) }

// NewBlockViewWithContext and NewVariantViewWithContext are the
// cancellation-aware constructors for Process-override tools: the view's
// Context() returns ctx, so handlers can honour deadlines/cancellation.
func NewBlockViewWithContext(ctx context.Context, b *model.Block) BlockView {
	return newBlockView(ctx, b)
}
func NewVariantViewWithContext(ctx context.Context, b *model.Block) VariantView {
	return newBlockView(ctx, b)
}

// Context reports the dispatch context, defaulting to context.Background().
func (v *blockView) Context() context.Context {
	if v.ctx != nil {
		return v.ctx
	}
	return context.Background()
}

// Reads.
func (v *blockView) ID() string                     { return v.b.ID }
func (v *blockView) Name() string                   { return v.b.Name }
func (v *blockView) Type() string                   { return v.b.Type }
func (v *blockView) MimeType() string               { return v.b.MimeType }
func (v *blockView) Translatable() bool             { return v.b.Translatable }
func (v *blockView) SourceLocale() model.LocaleID   { return v.b.SourceLocale }
func (v *blockView) Identity() *model.BlockIdentity { return v.b.Identity }
func (v *blockView) ChainUnit() string              { return v.b.ChainUnit() }
func (v *blockView) PreserveWhitespace() bool       { return v.b.PreserveWhitespace }

func (v *blockView) SourceRuns() []model.Run             { return authoritative(v.b).Runs }
func (v *blockView) SourceText() string                  { return v.b.SourceText() }
func (v *blockView) WordCount() int                      { return v.b.WordCount() }
func (v *blockView) SourceSegmentation() *model.Overlay  { return v.b.SourceSegmentation() }
func (v *blockView) SourceSegmentCount() int             { return v.b.SourceSegmentCount() }
func (v *blockView) SourceSegmentRuns(i int) []model.Run { return v.b.SourceSegmentRuns(i) }

func (v *blockView) SourceUnits(layer string) iter.Seq[Unit] { return sourceUnits(v.b, layer) }

func (v *blockView) HasTarget(loc model.LocaleID) bool         { return v.b.HasTarget(loc) }
func (v *blockView) TargetLocales() []model.LocaleID           { return v.b.TargetLocales() }
func (v *blockView) TargetRuns(loc model.LocaleID) []model.Run { return v.b.TargetRuns(loc) }
func (v *blockView) TargetText(loc model.LocaleID) string      { return v.b.TargetText(loc) }
func (v *blockView) Target(loc model.LocaleID) *model.Edition {
	e, ok := v.b.TargetEdition(loc)
	if !ok {
		return nil
	}
	return &e
}

// Overlays / annotations / properties (writable output surface).
func (v *blockView) Overlays() []model.Overlay { return v.b.Overlays }
func (v *blockView) SegmentationFor(k model.EditionKey) *model.Overlay {
	return v.b.SegmentationFor(k)
}
func (v *blockView) SegmentationLayerFor(k model.EditionKey, layer string) *model.Overlay {
	return v.b.SegmentationLayerFor(k, layer)
}
func (v *blockView) SetSegmentation(k model.EditionKey, spans []model.Span) {
	v.SetSegmentationLayer(k, model.LayerPrimary, spans)
}
func (v *blockView) SetSegmentationLayer(k model.EditionKey, layer string, spans []model.Span) {
	v.apply(change.Op{Kind: change.KindAnnotate, At: v.ref(k),
		Body: &change.Annotate{Type: string(model.OverlaySegmentation), Layer: layer, Spans: spans, Replace: true}})
}
func (v *blockView) AddOverlay(o model.Overlay) {
	spans := o.Spans
	if spans == nil {
		spans = []model.Span{}
	}
	v.apply(change.Op{Kind: change.KindAnnotate, At: v.ref(o.Edition),
		Body: &change.Annotate{Type: string(o.Type), Layer: o.Layer, Spans: spans}})
}
func (v *blockView) AddOverlaySpan(t model.OverlayType, s model.Span) {
	v.apply(change.Op{Kind: change.KindAnnotate, At: v.ref(model.EditionKey{}),
		Body: &change.Annotate{Type: string(t), Spans: []model.Span{s}}})
}
func (v *blockView) OverlaySpans(t model.OverlayType) []model.Span {
	if f := v.b.OverlayOf(t); f != nil {
		return f.Spans
	}
	return nil
}
func (v *blockView) RemoveOverlay(t model.OverlayType) {
	v.apply(change.Op{Kind: change.KindUnannotate, At: v.ref(model.EditionKey{}),
		Body: &change.Unannotate{Type: string(t), All: true}})
}
func (v *blockView) AddAltTranslation(a *model.AltTranslation) { v.b.AddAltTranslation(a) }
func (v *blockView) AddNote(n *model.NoteAnnotation)           { v.b.AddNote(n) }
func (v *blockView) AppendAltUnder(key string, a *model.AltTranslation) {
	v.b.AppendAltUnder(key, a)
}

// Annotations and Properties return snapshots (per the BlockReader contract):
// handing the tool the live map would open an unchecked mutation channel past
// Annotate/SetProperty, the sanctioned write paths.
func (v *blockView) Annotations() map[string]model.Payload { return maps.Clone(v.b.AnnoMap()) }
func (v *blockView) Annotate(key string, a model.Payload)  { v.b.SetAnno(key, a) }
func (v *blockView) RemoveAnnotation(key string)           { v.b.DelAnno(key) }
func (v *blockView) Properties() map[string]string         { return maps.Clone(v.b.Properties) }
func (v *blockView) SetProperty(key, value string) {
	if v.b.Properties == nil {
		v.b.Properties = make(map[string]string)
	}
	v.b.Properties[key] = value
}
func (v *blockView) Property(key string) string { return v.b.Properties[key] }

func (v *blockView) SourceStatus() model.SourceStatus {
	return model.SourceStatus(authoritative(v.b).Status)
}

// SetSourceStatus stamps the authoritative edition's status and changes
// nothing else. It goes through SetEditionStatus because SetEdition on that
// edition is an edit: it records the source as read and rewrites the
// source-origin annotation, and a status stamp is neither. The zero key names
// the edition the view reads the source from (see authoritative).
func (v *blockView) SetSourceStatus(s model.SourceStatus) {
	v.b.SetEditionStatus(model.EditionKey{}, model.Status(s))
}

// authoritative returns the block's authoritative edition under the empty
// policy, the edition the block was read in: the edition a source read, a
// source rewrite and segmentation with no variant address. It is the edition
// b.Authoritative(model.AuthorityPolicy{}) returns, reached by the zero key,
// which names it without resolving a locale; every tool reads the source
// through here for every block.
func authoritative(b *model.Block) model.Edition {
	e, _ := b.Edition(model.EditionKey{})
	return e
}

func (v *blockView) Drop() { v.dropped = true }

// result maps the view's post-handler state back to a Part for the dispatcher:
// the original part when kept, or nil when the handler called Drop().
func (v *blockView) result(part *model.Part) *model.Part {
	if v.dropped {
		return nil
	}
	return part
}

// Target writes (VariantView). Each is a set_content, remove_edition or
// provenance operation on the edition a locale or variant key names.
func (v *blockView) SetEdition(key model.EditionKey, e model.Edition) {
	score := e.Score
	v.setRuns(key, e.Runs, v.provenance(key, e.Status, e.Origin, &score))
}
func (v *blockView) SetTargetRuns(loc model.LocaleID, runs []model.Run) {
	v.setRuns(model.Variant(loc), runs)
}
func (v *blockView) SetTargetText(loc model.LocaleID, text string) {
	v.setRuns(model.Variant(loc), []model.Run{{Text: &model.TextRun{Text: text}}})
}
func (v *blockView) StampTargetProvenance(loc model.LocaleID, status model.TargetStatus, origin model.Origin) {
	key := model.Variant(loc)
	if v.b.IsSourceEdition(key) {
		return // no target holds the key; the source takes no target status
	}
	v.apply(v.provenance(key, model.Status(status), origin, nil))
}
func (v *blockView) TargetUnits(loc model.LocaleID, layer string) iter.Seq[WritableUnit] {
	return targetUnits(v, loc, layer)
}
func (v *blockView) RemoveTarget(loc model.LocaleID) { v.RemoveEdition(model.Variant(loc)) }
func (v *blockView) ClearTargets() {
	var ops []change.Op
	for _, key := range v.b.EditionKeys() {
		if v.b.IsSourceEdition(key) {
			continue
		}
		ops = append(ops, change.Op{Kind: change.KindRemoveEdition, At: v.ref(key), IfMatch: change.AnyRevision, Body: &change.RemoveEdition{}})
	}
	if len(ops) > 0 {
		v.apply(ops...)
	}
}

// RemoveEdition removes a derived edition. The source has no target to remove.
func (v *blockView) RemoveEdition(key model.EditionKey) {
	if v.b.IsSourceEdition(key) {
		return
	}
	v.apply(change.Op{Kind: change.KindRemoveEdition, At: v.ref(key), IfMatch: change.AnyRevision, Body: &change.RemoveEdition{}})
}

// Compile-time checks that blockView satisfies every view tier.
var (
	_ BlockView   = (*blockView)(nil)
	_ VariantView = (*blockView)(nil)
)
