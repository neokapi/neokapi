package docling_test

import (
	"bufio"
	"bytes"
	"context"
	"os"
	"regexp"
	"strings"
	"testing"

	doclingfmt "github.com/neokapi/neokapi/core/formats/docling"
	markdownfmt "github.com/neokapi/neokapi/core/formats/markdown"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
)

// Cross-implementation parity: neokapi reads a real DoclingDocument and projects
// it to Markdown; we compare that projection against Docling's OWN
// export_to_markdown groundtruth (vendored as parity/<name>.gt.md). This is the
// strongest faithfulness signal available for a format with no Okapi bridge —
// "does neokapi extract the same content, in the same order, as the reference
// implementation?"
//
// The two Markdowns are NOT byte-identical, by design — each tool normalizes
// formatting differently. The test compares the SEMANTIC content (heading text
// sequence + normalized content units), and the differences are an explicit,
// closed ledger:
//
//	DIVERGENCE LEDGER (neokapi vs Docling export_to_markdown), polymers.json:
//	  1. Heading depth — Docling reserves H1 for the document title and renders
//	     section_headers one level deeper ("## "); neokapi renders RoleHeading at
//	     its own level. Heading TEXT and order are identical; only "#" count
//	     differs. (Compared depth-agnostically.)
//	  2. Inline bold/italic — a DoclingDocument TextItem.text is plain, so
//	     emphasis on a list lead-in ("**Barrier to gases**") is not recoverable
//	     from the JSON; both sides carry the same text, ours unbolded. (Emphasis
//	     markers stripped before compare.)
//	  3. Nested-list indentation — Docling indents sub-items; neokapi's Markdown
//	     projection emits flat "- ". (List markers stripped before compare.)
//	  4. key : value join — Docling joins a key node and its value node onto one
//	     line ("What it is : ..."); neokapi keeps them as separate blocks (two
//	     lines). Same text. (Split on " : " and colon-stripped before compare.)
//	  5. Image / page-break — Docling emits "<!-- image -->" / "<!-- page break -->"
//	     comments; neokapi emits neither. (Comment lines dropped before compare.)
//
// Anything OUTSIDE this ledger — dropped/added content, reordered headings — is
// a real regression and fails the test. (Wiring this up is what caught a reader
// bug that dropped all nested list content; see emitText child recursion.)

var headingRE = regexp.MustCompile(`^#{1,6}\s+(.+?)\s*$`)

func renderMarkdown(t *testing.T, jsonPath string) string {
	t.Helper()
	data, err := os.ReadFile(jsonPath)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	r := doclingfmt.NewReader()
	if err := r.Open(ctx, testutil.RawDocFromString(string(data), model.LocaleEnglish)); err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	parts := testutil.CollectParts(t, r.Read(ctx))

	w := markdownfmt.NewWriter() // no skeleton store → semantic (writeFromBlocks) mode
	var buf bytes.Buffer
	if err := w.SetOutputWriter(&buf); err != nil {
		t.Fatal(err)
	}
	ch := make(chan *model.Part, len(parts)+1)
	for _, p := range parts {
		ch <- p
	}
	close(ch)
	if err := w.Write(ctx, ch); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}

// headingTexts returns the ordered heading texts (depth-agnostic — ledger #1).
func headingTexts(md string) []string {
	var out []string
	sc := bufio.NewScanner(strings.NewReader(md))
	for sc.Scan() {
		if m := headingRE.FindStringSubmatch(sc.Text()); m != nil {
			out = append(out, strings.TrimSpace(m[1]))
		}
	}
	return out
}

// contentUnits returns the ordered non-heading content, normalized per the
// divergence ledger (emphasis/list markers stripped, key:value split, image/
// page-break comments + table rows dropped, whitespace + case folded).
func contentUnits(md string) []string {
	var out []string
	sc := bufio.NewScanner(strings.NewReader(md))
	sc.Buffer(make([]byte, 1024*1024), 1024*1024)
	for sc.Scan() {
		s := sc.Text()
		if headingRE.MatchString(s) {
			continue
		}
		trimmed := strings.TrimSpace(s)
		if trimmed == "" || strings.HasPrefix(trimmed, "<!--") || strings.HasPrefix(trimmed, "|") {
			continue // ledger #5 (comments) + GFM table rows
		}
		s = strings.TrimLeft(s, " \t")                                          // ledger #3 (indentation)
		s = strings.TrimPrefix(strings.TrimPrefix(s, "- "), "* ")               // ledger #3 (list markers)
		s = strings.NewReplacer("**", "", "*", "", "`", "", "_", "").Replace(s) // ledger #2 (emphasis)
		for piece := range strings.SplitSeq(s, " : ") {                         // ledger #4 (key:value join)
			p := strings.TrimSpace(strings.Trim(strings.TrimSpace(piece), ":"))
			p = strings.ToLower(strings.Join(strings.Fields(p), " "))
			if p != "" {
				out = append(out, p)
			}
		}
	}
	return out
}

func TestParity_Polymers(t *testing.T) {
	const jsonPath = "testdata/parity/polymers.json"
	const gtPath = "testdata/parity/polymers.gt.md"

	ours := renderMarkdown(t, jsonPath)
	gtBytes, err := os.ReadFile(gtPath)
	if err != nil {
		t.Fatal(err)
	}
	gt := string(gtBytes)

	// Heading text + order must match Docling exactly (depth-agnostic).
	oh, gh := headingTexts(ours), headingTexts(gt)
	if len(oh) != len(gh) {
		t.Fatalf("heading count differs: ours=%d docling=%d\nours=%v\ndocling=%v", len(oh), len(gh), oh, gh)
	}
	for i := range gh {
		if oh[i] != gh[i] {
			t.Errorf("heading %d differs:\n ours=%q\n docling=%q", i, oh[i], gh[i])
		}
	}

	// Content units must match Docling exactly after ledger normalization —
	// proving no body content is dropped, added, or reordered.
	ou, gu := contentUnits(ours), contentUnits(gt)
	if len(ou) != len(gu) {
		t.Fatalf("content-unit count differs: ours=%d docling=%d", len(ou), len(gu))
	}
	for i := range gu {
		if ou[i] != gu[i] {
			t.Errorf("content unit %d differs (outside the divergence ledger):\n ours=%q\n docling=%q", i, ou[i], gu[i])
		}
	}

	// And no source-format markup may leak into the Markdown.
	for _, leak := range []string{"<text>", "<heading", "<bold>", "<ched/>", "<fcel/>", "$ref"} {
		if strings.Contains(ours, leak) {
			t.Errorf("source markup leaked into Markdown: %q", leak)
		}
	}
}
