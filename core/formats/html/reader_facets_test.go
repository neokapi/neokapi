package html_test

import (
	"strings"
	"testing"

	htmlfmt "github.com/neokapi/neokapi/core/formats/html"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
)

// Phase 1 (structure-geometry-landscape.md §8): the HTML reader derives the
// plane (layout layer) and visibility (presence condition) facets from markup —
// no layout engine. A modal, a hidden block, an sr-only block, and the <title>
// are distinguished from ordinary body content.
func TestHTMLReaderDerivesStructureFacets(t *testing.T) {
	ctx := t.Context()
	src := `<html>` +
		`<head><title>Doc title</title></head>` +
		`<body>` +
		`<header><p>Site banner</p></header>` +
		`<main><p>Body para</p></main>` +
		`<dialog><p>Modal body</p></dialog>` +
		`<p aria-hidden="true">Hidden para</p>` +
		`<p class="sr-only">Screen-reader note</p>` +
		`<footer><p>Footer matter</p></footer>` +
		`</body></html>`

	reader := htmlfmt.NewReader()
	if err := reader.Open(ctx, testutil.RawDocFromString(src, model.LocaleEnglish)); err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	blocks := testutil.CollectBlocks(t, reader.Read(ctx))

	facets := func(text string) (plane, vis string) {
		for _, b := range blocks {
			if strings.TrimSpace(b.SourceText()) == text {
				return b.LayoutLayer(), b.Visibility()
			}
		}
		t.Fatalf("no block with text %q (have: %v)", text, testutil.BlockTexts(blocks))
		return "", ""
	}

	cases := []struct {
		text, wantPlane, wantVis string
	}{
		{"Doc title", model.LayerMetadata, model.VisibilityVisible},
		{"Site banner", model.LayerFurniture, model.VisibilityVisible},
		{"Body para", "", model.VisibilityVisible},
		{"Modal body", model.LayerOverlay, model.VisibilityConditional},
		{"Hidden para", "", model.VisibilityHidden},
		{"Screen-reader note", "", model.VisibilityScreenOnly},
		{"Footer matter", model.LayerFurniture, model.VisibilityVisible},
	}
	for _, c := range cases {
		plane, vis := facets(c.text)
		if plane != c.wantPlane {
			t.Errorf("%q plane=%q, want %q", c.text, plane, c.wantPlane)
		}
		if vis != c.wantVis {
			t.Errorf("%q visibility=%q, want %q", c.text, vis, c.wantVis)
		}
	}
}
