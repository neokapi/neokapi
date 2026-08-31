package tmx

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
)

// Writer implements DataFormatWriter for TMX files.
type Writer struct {
	format.BaseFormatWriter
	skeletonStore *format.SkeletonStore
	headerProps   map[string]string
	blocks        []*model.Block
}

// Ensure Writer implements SkeletonStoreConsumer.
var _ format.SkeletonStoreConsumer = (*Writer)(nil)

// Ensure Writer satisfies StreamingWriter: paired with the streaming reader it
// pulls each TU block on demand from a streaming skeleton store, so the writer
// side is bounded too. See streamWrite.
var _ format.StreamingWriter = (*Writer)(nil)

// StreamingWriter marks the bounded-memory interleaved write path.
func (w *Writer) StreamingWriter() {}

// NewWriter creates a new TMX writer.
func NewWriter() *Writer {
	return &Writer{
		FormatName:  "tmx",
		Interchange: true,
		headerProps: make(map[string]string),
	}
}

// SetSkeletonStore sets the skeleton store for byte-exact output.
func (w *Writer) SetSkeletonStore(store *format.SkeletonStore) {
	w.skeletonStore = store
}

// Write consumes Parts from a channel and writes TMX XML.
func (w *Writer) Write(ctx context.Context, parts <-chan *model.Part) error {
	// Streaming skeleton round-trip: interleave skeleton consumption with the
	// Part stream, pulling each TU block on demand rather than buffering them
	// all, so the writer side is bounded too.
	if w.skeletonStore != nil && w.skeletonStore.IsStreaming() {
		return w.streamWrite(ctx, parts)
	}
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case part, ok := <-parts:
			if !ok {
				if w.skeletonStore != nil {
					return w.writeFromSkeleton()
				}
				return w.flush()
			}
			w.collectPart(part)
		}
	}
}

// streamWrite consumes a streaming skeleton store interleaved with the Part
// stream. TU blocks arrive in document order, so the Nth block is tuIdx N; a
// ref "N:lang" pulls forward to block N (the reader emits a TU's seg refs before
// the block itself, so the pull waits for it via the channel). Blocks below the
// current TU are evicted, keeping the pulled window at ~one TU.
func (w *Writer) streamWrite(ctx context.Context, parts <-chan *model.Part) error {
	pulled := map[int]*model.Block{}
	count := 0
	closed := false
	pull := func() {
		for !closed {
			select {
			case <-ctx.Done():
				closed = true
				return
			case p, ok := <-parts:
				if !ok {
					closed = true
					return
				}
				if p == nil {
					continue
				}
				switch p.Type {
				case model.PartData:
					if d, ok := p.Resource.(*model.Data); ok && d.Name == "tmx-header" {
						w.headerProps = d.Properties
					}
				case model.PartBlock:
					if b, ok := p.Resource.(*model.Block); ok {
						pulled[count] = b
						count++
						return
					}
				}
			}
		}
	}
	lastTU := -1
	blockAt := func(i int) *model.Block {
		if i < 0 {
			return nil
		}
		for count <= i && !closed {
			pull()
		}
		// Evict TUs before the one being resolved — refs advance monotonically,
		// so earlier blocks are no longer needed (keeps the window at ~one TU).
		if i > lastTU {
			for k := range pulled {
				if k < i {
					delete(pulled, k)
				}
			}
			lastTU = i
		}
		return pulled[i]
	}
	err := w.replaySkeleton(blockAt)
	// Drain any remaining parts so the executor's tool goroutines can finish.
	for !closed {
		pull()
	}
	return err
}

func (w *Writer) collectPart(part *model.Part) {
	switch part.Type {
	case model.PartBlock:
		if block, ok := part.Resource.(*model.Block); ok {
			w.blocks = append(w.blocks, block)
		}
	case model.PartData:
		if data, ok := part.Resource.(*model.Data); ok {
			if data.Name == "tmx-header" {
				w.headerProps = data.Properties
			}
		}
	}
}

// writeFromSkeleton reads skeleton entries and fills in block content.
//
// Skeleton refs only cover the original `<seg>` positions captured by
// the reader. When downstream tools (e.g. pseudo-translate) add a target
// TUV that wasn't in the source TMX, we have to inject a fresh
// `<tuv>...</tuv>` block before the `</tu>` that closes the current TU.
// We track the most recent (tuIdx, langs-emitted) and, when the next
// text chunk is about to advance past `</tu>`, we splice the missing
// target TUVs in just before it.
func (w *Writer) writeFromSkeleton() error {
	if err := w.skeletonStore.Flush(); err != nil {
		return fmt.Errorf("tmx writer: flush skeleton: %w", err)
	}
	return w.replaySkeleton(func(i int) *model.Block {
		if i < 0 || i >= len(w.blocks) {
			return nil
		}
		return w.blocks[i]
	})
}

// replaySkeleton walks the skeleton store, resolving each "tuIdx:lang" ref via
// blockAt (a whole-slice index in buffered mode, or an on-demand pull in
// streaming mode). It is shared by writeFromSkeleton (buffered) and streamWrite.
func (w *Writer) replaySkeleton(blockAt func(int) *model.Block) error {
	srcLang := strings.ToLower(w.headerProps["srclang"])
	if srcLang == "" {
		srcLang = "en"
	}

	curTU := -1
	emittedLangs := map[string]bool{}

	flushPendingTUVs := func(text []byte) []byte {
		block := blockAt(curTU)
		if curTU < 0 || block == nil {
			return text
		}
		idx := bytes.Index(text, []byte("</tu>"))
		if idx < 0 {
			return text
		}
		var inject strings.Builder
		// Order targets deterministically (skeleton can't tell us
		// the source's order, but pseudo-translate produces a single
		// target so map iteration order matches the user's expectation).
		for _, locale := range block.TargetLocales() {
			runs := block.TargetRuns(locale)
			if len(runs) == 0 {
				continue
			}
			lang := string(locale)
			if emittedLangs[strings.ToLower(lang)] {
				continue
			}
			inject.WriteString(`<tuv xml:lang="`)
			inject.WriteString(xmlEscapeAttr(lang))
			inject.WriteString(`"><seg>`)
			inject.WriteString(renderTMXSeg(runs))
			inject.WriteString(`</seg></tuv>`)
		}
		if inject.Len() == 0 {
			return text
		}
		// Splice injected TUVs in just before the `</tu>` close.
		out := make([]byte, 0, len(text)+inject.Len())
		out = append(out, text[:idx]...)
		out = append(out, inject.String()...)
		out = append(out, text[idx:]...)
		return out
	}

	for {
		entry, err := w.skeletonStore.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fmt.Errorf("tmx writer: read skeleton: %w", err)
		}
		switch entry.Type {
		case format.SkeletonText:
			data := flushPendingTUVs(entry.Data)
			if _, err := w.Output.Write(data); err != nil {
				return err
			}
		case format.SkeletonRef:
			refID := string(entry.Data)
			idxStr, refSuffix, ok := strings.Cut(refID, ":")
			if !ok {
				continue
			}
			tuIdx, err := strconv.Atoi(idxStr)
			if err != nil || tuIdx < 0 {
				continue
			}
			block := blockAt(tuIdx)
			if block == nil {
				continue
			}
			// Checked once per block rather than per <seg>: the target TUVs
			// spliced in by flushPendingTUVs come from the same block and
			// reach the output through a path with no error return.
			if err := format.CheckXMLBlock("tmx", block); err != nil {
				return err
			}
			if tuIdx != curTU {
				curTU = tuIdx
				emittedLangs = map[string]bool{}
			}
			lang := refSuffix
			emittedLangs[strings.ToLower(lang)] = true

			var runs []model.Run
			langLower := strings.ToLower(lang)
			if langMatches(langLower, srcLang) {
				runs = block.Source
			} else {
				localeID := model.LocaleID(lang)
				if block.HasTarget(localeID) {
					runs = block.TargetRuns(localeID)
				} else {
					runs = block.Source
				}
			}

			if _, err := io.WriteString(w.Output, renderTMXSeg(runs)); err != nil {
				return err
			}
		}
	}
	return nil
}

// renderTMXSeg walks a segment slice and produces the inner XML for a
// <seg> element. TextRun content is XML-escaped; Ph / PcOpen / PcClose
// runs are emitted as <ph>, <bpt>, <ept>, <it>, or <hi> based on the
// SubType set by the reader. Inline element Data is also XML-escaped.
//
// <hi> is special-cased — the reader emits PcOpen{hi}+Text+PcClose{hi}
// so the inner text remains translatable, but on output we collapse
// back to a single `<hi ...>text</hi>` element instead of emitting a
// self-closed open and a separate close.
//
// `x` follows the source's numbering: it equals the run's ID when that
// ID is a positive integer (the reader copies the source's i / x
// attribute into the ID), and falls back to a per-seg counter when the
// run has no numeric ID. ept emits no `x` attribute — the matching
// bpt's i= alone disambiguates the pair.
//
// Attribute order follows okapi's writer to maximise byte-level
// agreement: i (paired-code id), pos (it only), type, x.
func renderTMXSeg(runs []model.Run) string {
	var b strings.Builder
	xCounter := 0
	// xFor returns the `x=` value to emit on a TMX inline element. The
	// reader stashes the source's original `x=` attribute on each run's
	// Equiv field, so we prefer that when present (preserves
	// non-sequential or out-of-order source numbering like
	// <bpt x="2" i="1">). Otherwise we fall back to the run's ID when it
	// parses as a positive integer (covers the common case where ID was
	// taken from `i=` and authors reuse the same numbering for x), and
	// finally to a per-seg counter.
	xFor := func(id, equiv string) int {
		if n, err := strconv.Atoi(equiv); err == nil && n > 0 {
			return n
		}
		if n, err := strconv.Atoi(id); err == nil && n > 0 {
			return n
		}
		xCounter++
		return xCounter
	}
	for _, run := range runs {
		switch {
		case run.Text != nil:
			b.WriteString(xmlEscapeString(run.Text.Text))
		case run.Ph != nil:
			writeTMXPh(&b, run.Ph.ID, run.Ph.Type, run.Ph.Disp, xFor(run.Ph.ID, run.Ph.Equiv), run.Ph.Data)
		case run.PcOpen != nil:
			if run.PcOpen.SubType == "tmx-hi" {
				writeTMXHiOpen(&b, run.PcOpen.Type, xFor(run.PcOpen.ID, run.PcOpen.Equiv))
				continue
			}
			writeTMXInline(&b, "bpt", run.PcOpen.SubType, run.PcOpen.ID, run.PcOpen.Type, "begin", xFor(run.PcOpen.ID, run.PcOpen.Equiv), run.PcOpen.Data)
		case run.PcClose != nil:
			if run.PcClose.SubType == "tmx-hi" {
				b.WriteString("</hi>")
				continue
			}
			// ept (paired bpt close): no x — paired by i with
			// the matching bpt. it pos=end is an isolated marker
			// (no pair), needs its own x.
			closeX := 0
			if run.PcClose.SubType == "tmx-it-end" || run.PcClose.SubType == "tmx-it" {
				closeX = xFor(run.PcClose.ID, run.PcClose.Equiv)
			}
			writeTMXInline(&b, "ept", run.PcClose.SubType, run.PcClose.ID, run.PcClose.Type, "end", closeX, run.PcClose.Data)
		}
	}
	return b.String()
}

// writeTMXPh emits a self-closing `<ph ...>data</ph>` element. assoc
// is the TMX-specific anchor hint ("p"/"f"/"b") captured in the run's
// Disp field by the reader; emitted only when non-empty.
func writeTMXPh(b *strings.Builder, id, spanType, assoc string, x int, data string) {
	b.WriteString("<ph")
	if assoc != "" {
		b.WriteString(` assoc="`)
		b.WriteString(xmlEscapeAttr(assoc))
		b.WriteByte('"')
	}
	if spanType != "" {
		b.WriteString(` type="`)
		b.WriteString(xmlEscapeAttr(spanType))
		b.WriteByte('"')
	}
	if x > 0 {
		b.WriteString(` x="`)
		b.WriteString(strconv.Itoa(x))
		b.WriteByte('"')
	}
	b.WriteByte('>')
	writeInlineData(b, data)
	b.WriteString("</ph>")
}

// writeTMXHiOpen emits an opening `<hi ...>` tag without auto-closing,
// since `<hi>` wraps translatable inner text and the matching close tag
// arrives later as a PcClose run.
func writeTMXHiOpen(b *strings.Builder, spanType string, x int) {
	b.WriteString("<hi")
	if spanType != "" {
		b.WriteString(` type="`)
		b.WriteString(xmlEscapeAttr(spanType))
		b.WriteByte('"')
	}
	b.WriteString(` x="`)
	b.WriteString(strconv.Itoa(x))
	b.WriteString(`">`)
}

// writeTMXInline emits one self-closing TMX inline element (<ph>,
// <bpt>, <ept>, <it>): opening tag with attributes, escaped Data,
// closing tag. Use writeTMXHiOpen / </hi> for paired <hi> elements.
func writeTMXInline(b *strings.Builder, defaultElem, subType, id, spanType, defaultPos string, x int, data string) {
	elem := defaultElem
	pos := ""
	switch subType {
	case "tmx-bpt":
		elem = "bpt"
	case "tmx-ept":
		elem = "ept"
	case "tmx-ph":
		elem = "ph"
	case "tmx-it":
		elem = "it"
	case "tmx-it-begin":
		elem = "it"
		pos = "begin"
	case "tmx-it-end":
		elem = "it"
		pos = "end"
	}

	b.WriteByte('<')
	b.WriteString(elem)
	if pos != "" {
		b.WriteString(` pos="`)
		b.WriteString(xmlEscapeAttr(pos))
		b.WriteByte('"')
	} else if defaultPos != "" && elem == "it" {
		b.WriteString(` pos="`)
		b.WriteString(xmlEscapeAttr(defaultPos))
		b.WriteByte('"')
	}
	if elem == "bpt" || elem == "ept" {
		if id != "" {
			b.WriteString(` i="`)
			b.WriteString(xmlEscapeAttr(id))
			b.WriteByte('"')
		}
	}
	if spanType != "" {
		b.WriteString(` type="`)
		b.WriteString(xmlEscapeAttr(spanType))
		b.WriteByte('"')
	}
	if x > 0 {
		b.WriteString(` x="`)
		b.WriteString(strconv.Itoa(x))
		b.WriteByte('"')
	}
	b.WriteByte('>')
	writeInlineData(b, data)
	b.WriteString("</")
	b.WriteString(elem)
	b.WriteByte('>')
}

// xmlEscapeAttr escapes for attribute-value context (adds quote
// escaping on top of xmlEscapeString).
//
// DELIBERATELY not core/internal/xmlesc.Attr. It escapes the same characters,
// but this pair walks runes, so invalid UTF-8 in the input becomes U+FFFD
// rather than passing through byte-for-byte. Swapping in the shared helper
// would change what a TMX carrying undecodable bytes writes out.
func xmlEscapeAttr(s string) string {
	s = xmlEscapeString(s)
	return strings.ReplaceAll(s, `"`, "&quot;")
}

// xmlEscapeString escapes special XML characters in text content. Rune-walking,
// so U+FFFD-substituting on invalid UTF-8 — see xmlEscapeAttr above.
func xmlEscapeString(s string) string {
	var buf strings.Builder
	for _, r := range s {
		switch r {
		case '\r':
			// XML 1.0 §2.11 rewrites a literal CR to LF on the next read, so
			// only the character reference survives.
			buf.WriteString("&#13;")
		case '&':
			buf.WriteString("&amp;")
		case '<':
			buf.WriteString("&lt;")
		case '>':
			buf.WriteString("&gt;")
		default:
			buf.WriteRune(r)
		}
	}
	return buf.String()
}

// writeInlineData escapes inline element data while passing through
// `<sub>...</sub>` markers as raw XML. The reader wraps each `<sub>`
// element's open/close tags in \x01...\x02 sentinels (subOpenSentinel /
// subCloseSentinel); everything outside the sentinels is normal text
// data that needs XML escaping.
func writeInlineData(b *strings.Builder, data string) {
	if !strings.ContainsRune(data, '\x01') {
		b.WriteString(xmlEscapeString(data))
		return
	}
	for {
		open := strings.Index(data, "\x01")
		if open < 0 {
			b.WriteString(xmlEscapeString(data))
			return
		}
		b.WriteString(xmlEscapeString(data[:open]))
		rest := data[open+1:]
		before, after, ok := strings.Cut(rest, "\x02")
		if !ok {
			b.WriteString(xmlEscapeString(rest))
			return
		}
		b.WriteString(before)
		data = after
	}
}

// xmlTMX and related types for output.
type xmlTMX struct {
	XMLName xml.Name  `xml:"tmx"`
	Version string    `xml:"version,attr"`
	Header  xmlHeader `xml:"header"`
	Body    xmlBody   `xml:"body"`
}

type xmlHeader struct {
	CreationTool        string `xml:"creationtool,attr,omitempty"`
	CreationToolVersion string `xml:"creationtoolversion,attr,omitempty"`
	SegType             string `xml:"segtype,attr,omitempty"`
	OriginalFormat      string `xml:"o-tmf,attr,omitempty"`
	AdminLang           string `xml:"adminlang,attr,omitempty"`
	SrcLang             string `xml:"srclang,attr,omitempty"`
	DataType            string `xml:"datatype,attr,omitempty"`
}

type xmlBody struct {
	TUs []xmlTU `xml:"tu"`
}

type xmlTU struct {
	TUid string   `xml:"tuid,attr,omitempty"`
	TUVs []xmlTUV `xml:"tuv"`
}

type xmlTUV struct {
	Lang string `xml:"xml:lang,attr"`
	Seg  xmlSeg `xml:"seg"`
}

// xmlSeg renders the <seg> body via innerxml so we can splice inline
// codes (<ph>, <bpt>, <ept>, <it>, <hi>) generated by renderTMXSeg
// without xml.Encoder escaping them. The TextRun portions are
// XML-escaped inside renderTMXSeg before being concatenated.
type xmlSeg struct {
	Inner string `xml:",innerxml"`
}

func (w *Writer) flush() error {
	if w.Output == nil {
		return nil
	}

	version := w.headerProps["version"]
	if version == "" {
		version = "1.4"
	}

	srcLang := w.headerProps["srclang"]
	if srcLang == "" {
		srcLang = "en"
	}

	doc := xmlTMX{
		Version: version,
		Header: xmlHeader{
			CreationTool:        w.headerProps["creationtool"],
			CreationToolVersion: w.headerProps["creationtoolversion"],
			SegType:             w.headerProps["segtype"],
			OriginalFormat:      w.headerProps["o-tmf"],
			AdminLang:           w.headerProps["adminlang"],
			SrcLang:             srcLang,
			DataType:            w.headerProps["datatype"],
		},
	}

	for _, block := range w.blocks {
		if err := format.CheckXMLBlock("tmx", block); err != nil {
			return err
		}
		tu := xmlTU{
			// `tuid` is what the document says; a block's ID is an identity for
			// the store, and the two are the same for every document whose units
			// already identify themselves.
			TUid: model.DocumentID(block),
		}

		// Add source TUV
		tu.TUVs = append(tu.TUVs, xmlTUV{
			Lang: srcLang,
			Seg:  xmlSeg{Inner: renderTMXSeg(block.Source)},
		})

		// Add target TUVs
		for _, locale := range block.TargetLocales() {
			runs := block.TargetRuns(locale)
			if len(runs) == 0 {
				continue
			}
			tu.TUVs = append(tu.TUVs, xmlTUV{
				Lang: string(locale),
				Seg:  xmlSeg{Inner: renderTMXSeg(runs)},
			})
		}

		doc.Body.TUs = append(doc.Body.TUs, tu)
	}

	if _, err := fmt.Fprint(w.Output, xml.Header); err != nil {
		return err
	}

	encoder := xml.NewEncoder(w.Output)
	encoder.Indent("", "  ")
	if err := encoder.Encode(doc); err != nil {
		return fmt.Errorf("tmx writer: encoding: %w", err)
	}

	return nil
}
