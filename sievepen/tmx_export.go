package sievepen

import (
	"bufio"
	"errors"
	"fmt"
	"html"
	"io"
	"sort"
	"strings"

	"github.com/neokapi/neokapi/core/model"
)

// EntryProvider is implemented by TM backends that can list all entries.
type EntryProvider interface {
	Entries() []TMEntry
}

// ExportTMX writes multilingual TMX to writer. locales filters which
// variants are emitted (empty = every variant present on the entry). Each
// entry becomes a single <tu> with one <tuv> per selected variant.
func ExportTMX(tm TranslationMemory, writer io.Writer, locales []model.LocaleID) error {
	provider, ok := tm.(EntryProvider)
	if !ok {
		return errors.New("TM does not support entry listing")
	}

	allowed := make(map[model.LocaleID]bool, len(locales))
	for _, l := range locales {
		allowed[l] = true
	}
	filter := len(allowed) > 0

	entries := provider.Entries()

	bw := bufio.NewWriter(writer)
	defer bw.Flush()

	if _, err := bw.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n"); err != nil {
		return err
	}
	if _, err := bw.WriteString(`<!DOCTYPE tmx SYSTEM "tmx14.dtd">` + "\n"); err != nil {
		return err
	}
	if _, err := bw.WriteString(`<tmx version="1.4">` + "\n"); err != nil {
		return err
	}

	defaultSrcLang := "en"
	if len(entries) > 0 && entries[0].HintSrcLang != "" {
		defaultSrcLang = string(entries[0].HintSrcLang)
	}
	if _, err := bw.WriteString(fmt.Sprintf(
		`  <header creationtool="neokapi-sievepen" creationtoolversion="2.0" segtype="sentence" adminlang="%s" srclang="%s" datatype="plaintext" o-tmf="unknown"/>`+"\n",
		xmlAttr(defaultSrcLang), xmlAttr(defaultSrcLang))); err != nil {
		return err
	}
	if _, err := bw.WriteString("  <body>\n"); err != nil {
		return err
	}

	for _, entry := range entries {
		// Determine which variants to emit.
		var locales []model.LocaleID
		for loc := range entry.Variants {
			if filter && !allowed[loc] {
				continue
			}
			locales = append(locales, loc)
		}
		if len(locales) == 0 {
			continue
		}
		sort.Slice(locales, func(i, j int) bool { return locales[i] < locales[j] })

		srcLang := entry.HintSrcLang
		if srcLang == "" {
			srcLang = locales[0]
		}

		fmt.Fprintf(bw, `    <tu tuid="%s" srclang="%s"`, xmlAttr(entry.ID), xmlAttr(string(srcLang)))
		if !entry.CreatedAt.IsZero() {
			fmt.Fprintf(bw, ` creationdate="%s"`, entry.CreatedAt.UTC().Format("20060102T150405Z"))
		}
		if !entry.UpdatedAt.IsZero() {
			fmt.Fprintf(bw, ` changedate="%s"`, entry.UpdatedAt.UTC().Format("20060102T150405Z"))
		}
		_, _ = bw.WriteString(">\n")

		// TU-level props: entry properties + entity markers.
		for k, v := range entry.Properties {
			fmt.Fprintf(bw, `      <prop type="%s">%s</prop>`+"\n", xmlAttr(k), xmlEscape(v))
		}
		if entry.Note != "" {
			fmt.Fprintf(bw, `      <note>%s</note>`+"\n", xmlEscape(entry.Note))
		}

		for _, loc := range locales {
			runs := entry.Variants[loc]
			if len(runs) == 0 {
				continue
			}
			fmt.Fprintf(bw, `      <tuv xml:lang="%s">`+"\n", xmlAttr(string(loc)))
			_, _ = bw.WriteString(`        <seg>`)
			if err := fragmentToTMXSeg(bw, model.RunsToFragment(runs)); err != nil {
				return err
			}
			_, _ = bw.WriteString("</seg>\n")
			_, _ = bw.WriteString("      </tuv>\n")
		}

		_, _ = bw.WriteString("    </tu>\n")
	}

	_, _ = bw.WriteString("  </body>\n")
	_, _ = bw.WriteString("</tmx>\n")
	return nil
}

// ExportTMXBilingual writes a bilingual TMX containing only variants for
// the given source and target locales. Preserved for callers still passing
// a (src, tgt) pair.
func ExportTMXBilingual(tm TranslationMemory, writer io.Writer, src, tgt model.LocaleID) error {
	return ExportTMX(tm, writer, []model.LocaleID{src, tgt})
}

// fragmentToTMXSeg serializes a Fragment's CodedText + Spans back to TMX
// inline markup. Dispatches on Span.SubType for TMX-native spans and
// falls back to <ph type="..."> for cross-format spans.
func fragmentToTMXSeg(w *bufio.Writer, frag *model.Fragment) error {
	if frag == nil {
		return nil
	}
	spanIdx := 0
	for _, r := range frag.CodedText {
		switch r {
		case '\uE001', '\uE002', '\uE003':
			if spanIdx >= len(frag.Spans) {
				continue
			}
			s := frag.Spans[spanIdx]
			spanIdx++
			if err := writeSpanAsTMX(w, s); err != nil {
				return err
			}
		default:
			if _, err := w.WriteString(xmlEscape(string(r))); err != nil {
				return err
			}
		}
	}
	return nil
}

func writeSpanAsTMX(w *bufio.Writer, s *model.Span) error {
	subType := s.SubType
	switch subType {
	case "tmx:ph":
		fmt.Fprintf(w, `<ph x="%s">%s</ph>`, xmlAttr(s.ID), xmlEscape(s.Data))
	case "tmx:bpt":
		fmt.Fprintf(w, `<bpt i="%s">%s</bpt>`, xmlAttr(s.ID), xmlEscape(s.Data))
	case "tmx:ept":
		fmt.Fprintf(w, `<ept i="%s">%s</ept>`, xmlAttr(s.ID), xmlEscape(s.Data))
	case "tmx:it":
		pos := "begin"
		if s.SpanType == model.SpanClosing {
			pos = "end"
		}
		fmt.Fprintf(w, `<it pos="%s">%s</it>`, pos, xmlEscape(s.Data))
	case "tmx:hi":
		if s.SpanType == model.SpanOpening {
			if s.Type != "" {
				fmt.Fprintf(w, `<hi type="%s">`, xmlAttr(s.Type))
			} else {
				_, _ = w.WriteString(`<hi>`)
			}
		} else if s.SpanType == model.SpanClosing {
			_, _ = w.WriteString(`</hi>`)
		}
	case "tmx:sub":
		// Data already contains the serialized <sub>...</sub> markup.
		_, _ = w.WriteString(s.Data)
	case "tmx:ut":
		fmt.Fprintf(w, `<ut>%s</ut>`, xmlEscape(s.Data))
	case "tmx:raw":
		// Raw XML round-trip.
		_, _ = w.WriteString(s.Data)
	default:
		// Non-TMX span (e.g. from HTML/XLIFF readers) — encode as <ph> with
		// the vocabulary type recorded so that round-trip retains the type.
		id := s.ID
		if id == "" {
			id = "1"
		}
		typeAttr := s.Type
		if typeAttr == "" {
			typeAttr = "code:markup"
		}
		data := s.Data
		if data == "" {
			data = s.EquivText
		}
		fmt.Fprintf(w, `<ph x="%s" type="%s">%s</ph>`, xmlAttr(id), xmlAttr(typeAttr), xmlEscape(data))
	}
	return nil
}

// xmlEscape is a minimal character-data escaper compatible with TMX
// (we handle &, <, >, and forbidden control characters).
func xmlEscape(s string) string {
	if !strings.ContainsAny(s, "&<>") {
		return s
	}
	return html.EscapeString(s)
}

// xmlAttr escapes characters illegal inside an XML attribute value.
func xmlAttr(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "\"", "&quot;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	return s
}
