package model

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
	// AnnoStructure carries a *StructureAnnotation (role, layout layer, level).
	AnnoStructure = "structure"
	// AnnoGeometry carries a *GeometryAnnotation (page + bounding box).
	AnnoGeometry = "geometry"
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
)

// Layout layers, aligned with DocLang's <layer> values. Body is the main
// content; furniture is repeated/supplementary matter (running headers/footers,
// page numbers); background is watermarks and the like.
const (
	LayerBody       = "body"
	LayerFurniture  = "furniture"
	LayerBackground = "background"
)

// StructureAnnotation is the block-scoped record of a block's logical role.
// It is positionless (block-scoped), so a source rewrite never invalidates it.
type StructureAnnotation struct {
	// Role is the normalized semantic role (see Role* constants). "" = unset.
	Role string `json:"role,omitempty"`
	// Layer is the layout layer (see Layer* constants). "" = body/unspecified.
	Layer string `json:"layer,omitempty"`
	// Level is the nesting/heading level where meaningful (heading 1–6, list
	// nesting depth). 0 = unset/not applicable.
	Level int `json:"level,omitempty"`
	// ReadingOrder is an explicit reading-order index when the source provides
	// one. 0 = unset; fall back to Part-stream order.
	ReadingOrder int `json:"readingOrder,omitempty"`
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
	// uses 512). 0 = coordinates are absolute pixels/points.
	Resolution int `json:"resolution,omitempty"`
	// Origin names the coordinate origin; "" defaults to "top-left".
	Origin string `json:"origin,omitempty"`
	// SourceRef is an optional provenance pointer back to the originating item
	// (e.g. a DoclingDocument JSON pointer) for debugging/round-trip.
	SourceRef string `json:"sourceRef,omitempty"`
}

// TypeName implements Payload.
func (*GeometryAnnotation) TypeName() string { return AnnoGeometry }

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

// Geometry returns the block's GeometryAnnotation, or (nil, false).
func (b *Block) Geometry() (*GeometryAnnotation, bool) {
	return AnnoAs[*GeometryAnnotation](b, AnnoGeometry)
}

// SetGeometry stores the block's GeometryAnnotation.
func (b *Block) SetGeometry(g *GeometryAnnotation) { b.SetAnno(AnnoGeometry, g) }
