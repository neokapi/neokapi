package ttx

import (
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

// Writer implements DataFormatWriter for Trados TagEditor TTX files.
type Writer struct {
	format.BaseFormatWriter
	cfg           *Config
	skeletonStore *format.SkeletonStore
	blocks        []*model.Block
}

// Ensure Writer implements SkeletonStoreConsumer.
var _ format.SkeletonStoreConsumer = (*Writer)(nil)

// NewWriter creates a new TTX writer.
func NewWriter() *Writer {
	cfg := &Config{}
	cfg.Reset()
	return &Writer{
		BaseFormatWriter: format.BaseFormatWriter{
			FormatName: "ttx",
		},
		cfg: cfg,
	}
}

// Config returns the writer configuration for external modification.
func (w *Writer) Config() *Config { return w.cfg }

// SetSkeletonStore sets the skeleton store for byte-exact output.
func (w *Writer) SetSkeletonStore(store *format.SkeletonStore) {
	w.skeletonStore = store
}

// Write consumes Parts from a channel and writes reconstructed TTX XML.
func (w *Writer) Write(ctx context.Context, parts <-chan *model.Part) error {
	if w.skeletonStore != nil {
		// Collect all parts, then write from skeleton
		for {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case part, ok := <-parts:
				if !ok {
					return w.writeFromSkeleton()
				}
				if part.Type == model.PartBlock {
					if block, ok := part.Resource.(*model.Block); ok {
						w.blocks = append(w.blocks, block)
					}
				}
			}
		}
	}

	enc := xml.NewEncoder(w.Output)
	enc.Indent("", "  ")

	// Write XML declaration
	if _, err := io.WriteString(w.Output, `<?xml version="1.0" encoding="utf-8"?>`+"\n"); err != nil {
		return err
	}

	// Open TRADOStag
	if _, err := io.WriteString(w.Output, `<TRADOStag Version="2.0">`+"\n"); err != nil {
		return err
	}
	if _, err := io.WriteString(w.Output, "<Body>\n<Raw>\n"); err != nil {
		return err
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case part, ok := <-parts:
			if !ok {
				// Close tags
				if _, err := io.WriteString(w.Output, "</Raw>\n</Body>\n</TRADOStag>\n"); err != nil {
					return err
				}
				return nil
			}
			if err := w.writePart(part); err != nil {
				return err
			}
		}
	}
}

// writeFromSkeleton reads skeleton entries and fills in block content.
func (w *Writer) writeFromSkeleton() error {
	if err := w.skeletonStore.Flush(); err != nil {
		return fmt.Errorf("ttx writer: flush skeleton: %w", err)
	}

	for {
		entry, err := w.skeletonStore.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fmt.Errorf("ttx writer: read skeleton: %w", err)
		}
		switch entry.Type {
		case format.SkeletonText:
			if _, err := w.Output.Write(entry.Data); err != nil {
				return err
			}
		case format.SkeletonRef:
			refID := string(entry.Data)
			if rest, ok := strings.CutPrefix(refID, "u"); ok {
				// Unsegmented run: wrap the (translated) text in a fresh
				// <Tu MatchPercent="0">…</Tu>, mirroring Okapi's
				// TTXSkeletonWriter.processSegment (the source's bare text
				// becomes a Tu segment on output). The surrounding markup
				// stayed verbatim in the skeleton text entries.
				tuIdx, err := strconv.Atoi(rest)
				if err != nil || tuIdx < 0 || tuIdx >= len(w.blocks) {
					continue
				}
				if err := w.writeUnsegmentedTu(w.blocks[tuIdx]); err != nil {
					return err
				}
				continue
			}
			// Ref ID is "tuIdx:tuvIdx"
			idxStr, refSuffix, ok := strings.Cut(refID, ":")
			if !ok {
				continue
			}
			tuIdx, err := strconv.Atoi(idxStr)
			if err != nil || tuIdx < 0 || tuIdx >= len(w.blocks) {
				continue
			}
			tuvIdx, err := strconv.Atoi(refSuffix)
			if err != nil {
				continue
			}
			block := w.blocks[tuIdx]

			var text string
			if tuvIdx == 0 {
				// Source TUV
				text = block.SourceText()
			} else {
				// Target TUV - find the first target
				text = block.SourceText() // fallback
				for _, locale := range block.TargetLocales() {
					if block.HasTarget(locale) {
						text = block.TargetText(locale)
						break
					}
				}
			}

			// Honor the EscapeGT option on the skeleton-fill path too, so a
			// non-translating round-trip preserves the source's `>` bytes
			// (Okapi only escapes `>` when EscapeGT is set).
			if _, err := io.WriteString(w.Output, xmlEscapeWith(text, w.cfg.EscapeGT)); err != nil {
				return err
			}
		}
	}
	return nil
}

// writeUnsegmentedTu emits a fresh <Tu> wrapping an unsegmented run's source
// and translated target. Okapi's TTXSkeletonWriter.processSegment writes
// `<Tu MatchPercent="0"><Tuv Lang="SRC">src</Tuv><Tuv Lang="TRG">trg</Tuv></Tu>`
// (TTXSkeletonWriter.java:143-157); the source text the reader captured keeps
// its trailing whitespace, which lands inside the <Tuv>.
func (w *Writer) writeUnsegmentedTu(block *model.Block) error {
	escape := func(s string) string { return xmlEscapeWith(s, w.cfg.EscapeGT) }

	sourceLang := block.Properties["source-lang"]
	if sourceLang == "" {
		sourceLang = "EN-US"
	}

	sourceText := block.SourceText()
	targetText := sourceText // fall back to source if no translation
	for _, locale := range block.TargetLocales() {
		if block.HasTarget(locale) {
			targetText = block.TargetText(locale)
			break
		}
	}
	// Okapi uppercases the target language code (TTXFilter.open:
	// trgLangCode = trgLoc.toString().toUpperCase()).
	targetLang := strings.ToUpper(string(w.Locale))
	if targetLang == "" {
		targetLang = sourceLang
	}

	if _, err := fmt.Fprintf(w.Output, `<Tu MatchPercent="0"><Tuv Lang="%s">%s</Tuv><Tuv Lang="%s">%s</Tuv></Tu>`,
		escape(sourceLang), escape(sourceText), escape(targetLang), escape(targetText)); err != nil {
		return err
	}
	return nil
}

func (w *Writer) writePart(part *model.Part) error {
	switch part.Type {
	case model.PartBlock:
		return w.writeBlock(part)
	default:
		return nil
	}
}

func (w *Writer) writeBlock(part *model.Part) error {
	block, ok := part.Resource.(*model.Block)
	if !ok {
		return errors.New("ttx writer: expected Block resource")
	}

	sourceText := block.SourceText()
	targetText := ""
	targetLang := ""

	if !w.Locale.IsEmpty() && block.HasTarget(w.Locale) {
		targetText = block.TargetText(w.Locale)
		targetLang = string(w.Locale)
	}

	sourceLang := block.Properties["source-lang"]
	if sourceLang == "" {
		sourceLang = "EN-US"
	}

	matchPercent := block.Properties["match-percent"]
	if matchPercent == "" {
		matchPercent = "0"
	}

	escape := func(s string) string { return xmlEscapeWith(s, w.cfg.EscapeGT) }

	if _, err := fmt.Fprintf(w.Output, `<Tu MatchPercent="%s">`+"\n", escape(matchPercent)); err != nil {
		return err
	}
	// Real TTX (TRADOStag) places translatable text directly inside <Tuv>;
	// there is no <Seg> wrapper element in the format.
	if _, err := fmt.Fprintf(w.Output, `<Tuv Lang="%s">%s</Tuv>`+"\n", escape(sourceLang), escape(sourceText)); err != nil {
		return err
	}

	if targetText != "" && targetLang != "" {
		if _, err := fmt.Fprintf(w.Output, `<Tuv Lang="%s">%s</Tuv>`+"\n", escape(targetLang), escape(targetText)); err != nil {
			return err
		}
	}

	if _, err := io.WriteString(w.Output, "</Tu>\n"); err != nil {
		return err
	}

	return nil
}

// xmlEscapeWith escapes XML special characters in element character data,
// optionally escaping >.
//
// The literal sequence "]]>" is always escaped (the `>` is emitted as `&gt;`)
// even when escapeGT is false: XML 1.0 §2.4 forbids the literal string "]]>"
// appearing in character data outside a CDATA section ("the string \"]]>\"
// MUST NOT appear in content unless used to mark the end of a CDATA section").
// Some TTX source text — e.g. a JavaScript "//]]>" CDATA-close marker carried
// as content — contains it, and emitting it raw produces XML that no conformant
// parser will read. Okapi avoids this by keeping such markers inside inline
// codes; native escapes the `>` of "]]>" to stay well-formed.
func xmlEscapeWith(s string, escapeGT bool) string {
	var buf []byte
	for i := range len(s) {
		switch s[i] {
		case '&':
			buf = append(buf, []byte("&amp;")...)
		case '<':
			buf = append(buf, []byte("&lt;")...)
		case '>':
			// Escape `>` when EscapeGT is set, or when it closes a "]]>"
			// run (i >= 2 && previous two bytes are "]]") to keep the
			// output well-formed per XML 1.0 §2.4.
			if escapeGT || (i >= 2 && s[i-1] == ']' && s[i-2] == ']') {
				buf = append(buf, []byte("&gt;")...)
			} else {
				buf = append(buf, '>')
			}
		case '"':
			buf = append(buf, []byte("&quot;")...)
		default:
			buf = append(buf, s[i])
		}
	}
	return string(buf)
}
