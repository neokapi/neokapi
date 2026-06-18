package doclang_test

import (
	"bytes"
	"os"
	"reflect"
	"strings"
	"testing"

	doclangfmt "github.com/neokapi/neokapi/core/formats/doclang"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
)

// TestDocLangRoundTrip reads a DocLang document, writes it back out, and reads
// the result again — asserting the structural content (block text + role +
// heading geometry) is preserved. This is the faithful DocLang↔DocLang path.
func TestDocLangRoundTrip(t *testing.T) {
	ctx := t.Context()
	data, err := os.ReadFile("testdata/sample.dclg.xml")
	if err != nil {
		t.Fatal(err)
	}

	// First read → full Part stream.
	r1 := doclangfmt.NewReader()
	if err := r1.Open(ctx, testutil.RawDocFromString(string(data), model.LocaleEnglish)); err != nil {
		t.Fatal(err)
	}
	parts := testutil.CollectParts(t, r1.Read(ctx))
	_ = r1.Close()
	blocks1 := testutil.FilterBlocks(parts)

	// Write the Part stream back to DocLang.
	var buf bytes.Buffer
	w := doclangfmt.NewWriter()
	if err := w.SetOutputWriter(&buf); err != nil {
		t.Fatal(err)
	}
	ch := make(chan *model.Part)
	go func() {
		for _, p := range parts {
			ch <- p
		}
		close(ch)
	}()
	if err := w.Write(ctx, ch); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "<doclang") || !strings.Contains(out, "</doclang>") {
		t.Fatalf("written output is not a DocLang document:\n%s", out)
	}

	// Re-read the written DocLang.
	r2 := doclangfmt.NewReader()
	if err := r2.Open(ctx, testutil.RawDocFromString(out, model.LocaleEnglish)); err != nil {
		t.Fatal(err)
	}
	blocks2 := testutil.CollectBlocks(t, r2.Read(ctx))
	_ = r2.Close()

	if len(blocks1) != len(blocks2) {
		t.Fatalf("block count changed across round-trip: %d → %d\noutput:\n%s",
			len(blocks1), len(blocks2), out)
	}
	for i := range blocks1 {
		a, b := blocks1[i], blocks2[i]
		if at, bt := strings.TrimSpace(a.SourceText()), strings.TrimSpace(b.SourceText()); at != bt {
			t.Errorf("block %d text: %q → %q", i, at, bt)
		}
		if a.SemanticRole() != b.SemanticRole() {
			t.Errorf("block %d (%q) role: %q → %q", i, a.SourceText(), a.SemanticRole(), b.SemanticRole())
		}
		ga, oka := a.Geometry()
		gb, okb := b.Geometry()
		if oka != okb {
			t.Errorf("block %d geometry presence changed: %v → %v", i, oka, okb)
		} else if oka && !reflect.DeepEqual(*ga, *gb) {
			t.Errorf("block %d geometry: %+v → %+v", i, *ga, *gb)
		}
	}
}
