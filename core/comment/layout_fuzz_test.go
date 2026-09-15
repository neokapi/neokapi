package comment

import (
	"strings"
	"testing"
)

// fuzzBlockSpans are delimited comments in the layouts TypeScript, TSX and
// JavaScript write, JSDoc and a JSX comment among them.
var fuzzBlockSpans = []struct{ span, indent string }{
	{"/* Parses the input. */", ""},
	{"/** Parses the input. */", ""},
	{"/**\n * Parses the input.\n *\n * @param src - the source\n */", ""},
	{"/**\n   * Indented under a class.\n   */", "  "},
	{"/* Starts on the opener\n   and ends on the closer. */", ""},
	{"/*\n  Bare lines.\n*/", ""},
	{"/*A comment inside JSX.*/", ""},
	{"/*\r\n * CRLF.\r\n */", ""},
}

// fuzzLineSources are small files each holding one line comment group, which
// opens at the first "//" and ends at the end of the line holding last.
var fuzzLineSources = []struct{ src, last string }{
	{"// Parses the input.\n// Stops at the end.\nexport const x = 1;\n", "Stops at the end."},
	{"  //Tight against the marker.\n  //Twice.\nx\n", "Twice."},
	{"/// A marker written longer.\nexport const x = 1;\n", "written longer."},
	{"export const x = 1; // after code\n", "after code"},
}

// FuzzLayoutRender renders arbitrary text into each delimited layout. A render
// is refused with a reason, or reads back through ParseLayout, holds its closer
// only at its end, and renders its own text to its own bytes.
func FuzzLayoutRender(f *testing.F) {
	for _, seed := range []string{
		"Parses */ and resumes.", "*", "* ", "/", "ends with *", "/starts with a slash",
		"@param src - the source", "{@link parse}", "one\n\ntwo", " leading space", "<p>JSX</p>", "",
	} {
		f.Add(seed)
	}
	marker := BlockMarker{Open: "/*", Close: "*/"}
	f.Fuzz(func(t *testing.T, text string) {
		lines, err := TextLines(text)
		if err != nil {
			return
		}
		for _, s := range fuzzBlockSpans {
			l, err := ParseLayout([]byte(s.span), s.indent, marker)
			if err != nil {
				t.Fatalf("%q: the seed layout does not parse: %v", s.span, err)
			}
			got, err := l.Render(lines)
			if err != nil {
				if _, ok := AsRefusal(err); !ok {
					t.Fatalf("%q in %q: an error that is not a refusal: %v", text, s.span, err)
				}
				continue
			}
			if i := strings.Index(string(got)[len(marker.Open):], marker.Close); i != len(got)-len(marker.Open)-len(marker.Close) {
				t.Fatalf("%q in %q: the rendered comment closes before its end: %q", text, s.span, got)
			}
			again, err := ParseLayout(got, s.indent, marker)
			if err != nil {
				t.Fatalf("%q in %q: the rendered comment %q has no layout: %v", text, s.span, got, err)
			}
			same, err := again.Render(strings.Split(again.Text(), "\n"))
			if err != nil || string(same) != string(got) {
				t.Fatalf("%q in %q: rendering %q's own text gives %q: %v", text, s.span, got, same, err)
			}
		}
	})
}

// FuzzLineLayoutRender renders arbitrary text into each line comment layout. A
// render is refused with a reason, or reads back through ParseLineLayout and
// renders its own text to its own bytes.
func FuzzLineLayoutRender(f *testing.F) {
	for _, seed := range []string{
		"Parses the input.", "/", "//", "/ ", " leading space", "one\n\ntwo", "eslint-disable-next-line", "",
	} {
		f.Add(seed)
	}
	markers := Markers{Line: []string{"//", "#!"}, Block: []BlockMarker{{Open: "/*", Close: "*/"}}}
	f.Fuzz(func(t *testing.T, text string) {
		lines, err := TextLines(text)
		if err != nil {
			return
		}
		for _, seed := range fuzzLineSources {
			src := seed.src
			start := strings.Index(src, "//")
			end := strings.LastIndex(src, seed.last) + len(seed.last)
			c := Comment{Start: start, End: end, Style: StyleLine}
			l, err := ParseLineLayout([]byte(src), c, markers)
			if err != nil {
				t.Fatalf("%q: the seed layout does not parse: %v", src, err)
			}
			got, err := l.Render(lines)
			if err != nil {
				if _, ok := AsRefusal(err); !ok {
					t.Fatalf("%q in %q: an error that is not a refusal: %v", text, src, err)
				}
				continue
			}
			after := src[:start] + string(got) + src[end:]
			again, err := ParseLineLayout([]byte(after), Comment{Start: start, End: start + len(got), Style: StyleLine}, markers)
			if err != nil {
				t.Fatalf("%q in %q: the rendered comment %q has no layout: %v", text, src, got, err)
			}
			same, err := again.Render(strings.Split(again.Text(), "\n"))
			if err != nil || string(same) != string(got) {
				t.Fatalf("%q in %q: rendering %q's own text gives %q: %v", text, src, got, same, err)
			}
		}
	})
}
