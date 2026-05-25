package tool

import "github.com/neokapi/neokapi/core/model"

// BlockView is the surface a tool sees for a Block (AD-002 / AD-006
// immutability model). Source and target content are READ-ONLY through this
// view; the writable output surface is overlays, annotations, and properties.
//
// A tool's capability is expressed by which handler field it sets on BaseTool:
//   - Annotate(BlockView)   — analysis / annotation (the default; no content
//     writes possible — the methods simply don't exist here)
//   - Translate(TargetView) — writes target content
//   - Transform(SourceView) — rewrites source (and may write target); runs in
//     the early phase, before any stand-off overlay is attached
//
// Because the read methods hand back the Block's live run slices, tools must
// treat returned runs as read-only; the dispatcher's backstop check
// (EnforceImmutability) catches accidental in-place edits in dev/test.
type BlockView interface {
	ID() string
	Name() string
	Type() string
	MimeType() string
	Translatable() bool
	SourceLocale() model.LocaleID
	Identity() *model.BlockIdentity
	PreserveWhitespace() bool

	// Source (read-only).
	SourceRuns() []model.Run
	SourceText() string
	WordCount() int
	SourceSegmentation() *model.Overlay
	SourceSegmentCount() int
	SourceSegmentRuns(i int) []model.Run

	// Targets (read-only).
	HasTarget(loc model.LocaleID) bool
	TargetLocales() []model.LocaleID
	TargetRuns(loc model.LocaleID) []model.Run
	TargetText(loc model.LocaleID) string
	Target(loc model.LocaleID) *model.Target

	// Overlays / annotations / properties — the writable output surface.
	Overlays() []model.Overlay
	SegmentationFor(variant *model.VariantKey) *model.Overlay
	SetSegmentation(variant *model.VariantKey, spans []model.Span)
	AddOverlay(o model.Overlay)
	Annotations() map[string]model.Annotation
	Annotate(key string, a model.Annotation)
	Properties() map[string]string
	SetProperty(key, value string)
	Property(key string) string

	// Drop removes this block from the stream (e.g. remove-target with
	// RemoveBlockIfEmpty). The dispatcher emits nothing for a dropped block.
	Drop()
}

// TargetView adds target-write access. Tools that translate or edit targets
// receive this via TranslateBlockFn.
type TargetView interface {
	BlockView
	SetTarget(loc model.LocaleID, t *model.Target)
	SetTargetVariant(key model.VariantKey, t *model.Target)
	SetTargetRuns(loc model.LocaleID, runs []model.Run)
	SetTargetText(loc model.LocaleID, text string)
	RemoveTarget(loc model.LocaleID)
	ClearTargets()
}

// SourceView adds source-write access (and includes target writes). Tools that
// transform source — redaction, normalization, case/encoding conversion —
// receive this via TransformBlockFn and must run before overlays exist.
type SourceView interface {
	TargetView
	SetSourceRuns(runs []model.Run)
	SetSourceText(text string)
}

// blockView is the single concrete view; the handler field's parameter type
// (BlockView / TargetView / SourceView) narrows which methods a tool can call.
type blockView struct {
	b       *model.Block
	dropped bool
}

func newBlockView(b *model.Block) *blockView { return &blockView{b: b} }

// NewBlockView, NewTargetView and NewSourceView build an explicit view over a
// Block at the matching capability tier. Dispatched handlers receive a view
// automatically; these constructors are for Process-override tools (batched or
// session-aware translators, stream operators) that hold a *model.Block
// directly and want to reuse the same capability-scoped surface.
func NewBlockView(b *model.Block) BlockView   { return newBlockView(b) }
func NewTargetView(b *model.Block) TargetView { return newBlockView(b) }
func NewSourceView(b *model.Block) SourceView { return newBlockView(b) }

// Reads.
func (v *blockView) ID() string                     { return v.b.ID }
func (v *blockView) Name() string                   { return v.b.Name }
func (v *blockView) Type() string                   { return v.b.Type }
func (v *blockView) MimeType() string               { return v.b.MimeType }
func (v *blockView) Translatable() bool             { return v.b.Translatable }
func (v *blockView) SourceLocale() model.LocaleID   { return v.b.SourceLocale }
func (v *blockView) Identity() *model.BlockIdentity { return v.b.Identity }
func (v *blockView) PreserveWhitespace() bool       { return v.b.PreserveWhitespace }

func (v *blockView) SourceRuns() []model.Run             { return v.b.Source }
func (v *blockView) SourceText() string                  { return v.b.SourceText() }
func (v *blockView) WordCount() int                      { return v.b.WordCount() }
func (v *blockView) SourceSegmentation() *model.Overlay  { return v.b.SourceSegmentation() }
func (v *blockView) SourceSegmentCount() int             { return v.b.SourceSegmentCount() }
func (v *blockView) SourceSegmentRuns(i int) []model.Run { return v.b.SourceSegmentRuns(i) }

func (v *blockView) HasTarget(loc model.LocaleID) bool       { return v.b.HasTarget(loc) }
func (v *blockView) TargetLocales() []model.LocaleID         { return v.b.TargetLocales() }
func (v *blockView) TargetRuns(loc model.LocaleID) []model.Run { return v.b.TargetRuns(loc) }
func (v *blockView) TargetText(loc model.LocaleID) string    { return v.b.TargetText(loc) }
func (v *blockView) Target(loc model.LocaleID) *model.Target { return v.b.Target(loc) }

// Overlays / annotations / properties (writable output surface).
func (v *blockView) Overlays() []model.Overlay { return v.b.Overlays }
func (v *blockView) SegmentationFor(variant *model.VariantKey) *model.Overlay {
	return v.b.SegmentationFor(variant)
}
func (v *blockView) SetSegmentation(variant *model.VariantKey, spans []model.Span) {
	v.b.SetSegmentation(variant, spans)
}
func (v *blockView) AddOverlay(o model.Overlay) { v.b.Overlays = append(v.b.Overlays, o) }
func (v *blockView) Annotations() map[string]model.Annotation {
	if v.b.Annotations == nil {
		v.b.Annotations = make(map[string]model.Annotation)
	}
	return v.b.Annotations
}
func (v *blockView) Annotate(key string, a model.Annotation) { v.Annotations()[key] = a }
func (v *blockView) Properties() map[string]string {
	if v.b.Properties == nil {
		v.b.Properties = make(map[string]string)
	}
	return v.b.Properties
}
func (v *blockView) SetProperty(key, value string) { v.Properties()[key] = value }
func (v *blockView) Property(key string) string    { return v.b.Properties[key] }

func (v *blockView) Drop() { v.dropped = true }

// result maps the view's post-handler state back to a Part for the dispatcher:
// the original part when kept, or nil when the handler called Drop().
func (v *blockView) result(part *model.Part) *model.Part {
	if v.dropped {
		return nil
	}
	return part
}

// Target writes (TargetView).
func (v *blockView) SetTarget(loc model.LocaleID, t *model.Target) { v.b.SetTarget(loc, t) }
func (v *blockView) SetTargetVariant(key model.VariantKey, t *model.Target) {
	v.b.SetTargetVariant(key, t)
}
func (v *blockView) SetTargetRuns(loc model.LocaleID, runs []model.Run) { v.b.SetTargetRuns(loc, runs) }
func (v *blockView) SetTargetText(loc model.LocaleID, text string)      { v.b.SetTargetText(loc, text) }
func (v *blockView) RemoveTarget(loc model.LocaleID)                    { delete(v.b.Targets, model.Variant(loc)) }
func (v *blockView) ClearTargets() {
	v.b.Targets = make(map[model.VariantKey]*model.Target)
}

// Source writes (SourceView).
func (v *blockView) SetSourceRuns(runs []model.Run) { v.b.SetSourceRuns(runs) }
func (v *blockView) SetSourceText(text string)      { v.b.SetSourceText(text) }

// Compile-time checks that blockView satisfies every view tier.
var (
	_ BlockView  = (*blockView)(nil)
	_ TargetView = (*blockView)(nil)
	_ SourceView = (*blockView)(nil)
)
