package openxml

import (
	"archive/zip"
	"bytes"
	"fmt"
	"os"
	"strings"
	"testing"
)

// buildUnfusedWML generates a representative WML body containing the
// un-fused run adjacencies the post-serialization fuses target, repeated
// to reach roughly targetKB. Mirrors what the writer's skeleton path
// emits before the fuse stack runs: one <w:r> per source run.
func buildUnfusedWML(targetKB int) []byte {
	// One paragraph exercising br→text, the ptab alternation, fldChar-end
	// →text, and a bare pict→text adjacency — all as separate <w:r>.
	para := `<w:p><w:pPr><w:pStyle w:val="Normal"/></w:pPr>` +
		`<w:r><w:br/></w:r><w:r><w:t xml:space="preserve"> lead</w:t></w:r>` +
		`<w:r><w:t>left</w:t></w:r><w:r><w:ptab w:relativeTo="margin" w:alignment="center" w:leader="none"/></w:r>` +
		`<w:r><w:t>center</w:t></w:r><w:r><w:ptab w:relativeTo="margin" w:alignment="right" w:leader="none"/></w:r>` +
		`<w:r><w:t>right</w:t></w:r>` +
		`<w:r><w:fldChar w:fldCharType="end"/></w:r><w:r><w:t>.</w:t></w:r>` +
		`<w:r><w:rPr><w:rFonts w:ascii="Arial" w:hAnsi="Arial"/></w:rPr><w:t>styled</w:t></w:r>` +
		`<w:r><w:rPr><w:rFonts w:ascii="Arial" w:hAnsi="Arial"/></w:rPr><w:t> more</w:t></w:r>` +
		`</w:p>`
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>`)
	b.WriteString(`<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body>`)
	target := targetKB * 1024
	for b.Len() < target {
		b.WriteString(para)
	}
	b.WriteString(`</w:body></w:document>`)
	return []byte(b.String())
}

// regexFuseStack applies the four post-serialization run-fuse passes in
// the same order postNonWSOForName + postWML invoke them. This is the
// cost baseline we're trying to beat.
func regexFuseStack(data []byte) []byte {
	if bytes.Contains(data, []byte(`<w:br`)) {
		// (fuseBareBrAndTextRuns was retired in f786bd3d — the structural
		// emitRunEnvelopes handles br→text now. Excluded from the stack.)
	}
	if bytes.Contains(data, []byte(`fldCharType="end"`)) {
		data = fuseBareFldCharEndAndTextRuns(data)
	}
	if bytes.Contains(data, []byte(`<w:ptab`)) {
		data = fuseBareTextAndPTabRuns(data)
	}
	if bytes.Contains(data, []byte(`<w:annotationRef/>`)) {
		data = fuseSameRPrAnnotationRefAndTextRuns(data)
	}
	if bytes.Contains(data, []byte(`<w:r><w:pict>`)) {
		data = fuseBarePictAndRPrTextRuns(data)
	}
	return data
}

func benchSizes() []int { return []int{16, 128, 512} } // KB

func BenchmarkRegexFuseStack(b *testing.B) {
	for _, kb := range benchSizes() {
		in := buildUnfusedWML(kb)
		b.Run(fmt.Sprintf("%dKB", kb), func(b *testing.B) {
			b.SetBytes(int64(len(in)))
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				cp := make([]byte, len(in))
				copy(cp, in)
				_ = regexFuseStack(cp)
			}
		})
	}
}

func BenchmarkStreamingRunMerge(b *testing.B) {
	for _, kb := range benchSizes() {
		in := buildUnfusedWML(kb)
		b.Run(fmt.Sprintf("%dKB", kb), func(b *testing.B) {
			b.SetBytes(int64(len(in)))
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				_ = streamingRunMerge(in)
			}
		})
	}
}

// BenchmarkRealPart runs both on a real fixture's document.xml when the
// parity corpus is present (skipped otherwise).
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
			doc, _ = readAllSmall(rc)
			rc.Close()
			break
		}
	}
	if doc == nil {
		b.Skip("no word/document.xml")
	}
	b.Run("regex", func(b *testing.B) {
		b.SetBytes(int64(len(doc)))
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			cp := make([]byte, len(doc))
			copy(cp, doc)
			_ = regexFuseStack(cp)
		}
	})
	b.Run("streaming", func(b *testing.B) {
		b.SetBytes(int64(len(doc)))
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_ = streamingRunMerge(doc)
		}
	})
}

func readAllSmall(r interface{ Read([]byte) (int, error) }) ([]byte, error) {
	var buf bytes.Buffer
	tmp := make([]byte, 32*1024)
	for {
		n, err := r.Read(tmp)
		if n > 0 {
			buf.Write(tmp[:n])
		}
		if err != nil {
			if err.Error() == "EOF" {
				return buf.Bytes(), nil
			}
			return buf.Bytes(), nil // zip reader returns io.EOF; treat all stop as done
		}
	}
}

// TestStreamingRunMergeFusesAdjacentEqualRPr is a smoke test that the
// prototype actually fuses (so the benchmark isn't measuring a no-op).
func TestStreamingRunMergeFusesAdjacentEqualRPr(t *testing.T) {
	in := []byte(`<w:p>` +
		`<w:r><w:t>a</w:t></w:r><w:r><w:t>b</w:t></w:r>` + // empty rPr ×2 → fuse
		`<w:r><w:rPr><w:b/></w:rPr><w:t>c</w:t></w:r>` + // different rPr → boundary
		`</w:p>`)
	got := string(streamingRunMerge(in))
	wantFused := `<w:r><w:t>a</w:t><w:t>b</w:t></w:r>`
	if !strings.Contains(got, wantFused) {
		t.Errorf("expected adjacent empty-rPr runs to fuse into %q; got %q", wantFused, got)
	}
	if !strings.Contains(got, `<w:r><w:rPr><w:b/></w:rPr><w:t>c</w:t></w:r>`) {
		t.Errorf("expected the <w:b/> run to stay separate; got %q", got)
	}
	// Idempotence: merging twice == once.
	if string(streamingRunMerge([]byte(got))) != got {
		t.Errorf("streamingRunMerge is not idempotent")
	}
}
