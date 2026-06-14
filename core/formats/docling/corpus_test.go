package docling_test

import (
	"os"
	"path/filepath"
	"testing"

	doclingfmt "github.com/neokapi/neokapi/core/formats/docling"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
)

// Corpus ingestion: read REAL DoclingDocument JSON vendored from docling-core's
// own test suite (testdata/corpus/) and assert our reader extracts the expected
// roles. This proves we ingest genuine Docling output, not just hand-authored
// fixtures.

func readCorpus(t *testing.T, path string) ([]*model.Block, []*model.Part) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	r := doclingfmt.NewReader()
	if err := r.Open(t.Context(), testutil.RawDocFromString(string(data), model.LocaleEnglish)); err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	parts := testutil.CollectParts(t, r.Read(t.Context()))
	var blocks []*model.Block
	for _, p := range parts {
		if p.Type == model.PartBlock {
			if b, ok := p.Resource.(*model.Block); ok {
				blocks = append(blocks, b)
			}
		}
	}
	return blocks, parts
}

func roleSet(blocks []*model.Block) map[string]int {
	m := map[string]int{}
	for _, b := range blocks {
		m[b.SemanticRole()]++
	}
	return m
}

func groupTypes(parts []*model.Part) map[string]int {
	m := map[string]int{}
	for _, p := range parts {
		if p.Type == model.PartGroupStart {
			if g, ok := p.Resource.(*model.GroupStart); ok {
				m[g.Type]++
			}
		}
	}
	return m
}

func TestCorpus_IngestsWithoutError(t *testing.T) {
	fixtures, _ := filepath.Glob("testdata/corpus/*.json")
	if len(fixtures) == 0 {
		t.Fatal("no vendored corpus fixtures")
	}
	for _, fx := range fixtures {
		t.Run(filepath.Base(fx), func(t *testing.T) {
			blocks, _ := readCorpus(t, fx)
			if len(blocks) == 0 {
				t.Errorf("%s produced no blocks", filepath.Base(fx))
			}
		})
	}
}

// TestCorpus_Roles asserts the roles each vendored DoclingDocument exercises.
func TestCorpus_Roles(t *testing.T) {
	t.Run("page_without_pic: heading/list/caption + furniture header + picture", func(t *testing.T) {
		blocks, parts := readCorpus(t, "testdata/corpus/page_without_pic.json")
		roles := roleSet(blocks)
		for _, want := range []string{model.RoleHeading, model.RoleListItem, model.RoleCaption} {
			if roles[want] == 0 {
				t.Errorf("expected role %q; roles=%v", want, roles)
			}
		}
		var furniture int
		for _, b := range blocks {
			if b.LayoutLayer() == model.LayerFurniture {
				furniture++
			}
		}
		if furniture == 0 {
			t.Errorf("expected a furniture-layer block (page_header)")
		}
		if groupTypes(parts)["picture"] == 0 {
			t.Errorf("expected a picture group; groups=%v", groupTypes(parts))
		}
	})

	t.Run("flattened: title→heading, table cells, captured field text", func(t *testing.T) {
		blocks, parts := readCorpus(t, "testdata/corpus/flattened.json")
		roles := roleSet(blocks)
		// A DoclingDocument "title" label projects to the heading role.
		if roles[model.RoleHeading] == 0 {
			t.Errorf("expected heading roles (title + section_headers); roles=%v", roles)
		}
		if groupTypes(parts)["table"] == 0 {
			t.Errorf("expected a table group; groups=%v", groupTypes(parts))
		}
		if roles[model.RoleTableCell]+roles[model.RoleTableHeader] == 0 {
			t.Errorf("expected table cells; roles=%v", roles)
		}
		// Unmapped labels (field_key/field_value) still capture their text as
		// role-less blocks — content is never silently dropped.
		var textLen int
		for _, b := range blocks {
			textLen += len(b.SourceText())
		}
		if textLen < 20 {
			t.Errorf("expected substantial captured text from a flattened doc")
		}
	})
}
