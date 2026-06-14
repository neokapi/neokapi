package model

import (
	"encoding/json"
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

// The structural payloads must rehydrate through the global payload registry,
// which is what lets the wire/store layers serialize them generically with no
// schema change.
func TestStructuralPayloadsRegistered(t *testing.T) {
	for _, tn := range []string{AnnoStructure, AnnoGeometry} {
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
	s := &StructureAnnotation{Role: RoleListItem, Layer: LayerBody, Level: 2, ReadingOrder: 5}
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
	if gback != *g {
		t.Fatalf("geometry round-trip: got %+v want %+v", gback, *g)
	}
}
