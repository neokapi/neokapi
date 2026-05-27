package html_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/format"
	htmlfmt "github.com/neokapi/neokapi/core/formats/html"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// readDOMBlocks reads HTML with the read-only DOM path (no skeleton store),
// the path used by word-count / segment-count / kgrep.
func readDOMBlocks(t *testing.T, src string) []*model.Block {
	t.Helper()
	ctx := t.Context()
	r := htmlfmt.NewReader()
	require.NoError(t, r.Open(ctx, testutil.RawDocFromString(src, model.LocaleEnglish)))
	defer r.Close()
	return testutil.CollectBlocks(t, r.Read(ctx))
}

// readTokenBlocks reads HTML with the tokenizer/skeleton path (a skeleton
// store wired), the authoritative path used by pseudo-translate / extract /
// merge / faithful round-trips.
func readTokenBlocks(t *testing.T, src string) []*model.Block {
	t.Helper()
	ctx := t.Context()
	r := htmlfmt.NewReader()
	r.SetSkeletonStore(format.NewMemorySkeletonStore())
	require.NoError(t, r.Open(ctx, testutil.RawDocFromString(src, model.LocaleEnglish)))
	defer r.Close()
	return testutil.CollectBlocks(t, r.Read(ctx))
}

// countWords mirrors the word-count tool's strings.Fields counting so the test
// can assert source-word totals without depending on the CLI wiring.
func countWords(s string) int {
	return len(strings.Fields(s))
}

func sumWords(blocks []*model.Block) int {
	total := 0
	for _, b := range blocks {
		total += countWords(b.SourceText())
	}
	return total
}

// The #721 demo fixture: title, h1, p, then a body-level standalone <button>
// and <a>. The read-only DOM path used to drop the button/anchor (3 blocks /
// 14 words) while the writer/skeleton path localized all 5.
const checkoutFixture = "<!DOCTYPE html>\n" +
	"<html lang=\"en\">\n" +
	"  <head>\n" +
	"    <meta charset=\"utf-8\" />\n" +
	"    <title>Acme Checkout</title>\n" +
	"  </head>\n" +
	"  <body>\n" +
	"    <h1>Complete your purchase</h1>\n" +
	"    <p>Review your order and confirm payment to finish checkout.</p>\n" +
	"    <button>Pay now</button>\n" +
	"    <a href=\"/help\">Need help?</a>\n" +
	"  </body>\n" +
	"</html>"

// TestStandaloneInline_PathsAgree is the core #721 assertion: the read-only
// DOM path and the authoritative tokenizer path emit the SAME translatable
// block set/count for body-level (and any block-container-level) standalone
// inline elements.
func TestStandaloneInline_PathsAgree(t *testing.T) {
	cases := []struct {
		name string
		src  string
		// wantTexts is the expected translatable block source-text set, in
		// reader emission order. Both paths must match it exactly.
		wantTexts []string
	}{
		{
			name:      "checkout fixture: title h1 p button a",
			src:       checkoutFixture,
			wantTexts: []string{"Acme Checkout", "Complete your purchase", "Review your order and confirm payment to finish checkout.", "Pay now", "Need help?"},
		},
		{
			name:      "standalone button + a + span after a block sibling",
			src:       `<html><body><p>Lead in.</p><button>Pay now</button> <a href="/h">Need help?</a> <span>Or call us</span></body></html>`,
			wantTexts: []string{"Lead in.", "Pay now", "Need help?", "Or call us"},
		},
		{
			name:      "single standalone button after block",
			src:       `<html><body><p>Para</p><button>Buy</button></body></html>`,
			wantTexts: []string{"Para", "Buy"},
		},
		{
			name:      "standalone inlines separated by whitespace inside a div container",
			src:       `<html><body><div><h2>Menu</h2><button>One</button> <button>Two</button></div></body></html>`,
			wantTexts: []string{"Menu", "One", "Two"},
		},
		{
			name:      "standalone anchor between block siblings",
			src:       `<html><body><p>Before</p><a href="/x">Link</a><p>After</p></body></html>`,
			wantTexts: []string{"Before", "Link", "After"},
		},
		{
			name:      "empty standalone inline carrying only an id is not translatable",
			src:       `<html><body><p>x</p><span id="anchor"></span><button>Go</button></body></html>`,
			wantTexts: []string{"x", "Go"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dom := readDOMBlocks(t, tc.src)
			tok := readTokenBlocks(t, tc.src)

			domTexts := testutil.BlockTexts(dom)
			tokTexts := testutil.BlockTexts(tok)

			assert.Equal(t, tc.wantTexts, domTexts, "DOM path block texts")
			assert.Equal(t, tc.wantTexts, tokTexts, "tokenizer path block texts")
			assert.Equal(t, domTexts, tokTexts, "DOM and tokenizer paths must agree on the block set")
			assert.Equal(t, len(tc.wantTexts), len(dom), "DOM path block count")
			assert.Equal(t, len(tc.wantTexts), len(tok), "tokenizer path block count")
		})
	}
}

// TestStandaloneInline_FixtureCounts pins the exact before/after numbers from
// the issue: 5 translatable blocks, 18 source words — and that the two reader
// paths produce identical counts.
func TestStandaloneInline_FixtureCounts(t *testing.T) {
	dom := readDOMBlocks(t, checkoutFixture)
	tok := readTokenBlocks(t, checkoutFixture)

	require.Len(t, dom, 5, "DOM path should now emit 5 blocks (title, h1, p, button, a)")
	require.Len(t, tok, 5, "tokenizer path emits 5 blocks")

	assert.Equal(t, 18, sumWords(dom), "DOM path total source words")
	assert.Equal(t, 18, sumWords(tok), "tokenizer path total source words")
	assert.Equal(t, sumWords(dom), sumWords(tok), "word totals must agree across paths")

	// The button and anchor text must be present as their own blocks.
	domTexts := testutil.BlockTexts(dom)
	assert.Contains(t, domTexts, "Pay now")
	assert.Contains(t, domTexts, "Need help?")
}

// TestStandaloneInline_InlineWithinTextStaysInline guards the existing correct
// behavior: an inline element WITHIN surrounding text stays an inline run in a
// single block — it must NOT be split into its own standalone block.
func TestStandaloneInline_InlineWithinTextStaysInline(t *testing.T) {
	cases := []struct {
		name    string
		src     string
		wantOne string // expected single combined block source text
	}{
		{
			name:    "inline within paragraph text",
			src:     `<html><body><p>Click <b>here</b> for info</p></body></html>`,
			wantOne: "Click here for info",
		},
		{
			name:    "inline glued to container-level text after a block sibling",
			src:     `<html><body><h2>Title</h2><div>Lead <b>bold</b> tail</div></body></html>`,
			wantOne: "Lead bold tail",
		},
		{
			name:    "anchor inside sentence",
			src:     `<html><body><p>Visit <a href="http://example.com">our site</a> today</p></body></html>`,
			wantOne: "Visit our site today",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dom := readDOMBlocks(t, tc.src)

			// Find the block that holds the glued inline content.
			var combined *model.Block
			for _, b := range dom {
				if b.SourceText() == tc.wantOne {
					combined = b
					break
				}
			}
			require.NotNil(t, combined, "expected a single combined block %q, got %v", tc.wantOne, testutil.BlockTexts(dom))

			// The combined block must carry inline runs (PcOpen/PcClose or
			// Ph), proving the inline element stayed inline rather than being
			// promoted to its own block.
			hasInlineRun := false
			for _, r := range combined.Source {
				if r.Text == nil {
					hasInlineRun = true
				}
			}
			assert.True(t, hasInlineRun, "glued inline element should remain an inline run inside the combined block")

			// And the inline element's text must NOT also appear as its own
			// standalone block.
			texts := testutil.BlockTexts(dom)
			for _, fragment := range []string{"here", "bold", "our site"} {
				if strings.Contains(tc.wantOne, fragment) {
					assert.NotContains(t, texts, fragment, "inline fragment %q must not be a standalone block", fragment)
				}
			}
		})
	}
}

// TestStandaloneInline_PseudoTranslateRoundTrip drives the authoritative
// writer/skeleton path (what `pseudo-translate -o` uses) end-to-end: every
// visible string is localized, markup/attributes are preserved, and the
// document `lang` is retargeted. The localized strings are exactly the
// translatable blocks the analysis paths now also see.
func TestStandaloneInline_PseudoTranslateRoundTrip(t *testing.T) {
	ctx := t.Context()
	locale := model.LocaleID("fr")

	reader := htmlfmt.NewReader()
	writer := htmlfmt.NewWriter()

	store, err := format.NewSkeletonStore()
	require.NoError(t, err)
	defer store.Close()
	reader.SetSkeletonStore(store)
	writer.SetSkeletonStore(store)

	require.NoError(t, reader.Open(ctx, testutil.RawDocFromString(checkoutFixture, model.LocaleEnglish)))
	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	// Pseudo-translate every translatable block: prefix the source so we can
	// assert every visible string was localized.
	translations := map[string]string{}
	var localizedCount int
	for _, p := range parts {
		if p.Type != model.PartBlock {
			continue
		}
		b := p.Resource.(*model.Block)
		if !b.Translatable {
			continue
		}
		src := b.SourceText()
		tgt := "[" + src + "]"
		translations[src] = tgt
		b.SetTargetRuns(locale, []model.Run{{Text: &model.TextRun{Text: tgt}}})
		localizedCount++
	}

	require.Equal(t, 5, localizedCount, "all 5 visible strings should be localized")

	var buf bytes.Buffer
	writer.SetLocale(locale)
	require.NoError(t, writer.SetOutputWriter(&buf))
	ch := testutil.PartsToChannel(parts)
	require.NoError(t, writer.Write(ctx, ch))
	writer.Close()

	out := buf.String()

	// Every visible string was localized; no source string leaks through.
	for src, tgt := range translations {
		assert.Contains(t, out, tgt, "output should contain localized %q", src)
		assert.NotContains(t, out, ">"+src+"<", "raw source %q should not survive between tags", src)
	}

	// Markup and attributes preserved: the button/anchor tags and href stay.
	assert.Contains(t, out, "<button>")
	assert.Contains(t, out, "</button>")
	assert.Contains(t, out, `<a href="/help">`)
	assert.Contains(t, out, "</a>")
	assert.Contains(t, out, `<meta charset="utf-8"`)

	// lang retargeted en -> fr on the <html> element.
	assert.Contains(t, out, `lang="fr"`)
	assert.NotContains(t, out, `lang="en"`)
}
