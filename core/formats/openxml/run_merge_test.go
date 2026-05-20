package openxml

import (
	"archive/zip"
	"bytes"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// run_merge_test.go validates the production run-envelope scanners
// (run_merge.go) against the REGEXES they replaced (#602/#603). The regex
// forms below are the retired implementations, kept here only as a
// behavioural oracle: every scanner must be byte-identical to its regex on
// the documented fixture shapes plus structural neighbours that must NOT
// fuse, so a divergence shows up in CI before the parity suite. The
// benchmarks quantify the win that motivated the migration (single-pass
// splice vs regex backtracking + the ptab fixpoint loop).

// ── Retired-regex oracle ────────────────────────────────────────────────

var (
	oracleFldCharRE = regexp.MustCompile(
		`(<w:r><w:t\b[^>]*>[^<]*</w:t></w:r>)<w:r>(<w:fldChar\b[^>]*\bw:fldCharType="end"[^>]*(?:/>|></w:fldChar>))</w:r><w:r>(<w:t\b[^>]*>[^<]*</w:t>)</w:r>`)
	oracleTextThenPtabRE = regexp.MustCompile(
		`<w:r>(<w:t\b[^>]*>[^<]*</w:t>)</w:r><w:r>(<w:ptab\b[^>]*(?:/>|></w:ptab>))</w:r>`)
	oraclePtabThenTextRE = regexp.MustCompile(
		`<w:r>(<w:ptab\b[^>]*(?:/>|></w:ptab>))</w:r><w:r>(<w:t\b[^>]*>[^<]*</w:t>)</w:r>`)
	oracleTextPtabPairRE = regexp.MustCompile(
		`<w:r>((?:<w:t\b[^>]*>[^<]*</w:t>|<w:ptab\b[^>]*(?:/>|></w:ptab>))+)</w:r>` +
			`<w:r>((?:<w:t\b[^>]*>[^<]*</w:t>|<w:ptab\b[^>]*(?:/>|></w:ptab>))+)</w:r>`)
	oracleAnnotationRE = regexp.MustCompile(
		`<w:r><w:rPr><w:rStyle w:val="CommentReference"(?:/>|></w:rStyle>)</w:rPr><w:annotationRef/></w:r>` +
			`<w:r><w:rPr><w:rStyle w:val="CommentReference"(?:/>|></w:rStyle>)</w:rPr>(<w:t\b[^>]*>[^<]*</w:t>)</w:r>`)
)

func oracleFldCharEndText(data []byte) []byte {
	if !bytes.Contains(data, []byte("<w:r><w:fldChar")) {
		return data
	}
	return oracleFldCharRE.ReplaceAll(data, []byte(`$1<w:r>$2$3</w:r>`))
}

func oracleTextPtab(data []byte) []byte {
	if !bytes.Contains(data, []byte("<w:ptab")) {
		return data
	}
	for {
		next := oracleTextThenPtabRE.ReplaceAll(data, []byte(`<w:r>$1$2</w:r>`))
		next = oraclePtabThenTextRE.ReplaceAll(next, []byte(`<w:r>$1$2</w:r>`))
		next = oracleTextPtabPairRE.ReplaceAll(next, []byte(`<w:r>$1$2</w:r>`))
		if bytes.Equal(next, data) {
			return data
		}
		data = next
	}
}

func oracleAnnotationRefText(data []byte) []byte {
	if !bytes.Contains(data, []byte("<w:annotationRef/>")) {
		return data
	}
	return oracleAnnotationRE.ReplaceAll(data,
		[]byte(`<w:r><w:rPr><w:rStyle w:val="CommentReference"/></w:rPr><w:annotationRef/>$1</w:r>`))
}

func oracleFuseStack(data []byte) []byte {
	data = oracleFldCharEndText(data)
	data = oracleTextPtab(data)
	data = oracleAnnotationRefText(data)
	return data
}

func prodFuseStack(data []byte) []byte {
	data = fuseFldCharEndText(data)
	data = fuseTextPtabEnvelopes(data)
	data = fuseAnnotationRefText(data)
	return data
}

// ── Differential tests (scanner == retired regex) ─────────────────────────

func diffEqual(t *testing.T, name string, got, want []byte) {
	t.Helper()
	if string(got) != string(want) {
		t.Errorf("%s diverged from regex:\n  scanner: %q\n  regex:   %q", name, got, want)
	}
}

func TestFuseFldCharEndText_MatchesRegex(t *testing.T) {
	cases := []string{
		// 830-4.docx shape: leading display text, bare fldChar-end, trailing ".".
		`<w:p><w:r><w:t xml:space="preserve">disp</w:t></w:r><w:r><w:fldChar w:fldCharType="end"/></w:r><w:r><w:t>.</w:t></w:r></w:p>`,
		// open/close fldChar form.
		`<w:r><w:t>a</w:t></w:r><w:r><w:fldChar w:fldCharType="end"></w:fldChar></w:r><w:r><w:t>b</w:t></w:r>`,
		// No leading text (XE-marker shape) — must NOT fuse.
		`<w:r><w:instrText>XE</w:instrText></w:r><w:r><w:fldChar w:fldCharType="end"/></w:r><w:r><w:t>x</w:t></w:r>`,
		// fldChar begin — must NOT match (only end fuses).
		`<w:r><w:t>a</w:t></w:r><w:r><w:fldChar w:fldCharType="begin"/></w:r><w:r><w:t>b</w:t></w:r>`,
		// Two triplets in a row (non-overlapping ReplaceAll semantics).
		`<w:r><w:t>a</w:t></w:r><w:r><w:fldChar w:fldCharType="end"/></w:r><w:r><w:t>b</w:t></w:r>` +
			`<w:r><w:t>c</w:t></w:r><w:r><w:fldChar w:fldCharType="end"/></w:r><w:r><w:t>d</w:t></w:r>`,
		// Trailing text missing — no fuse.
		`<w:r><w:t>a</w:t></w:r><w:r><w:fldChar w:fldCharType="end"/></w:r><w:p/>`,
		// No fldChar at all — no-op.
		`<w:r><w:t>plain</w:t></w:r>`,
	}
	for i, c := range cases {
		in := []byte(c)
		diffEqual(t, "fldChar#"+itoa(i), fuseFldCharEndText(in), oracleFldCharEndText(in))
	}
}

func TestFuseTextPtabEnvelopes_MatchesRegex(t *testing.T) {
	cases := []string{
		// OpenXML_text_reference header2.xml shape: t/ptab/t/ptab/t alternation.
		`<w:r><w:t>left</w:t></w:r><w:r><w:ptab w:relativeTo="margin" w:alignment="center" w:leader="none"/></w:r>` +
			`<w:r><w:t>center</w:t></w:r><w:r><w:ptab w:relativeTo="margin" w:alignment="right" w:leader="none"/></w:r>` +
			`<w:r><w:t>right</w:t></w:r>`,
		// ptab-first adjacency.
		`<w:r><w:ptab w:alignment="center"/></w:r><w:r><w:t>x</w:t></w:r>`,
		// open/close ptab form.
		`<w:r><w:t>a</w:t></w:r><w:r><w:ptab w:alignment="left"></w:ptab></w:r>`,
		// ptab with surrounding non-fuseable content.
		`<w:p><w:pPr><w:pStyle w:val="X"/></w:pPr><w:r><w:t>a</w:t></w:r><w:r><w:ptab w:alignment="left"/></w:r><w:r><w:t>b</w:t></w:r></w:p>`,
		// Lone ptab (no neighbour) — no fuse.
		`<w:r><w:ptab w:alignment="left"/></w:r>`,
		// rPr-bearing text next to ptab — must NOT fuse (not bare).
		`<w:r><w:rPr><w:b/></w:rPr><w:t>a</w:t></w:r><w:r><w:ptab w:alignment="left"/></w:r>`,
		// No ptab — no-op.
		`<w:r><w:t>a</w:t></w:r><w:r><w:t>b</w:t></w:r>`,
	}
	for i, c := range cases {
		in := []byte(c)
		diffEqual(t, "ptab#"+itoa(i), fuseTextPtabEnvelopes(in), oracleTextPtab(in))
	}
}

func TestFuseAnnotationRefText_MatchesRegex(t *testing.T) {
	cases := []string{
		// comments.xml shape: CommentReference annotationRef marker + text.
		`<w:r><w:rPr><w:rStyle w:val="CommentReference"/></w:rPr><w:annotationRef/></w:r>` +
			`<w:r><w:rPr><w:rStyle w:val="CommentReference"/></w:rPr><w:t>note</w:t></w:r>`,
		// open/close rStyle form on both sides.
		`<w:r><w:rPr><w:rStyle w:val="CommentReference"></w:rStyle></w:rPr><w:annotationRef/></w:r>` +
			`<w:r><w:rPr><w:rStyle w:val="CommentReference"></w:rStyle></w:rPr><w:t>n</w:t></w:r>`,
		// Mixed self-close marker, open/close text.
		`<w:r><w:rPr><w:rStyle w:val="CommentReference"/></w:rPr><w:annotationRef/></w:r>` +
			`<w:r><w:rPr><w:rStyle w:val="CommentReference"></w:rStyle></w:rPr><w:t>m</w:t></w:r>`,
		// annotationRef without a following CommentReference text — no fuse.
		`<w:r><w:rPr><w:rStyle w:val="CommentReference"/></w:rPr><w:annotationRef/></w:r><w:r><w:t>x</w:t></w:r>`,
		// Different rStyle val — no fuse.
		`<w:r><w:rPr><w:rStyle w:val="Other"/></w:rPr><w:annotationRef/></w:r>` +
			`<w:r><w:rPr><w:rStyle w:val="CommentReference"/></w:rPr><w:t>x</w:t></w:r>`,
		// No annotationRef — no-op.
		`<w:r><w:t>plain</w:t></w:r>`,
	}
	for i, c := range cases {
		in := []byte(c)
		diffEqual(t, "annot#"+itoa(i), fuseAnnotationRefText(in), oracleAnnotationRefText(in))
	}
}

func itoa(i int) string { return strconv.Itoa(i) }

// ── Benchmarks (regex backtracking vs single-pass splice) ─────────────────

// buildUnfusedWML generates a representative WML body containing the
// un-fused run adjacencies the fuses target, repeated to reach ~targetKB.
func buildUnfusedWML(targetKB int) []byte {
	para := `<w:p><w:pPr><w:pStyle w:val="Normal"/></w:pPr>` +
		`<w:r><w:t>left</w:t></w:r><w:r><w:ptab w:relativeTo="margin" w:alignment="center" w:leader="none"/></w:r>` +
		`<w:r><w:t>center</w:t></w:r><w:r><w:ptab w:relativeTo="margin" w:alignment="right" w:leader="none"/></w:r>` +
		`<w:r><w:t>right</w:t></w:r>` +
		`<w:r><w:t>disp</w:t></w:r><w:r><w:fldChar w:fldCharType="end"/></w:r><w:r><w:t>.</w:t></w:r>` +
		`<w:r><w:rPr><w:rStyle w:val="CommentReference"/></w:rPr><w:annotationRef/></w:r>` +
		`<w:r><w:rPr><w:rStyle w:val="CommentReference"/></w:rPr><w:t>note</w:t></w:r>` +
		`</w:p>`
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>`)
	b.WriteString(`<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body>`)
	for b.Len() < targetKB*1024 {
		b.WriteString(para)
	}
	b.WriteString(`</w:body></w:document>`)
	return []byte(b.String())
}

func benchSizes() []int { return []int{16, 128, 512} } // KB

func BenchmarkOracleFuseStack(b *testing.B) {
	for _, kb := range benchSizes() {
		in := buildUnfusedWML(kb)
		b.Run(fmt.Sprintf("%dKB", kb), func(b *testing.B) {
			b.SetBytes(int64(len(in)))
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				cp := make([]byte, len(in))
				copy(cp, in)
				_ = oracleFuseStack(cp)
			}
		})
	}
}

func BenchmarkProdFuseStack(b *testing.B) {
	for _, kb := range benchSizes() {
		in := buildUnfusedWML(kb)
		b.Run(fmt.Sprintf("%dKB", kb), func(b *testing.B) {
			b.SetBytes(int64(len(in)))
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				_ = prodFuseStack(in)
			}
		})
	}
}

// BenchmarkRealPart runs both on a real fixture's document.xml when the
// parity corpus is present (skipped otherwise), and asserts the two stacks
// agree on real bytes.
func BenchmarkRealPart(b *testing.B) {
	path := os.Getenv("OPENXML_BENCH_DOCX")
	if path == "" {
		path = "../../../.parity/okapi-testdata/1.48.0/okapi/filters/openxml/src/test/resources/Hangs.docx"
	}
	data, err := os.ReadFile(path)
	if err != nil {
		b.Skipf("fixture not available: %v", err)
	}
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		b.Skipf("not a zip: %v", err)
	}
	var doc []byte
	for _, f := range zr.File {
		if f.Name == "word/document.xml" {
			rc, _ := f.Open()
			var buf bytes.Buffer
			_, _ = buf.ReadFrom(rc)
			rc.Close()
			doc = buf.Bytes()
			break
		}
	}
	if doc == nil {
		b.Skip("no word/document.xml")
	}
	if !bytes.Equal(oracleFuseStack(append([]byte(nil), doc...)), prodFuseStack(doc)) {
		b.Fatal("scanner and regex disagree on real part")
	}
	b.Run("regex", func(b *testing.B) {
		b.SetBytes(int64(len(doc)))
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			cp := make([]byte, len(doc))
			copy(cp, doc)
			_ = oracleFuseStack(cp)
		}
	})
	b.Run("scanner", func(b *testing.B) {
		b.SetBytes(int64(len(doc)))
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_ = prodFuseStack(doc)
		}
	})
}
