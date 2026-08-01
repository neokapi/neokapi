package openxml

import (
	"archive/zip"
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
)

// This file is `package openxml` rather than `openxml_test` because every test
// in the package is — there is no external test package here.

// fuzzReadOpenXML drives the OpenXML reader over input without ever calling
// t.Fatal, so it is safe on arbitrary fuzz input. The optional skeleton store
// is shared with the writer on the round-trip path.
func fuzzReadOpenXML(ctx context.Context, input []byte, store *format.SkeletonStore) (parts []*model.Part, hadErr bool) {
	reader := NewReader()
	if store != nil {
		reader.SetSkeletonStore(store)
	}
	doc := testutil.RawDocFromReader(bytes.NewReader(input), "test://input.docx", model.LocaleEnglish)
	if err := reader.Open(ctx, doc); err != nil {
		return nil, true
	}
	defer reader.Close()
	for result := range reader.Read(ctx) {
		if result.Error != nil {
			hadErr = true
			continue
		}
		if result.Part != nil {
			parts = append(parts, result.Part)
		}
	}
	return parts, hadErr
}

func fuzzOpenXMLTexts(parts []*model.Part) []string {
	var texts []string
	for _, b := range testutil.FilterBlocks(parts) {
		texts = append(texts, b.SourceText())
	}
	sort.Strings(texts)
	return texts
}

func fuzzOpenXMLSeedFile(f *testing.F, name string) {
	f.Helper()
	if data, err := os.ReadFile(filepath.Join("testdata", name)); err == nil {
		f.Add(data)
	}
}

// fuzzRepackDocx rebuilds an OPC package with some entries replaced, so a seed
// can be structurally damaged while still being a readable zip. Returns nil if
// the input is not a readable zip.
func fuzzRepackDocx(data []byte, replace map[string][]byte) []byte {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil
	}
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	seen := map[string]bool{}
	for _, f := range zr.File {
		body, ok := replace[f.Name]
		if ok {
			seen[f.Name] = true
		} else {
			rc, err := f.Open()
			if err != nil {
				return nil
			}
			body, err = io.ReadAll(rc)
			rc.Close()
			if err != nil {
				return nil
			}
		}
		w, err := zw.Create(f.Name)
		if err != nil {
			return nil
		}
		if _, err := w.Write(body); err != nil {
			return nil
		}
	}
	for name, body := range replace {
		if seen[name] {
			continue
		}
		w, err := zw.Create(name)
		if err != nil {
			return nil
		}
		if _, err := w.Write(body); err != nil {
			return nil
		}
	}
	if err := zw.Close(); err != nil {
		return nil
	}
	return buf.Bytes()
}

// seedDamagedOpenXML adds packages that are structurally damaged but still
// PARSEABLE. Corrupt zip framing and non-zip garbage are already covered by
// malformed_test.go; the region that is not covered is an OPC package whose
// parts disagree with one another — a content-types map that does not describe
// the parts present, a relationship pointing at a missing target, a document
// part whose XML lost a terminator.
func seedDamagedOpenXML(f *testing.F) {
	f.Helper()
	base, err := os.ReadFile(filepath.Join("testdata", "simple.docx"))
	if err != nil {
		return
	}
	variants := []map[string][]byte{
		// [Content_Types].xml emptied of its overrides.
		{"[Content_Types].xml": []byte(`<?xml version="1.0"?><Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"/>`)},
		// The package relationships point at a part that is not present.
		{"_rels/.rels": []byte(`<?xml version="1.0"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/missing.xml"/></Relationships>`)},
		// A relationship target that climbs out of the package.
		{"_rels/.rels": []byte(`<?xml version="1.0"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="../../../etc/passwd"/></Relationships>`)},
		// The document part with a lost </w:p> — the #1588 shape inside the
		// container's own XML, where two paragraphs may silently become one.
		{"word/document.xml": []byte(`<?xml version="1.0"?><w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body><w:p><w:r><w:t>one</w:t></w:r><w:p><w:r><w:t>two</w:t></w:r></w:p></w:body></w:document>`)},
		// A run with no text element.
		{"word/document.xml": []byte(`<?xml version="1.0"?><w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body><w:p><w:r/></w:p></w:body></w:document>`)},
		// Deeply nested paragraphs, which OOXML does not allow.
		{"word/document.xml": []byte(`<?xml version="1.0"?><w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body><w:p><w:p><w:p><w:r><w:t>deep</w:t></w:r></w:p></w:p></w:p></w:body></w:document>`)},
		// An empty document part.
		{"word/document.xml": {}},
		// An extra part nobody declared.
		{"word/smuggled.xml": []byte(`<?xml version="1.0"?><x/>`)},
	}
	for _, v := range variants {
		if b := fuzzRepackDocx(base, v); b != nil {
			f.Add(b)
		}
	}
}

// FuzzReadOpenXML asserts the OpenXML reader never panics and always terminates
// on arbitrary input. An OPC package is a zip of XML parts wired together by
// relationships, so this covers the container, the part graph, and the
// WordprocessingML parse in one target.
func FuzzReadOpenXML(f *testing.F) {
	fuzzOpenXMLSeedFile(f, "simple.docx")
	fuzzOpenXMLSeedFile(f, "formatted.docx")
	seedDamagedOpenXML(f)
	f.Add([]byte("not a zip"))
	f.Add([]byte("PK\x03\x04"))
	f.Add([]byte(""))

	f.Fuzz(func(t *testing.T, data []byte) {
		_, _ = fuzzReadOpenXML(t.Context(), data, nil)
	})
}

// FuzzRoundTripOpenXML asserts read → write → read is stable in content.
//
// The writer rewrites the original container rather than generating one, so it
// requires both the original bytes (SetOriginalContent) and a skeleton store;
// the same input serves as both, which keeps the round-trip in memory.
func FuzzRoundTripOpenXML(f *testing.F) {
	fuzzOpenXMLSeedFile(f, "simple.docx")
	fuzzOpenXMLSeedFile(f, "formatted.docx")
	seedDamagedOpenXML(f)

	f.Fuzz(func(t *testing.T, data []byte) {
		ctx := t.Context()
		store := format.NewMemorySkeletonStore()
		parts1, hadErr := fuzzReadOpenXML(ctx, data, store)
		if hadErr || len(testutil.FilterBlocks(parts1)) == 0 {
			return
		}
		texts1 := fuzzOpenXMLTexts(parts1)

		var buf bytes.Buffer
		writer := NewWriter()
		writer.SetOriginalContent(data)
		writer.SetSkeletonStore(store)
		if err := writer.SetOutputWriter(&buf); err != nil {
			return
		}
		if err := writer.Write(ctx, testutil.PartsToChannel(parts1)); err != nil {
			return
		}
		writer.Close()

		parts2, hadErr2 := fuzzReadOpenXML(ctx, buf.Bytes(), nil)
		if hadErr2 {
			t.Fatalf("re-reading written OPC package failed; round-trip is not stable\ninput len: %d\noutput len: %d", len(data), buf.Len())
		}
		texts2 := fuzzOpenXMLTexts(parts2)
		if len(texts1) != len(texts2) {
			t.Fatalf("round-trip changed block count: %d -> %d\npass1: %q\npass2: %q", len(texts1), len(texts2), texts1, texts2)
		}
		for i := range texts1 {
			if texts1[i] != texts2[i] {
				t.Fatalf("round-trip drift in source text\npass1: %q\npass2: %q", texts1, texts2)
			}
		}
	})
}
