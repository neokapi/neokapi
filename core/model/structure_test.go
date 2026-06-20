package model

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestSemanticRoleAccessors(t *testing.T) {
	b := NewBlock("b1", "Chapter One")

	if got := b.SemanticRole(); got != "" {
		t.Fatalf("new block role = %q, want empty", got)
	}

	b.SetSemanticRole(RoleHeading, 1)
	if got := b.SemanticRole(); got != RoleHeading {
		t.Fatalf("role = %q, want %q", got, RoleHeading)
	}
	s, ok := b.Structure()
	if !ok || s == nil {
		t.Fatal("Structure() returned no annotation after SetSemanticRole")
	}
	if s.Level != 1 {
		t.Fatalf("level = %d, want 1", s.Level)
	}

	// SetLayoutLayer must upsert, not clobber, the existing role.
	b.SetLayoutLayer(LayerFurniture)
	if got := b.SemanticRole(); got != RoleHeading {
		t.Fatalf("role after SetLayoutLayer = %q, want %q (must not clobber)", got, RoleHeading)
	}
	if got := b.LayoutLayer(); got != LayerFurniture {
		t.Fatalf("layer = %q, want %q", got, LayerFurniture)
	}
}

func TestGeometryAccessors(t *testing.T) {
	b := NewBlock("b2", "caption text")
	if _, ok := b.Geometry(); ok {
		t.Fatal("new block unexpectedly has geometry")
	}
	g := &GeometryAnnotation{Page: 3, BBox: Rect{X: 10, Y: 20, W: 100, H: 12}, Resolution: 512}
	b.SetGeometry(g)
	got, ok := b.Geometry()
	if !ok || got == nil {
		t.Fatal("Geometry() returned nothing after SetGeometry")
	}
	if got.Page != 3 || got.BBox.W != 100 || got.Resolution != 512 {
		t.Fatalf("geometry round-trip mismatch: %+v", got)
	}
}

func TestGeometryPerAxisResolution(t *testing.T) {
	b := NewBlock("b2y", "asymmetric grid")
	b.SetGeometry(&GeometryAnnotation{BBox: Rect{X: 0, Y: 0, W: 800, H: 400}, Resolution: 1000, ResolutionY: 512})
	got, _ := b.Geometry()
	if got.Resolution != 1000 || got.ResolutionY != 512 {
		t.Fatalf("per-axis resolution mismatch: x=%d y=%d, want 1000/512", got.Resolution, got.ResolutionY)
	}
}

func TestStructureColRowSpan(t *testing.T) {
	b := NewBlock("cell", "merged")
	b.SetStructure(&StructureAnnotation{Role: RoleTableCell, ColSpan: 3, RowSpan: 2})
	s, _ := b.Structure()
	if s.ColSpan != 3 || s.RowSpan != 2 {
		t.Fatalf("span mismatch: col=%d row=%d, want 3/2", s.ColSpan, s.RowSpan)
	}
}

func TestBlockPropertyConventions(t *testing.T) {
	b := NewBlock("field", "Status")
	if b.CheckboxChecked() || b.FieldFillable() {
		t.Fatal("new block should default to unchecked / read-only")
	}
	b.SetCheckboxChecked(true)
	b.SetFieldFillable(true)
	b.SetCodeLanguage("Python")
	b.SetPictureSubclass("bar_chart")
	b.SetTableHeaderKind(TableHeaderColumn)
	if !b.CheckboxChecked() {
		t.Errorf("checkbox checked = false, want true (%q)", b.Properties[PropCheckboxChecked])
	}
	if !b.FieldFillable() {
		t.Errorf("field fillable = false, want true")
	}
	if b.CodeLanguage() != "Python" {
		t.Errorf("code language = %q, want Python", b.CodeLanguage())
	}
	if b.PictureSubclass() != "bar_chart" {
		t.Errorf("picture subclass = %q, want bar_chart", b.PictureSubclass())
	}
	if b.TableHeaderKind() != TableHeaderColumn {
		t.Errorf("header kind = %q, want column", b.TableHeaderKind())
	}
}

func TestContinuesRelation(t *testing.T) {
	b := NewBlock("frag2", "…continued paragraph")
	b.AddRelation(RelContinues, "frag1")
	r, ok := b.Relations()
	if !ok || len(r.Relations) != 1 || r.Relations[0].Type != RelContinues || r.Relations[0].Target != "frag1" {
		t.Fatalf("continues relation = %+v", r)
	}
}

func TestVisibilityAccessors(t *testing.T) {
	b := NewBlock("b3", "modal body")
	if got := b.Visibility(); got != VisibilityVisible {
		t.Fatalf("new block visibility = %q, want empty", got)
	}
	// Visibility must upsert alongside plane without clobbering it.
	b.SetLayoutLayer(LayerOverlay)
	b.SetVisibility(VisibilityConditional)
	if got := b.Visibility(); got != VisibilityConditional {
		t.Fatalf("visibility = %q, want %q", got, VisibilityConditional)
	}
	if got := b.LayoutLayer(); got != LayerOverlay {
		t.Fatalf("layer after SetVisibility = %q, want %q (must not clobber)", got, LayerOverlay)
	}
}

func TestRelationAccessors(t *testing.T) {
	b := NewBlock("cap1", "Figure 1: the pipeline")
	if _, ok := b.Relations(); ok {
		t.Fatal("new block unexpectedly has relations")
	}
	b.AddRelation(RelCaptionOf, "fig1")
	b.AddRelation(RelCaptionOf, "fig1") // duplicate, must be ignored
	b.AddRelation(RelReferences, "sec2")
	r, ok := b.Relations()
	if !ok || r == nil {
		t.Fatal("Relations() returned nothing after AddRelation")
	}
	if len(r.Relations) != 2 {
		t.Fatalf("relation count = %d, want 2 (dup must be deduped): %+v", len(r.Relations), r.Relations)
	}
	if r.Relations[0].Type != RelCaptionOf || r.Relations[0].Target != "fig1" {
		t.Fatalf("first relation = %+v", r.Relations[0])
	}
}

// The structural payloads must rehydrate through the global payload registry,
// which is what lets the wire/store layers serialize them generically with no
// schema change.
func TestStructuralPayloadsRegistered(t *testing.T) {
	for _, tn := range []string{AnnoStructure, AnnoGeometry, AnnoRelations} {
		p, ok := NewPayload(tn)
		if !ok || p == nil {
			t.Fatalf("payload %q not registered", tn)
		}
		if p.TypeName() != tn {
			t.Fatalf("payload %q TypeName() = %q", tn, p.TypeName())
		}
	}
}

func TestStructuralPayloadsJSONRoundTrip(t *testing.T) {
	s := &StructureAnnotation{Role: RoleListItem, Layer: LayerOverlay, Visibility: VisibilityConditional, Level: 2, ReadingOrder: 5}
	data, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	var back StructureAnnotation
	if err := json.Unmarshal(data, &back); err != nil {
		t.Fatal(err)
	}
	if back != *s {
		t.Fatalf("structure round-trip: got %+v want %+v", back, *s)
	}

	g := &GeometryAnnotation{Page: 1, BBox: Rect{X: 1, Y: 2, W: 3, H: 4}, Resolution: 512, Origin: "top-left"}
	data, err = json.Marshal(g)
	if err != nil {
		t.Fatal(err)
	}
	var gback GeometryAnnotation
	if err := json.Unmarshal(data, &gback); err != nil {
		t.Fatal(err)
	}
	// reflect.DeepEqual (not !=): GeometryAnnotation now has a Glyphs slice.
	if !reflect.DeepEqual(gback, *g) {
		t.Fatalf("geometry round-trip: got %+v want %+v", gback, *g)
	}

	r := &RelationAnnotation{Relations: []Relation{{Type: RelCaptionOf, Target: "fig1"}, {Type: RelLabelFor, Target: "in7"}}}
	data, err = json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	var rback RelationAnnotation
	if err := json.Unmarshal(data, &rback); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(rback, *r) {
		t.Fatalf("relation round-trip: got %+v want %+v", rback, *r)
	}
}
