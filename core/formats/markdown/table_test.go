package markdown

import (
	"bytes"
	"context"
	"strconv"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/model"
)

// writeParts runs the Markdown writer in its no-skeleton (semantic export)
// mode over an ordered Part stream and returns the produced Markdown.
func writeParts(t *testing.T, parts ...*model.Part) string {
	t.Helper()
	var buf bytes.Buffer
	w := NewWriter()
	if err := w.SetOutputWriter(&buf); err != nil {
		t.Fatalf("SetOutputWriter: %v", err)
	}
	ch := make(chan *model.Part)
	go func() {
		for _, p := range parts {
			ch <- p
		}
		close(ch)
	}()
	if err := w.Write(context.Background(), ch); err != nil {
		t.Fatalf("Write: %v", err)
	}
	return buf.String()
}

func groupStart(id, typ string, props map[string]string) *model.Part {
	return &model.Part{Type: model.PartGroupStart, Resource: &model.GroupStart{ID: id, Name: typ, Type: typ, Properties: props}}
}

func groupEnd(id string) *model.Part {
	return &model.Part{Type: model.PartGroupEnd, Resource: &model.GroupEnd{ID: id}}
}

func headerCell(col int, text string) *model.Part {
	b := model.NewBlock("h"+text, text)
	b.Translatable = false
	b.SetSemanticRole(model.RoleTableHeader, 0)
	b.Properties["column"] = strconv.Itoa(col)
	return &model.Part{Type: model.PartBlock, Resource: b}
}

func dataCell(col int, text string) *model.Part {
	b := model.NewBlock("c"+text, text)
	b.SetSemanticRole(model.RoleTableCell, 0)
	b.Properties["column"] = strconv.Itoa(col)
	return &model.Part{Type: model.PartBlock, Resource: b}
}

// A column-indexed table (the CSV projection: cells carry a "column" property)
// renders as a GFM table with the flagged header row first.
func TestTableColumnIndexed(t *testing.T) {
	out := writeParts(t,
		groupStart("tbl", "table", nil),
		groupStart("trh", "table-row", map[string]string{"header": "true"}),
		headerCell(0, "name"), headerCell(1, "price"),
		groupEnd("trh"),
		groupStart("tr1", "table-row", nil),
		dataCell(0, "Apple"), dataCell(1, "1.20"),
		groupEnd("tr1"),
		groupStart("tr2", "table-row", nil),
		dataCell(0, "Banana"), dataCell(1, "0.50"),
		groupEnd("tr2"),
		groupEnd("tbl"),
	)

	want := "| name | price |\n| --- | --- |\n| Apple | 1.20 |\n| Banana | 0.50 |"
	if !strings.Contains(out, want) {
		t.Errorf("GFM table not rendered as expected.\nwant to contain:\n%s\n\ngot:\n%s", want, out)
	}
}

// A gap in a row (an empty/skipped cell, as the CSV reader produces) is padded
// by column index so the grid stays aligned.
func TestTableColumnGap(t *testing.T) {
	out := writeParts(t,
		groupStart("tbl", "table", nil),
		groupStart("trh", "table-row", map[string]string{"header": "true"}),
		headerCell(0, "a"), headerCell(1, "b"), headerCell(2, "c"),
		groupEnd("trh"),
		groupStart("tr1", "table-row", nil),
		dataCell(0, "x"), dataCell(2, "z"), // column 1 missing
		groupEnd("tr1"),
		groupEnd("tbl"),
	)

	if !strings.Contains(out, "| x |  | z |") {
		t.Errorf("missing column not padded; got:\n%s", out)
	}
}

// A sequential table (the DocLang/Docling projection: cells carry no "column"
// property) places cells in order and detects the header from the cell role.
func TestTableSequential(t *testing.T) {
	hc := func(text string) *model.Part {
		b := model.NewBlock("h"+text, text)
		b.SetSemanticRole(model.RoleTableHeader, 0)
		return &model.Part{Type: model.PartBlock, Resource: b}
	}
	dc := func(text string) *model.Part {
		b := model.NewBlock("c"+text, text)
		b.SetSemanticRole(model.RoleTableCell, 0)
		return &model.Part{Type: model.PartBlock, Resource: b}
	}
	out := writeParts(t,
		groupStart("tbl", "table", nil),
		groupStart("r0", "table-row", nil),
		hc("Region"), hc("Revenue"),
		groupEnd("r0"),
		groupStart("r1", "table-row", nil),
		dc("North"), dc("1200"),
		groupEnd("r1"),
		groupEnd("tbl"),
	)

	want := "| Region | Revenue |\n| --- | --- |\n| North | 1200 |"
	if !strings.Contains(out, want) {
		t.Errorf("sequential GFM table not rendered.\nwant to contain:\n%s\n\ngot:\n%s", want, out)
	}
}

// A pipe inside a cell is escaped so it does not break the table layout.
func TestTableCellEscaping(t *testing.T) {
	out := writeParts(t,
		groupStart("tbl", "table", nil),
		groupStart("trh", "table-row", map[string]string{"header": "true"}),
		headerCell(0, "expr"),
		groupEnd("trh"),
		groupStart("tr1", "table-row", nil),
		dataCell(0, "a|b"),
		groupEnd("tr1"),
		groupEnd("tbl"),
	)

	if !strings.Contains(out, `| a\|b |`) {
		t.Errorf("pipe not escaped in cell; got:\n%s", out)
	}
}
