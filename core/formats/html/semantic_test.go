package html_test

import (
	"bytes"
	"context"
	"strconv"
	"strings"
	"testing"

	htmlfmt "github.com/neokapi/neokapi/core/formats/html"
	"github.com/neokapi/neokapi/core/model"
)

// writeSemanticParts runs the HTML writer in its block-only (semantic export)
// mode over an ordered part stream and returns the produced HTML.
func writeSemanticParts(t *testing.T, parts ...*model.Part) string {
	t.Helper()
	var buf bytes.Buffer
	w := htmlfmt.NewWriter()
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

func roleBlock(id, text, role string, level int) *model.Part {
	b := model.NewBlock(id, text)
	b.SetSemanticRole(role, level)
	return &model.Part{Type: model.PartBlock, Resource: b}
}

func groupStart(id, typ string) *model.Part {
	return &model.Part{Type: model.PartGroupStart, Resource: &model.GroupStart{ID: id, Name: typ, Type: typ}}
}

func groupEnd(id string) *model.Part {
	return &model.Part{Type: model.PartGroupEnd, Resource: &model.GroupEnd{ID: id}}
}

// layerStart declares the document's source locale, as a reader emits before
// the block stream; the writer uses it for the exported <html lang> attribute.
func layerStart(loc model.LocaleID) *model.Part {
	return &model.Part{Type: model.PartLayerStart, Resource: &model.Layer{Locale: loc}}
}

// TestSemanticExport_Structure verifies a role/group event stream (as a
// DocLang/Docling/DOCX source would produce) renders clean structured HTML.
func TestSemanticExport_Structure(t *testing.T) {
	out := writeSemanticParts(t,
		roleBlock("t", "My Report", model.RoleTitle, 0),
		roleBlock("h", "Overview", model.RoleHeading, 2),
		roleBlock("p", "Intro text.", model.RoleParagraph, 0),
		// explicit list group
		groupStart("g1", "list"),
		roleBlock("li1", "First", model.RoleListItem, 0),
		roleBlock("li2", "Second", model.RoleListItem, 0),
		groupEnd("g1"),
		// table with a header row and a data row
		groupStart("g2", "table"),
		groupStart("g3", "table-row"),
		roleBlock("c1", "Region", model.RoleTableHeader, 0),
		roleBlock("c2", "Sales", model.RoleTableHeader, 0),
		groupEnd("g3"),
		groupStart("g4", "table-row"),
		roleBlock("c3", "EU", model.RoleTableCell, 0),
		roleBlock("c4", "100", model.RoleTableCell, 0),
		groupEnd("g4"),
		groupEnd("g2"),
	)

	for _, want := range []string{
		"<h1>My Report</h1>",
		"<h2>Overview</h2>",
		"<p>Intro text.</p>",
		"<ul><li>First</li><li>Second</li></ul>",
		"<table>",
		"<tr><th>Region</th><th>Sales</th></tr>",
		"<tr><td>EU</td><td>100</td></tr>",
		"</table>",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in output; got:\n%s", want, out)
		}
	}
}

// TestSemanticExport_DocumentScaffold verifies the cross-format export wraps
// its body content in a complete, valid HTML5 document — doctype, <html> with a
// lang attribute, a <head> with charset + <title>, and a <body> — rather than
// emitting a bare fragment (so `kconv x.csv --to html` yields a standalone page).
func TestSemanticExport_DocumentScaffold(t *testing.T) {
	out := writeSemanticParts(t,
		layerStart(model.LocaleEnglish),
		roleBlock("h", "My Report", model.RoleTitle, 0),
		roleBlock("p", "Body text.", model.RoleParagraph, 0),
	)
	for _, want := range []string{
		"<!DOCTYPE html>",
		"<html lang=\"en\">",
		"<meta charset=\"utf-8\">",
		"<title>My Report</title>", // title taken from the first heading/title block
		"<body>",
		"<h1>My Report</h1>",
		"<p>Body text.</p>",
		"</body>",
		"</html>",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in output; got:\n%s", want, out)
		}
	}
	// Body content must sit between <body> and </body>.
	if !(strings.Index(out, "<body>") < strings.Index(out, "<h1>My Report</h1>") &&
		strings.Index(out, "<h1>My Report</h1>") < strings.Index(out, "</body>")) {
		t.Errorf("body content not enclosed in <body>; got:\n%s", out)
	}
}

// TestSemanticExport_DocumentTitleFallback verifies the <title> falls back to a
// generic value when the content carries no heading (e.g. a headerless CSV).
func TestSemanticExport_DocumentTitleFallback(t *testing.T) {
	out := writeSemanticParts(t,
		roleBlock("p", "Just a paragraph.", model.RoleParagraph, 0),
	)
	if !strings.Contains(out, "<title>Document</title>") {
		t.Errorf("expected fallback <title>Document</title>; got:\n%s", out)
	}
}

// colCell builds a table cell carrying an explicit 0-based column index, as
// the CSV reader produces (it skips empty cells, leaving holes in a row).
func colCell(id, text, role string, col int) *model.Part {
	b := model.NewBlock(id, text)
	b.SetSemanticRole(role, 0)
	b.Properties["column"] = strconv.Itoa(col)
	return &model.Part{Type: model.PartBlock, Resource: b}
}

// TestSemanticExport_TableColumnGaps verifies that a row missing a column (a
// skipped empty CSV cell) is padded so the grid stays rectangular. The header
// row establishes the width.
func TestSemanticExport_TableColumnGaps(t *testing.T) {
	out := writeSemanticParts(t,
		groupStart("tbl", "table"),
		groupStart("trh", "table-row"),
		colCell("h0", "a", model.RoleTableHeader, 0),
		colCell("h1", "b", model.RoleTableHeader, 1),
		colCell("h2", "c", model.RoleTableHeader, 2),
		groupEnd("trh"),
		groupStart("tr1", "table-row"),
		colCell("c0", "x", model.RoleTableCell, 0),
		colCell("c2", "z", model.RoleTableCell, 2), // column 1 missing
		groupEnd("tr1"),
		groupEnd("tbl"),
	)

	if !strings.Contains(out, "<tr><th>a</th><th>b</th><th>c</th></tr>") {
		t.Errorf("header row not rendered; got:\n%s", out)
	}
	if !strings.Contains(out, "<tr><td>x</td><td></td><td>z</td></tr>") {
		t.Errorf("missing middle column not padded; got:\n%s", out)
	}
}

// TestSemanticExport_TableShortRowPadded verifies a trailing-short row is padded
// out to the table width.
func TestSemanticExport_TableShortRowPadded(t *testing.T) {
	out := writeSemanticParts(t,
		groupStart("tbl", "table"),
		groupStart("trh", "table-row"),
		colCell("h0", "a", model.RoleTableHeader, 0),
		colCell("h1", "b", model.RoleTableHeader, 1),
		groupEnd("trh"),
		groupStart("tr1", "table-row"),
		colCell("c0", "x", model.RoleTableCell, 0), // column 1 absent at the end
		groupEnd("tr1"),
		groupEnd("tbl"),
	)
	if !strings.Contains(out, "<tr><td>x</td><td></td></tr>") {
		t.Errorf("trailing missing column not padded; got:\n%s", out)
	}
}

// TestSemanticExport_BareListItems verifies list items with NO surrounding list
// group (as DOCX produces) are auto-wrapped in a single <ul>, and that a
// following non-list block closes it.
func TestSemanticExport_BareListItems(t *testing.T) {
	out := writeSemanticParts(t,
		roleBlock("li1", "One", model.RoleListItem, 0),
		roleBlock("li2", "Two", model.RoleListItem, 0),
		roleBlock("p", "After", model.RoleParagraph, 0),
	)
	if !strings.Contains(out, "<ul><li>One</li><li>Two</li></ul>") {
		t.Errorf("bare list items not auto-wrapped in <ul>; got:\n%s", out)
	}
	if !strings.Contains(out, "<p>After</p>") {
		t.Errorf("trailing paragraph missing/misplaced; got:\n%s", out)
	}
	// The <ul> must close before the <p>.
	if strings.Index(out, "</ul>") > strings.Index(out, "<p>After") {
		t.Errorf("auto <ul> not closed before following paragraph; got:\n%s", out)
	}
}

// TestSemanticExport_Escaping verifies text content is HTML-escaped and known
// inline formatting renders as HTML tags.
func TestSemanticExport_Escaping(t *testing.T) {
	plain := model.NewBlock("p", "a < b && c > d")
	plain.SetSemanticRole(model.RoleParagraph, 0)

	// bold inline run via vocabulary type.
	bold := &model.Block{ID: "b", Translatable: true, Source: []model.Run{
		{Text: &model.TextRun{Text: "see "}},
		{PcOpen: &model.PcOpenRun{ID: "1", Type: "fmt:bold"}},
		{Text: &model.TextRun{Text: "this"}},
		{PcClose: &model.PcCloseRun{}},
	}}
	bold.SetSemanticRole(model.RoleParagraph, 0)

	out := writeSemanticParts(t,
		&model.Part{Type: model.PartBlock, Resource: plain},
		&model.Part{Type: model.PartBlock, Resource: bold},
	)
	if !strings.Contains(out, "a &lt; b &amp;&amp; c &gt; d") {
		t.Errorf("text not HTML-escaped; got:\n%s", out)
	}
	if !strings.Contains(out, "see <strong>this</strong>") {
		t.Errorf("inline bold not rendered as <strong>; got:\n%s", out)
	}
}
