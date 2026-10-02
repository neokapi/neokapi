package html_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	htmlfmt "github.com/neokapi/neokapi/core/formats/html"
	mdfmt "github.com/neokapi/neokapi/core/formats/markdown"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
)

// WS2: the HTML reader populates the normalized SemanticRole (and heading
// level) so HTML content carries the same roles as any other source — the
// basis for the structure view and clean cross-format export.
func TestHTMLReaderPopulatesSemanticRole(t *testing.T) {
	ctx := t.Context()
	reader := htmlfmt.NewReader()
	src := `<html><body>` +
		`<h2>Title</h2>` +
		`<p>Para</p>` +
		`<ul><li>Item one</li></ul>` +
		`</body></html>`
	if err := reader.Open(ctx, testutil.RawDocFromString(src, model.LocaleEnglish)); err != nil {
		t.Fatal(err)
	}
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))

	roleOf := func(text string) (string, int) {
		for _, b := range blocks {
			if strings.TrimSpace(b.SourceText()) == text {
				if s, ok := b.Structure(); ok && s != nil {
					return s.Role, s.Level
				}
				return "", 0
			}
		}
		t.Fatalf("no block with text %q (have texts: %v)", text, testutil.BlockTexts(blocks))
		return "", 0
	}

	if r, lvl := roleOf("Title"); r != model.RoleHeading || lvl != 2 {
		t.Errorf("h2 → role=%q level=%d, want %q/2", r, lvl, model.RoleHeading)
	}
	if r, _ := roleOf("Para"); r != model.RoleParagraph {
		t.Errorf("p → role=%q, want %q", r, model.RoleParagraph)
	}
	if r, _ := roleOf("Item one"); r != model.RoleListItem {
		t.Errorf("li → role=%q, want %q", r, model.RoleListItem)
	}
}

// `kapi inspect` reads a document the way the edit path does, which for HTML
// is the byte-faithful tokenizer, so a leaf element's role and heading level
// come out the same whichever engine read it.
func TestHTMLReaderRolesMatchAcrossEngines(t *testing.T) {
	const src = `<html><body>` +
		`<h2>Title</h2>` +
		`<p>Para</p>` +
		`<ul><li>Item one</li></ul>` +
		`<table><tr><th>Head</th><td colspan="2">Cell</td></tr></table>` +
		`</body></html>`
	tests := []struct {
		text    string
		role    string
		level   int
		colSpan int
	}{
		{text: "Title", role: model.RoleHeading, level: 2},
		{text: "Para", role: model.RoleParagraph},
		{text: "Item one", role: model.RoleListItem},
		{text: "Head", role: model.RoleTableHeader},
		{text: "Cell", role: model.RoleTableCell, colSpan: 2},
	}
	for _, mode := range htmlModes() {
		t.Run(mode.name, func(t *testing.T) {
			ctx := t.Context()
			reader := mode.newRead(t)
			require.NoError(t, reader.Open(ctx, testutil.RawDocFromString(src, model.LocaleEnglish)))
			defer reader.Close()
			blocks := testutil.CollectBlocks(t, reader.Read(ctx))
			for _, tc := range tests {
				var got *model.StructureAnnotation
				for _, b := range blocks {
					if strings.TrimSpace(b.SourceText()) == tc.text {
						got, _ = b.Structure()
					}
				}
				require.NotNil(t, got, "%s carries a structure annotation", tc.text)
				assert.Equal(t, tc.role, got.Role, tc.text)
				assert.Equal(t, tc.level, got.Level, tc.text)
				if tc.colSpan > 0 {
					assert.Equal(t, tc.colSpan, got.ColSpan, tc.text)
				}
			}
		})
	}
}

// WS2+WS6 end to end: read HTML, then emit clean Markdown driven purely by the
// normalized roles (no Markdown skeleton). This is the cross-format
// "model → clean Markdown" projection on a real source.
func TestHTMLToCleanMarkdown(t *testing.T) {
	ctx := t.Context()
	reader := htmlfmt.NewReader()
	src := `<html><body><h2>Title</h2><p>Para</p><ul><li>Item one</li></ul></body></html>`
	if err := reader.Open(ctx, testutil.RawDocFromString(src, model.LocaleEnglish)); err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	blocks := testutil.CollectBlocks(t, reader.Read(ctx))

	var buf bytes.Buffer
	w := mdfmt.NewWriter()
	if err := w.SetOutputWriter(&buf); err != nil {
		t.Fatal(err)
	}
	parts := make(chan *model.Part)
	go func() {
		for _, b := range blocks {
			parts <- &model.Part{Type: model.PartBlock, Resource: b}
		}
		close(parts)
	}()
	if err := w.Write(ctx, parts); err != nil {
		t.Fatal(err)
	}

	out := buf.String()
	for _, want := range []string{"## Title", "Para", "- Item one"} {
		if !strings.Contains(out, want) {
			t.Errorf("HTML→Markdown missing %q; got:\n%s", want, out)
		}
	}
}
