package main

import (
	"archive/zip"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"path"
	"slices"
	"sort"
	"strings"
)

// Ground truth is read from the document, not from another tool.
//
// This is the whole reason the eval can exist. A comparison scored against
// pandoc's output measures agreement with pandoc; a comparison scored against
// the file's own text nodes measures whether a converter saw what the format
// says is there. OOXML makes that available: the spec designates exactly which
// elements carry user-visible text, and reading them needs a zip reader and an
// XML parser rather than an opinion.
//
// What this does NOT establish:
//
//   - Structure. Headings, lists, tables and their nesting are not compared. A
//     converter that emits every word as one paragraph scores the same as one
//     that preserves the outline.
//   - Order. The metric is a multiset, so text that survives in the wrong place
//     counts as present.
//   - Anything outside the body part. Headers, footers, footnotes, comments and
//     speaker notes are excluded, because converters disagree about whether
//     those belong in the output at all and counting them would score that
//     disagreement rather than fidelity.
//
// What it does establish is the failure that matters most and is easiest to
// ship: content silently disappearing.

// bodyPart names, per format, the parts whose text nodes are the ground truth
// and the element that carries the text.
type spec struct {
	// match reports whether a zip entry is a body part.
	match func(name string) bool
	// element is the local name of the text-bearing element.
	element string
}

var specs = map[string]spec{
	".docx": {
		// One part: word/document.xml. Headers, footnotes and comments live in
		// sibling parts and are left out on purpose — see the note above.
		match:   func(n string) bool { return n == "word/document.xml" },
		element: "t",
	},
	".pptx": {
		// Every slide, and not the notes: a converter that omits speaker notes
		// is making an editorial choice rather than losing content.
		match: func(n string) bool {
			return strings.HasPrefix(n, "ppt/slides/slide") && path.Ext(n) == ".xml"
		},
		element: "t",
	},
	// .xlsx is handled by xlsxCellText rather than by a spec entry. A
	// spreadsheet's text lives in its cells, and the cells refer to a shared
	// string table rather than carrying their words, so no single part holds
	// the answer.
}

// supportedExts is every extension ground truth can be read for, sorted so a
// report reads the same twice.
//
// Not derived from specs alone: .xlsx has no spec entry because its text is
// assembled from two parts rather than read out of one.
func supportedExts() []string {
	out := make([]string, 0, len(specs)+1)
	for k := range specs {
		out = append(out, k)
	}
	out = append(out, ".xlsx")
	sort.Strings(out)
	return out
}

// readable reports whether ground truth can be read for an extension.
func readable(ext string) bool {
	return slices.Contains(supportedExts(), ext)
}

// groundTruth returns the text a document's own parts declare.
func groundTruth(file, ext string) ([]string, error) {
	if ext == ".xlsx" {
		return xlsxCellText(file)
	}
	sp, ok := specs[ext]
	if !ok {
		return nil, fmt.Errorf("no ground-truth spec for %s", ext)
	}
	z, err := zip.OpenReader(file)
	if err != nil {
		return nil, err
	}
	defer z.Close()

	var out []string
	for _, f := range z.File {
		if !sp.match(f.Name) {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return nil, err
		}
		texts, err := textNodes(rc, sp.element)
		_ = rc.Close()
		if err != nil {
			return nil, fmt.Errorf("%s: %w", f.Name, err)
		}
		out = append(out, texts...)
	}
	return out, nil
}

// textNodes pulls the character data out of every <element> in the stream.
//
// Streamed rather than unmarshalled into a struct, because the parts differ per
// format and only one element matters in each. The XML decoder handles entity
// unescaping, which a regex over the raw bytes would get wrong on the first
// document containing an ampersand.
func textNodes(r io.Reader, element string) ([]string, error) {
	dec := xml.NewDecoder(r)
	var out []string
	var depth int
	var buf strings.Builder
	for {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			return out, nil
		}
		if err != nil {
			return nil, err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			if t.Name.Local == element {
				depth++
			}
		case xml.CharData:
			if depth > 0 {
				buf.Write(t)
			}
		case xml.EndElement:
			if t.Name.Local == element && depth > 0 {
				depth--
				if depth == 0 {
					out = append(out, buf.String())
					buf.Reset()
				}
			}
		}
	}
}
