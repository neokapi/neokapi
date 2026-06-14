package doclang_test

import (
	"os"
	"strings"
	"testing"

	doclangfmt "github.com/neokapi/neokapi/core/formats/doclang"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
)

func readSample(t *testing.T) []*model.Block {
	t.Helper()
	data, err := os.ReadFile("testdata/sample.dclg.xml")
	if err != nil {
		t.Fatal(err)
	}
	ctx := t.Context()
	r := doclangfmt.NewReader()
	if err := r.Open(ctx, testutil.RawDocFromString(string(data), model.LocaleEnglish)); err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	return testutil.CollectBlocks(t, r.Read(ctx))
}

func TestReadDocLangRolesAndGeometry(t *testing.T) {
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

	// Heading: role + level + geometry.
	h := find("Quarterly Report")
	if s, ok := h.Structure(); !ok || s.Role != model.RoleHeading || s.Level != 1 {
		t.Errorf("heading structure = %+v, want role=heading level=1", s)
	}
	g, ok := h.Geometry()
	if !ok {
		t.Fatalf("heading missing geometry")
	}
	if g.BBox != (model.Rect{X: 60, Y: 40, W: 392, H: 24}) || g.Resolution != 512 {
		t.Errorf("heading geometry = %+v (res %d), want {60,40,392,24} res 512", g.BBox, g.Resolution)
	}

	// Paragraph with inline bold — flattened text includes "Q1".
	p := find("This report summarizes Q1 results and lists the key action items.")
	if p.SemanticRole() != model.RoleParagraph {
		t.Errorf("paragraph role = %q, want paragraph", p.SemanticRole())
	}

	// List items (a <text> inside <list> is an item; the <ldiv> marker is dropped).
	for _, item := range []string{"Review the sales figures.", "Approve the budget."} {
		if r := find(item).SemanticRole(); r != model.RoleListItem {
			t.Errorf("list item %q role = %q, want list-item", item, r)
		}
	}

	// OTSL table: header cells + data cells.
	if r := find("Region").SemanticRole(); r != model.RoleTableHeader {
		t.Errorf("'Region' role = %q, want table-header", r)
	}
	for _, cell := range []string{"North", "1200", "South", "980"} {
		if r := find(cell).SemanticRole(); r != model.RoleTableCell {
			t.Errorf("cell %q role = %q, want table-cell", cell, r)
		}
	}
}
