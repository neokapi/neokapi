package formats_test

// Conversion-quality regressions found by blind-judging real conversions
// against rendered pages. Each test pins one judged defect on the generative
// path, end to end: real fixture in, Markdown out, one assertion on the shape
// the judge flagged. The skeleton (round-trip) path is untouched by all of
// them.

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/formats"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// convFixture names one document: a path relative to this package and the
// format that reads it. Format is explicit rather than detected so a detection
// regression surfaces as a detection test failure, not as a silent change in
// what these tests exercise.
type convFixture struct {
	name   string
	path   string
	format string
}

// convRegistry builds a registry with every built-in format registered — the
// same construction the CLI uses, so the tests exercise the real factories.
func convRegistry() *registry.FormatRegistry {
	reg := registry.NewFormatRegistry()
	formats.RegisterAll(reg)
	return reg
}

func loadFixture(tb testing.TB, path string) []byte {
	tb.Helper()
	data, err := os.ReadFile(path)
	require.NoError(tb, err, "fixture %s", path)
	return data
}

// convertOnce is one full in-process generative conversion: bytes in, Markdown
// out — the same reader → Part stream → Markdown writer path `kapi kconv`
// takes, with no process spawn.
func convertOnce(reg *registry.FormatRegistry, data []byte, f convFixture) ([]byte, error) {
	r, err := reg.NewReader(registry.FormatID(f.format))
	if err != nil {
		return nil, err
	}
	if sa, ok := r.(format.SubfilterAware); ok {
		sa.SetSubfilterResolver(reg)
	}
	ctx := context.Background()
	if err := r.Open(ctx, rawDoc(data, f.path, f.format)); err != nil {
		return nil, err
	}
	var parts []*model.Part
	for pr := range r.Read(ctx) {
		if pr.Error != nil {
			_ = r.Close()
			return nil, pr.Error
		}
		parts = append(parts, pr.Part)
	}
	if err := r.Close(); err != nil {
		return nil, err
	}

	w, err := reg.NewWriter("markdown")
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	if err := w.SetOutputWriter(&buf); err != nil {
		return nil, err
	}
	ch := make(chan *model.Part, len(parts))
	for _, p := range parts {
		ch <- p
	}
	close(ch)
	if err := w.Write(ctx, ch); err != nil {
		return nil, err
	}
	return buf.Bytes(), w.Close()
}

// A document's core properties are metadata about the file, not part of its
// flow: converting math_block.docx used to end with "Matt Valley" — the
// dc:creator from docProps/core.xml — as a trailing paragraph (#1679). The
// properties stay extracted (they are real translation units); they just must
// not render as body text.
func TestDocPropertiesStayOutOfMarkdownFlow(t *testing.T) {
	reg := convRegistry()
	f := convFixture{name: "docx/math", path: "openxml/testdata/math_block.docx", format: "openxml"}
	md, err := convertOnce(reg, loadFixture(t, f.path), f)
	require.NoError(t, err)

	assert.NotContains(t, string(md), "Matt Valley",
		"docProps/core.xml creator rendered as document flow")
	// The real flow is still there.
	assert.Contains(t, string(md), "Computer science")
}

// A worksheet whose first row is a lone title cell above a wider grid is a
// captioned table, not a table whose header row is the title padded with
// empty cells (#1680). The title renders as a caption line ahead of the
// table and the first full row keeps its header job.
func TestWorksheetTitleRowBecomesCaptionNotHeader(t *testing.T) {
	reg := convRegistry()
	f := convFixture{name: "xlsx/titled", path: "../../harness/demos/09-toolbox-find-replace/fixtures/pricing.xlsx", format: "openxml"}
	md, err := convertOnce(reg, loadFixture(t, f.path), f)
	require.NoError(t, err)
	out := string(md)

	assert.NotContains(t, out, "| SalesPilot — plan pricing |",
		"title row rendered as a table row")
	assert.Contains(t, out, "**SalesPilot — plan pricing**",
		"title row should render as a caption ahead of the table")
	// The real header row keeps its header job: it is immediately followed by
	// the GFM delimiter row.
	idx := strings.Index(out, "| Plan | Seats |")
	require.GreaterOrEqual(t, idx, 0, "header row missing")
	lines := strings.SplitN(out[idx:], "\n", 3)
	require.Len(t, lines, 3)
	assert.True(t, strings.HasPrefix(lines[1], "| ---"),
		"row after %q should be the delimiter row, got %q", lines[0], lines[1])
}

// A deck has one deck title and many slide titles, and PresentationML says
// which is which (ctrTitle vs title). Flattening them all to h1 loses the
// outline (#1681): the title slide heads the document, each further slide
// heads a section.
func TestSlideTitlesNestUnderTheDeckTitle(t *testing.T) {
	reg := convRegistry()
	f := convFixture{name: "pptx/deck", path: "../../apps/kapi-desktop/backend/sample/kapimart/marketing/en/onboarding-deck.pptx", format: "openxml"}
	md, err := convertOnce(reg, loadFixture(t, f.path), f)
	require.NoError(t, err)
	out := string(md)

	assert.Contains(t, out, "# Welcome to KapiMart\n", "ctrTitle is the deck's h1")
	assert.Contains(t, out, "## Getting Started", "a slide title is an h2")
	assert.Contains(t, out, "## Next Steps", "a slide title is an h2")

	var h1s int
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, "# ") {
			h1s++
		}
	}
	assert.Equal(t, 1, h1s, "exactly one h1: the deck title")
}
