package memory

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"html"
	"io"
	"slices"
	"strings"

	"github.com/neokapi/neokapi/core/model"
)

// EntryProvider is implemented by content memory backends that can list all entries.
type EntryProvider interface {
	Entries(ctx context.Context) ([]Entry, error)
}

// ExportTMX writes multilingual TMX to writer. locales filters which
// variants are emitted (empty = every variant present on the entry). Each
// entry becomes a single <tu> with one <tuv> per selected variant.
func ExportTMX(ctx context.Context, tm ContentMemory, writer io.Writer, locales []model.LocaleID) error {
	provider, ok := tm.(EntryProvider)
	if !ok {
		return errors.New("content memory does not support entry listing")
	}

	allowed := make(map[model.LocaleID]bool, len(locales))
	for _, l := range locales {
		allowed[l] = true
	}
	filter := len(allowed) > 0

	entries, err := provider.Entries(ctx)
	if err != nil {
		return fmt.Errorf("list entries: %w", err)
	}

	w := &tmxWriter{bw: bufio.NewWriter(writer)}

	w.str(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	w.str(`<!DOCTYPE tmx SYSTEM "tmx14.dtd">` + "\n")
	w.str(`<tmx version="1.4">` + "\n")

	defaultSrcLang := "en"
	if len(entries) > 0 && entries[0].HintSrcLang != "" {
		defaultSrcLang = string(entries[0].HintSrcLang)
	}
	w.printf(
		`  <header creationtool="neokapi-memory" creationtoolversion="2.0" segtype="sentence" adminlang="%s" srclang="%s" datatype="plaintext" o-tmf="unknown"/>`+"\n",
		xmlAttr(defaultSrcLang), xmlAttr(defaultSrcLang))
	w.str("  <body>\n")

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

		w.printf(`    <tu tuid="%s" srclang="%s"`, xmlAttr(entry.ID), xmlAttr(string(srcLang)))
		if !entry.CreatedAt.IsZero() {
			w.printf(` creationdate="%s"`, entry.CreatedAt.UTC().Format("20060102T150405Z"))
		}
		if !entry.UpdatedAt.IsZero() {
			w.printf(` changedate="%s"`, entry.UpdatedAt.UTC().Format("20060102T150405Z"))
		}
		w.str(">\n")

		// TU-level props: entry properties + entity markers.
		for k, v := range entry.Properties {
			w.printf(`      <prop type="%s">%s</prop>`+"\n", xmlAttr(k), xmlEscape(v))
		}
		if entry.Note != "" {
			w.printf(`      <note>%s</note>`+"\n", xmlEscape(entry.Note))
		}

		for _, loc := range locales {
			runs := entry.Variants[loc]
			if len(runs) == 0 {
				continue
			}
			w.printf(`      <tuv xml:lang="%s">`+"\n", xmlAttr(string(loc)))
			w.str(`        <seg>`)
			runsToTMXSeg(w, runs)
			w.str("</seg>\n")
			w.str("      </tuv>\n")
		}

		w.str("    </tu>\n")

		// Stop at the first failed write rather than formatting the rest of
		// the store into a writer that is only going to reject it.
		if w.err != nil {
			return w.flush()
		}
	}

	w.str("  </body>\n")
	w.str("</tmx>\n")
	return w.flush()
}

// tmxWriter is a write-error-latching wrapper over the buffered output.
//
// The export emits a long, branchy sequence of small writes, and threading an
// `if err != nil { return err }` through every one of them is what made the
// original inconsistent with itself: the header writes were checked and the
// entire <body> was not, so a disk-full mid-export produced a truncated TMX
// with a nil return — the caller believed it had written a complete file.
// Latching keeps the emit code readable while making "did every byte land?" a
// single question, asked at flush.
//
// Once an error is latched every later write is a no-op, so a failing export
// stops formatting rather than racing on.
type tmxWriter struct {
	bw  *bufio.Writer
	err error
}

func (w *tmxWriter) str(s string) {
	if w.err != nil {
		return
	}
	_, w.err = w.bw.WriteString(s)
}

func (w *tmxWriter) printf(format string, args ...any) {
	if w.err != nil {
		return
	}
	_, w.err = fmt.Fprintf(w.bw, format, args...)
}

// flush reports the first latched write error, or the error from flushing the
// buffered tail. The tail matters on its own: an export small enough to fit in
// bufio's buffer reaches the underlying writer ONLY at flush, so discarding
// this error meant a wholly-failed export still returned nil.
func (w *tmxWriter) flush() error {
	if w.err != nil {
		return fmt.Errorf("write TMX: %w", w.err)
	}
	if err := w.bw.Flush(); err != nil {
		return fmt.Errorf("flush TMX: %w", err)
	}
	return nil
}

// runsToTMXSeg serializes a Run sequence back to TMX inline markup.
// Dispatches on the run's SubType for TMX-native spans and falls
// back to <ph type="..."> for cross-format spans.
//
// Write failures are latched on the tmxWriter rather than returned: these
// helpers have no other failure mode, and an inline-code run must not be able
// to slip past unchecked the way it did while they wrote through a bare
// bufio.Writer.
func runsToTMXSeg(w *tmxWriter, runs []model.Run) {
	for _, r := range runs {
		switch {
		case r.Text != nil:
			w.str(xmlEscape(r.Text.Text))
		case r.Ph != nil:
			writePhAsTMX(w, r.Ph)
		case r.PcOpen != nil:
			writePcOpenAsTMX(w, r.PcOpen)
		case r.PcClose != nil:
			writePcCloseAsTMX(w, r.PcClose)
		case r.Sub != nil:
			// Sub refs materialize as <sub ref="..."/>; TMX natively
			// serialises sub content inline so the ref string carries
			// the full <sub>...</sub> markup.
			w.str(r.Sub.Ref)
		}
	}
}

func writePhAsTMX(w *tmxWriter, ph *model.PlaceholderRun) {
	switch ph.SubType {
	case "tmx:ph":
		w.printf(`<ph x="%s">%s</ph>`, xmlAttr(ph.ID), xmlEscape(ph.Data))
	case "tmx:ut":
		w.printf(`<ut>%s</ut>`, xmlEscape(ph.Data))
	case "tmx:sub":
		w.str(ph.Data)
	case "tmx:raw":
		w.str(ph.Data)
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
		w.printf(`<ph x="%s" type="%s">%s</ph>`, xmlAttr(id), xmlAttr(typeAttr), xmlEscape(data))
	}
}

func writePcOpenAsTMX(w *tmxWriter, o *model.PcOpenRun) {
	switch o.SubType {
	case "tmx:bpt":
		w.printf(`<bpt i="%s">%s</bpt>`, xmlAttr(o.ID), xmlEscape(o.Data))
	case "tmx:it":
		w.printf(`<it pos="begin">%s</it>`, xmlEscape(o.Data))
	case "tmx:hi":
		if o.Type != "" {
			w.printf(`<hi type="%s">`, xmlAttr(o.Type))
		} else {
			w.str(`<hi>`)
		}
	case "tmx:raw":
		w.str(o.Data)
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
		w.printf(`<ph x="%s" type="%s">%s</ph>`, xmlAttr(id), xmlAttr(typeAttr), xmlEscape(data))
	}
}

func writePcCloseAsTMX(w *tmxWriter, c *model.PcCloseRun) {
	switch c.SubType {
	case "tmx:ept":
		w.printf(`<ept i="%s">%s</ept>`, xmlAttr(c.ID), xmlEscape(c.Data))
	case "tmx:it":
		w.printf(`<it pos="end">%s</it>`, xmlEscape(c.Data))
	case "tmx:hi":
		w.str(`</hi>`)
	case "tmx:raw":
		w.str(c.Data)
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
		w.printf(`<ph x="%s" type="%s">%s</ph>`, xmlAttr(id), xmlAttr(typeAttr), xmlEscape(data))
	}
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
