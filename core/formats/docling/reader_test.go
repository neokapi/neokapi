package docling_test

import (
	"os"
	"strings"
	"testing"

	doclingfmt "github.com/neokapi/neokapi/core/formats/docling"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
)

func readSample(t *testing.T) []*model.Block {
	t.Helper()
	data, err := os.ReadFile("testdata/sample.docling.json")
	if err != nil {
		t.Fatal(err)
	}
	ctx := t.Context()
	r := doclingfmt.NewReader()
	if err := r.Open(ctx, testutil.RawDocFromString(string(data), model.LocaleEnglish)); err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	return testutil.CollectBlocks(t, r.Read(ctx))
}

func TestReadDoclingRolesAndGeometry(t *testing.T) {
	blocks := readSample(t)
	find := func(text string) *model.Block {
		for _, b := range blocks {
			if strings.TrimSpace(b.SourceText()) == text {
				return b
			}
		}
		t.Fatalf("no block with text %q (have: %v)", text, testutil.BlockTexts(blocks))
		return nil
	}

	// Title.
	if r := find("Annual Report").SemanticRole(); r != model.RoleTitle {
		t.Errorf("title role = %q, want title", r)
	}

	// Title geometry: TOPLEFT bbox {l:72,t:60,r:500,b:92} → {72,60,428,32}, page 1.
	g, ok := find("Annual Report").Geometry()
	if !ok {
		t.Fatalf("title missing geometry")
	}
	if g.Page != 1 || g.BBox != (model.Rect{X: 72, Y: 60, W: 428, H: 32}) || g.Origin != "top-left" {
		t.Errorf("title geometry = page %d %+v origin %q, want page 1 {72,60,428,32} top-left", g.Page, g.BBox, g.Origin)
	}
	if g.SourceRef != "#/texts/0" {
		t.Errorf("title geometry sourceRef = %q, want #/texts/0", g.SourceRef)
	}

	// section_header carries heading role + its explicit level.
	if s, ok := find("Overview").Structure(); !ok || s.Role != model.RoleHeading || s.Level != 2 {
		t.Errorf("section_header structure = %+v, want role=heading level=2", s)
	}

	// Plain paragraph.
	if r := find("Some intro text.").SemanticRole(); r != model.RoleParagraph {
		t.Errorf("paragraph role = %q, want paragraph", r)
	}

	// List items.
	for _, item := range []string{"First item", "Second item"} {
		if r := find(item).SemanticRole(); r != model.RoleListItem {
			t.Errorf("list item %q role = %q, want list-item", item, r)
		}
	}

	// Table: header cells vs data cells.
	for _, h := range []string{"Region", "Sales"} {
		if r := find(h).SemanticRole(); r != model.RoleTableHeader {
			t.Errorf("header cell %q role = %q, want table-header", h, r)
		}
	}
	for _, cell := range []string{"EU", "100"} {
		if r := find(cell).SemanticRole(); r != model.RoleTableCell {
			t.Errorf("data cell %q role = %q, want table-cell", cell, r)
		}
	}

	// Captions (referenced from table & picture, not in body.children) become
	// caption-role blocks.
	if r := find("Table 1: Sales by region").SemanticRole(); r != model.RoleCaption {
		t.Errorf("table caption role = %q, want caption", r)
	}
	if r := find("Figure 1: Quarterly trend").SemanticRole(); r != model.RoleCaption {
		t.Errorf("picture caption role = %q, want caption", r)
	}

	// page_header → furniture layout layer.
	ph := find("Confidential")
	if ph.SemanticRole() != model.RolePageHeader {
		t.Errorf("page_header role = %q, want page-header", ph.SemanticRole())
	}
	if ph.LayoutLayer() != model.LayerFurniture {
		t.Errorf("page_header layer = %q, want furniture", ph.LayoutLayer())
	}
}

// TestReadDoclingGroupStructure verifies the list/table/row/picture grouping is
// emitted as balanced Group start/end parts in reading order.
func TestReadDoclingGroupStructure(t *testing.T) {
	data, err := os.ReadFile("testdata/sample.docling.json")
	if err != nil {
		t.Fatal(err)
	}
	ctx := t.Context()
	r := doclingfmt.NewReader()
	if err := r.Open(ctx, testutil.RawDocFromString(string(data), model.LocaleEnglish)); err != nil {
		t.Fatal(err)
	}
	defer r.Close()

	var starts, ends int
	var groupTypes []string
	for res := range r.Read(ctx) {
		if res.Error != nil {
			t.Fatal(res.Error)
		}
		switch res.Part.Type {
		case model.PartGroupStart:
			starts++
			if g, ok := res.Part.Resource.(*model.GroupStart); ok {
				groupTypes = append(groupTypes, g.Type)
			}
		case model.PartGroupEnd:
			ends++
		}
	}
	if starts != ends {
		t.Errorf("unbalanced groups: %d starts, %d ends", starts, ends)
	}
	// list, table, table-row x2, picture.
	want := map[string]int{"list": 1, "table": 1, "table-row": 2, "picture": 1}
	got := map[string]int{}
	for _, gt := range groupTypes {
		got[gt]++
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("group type %q count = %d, want %d (all: %v)", k, got[k], v, groupTypes)
		}
	}
}

func TestDoclingRejectsNonDocling(t *testing.T) {
	ctx := t.Context()
	r := doclingfmt.NewReader()
	if err := r.Open(ctx, testutil.RawDocFromString(`{"schema_name":"SomethingElse"}`, model.LocaleEnglish)); err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	var sawErr bool
	for res := range r.Read(ctx) {
		if res.Error != nil {
			sawErr = true
		}
	}
	if !sawErr {
		t.Error("expected an error for non-DoclingDocument schema_name")
	}
}
