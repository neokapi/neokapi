package doclang_test

import (
	"os"
	"strings"
	"testing"

	doclangfmt "github.com/neokapi/neokapi/core/formats/doclang"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
)

// readFeatures reads the hand-authored feature-coverage fixture (spans, forms,
// threading, rtl/handwriting, page_break, asymmetric resolution) and returns its
// blocks. The fixture is XSD-valid (see TestFeaturesRoundTripSchemaValid).
func readFeatures(t *testing.T) []*model.Block {
	t.Helper()
	data, err := os.ReadFile("testdata/features.dclg.xml")
	if err != nil {
		t.Fatal(err)
	}
	r := doclangfmt.NewReader()
	if err := r.Open(t.Context(), testutil.RawDocFromString(string(data), model.LocaleEnglish)); err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	return testutil.CollectBlocks(t, r.Read(t.Context()))
}

func findBlock(t *testing.T, blocks []*model.Block, text string) *model.Block {
	t.Helper()
	for _, b := range blocks {
		if strings.TrimSpace(b.SourceText()) == text {
			return b
		}
	}
	t.Fatalf("no block with text %q (have: %v)", text, testutil.BlockTexts(blocks))
	return nil
}

func TestReadFeatures_FormsCluster(t *testing.T) {
	blocks := readFeatures(t)

	if b := findBlock(t, blocks, "Application"); b.SemanticRole() != model.RoleFieldHeading {
		t.Errorf("field_heading role = %q, want field-heading", b.SemanticRole())
	} else if s, _ := b.Structure(); s.Level != 2 {
		t.Errorf("field_heading level = %d, want 2", s.Level)
	}
	if b := findBlock(t, blocks, "Name"); b.SemanticRole() != model.RoleKey {
		t.Errorf("key role = %q, want key", b.SemanticRole())
	}
	v := findBlock(t, blocks, "Ada")
	if v.SemanticRole() != model.RoleValue || !v.FieldFillable() {
		t.Errorf("value role/fillable = %q/%v, want value/true", v.SemanticRole(), v.FieldFillable())
	}
	if b := findBlock(t, blocks, "Enter your full name"); b.SemanticRole() != model.RoleHint {
		t.Errorf("hint role = %q, want hint", b.SemanticRole())
	}
	// Checkbox: a non-translatable RoleCheckbox block carrying the checked state.
	var cb *model.Block
	for _, b := range blocks {
		if b.SemanticRole() == model.RoleCheckbox {
			cb = b
		}
	}
	if cb == nil {
		t.Fatal("no checkbox block")
	}
	if cb.Translatable || !cb.CheckboxChecked() {
		t.Errorf("checkbox translatable=%v checked=%v, want false/true", cb.Translatable, cb.CheckboxChecked())
	}
}

func TestReadFeatures_TableSpansAndHeaderKinds(t *testing.T) {
	blocks := readFeatures(t)

	corner := findBlock(t, blocks, "Region")
	if corner.TableHeaderKind() != model.TableHeaderCorner {
		t.Errorf("'Region' header kind = %q, want corner", corner.TableHeaderKind())
	}
	if findBlock(t, blocks, "Q1").TableHeaderKind() != model.TableHeaderColumn {
		t.Errorf("'Q1' header kind = %q, want column", findBlock(t, blocks, "Q1").TableHeaderKind())
	}
	if findBlock(t, blocks, "North").TableHeaderKind() != model.TableHeaderRow {
		t.Errorf("'North' header kind = %q, want row", findBlock(t, blocks, "North").TableHeaderKind())
	}

	// Colspan: "both" merges two columns (fcel + lcel).
	both := findBlock(t, blocks, "both")
	if s, _ := both.Structure(); s.ColSpan != 2 {
		t.Errorf("'both' ColSpan = %d, want 2", s.ColSpan)
	}
	// Rowspan: "tall" merges two rows (ucel below).
	tall := findBlock(t, blocks, "tall")
	if s, _ := tall.Structure(); s.RowSpan != 2 {
		t.Errorf("'tall' RowSpan = %d, want 2", s.RowSpan)
	}
}

func TestReadFeatures_ThreadingCodeAndInline(t *testing.T) {
	blocks := readFeatures(t)

	// Code language (DNT subtype).
	if c := findBlock(t, blocks, `print("hi")`); c.CodeLanguage() != "Python" {
		t.Errorf("code language = %q, want Python", c.CodeLanguage())
	}

	// Threading: the second fragment continues from the first.
	first := findBlock(t, blocks, "First part of a paragraph that")
	second := findBlock(t, blocks, "continues across a column break.")
	if _, ok := first.Relations(); ok {
		t.Errorf("origin fragment should carry no RelContinues edge")
	}
	rel, ok := second.Relations()
	if !ok || len(rel.Relations) != 1 || rel.Relations[0].Type != model.RelContinues || rel.Relations[0].Target != first.ID {
		t.Errorf("continuation edge = %+v, want RelContinues→%s", rel, first.ID)
	}

	// Inline rtl + handwriting become typed runs.
	para := findBlock(t, blocks, "Hebrew שלום and a signature here.")
	types := map[string]bool{}
	for _, r := range para.Source {
		if r.PcOpen != nil {
			types[r.PcOpen.Type] = true
		}
	}
	if !types["fmt:bidi"] || !types["fmt:handwriting"] {
		t.Errorf("inline run types = %v, want fmt:bidi + fmt:handwriting", types)
	}
}

func TestReadFeatures_PageAndResolution(t *testing.T) {
	blocks := readFeatures(t)

	// Page geometry: content after <page_break/> is on page 2.
	p2 := findBlock(t, blocks, "Content on the second page.")
	g, ok := p2.Geometry()
	if !ok || g.Page != 2 {
		t.Errorf("page-2 block geometry = %+v (ok=%v), want Page=2", g, ok)
	}

	// Per-axis resolution: a 1000×512 grid.
	pos := findBlock(t, blocks, "Positioned on a 1000x512 grid.")
	gg, ok := pos.Geometry()
	if !ok || gg.Resolution != 1000 || gg.ResolutionY != 512 {
		t.Errorf("asymmetric geometry = %+v, want res 1000 / resY 512", gg)
	}
}

// renderFeatures reads the fixture, writes it back, and returns the DocLang XML.
func renderFeatures(t *testing.T) []byte {
	t.Helper()
	data, err := os.ReadFile("testdata/features.dclg.xml")
	if err != nil {
		t.Fatal(err)
	}
	return renderDocLang(t, doclangfmt.NewReader(), string(data))
}

func TestFeaturesRoundTripSchemaValid(t *testing.T) {
	xmllint := xmllintPath(t)
	assertValidDocLang(t, xmllint, renderFeatures(t))
}

// TestFeaturesRoundTrip reads the fixture, writes it, re-reads, and asserts the
// enriched structure (spans, forms state, code language, threading, page,
// per-axis resolution, inline run types) survives the DocLang↔DocLang trip.
func TestFeaturesRoundTrip(t *testing.T) {
	out := renderFeatures(t)
	r := doclangfmt.NewReader()
	if err := r.Open(t.Context(), testutil.RawDocFromString(string(out), model.LocaleEnglish)); err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	blocks := testutil.CollectBlocks(t, r.Read(t.Context()))

	if s, _ := findBlock(t, blocks, "both").Structure(); s.ColSpan != 2 {
		t.Errorf("round-trip colspan lost: %+v\n%s", s, out)
	}
	if s, _ := findBlock(t, blocks, "tall").Structure(); s.RowSpan != 2 {
		t.Errorf("round-trip rowspan lost: %+v\n%s", s, out)
	}
	if !findBlock(t, blocks, "Ada").FieldFillable() {
		t.Errorf("round-trip fillable lost\n%s", out)
	}
	if findBlock(t, blocks, `print("hi")`).CodeLanguage() != "Python" {
		t.Errorf("round-trip code language lost\n%s", out)
	}
	if findBlock(t, blocks, "Region").TableHeaderKind() != model.TableHeaderCorner {
		t.Errorf("round-trip header sub-kind lost\n%s", out)
	}
	second := findBlock(t, blocks, "continues across a column break.")
	if rel, ok := second.Relations(); !ok || len(rel.Relations) != 1 || rel.Relations[0].Type != model.RelContinues {
		t.Errorf("round-trip threading lost: %+v\n%s", rel, out)
	}
	if g, _ := findBlock(t, blocks, "Positioned on a 1000x512 grid.").Geometry(); g.Resolution != 1000 || g.ResolutionY != 512 {
		t.Errorf("round-trip per-axis resolution lost: %+v\n%s", g, out)
	}
}
