package doclang_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	doclangfmt "github.com/neokapi/neokapi/core/formats/doclang"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
)

// Corpus ingestion: read REAL DocLang documents vendored from the upstream
// standard's own test suite (testdata/corpus/, from doclang-project/doclang) and
// assert our reader extracts the expected roles. This is the "does it ingest
// real DocLang, not just my hand-authored fixtures" check — the single biggest
// jump in confidence for a format with no Okapi parity bridge.

// readCorpus reads a vendored fixture and returns its translatable blocks plus
// the raw part stream (for group/geometry/layer assertions).
func readCorpus(t *testing.T, path string) ([]*model.Block, []*model.Part) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	r := doclangfmt.NewReader()
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

// TestCorpus_IngestsWithoutError: every vendored DocLang fixture reads cleanly
// and yields at least one block. CollectParts fails the test on any reader
// error, so this is also a no-panic / no-error guard across real documents.
func TestCorpus_IngestsWithoutError(t *testing.T) {
	fixtures, _ := filepath.Glob("testdata/corpus/*.dclg.xml")
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

// TestCorpus_Roles asserts the specific role/structure each fixture is supposed
// to exercise, so a regression in role mapping is caught against real DocLang.
func TestCorpus_Roles(t *testing.T) {
	t.Run("heading + paragraph (no namespace)", func(t *testing.T) {
		blocks, _ := readCorpus(t, "testdata/corpus/ok_no_namespace.dclg.xml")
		roles := roleSet(blocks)
		if roles[model.RoleHeading] == 0 {
			t.Errorf("expected a heading; roles=%v", roles)
		}
		if roles[model.RoleParagraph] == 0 {
			t.Errorf("expected paragraphs; roles=%v", roles)
		}
	})

	t.Run("list items", func(t *testing.T) {
		blocks, parts := readCorpus(t, "testdata/corpus/ok_list_wrapped_none.dclg.xml")
		if roleSet(blocks)[model.RoleListItem] == 0 {
			t.Errorf("expected a list-item; roles=%v", roleSet(blocks))
		}
		if groupTypes(parts)["list"] == 0 {
			t.Errorf("expected a list group; groups=%v", groupTypes(parts))
		}
	})

	t.Run("OTSL table header vs cell", func(t *testing.T) {
		blocks, parts := readCorpus(t, "testdata/corpus/ok_table_rectangular.dclg.xml")
		roles := roleSet(blocks)
		if roles[model.RoleTableHeader] == 0 {
			t.Errorf("expected table-header cells (ched/rhed/corn); roles=%v", roles)
		}
		if roles[model.RoleTableCell] == 0 {
			t.Errorf("expected table-cell cells (fcel/ecel); roles=%v", roles)
		}
		if groupTypes(parts)["table"] == 0 {
			t.Errorf("expected a table group; groups=%v", groupTypes(parts))
		}
	})

	t.Run("code block", func(t *testing.T) {
		blocks, _ := readCorpus(t, "testdata/corpus/ok_label_element_head.dclg.xml")
		if roleSet(blocks)[model.RoleCode] == 0 {
			t.Errorf("expected a code block; roles=%v", roleSet(blocks))
		}
	})

	t.Run("geometry from location block", func(t *testing.T) {
		blocks, _ := readCorpus(t, "testdata/corpus/ok_location_axis_limits.dclg.xml")
		var withGeo int
		for _, b := range blocks {
			if g, ok := b.Geometry(); ok && g != nil {
				withGeo++
			}
		}
		if withGeo == 0 {
			t.Errorf("expected at least one block with geometry from a 4-value <location> block")
		}
	})

	t.Run("page_header → furniture layer", func(t *testing.T) {
		blocks, _ := readCorpus(t, "testdata/corpus/ok_layer.dclg.xml")
		var furniture int
		for _, b := range blocks {
			if b.LayoutLayer() == model.LayerFurniture {
				furniture++
			}
		}
		if furniture == 0 {
			t.Errorf("expected a furniture-layer block (page_header); roles=%v", roleSet(blocks))
		}
	})
}

// TestCorpus_ReadingOrder is a reading-order *correctness* oracle (not the
// lenient G2 grep, which only proves a reader CAN express order via
// PartGroupStart): it asserts the exact document/logical order of the recovered
// translatable blocks against a fixture whose order is known by construction —
// including content threaded across a column break and across a <page_break/>.
// A regression that reorders, drops, or interleaves blocks fails here.
func TestCorpus_ReadingOrder(t *testing.T) {
	blocks := readFeatures(t)
	var got []string
	for _, b := range blocks {
		if txt := strings.TrimSpace(b.SourceText()); txt != "" {
			got = append(got, txt)
		}
	}
	want := []string{
		"Feature Coverage",
		"Hebrew שלום and a signature here.",
		`print("hi")`,
		"Application",
		"Name",
		"Ada",
		"Enter your full name",
		"Region", "Q1", "Q2",
		"North", "10", "20",
		"Wide", "both",
		"tall", "a",
		"b",
		"First part of a paragraph that",
		"continues across a column break.",
		"Positioned on a 1000x512 grid.",
		"Content on the second page.",
	}
	if len(got) != len(want) {
		t.Fatalf("reading-order block count = %d, want %d\ngot:  %v\nwant: %v", len(got), len(want), got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("reading order [%d] = %q, want %q", i, got[i], want[i])
		}
	}
}
