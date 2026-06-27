package model

import "strconv"

// Structural metadata layer (WS1 of the Docling/DocLang scope).
//
// A document's logical structure — what each block *is* (heading, caption,
// table cell, …), where it sits on a page (bounding box), and which layout
// layer it belongs to (body vs furniture) — is carried as two block-scoped
// stand-off payloads, NOT as new top-level Block fields. This keeps the
// addition strictly additive: both ride the existing payload registry
// (annotation_registry.go), so they serialize over every annotation-aware path
// (the gRPC bridge, the store) with no protobuf or KLF schema change. Readers
// populate them natively where the format carries the information (WS2); the
// Docling/DocLang ingestion path populates them from the model's structure +
// provenance (WS4/WS7); writers and the visual editor consume them (WS5/WS6).
//
// The role/layer values are an open but normalized vocabulary — the Role* and
// Layer* constants below are the canonical set (aligned with the DocLang
// taxonomy). Unknown values are permitted and degrade gracefully.

// Block-scoped annotation keys for the structural layer.
const (
	// AnnoStructure carries a *StructureAnnotation (role, plane, visibility, level).
	AnnoStructure = "structure"
	// AnnoGeometry carries a *GeometryAnnotation (page + bounding box) — the
	// spatial anchor facet, for content from a rendered medium.
	AnnoGeometry = "geometry"
	// AnnoTiming carries a *TimingAnnotation (time span) — the temporal anchor
	// facet, for content from timed media (audio, video).
	AnnoTiming = "timing"
	// AnnoRelations carries a *RelationAnnotation (cross-block edges: a caption's
	// figure, a footnote's marker, a label's field, a trigger's modal).
	AnnoRelations = "relations"
)

// Normalized semantic roles. Aligned with the DocLang / DoclingDocument
// taxonomy so a reader, the editor, an exporter, and a DocLang writer all speak
// the same role names. The set is open; these are the canonical values.
const (
	RoleParagraph   = "paragraph"
	RoleTitle       = "title"
	RoleHeading     = "heading"
	RoleCaption     = "caption"
	RoleFootnote    = "footnote"
	RoleList        = "list"
	RoleListItem    = "list-item"
	RoleTable       = "table"
	RoleTableCell   = "table-cell"
	RoleTableHeader = "table-header"
	RoleCode        = "code"
	RoleFormula     = "formula"
	RolePicture     = "picture"
	RolePageHeader  = "page-header"
	RolePageFooter  = "page-footer"
	RoleFormField   = "form-field"
	RoleSection     = "section"
	RoleIndex       = "index"  // a table of contents / index / glossary (DocLang <index>; OTSL-celled like a table)
	RoleMarker      = "marker" // a visible list/field glyph or number (DocLang <marker>)

	// Forms cluster — the canonical roles for AcroForm-style key/value/hint
	// fields (DocLang field_region/field_heading/field_item/key/value/hint/
	// checkbox). RoleFormField stays the coarse catch-all; these name the parts.
	RoleFieldRegion  = "field-region"  // a form-scoping container (DocLang <field_region>)
	RoleFieldHeading = "field-heading" // a heading scoped inside a field region (DocLang <field_heading>)
	RoleFieldItem    = "field-item"    // one key/value grouping inside a field region (DocLang <field_item>)
	RoleKey          = "key"           // a field's key/label text (DocLang <key>)
	RoleValue        = "value"         // a field's value text, read-only or fillable (DocLang <value>)
	RoleHint         = "hint"          // guidance text for a fillable field (DocLang <hint>)
	RoleCheckbox     = "checkbox"      // a selection control with a checked state (DocLang <checkbox>)
)

// Block.Properties convention keys for fine subtype / form state that has no
// typed home on StructureAnnotation but is faithfully carried as a string
// property. These are an open, normalized convention — readers populate them,
// writers and the editor consume them. Boolean-valued keys use "true"/"false".
const (
	// PropCheckboxChecked is "true" when a RoleCheckbox block is selected
	// (DocLang <checkbox class="selected">), "false"/absent when unselected.
	PropCheckboxChecked = "checkbox.checked"
	// PropFieldFillable is "true" when a RoleValue block is an editable/fillable
	// field, "false"/absent when read-only (DocLang <value class="fillable">).
	PropFieldFillable = "field.fillable"
	// PropCodeLanguage is the canonical programming-language key for code content
	// (DocLang <code> with a <label value="Python">; GitHub Linguist keys). A
	// do-not-translate / syntax-relevant signal for localization. On a RoleCode
	// block it is set/read via SetCodeLanguage/CodeLanguage; the same key names
	// the language on a non-extracted "code-block" Data part. It supersedes the
	// former format-local "language" property.
	PropCodeLanguage = "code.language"
	// PropPictureSubclass is the fine subclass of a RolePicture block — e.g. a
	// chart kind (bar/pie/line/…) (DocLang <picture class="chart"> + <label>).
	PropPictureSubclass = "picture.subclass"
	// PropTableHeaderKind names the OTSL header sub-kind of a RoleTableHeader
	// cell: TableHeaderColumn (ched), TableHeaderRow (rhed), TableHeaderCorner
	// (corn), or TableHeaderSection (srow). Absent = an unqualified header.
	PropTableHeaderKind = "table.header-kind"
	// PropContainerEntry is the slash-separated path of the archive entry a block
	// was read from, when the source document is a container (ZIP/TAR). The
	// archive reader stamps it on every block so consumers can attribute content
	// to `<archive>!<entry>` (the bang locator, AD-026 §6) without tracking the
	// enclosing child layer. Absent for blocks from a plain (non-container) file.
	PropContainerEntry = "container.entry"
)

// OTSL table-header sub-kinds — the values PropTableHeaderKind takes. DocLang
// distinguishes column (ched), row (rhed), corner (corn), and section (srow)
// headers; the coarse RoleTableHeader role plus this property names which.
const (
	TableHeaderColumn  = "column"
	TableHeaderRow     = "row"
	TableHeaderCorner  = "corner"
	TableHeaderSection = "section"
)

// Layout layers — the PLANE axis: which visual stratum a block belongs to.
// Aligned with DocLang's <layer> values and extended for the strata reflowable
// documents add. Body is the main content; furniture is repeated/supplementary
// matter (running headers/footers, nav, page numbers); background is watermarks
// and the like; overlay is content stacked above the body (modal/dialog/popover/
// tooltip); metadata is content outside the rendered flow (HTML <title>, meta
// description, alt text). The set is open; these are the canonical values.
const (
	LayerBody       = "body"
	LayerFurniture  = "furniture"
	LayerBackground = "background"
	LayerOverlay    = "overlay"
	LayerMetadata   = "metadata"
)

// Visibility — the presence-condition axis: whether a block is shown, and under
// what condition. Orthogonal to the plane (a modal is plane=overlay,
// visibility=conditional). The empty value means visible/unconditional. The set
// is open; these are the canonical values.
const (
	VisibilityVisible     = "" // shown unconditionally (default)
	VisibilityConditional = "conditional"
	VisibilityHidden      = "hidden"
	VisibilityPrintOnly   = "print-only"
	VisibilityScreenOnly  = "screen-only"
)

// Relation types — the kinds of cross-block edges a RelationAnnotation carries.
// The set is open; these are the canonical values.
const (
	RelCaptionOf  = "caption-of"  // a caption block → the figure/table it describes
	RelFootnoteOf = "footnote-of" // a footnote body → its in-text marker
	RelLabelFor   = "label-for"   // a label → the field/input it labels
	RelTriggers   = "triggers"    // a button/link → the modal/panel it opens
	RelReferences = "references"  // a generic cross-reference
	RelContinues  = "continues"   // a fragment → the prior fragment of the same logical flow it threads from (DocLang <thread>/<h_thread>: content split across columns/pages)
)

// StructureAnnotation is the block-scoped record of a block's logical role.
// It is positionless (block-scoped), so a source rewrite never invalidates it.
type StructureAnnotation struct {
	// Role is the normalized semantic role (see Role* constants). "" = unset.
	Role string `json:"role,omitempty"`
	// Layer is the layout layer / plane (see Layer* constants). "" = body/unspecified.
	Layer string `json:"layer,omitempty"`
	// Visibility is the presence condition (see Visibility* constants). "" =
	// visible/unconditional.
	Visibility string `json:"visibility,omitempty"`
	// Level is the nesting/heading level where meaningful (heading 1–6, list
	// nesting depth). 0 = unset/not applicable.
	Level int `json:"level,omitempty"`
	// ReadingOrder is an explicit reading-order index when the source provides
	// one. 0 = unset; fall back to Part-stream order.
	ReadingOrder int `json:"readingOrder,omitempty"`
	// ColSpan / RowSpan are the merged-cell extents of a table cell (DocLang
	// lcel/ucel/xcel). 0 or 1 = a normal single cell. Carried so spanned grids
	// reconstruct aligned; the Structure (G) axis certifies table *topology* at
	// G3, not span fidelity (see docs/internals/format-maturity.md §2.7).
	ColSpan int `json:"colSpan,omitempty"`
	RowSpan int `json:"rowSpan,omitempty"`
}

// TypeName implements Payload.
func (*StructureAnnotation) TypeName() string { return AnnoStructure }

// Rect is an axis-aligned bounding box in the coordinate space named by the
// owning GeometryAnnotation's Resolution/Origin.
type Rect struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
	W float64 `json:"w"`
	H float64 `json:"h"`
}

// GeometryAnnotation is the block-scoped record of where a block sits on a
// rendered page. It exists only for formats with intrinsic spatial layout
// (PDF, PPTX slide coordinates, XLSX cell grid, rendered HTML) or for content
// ingested from a layout-aware source (Docling/DocLang). It is read-only
// display/reconstruction metadata — native format writers ignore it; only the
// visual layout view and layout-target/DocLang serializers consume it.
type GeometryAnnotation struct {
	// Page is the 1-based page number. 0 = unpaginated/unknown.
	Page int `json:"page,omitempty"`
	// BBox is the bounding box (x_min, y_min, width, height) from the top-left.
	BBox Rect `json:"bbox"`
	// Resolution is the edge length of the normalized coordinate grid (DocLang
	// uses 512). 0 = coordinates are absolute pixels/points. When ResolutionY
	// is unset, Resolution is the bound on both axes.
	Resolution int `json:"resolution,omitempty"`
	// ResolutionY is the per-axis vertical bound when the grid is not square
	// (DocLang location@resolution defaults to default_resolution@width|height,
	// which may differ). 0 = the grid is square and Resolution bounds both axes;
	// when set, Resolution bounds X and ResolutionY bounds Y.
	ResolutionY int `json:"resolutionY,omitempty"`
	// Origin names the coordinate origin; "" defaults to "top-left".
	Origin string `json:"origin,omitempty"`
	// Z is the stacking order within the plane (higher = nearer the viewer). 0 =
	// unset/base plane; meaningful only where strata stack (e.g. overlays).
	Z int `json:"z,omitempty"`
	// SourceRef is an optional provenance pointer back to the originating item
	// (e.g. a DoclingDocument JSON pointer) for debugging/round-trip.
	SourceRef string `json:"sourceRef,omitempty"`
	// Glyphs is the optional per-character geometry within this block, in the
	// same coordinate space/origin as BBox. Empty by default; populated only
	// when a reader is asked for glyph-level geometry (e.g. the PDFium reader's
	// "glyphs" option). Lets a consumer render character-precise highlights
	// while blocks stay meaningful text units.
	Glyphs []GlyphBox `json:"glyphs,omitempty"`
}

// GlyphBox is one character's text and bounding box, used for per-glyph geometry
// (GeometryAnnotation.Glyphs).
type GlyphBox struct {
	Text string `json:"text,omitempty"`
	BBox Rect   `json:"bbox"`
}

// TypeName implements Payload.
func (*GeometryAnnotation) TypeName() string { return AnnoGeometry }

// TimingAnnotation is the block-scoped record of where a block sits in timed
// media — the temporal anchor facet, counterpart of GeometryAnnotation's spatial
// facet. Times are milliseconds from the start of the media. A block carries
// whichever facets its medium defines: on-screen text in a video frame carries
// both a GeometryAnnotation (where on the frame) and a TimingAnnotation (when).
// It is read-only reconstruction metadata: timed-text writers (WebVTT/SubRip/
// TTML) consume it; non-timed writers ignore it.
type TimingAnnotation struct {
	// StartMS is the cue start in milliseconds from media start.
	StartMS int64 `json:"startMs"`
	// EndMS is the cue end in milliseconds; 0 = open/unknown.
	EndMS int64 `json:"endMs,omitempty"`
	// SourceRef is an optional provenance pointer (e.g. a cue id) for round-trip.
	SourceRef string `json:"sourceRef,omitempty"`
}

// TypeName implements Payload.
func (*TimingAnnotation) TypeName() string { return AnnoTiming }

// Relation is a single typed edge from the owning block to another block,
// group, or layer (by ID). Edges capture non-containment relationships that the
// Layer/Group tree cannot — a caption's figure, a footnote's marker, a label's
// field, a trigger's modal.
type Relation struct {
	// Type is the edge kind (see Rel* constants).
	Type string `json:"type"`
	// Target is the ID of the related Block/Group/Layer.
	Target string `json:"target"`
}

// RelationAnnotation is the block-scoped set of outgoing relationship edges.
// Positionless (block-scoped), so a source rewrite never invalidates it.
type RelationAnnotation struct {
	Relations []Relation `json:"relations"`
}

// TypeName implements Payload.
func (*RelationAnnotation) TypeName() string { return AnnoRelations }

// --- Block accessors (field-like API over the stand-off payloads) ---

// Structure returns the block's StructureAnnotation, or (nil, false).
func (b *Block) Structure() (*StructureAnnotation, bool) {
	return AnnoAs[*StructureAnnotation](b, AnnoStructure)
}

// SetStructure stores the block's StructureAnnotation.
func (b *Block) SetStructure(s *StructureAnnotation) { b.SetAnno(AnnoStructure, s) }

// structureOrNew returns the existing StructureAnnotation or a fresh one.
func (b *Block) structureOrNew() *StructureAnnotation {
	if s, ok := b.Structure(); ok && s != nil {
		return s
	}
	return &StructureAnnotation{}
}

// SemanticRole returns the block's normalized role, or "" if unset.
func (b *Block) SemanticRole() string {
	if s, ok := b.Structure(); ok && s != nil {
		return s.Role
	}
	return ""
}

// SetSemanticRole sets the block's normalized role (upserting the structure
// annotation). An optional level applies to roles that carry one (heading,
// list nesting); pass 0 when not applicable.
func (b *Block) SetSemanticRole(role string, level int) {
	s := b.structureOrNew()
	s.Role = role
	s.Level = level
	b.SetStructure(s)
}

// LayoutLayer returns the block's layout layer, or "" if unset.
func (b *Block) LayoutLayer() string {
	if s, ok := b.Structure(); ok && s != nil {
		return s.Layer
	}
	return ""
}

// SetLayoutLayer sets the block's layout layer (upserting the structure
// annotation).
func (b *Block) SetLayoutLayer(layer string) {
	s := b.structureOrNew()
	s.Layer = layer
	b.SetStructure(s)
}

// Visibility returns the block's presence condition, or "" (visible) if unset.
func (b *Block) Visibility() string {
	if s, ok := b.Structure(); ok && s != nil {
		return s.Visibility
	}
	return ""
}

// SetVisibility sets the block's presence condition (upserting the structure
// annotation).
func (b *Block) SetVisibility(v string) {
	s := b.structureOrNew()
	s.Visibility = v
	b.SetStructure(s)
}

// Geometry returns the block's GeometryAnnotation, or (nil, false).
func (b *Block) Geometry() (*GeometryAnnotation, bool) {
	return AnnoAs[*GeometryAnnotation](b, AnnoGeometry)
}

// SetGeometry stores the block's GeometryAnnotation.
func (b *Block) SetGeometry(g *GeometryAnnotation) { b.SetAnno(AnnoGeometry, g) }

// Timing returns the block's TimingAnnotation, or (nil, false).
func (b *Block) Timing() (*TimingAnnotation, bool) {
	return AnnoAs[*TimingAnnotation](b, AnnoTiming)
}

// SetTiming stores the block's TimingAnnotation.
func (b *Block) SetTiming(t *TimingAnnotation) { b.SetAnno(AnnoTiming, t) }

// Relations returns the block's relationship edges, or (nil, false).
func (b *Block) Relations() (*RelationAnnotation, bool) {
	return AnnoAs[*RelationAnnotation](b, AnnoRelations)
}

// setProp upserts a Block.Properties entry, allocating the map if needed.
func (b *Block) setProp(key, val string) {
	if b.Properties == nil {
		b.Properties = map[string]string{}
	}
	b.Properties[key] = val
}

// CheckboxChecked reports whether a RoleCheckbox block is selected.
func (b *Block) CheckboxChecked() bool { return b.Properties[PropCheckboxChecked] == "true" }

// SetCheckboxChecked records a checkbox's selected state (PropCheckboxChecked).
func (b *Block) SetCheckboxChecked(checked bool) {
	b.setProp(PropCheckboxChecked, strconv.FormatBool(checked))
}

// FieldFillable reports whether a RoleValue block is an editable/fillable field.
func (b *Block) FieldFillable() bool { return b.Properties[PropFieldFillable] == "true" }

// SetFieldFillable records a value field's fillable-vs-read-only state.
func (b *Block) SetFieldFillable(fillable bool) {
	b.setProp(PropFieldFillable, strconv.FormatBool(fillable))
}

// CodeLanguage returns a RoleCode block's language key, or "".
func (b *Block) CodeLanguage() string { return b.Properties[PropCodeLanguage] }

// SetCodeLanguage records a code block's programming-language key.
func (b *Block) SetCodeLanguage(lang string) { b.setProp(PropCodeLanguage, lang) }

// PictureSubclass returns a RolePicture block's fine subclass (e.g. chart kind).
func (b *Block) PictureSubclass() string { return b.Properties[PropPictureSubclass] }

// SetPictureSubclass records a picture's fine subclass.
func (b *Block) SetPictureSubclass(sub string) { b.setProp(PropPictureSubclass, sub) }

// TableHeaderKind returns a RoleTableHeader cell's OTSL sub-kind, or "".
func (b *Block) TableHeaderKind() string { return b.Properties[PropTableHeaderKind] }

// SetTableHeaderKind records a header cell's OTSL sub-kind (TableHeader* consts).
func (b *Block) SetTableHeaderKind(kind string) { b.setProp(PropTableHeaderKind, kind) }

// AddRelation appends a typed edge from this block to the target ID (upserting
// the relation annotation). Duplicate (type,target) pairs are ignored.
func (b *Block) AddRelation(relType, target string) {
	r := &RelationAnnotation{}
	if existing, ok := b.Relations(); ok && existing != nil {
		r = existing
		for _, e := range r.Relations {
			if e.Type == relType && e.Target == target {
				return
			}
		}
	}
	r.Relations = append(r.Relations, Relation{Type: relType, Target: target})
	b.SetAnno(AnnoRelations, r)
}
