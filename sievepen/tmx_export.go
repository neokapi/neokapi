package sievepen

import (
	"bufio"
	"errors"
	"fmt"
	"html"
	"io"
	"slices"
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
		slices.Sort(locales)

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
			if err := runsToTMXSeg(bw, runs); err != nil {
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

// runsToTMXSeg serializes a Run sequence back to TMX inline markup.
// Dispatches on the run's SubType for TMX-native spans and falls
// back to <ph type="..."> for cross-format spans.
func runsToTMXSeg(w *bufio.Writer, runs []model.Run) error {
	for _, r := range runs {
		switch {
		case r.Text != nil:
			if _, err := w.WriteString(xmlEscape(r.Text.Text)); err != nil {
				return err
			}
		case r.Ph != nil:
			if err := writePhAsTMX(w, r.Ph); err != nil {
				return err
			}
		case r.PcOpen != nil:
			if err := writePcOpenAsTMX(w, r.PcOpen); err != nil {
				return err
			}
		case r.PcClose != nil:
			if err := writePcCloseAsTMX(w, r.PcClose); err != nil {
				return err
			}
		case r.Sub != nil:
			// Sub refs materialize as <sub ref="..."/>; TMX natively
			// serialises sub content inline so the ref string carries
			// the full <sub>...</sub> markup.
			if _, err := w.WriteString(r.Sub.Ref); err != nil {
				return err
			}
		}
	}
	return nil
}

func writePhAsTMX(w *bufio.Writer, ph *model.PlaceholderRun) error {
	switch ph.SubType {
	case "tmx:ph":
		fmt.Fprintf(w, `<ph x="%s">%s</ph>`, xmlAttr(ph.ID), xmlEscape(ph.Data))
	case "tmx:ut":
		fmt.Fprintf(w, `<ut>%s</ut>`, xmlEscape(ph.Data))
	case "tmx:sub":
		_, _ = w.WriteString(ph.Data)
	case "tmx:raw":
		_, _ = w.WriteString(ph.Data)
	default:
		id := ph.ID
		if id == "" {
			id = "1"
		}
		typeAttr := ph.Type
		if typeAttr == "" {
			typeAttr = "code:markup"
		}
		data := ph.Data
		if data == "" {
			data = ph.Equiv
		}
		fmt.Fprintf(w, `<ph x="%s" type="%s">%s</ph>`, xmlAttr(id), xmlAttr(typeAttr), xmlEscape(data))
	}
	return nil
}

func writePcOpenAsTMX(w *bufio.Writer, o *model.PcOpenRun) error {
	switch o.SubType {
	case "tmx:bpt":
		fmt.Fprintf(w, `<bpt i="%s">%s</bpt>`, xmlAttr(o.ID), xmlEscape(o.Data))
	case "tmx:it":
		fmt.Fprintf(w, `<it pos="begin">%s</it>`, xmlEscape(o.Data))
	case "tmx:hi":
		if o.Type != "" {
			fmt.Fprintf(w, `<hi type="%s">`, xmlAttr(o.Type))
		} else {
			_, _ = w.WriteString(`<hi>`)
		}
	case "tmx:raw":
		_, _ = w.WriteString(o.Data)
	default:
		id := o.ID
		if id == "" {
			id = "1"
		}
		typeAttr := o.Type
		if typeAttr == "" {
			typeAttr = "code:markup"
		}
		data := o.Data
		if data == "" {
			data = o.Equiv
		}
		fmt.Fprintf(w, `<ph x="%s" type="%s">%s</ph>`, xmlAttr(id), xmlAttr(typeAttr), xmlEscape(data))
	}
	return nil
}

func writePcCloseAsTMX(w *bufio.Writer, c *model.PcCloseRun) error {
	switch c.SubType {
	case "tmx:ept":
		fmt.Fprintf(w, `<ept i="%s">%s</ept>`, xmlAttr(c.ID), xmlEscape(c.Data))
	case "tmx:it":
		fmt.Fprintf(w, `<it pos="end">%s</it>`, xmlEscape(c.Data))
	case "tmx:hi":
		_, _ = w.WriteString(`</hi>`)
	case "tmx:raw":
		_, _ = w.WriteString(c.Data)
	default:
		id := c.ID
		if id == "" {
			id = "1"
		}
		typeAttr := c.Type
		if typeAttr == "" {
			typeAttr = "code:markup"
		}
		data := c.Data
		if data == "" {
			data = c.Equiv
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
