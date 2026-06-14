package doclang_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	doclangfmt "github.com/neokapi/neokapi/core/formats/doclang"
	"github.com/neokapi/neokapi/core/model"
)

// Synthetic schema-conformance probes: feed hand-built Part streams (the edge
// shapes the corpus fixtures don't cover) through the DocLang writer and assert
// the output validates against the vendored XSD. These guard the writer's
// schema-conformance fixes directly — nested lists/groups inside a list,
// continuation blocks, captions, and titles with non-default levels — without
// depending on a particular upstream fixture exercising them. (Uses xmllintPath
// + assertValidDocLang from conformance_test.go; self-skips without xmllint.)

func advBlock(id, text, role string, level int) *model.Part {
	b := model.NewBlock(id, text)
	b.SourceLocale = model.LocaleEnglish
	if role != "" {
		b.SetSemanticRole(role, level)
	}
	return &model.Part{Type: model.PartBlock, Resource: b}
}

func gStart(id, typ string) *model.Part {
	return &model.Part{Type: model.PartGroupStart, Resource: &model.GroupStart{ID: id, Name: typ, Type: typ}}
}

func gEnd(id string) *model.Part {
	return &model.Part{Type: model.PartGroupEnd, Resource: &model.GroupEnd{ID: id}}
}

// renderParts wraps a constructed Part stream in a layer and serializes it
// through the DocLang writer.
func renderParts(t *testing.T, parts []*model.Part) []byte {
	t.Helper()
	w := doclangfmt.NewWriter()
	var buf bytes.Buffer
	if err := w.SetOutputWriter(&buf); err != nil {
		t.Fatal(err)
	}
	layer := &model.Layer{ID: "doc1", Format: "doclang"}
	ch := make(chan *model.Part, len(parts)+2)
	ch <- &model.Part{Type: model.PartLayerStart, Resource: layer}
	for _, p := range parts {
		ch <- p
	}
	ch <- &model.Part{Type: model.PartLayerEnd, Resource: layer}
	close(ch)
	if err := w.Write(context.Background(), ch); err != nil {
		t.Fatalf("write: %v", err)
	}
	return buf.Bytes()
}

func TestWriterSchema_EdgeShapes(t *testing.T) {
	xmllint := xmllintPath(t)

	cases := []struct {
		name  string
		parts []*model.Part
	}{
		{"nested list after item", []*model.Part{
			gStart("g1", "list"),
			advBlock("b1", "item A", model.RoleListItem, 0),
			gStart("g2", "list"),
			advBlock("b2", "subitem A.1", model.RoleListItem, 0),
			gEnd("g2"),
			advBlock("b3", "item B", model.RoleListItem, 0),
			gEnd("g1"),
		}},
		{"nested list as first child", []*model.Part{
			gStart("g1", "list"),
			gStart("g2", "list"),
			advBlock("b1", "only subitem", model.RoleListItem, 0),
			gEnd("g2"),
			gEnd("g1"),
		}},
		{"group as list child (Docling inline group)", []*model.Part{
			gStart("g1", "list"),
			gStart("g2", "group"),
			advBlock("b1", "lead-in text", model.RoleParagraph, 0),
			gEnd("g2"),
			gEnd("g1"),
		}},
		{"multi-block list item", []*model.Part{
			gStart("g1", "list"),
			advBlock("b1", "the bullet", model.RoleListItem, 0),
			advBlock("b2", "continuation para", model.RoleParagraph, 0),
			gEnd("g1"),
		}},
		{"caption before rows", []*model.Part{
			gStart("g1", "table"),
			advBlock("b1", "the caption", model.RoleCaption, 0),
			gStart("g2", "table-row"),
			advBlock("b2", "ched", model.RoleTableHeader, 0),
			gEnd("g2"),
			gEnd("g1"),
		}},
		{"two captions fold to one", []*model.Part{
			gStart("g1", "table"),
			advBlock("b1", "caption one", model.RoleCaption, 0),
			advBlock("b2", "caption two", model.RoleCaption, 0),
			gStart("g2", "table-row"),
			advBlock("b3", "cell", model.RoleTableCell, 0),
			gEnd("g2"),
			gEnd("g1"),
		}},
		{"non-caption block under table is dropped", []*model.Part{
			gStart("g1", "table"),
			advBlock("b1", "a stray paragraph", model.RoleParagraph, 0),
			gStart("g2", "table-row"),
			advBlock("b2", "cell", model.RoleTableCell, 0),
			gEnd("g2"),
			gEnd("g1"),
		}},
		{"title with non-default level", []*model.Part{
			advBlock("b1", "My Title", model.RoleTitle, 3),
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assertValidDocLang(t, xmllint, renderParts(t, tc.parts))
		})
	}
}

// TestWriterSchema_ListItemLdiv pins the schema invariant that every direct
// child of a <list> begins with <ldiv/> — the fix that keeps nested
// groups/lists from emitting bare, schema-invalid list children.
func TestWriterSchema_ListItemLdiv(t *testing.T) {
	out := string(renderParts(t, []*model.Part{
		gStart("g1", "list"),
		advBlock("b1", "text item", model.RoleListItem, 0),
		gStart("g2", "list"), // nested list child → must get its own <ldiv/>
		advBlock("b2", "sub", model.RoleListItem, 0),
		gEnd("g2"),
		advBlock("b3", "continuation", model.RoleParagraph, 0), // non-item child → <ldiv/>
		gEnd("g1"),
	}))
	// outer list: text item, nested list, continuation → 3 ldivs; nested list's
	// own item → 1 more = 4 total.
	if got := strings.Count(out, "<ldiv/>"); got != 4 {
		t.Errorf("expected 4 <ldiv/> (3 outer children + 1 nested item), got %d:\n%s", got, out)
	}
	// And the nested <list> must be preceded by an <ldiv/>, not a bare child.
	if strings.Contains(out, "</text>\n<list") {
		t.Errorf("nested <list> emitted as a bare list child (no <ldiv/>):\n%s", out)
	}
}
