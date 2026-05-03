package xliff

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

// Writer implements DataFormatWriter for XLIFF 1.2 files.
type Writer struct {
	format.BaseFormatWriter
	skeletonStore *format.SkeletonStore
	parts         []*model.Part
	blocks        []*model.Block
	sourceLang    model.LocaleID
	targetLang    model.LocaleID
	fileName      string
}

// Ensure Writer implements SkeletonStoreConsumer.
var _ format.SkeletonStoreConsumer = (*Writer)(nil)

// NewWriter creates a new XLIFF 1.2 writer.
func NewWriter() *Writer {
	return &Writer{
		BaseFormatWriter: format.BaseFormatWriter{
			FormatName: "xliff",
		},
	}
}

// SetSkeletonStore sets the skeleton store for byte-exact output.
func (w *Writer) SetSkeletonStore(store *format.SkeletonStore) {
	w.skeletonStore = store
}

// Write consumes Parts from a channel and writes XLIFF 1.2 output.
func (w *Writer) Write(ctx context.Context, parts <-chan *model.Part) error {
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
			w.parts = append(w.parts, part)
			if part.Type == model.PartBlock {
				if block, ok := part.Resource.(*model.Block); ok {
					w.blocks = append(w.blocks, block)
				}
			}
			if part.Type == model.PartLayerStart {
				if layer, ok := part.Resource.(*model.Layer); ok {
					w.sourceLang = layer.Locale
					w.fileName = layer.Name
					if tl, ok := layer.Properties["target-language"]; ok {
						w.targetLang = model.LocaleID(tl)
					}
				}
			}
		}
	}
}

// writeFromSkeleton reads skeleton entries and fills in block content.
func (w *Writer) writeFromSkeleton() error {
	if err := w.skeletonStore.Flush(); err != nil {
		return fmt.Errorf("xliff writer: flush skeleton: %w", err)
	}

	targetLang := w.targetLang
	if !w.Locale.IsEmpty() {
		targetLang = w.Locale
	}

	// Wrap output to inject `target-language="..."` into the first
	// `<file ...>` start tag if the source didn't have one. okapi's
	// xliff writer emits target-language regardless of source presence,
	// so this keeps native canonical-equal on round-trip.
	out := newFileTagInjector(w.Output, string(targetLang))

	for {
		entry, err := w.skeletonStore.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fmt.Errorf("xliff writer: read skeleton: %w", err)
		}
		switch entry.Type {
		case format.SkeletonText:
			if _, err := out.Write(entry.Data); err != nil {
				return err
			}
		case format.SkeletonRef:
			refID := string(entry.Data)
			// Ref ID is "blockIdx:elemType"
			idxStr, refSuffix, ok := strings.Cut(refID, ":")
			if !ok {
				continue
			}
			blockIdx, err := strconv.Atoi(idxStr)
			if err != nil || blockIdx < 0 || blockIdx >= len(w.blocks) {
				continue
			}
			block := w.blocks[blockIdx]
			elemType := refSuffix

			var text string
			switch elemType {
			case "source":
				text = block.SourceText()
			case "target":
				if block.HasTarget(targetLang) {
					text = block.TargetText(targetLang)
				} else {
					// Fallback to original source text
					text = block.SourceText()
				}
			}

			if _, err := io.WriteString(out, xmlEscapeText(text)); err != nil {
				return err
			}
		}
	}
	return out.Flush()
}

// fileTagInjector wraps an io.Writer to ensure the first `<file ...>`
// start tag in the stream carries a `target-language="..."` attribute.
// okapi's xliff writer always emits target-language; native preserves
// the source skeleton, so files without that attribute diverge from
// okapi's output on round-trip. The injector buffers bytes only while
// it is inside the opening `<file` tag — once the tag is emitted (or
// confirmed absent at EOF), bytes pass through directly.
type fileTagInjector struct {
	out        io.Writer
	targetLang string
	done       bool         // true once the first <file ...> tag has been processed
	inTag      bool         // currently buffering bytes inside <file ...
	buf        []byte       // pending bytes once inTag
	tail       [10]byte     // sliding window of recent bytes (looking for "<file")
	tailLen    int
}

func newFileTagInjector(w io.Writer, targetLang string) *fileTagInjector {
	return &fileTagInjector{out: w, targetLang: targetLang}
}

// fileTagPrefix is the byte signature that triggers buffering.
var fileTagPrefix = []byte("<file")

// Write scans p for the first `<file ` opening tag and buffers from
// there until the closing `>`. When the tag closes, it injects
// target-language if missing, flushes the (possibly modified) tag,
// and disables further inspection.
func (f *fileTagInjector) Write(p []byte) (int, error) {
	if f.done && !f.inTag {
		return f.out.Write(p)
	}
	written := 0
	for i := 0; i < len(p); i++ {
		b := p[i]
		if f.inTag {
			f.buf = append(f.buf, b)
			if b == '>' {
				patched := injectTargetLanguage(f.buf, f.targetLang)
				if _, err := f.out.Write(patched); err != nil {
					return written, err
				}
				f.buf = nil
				f.inTag = false
				f.done = true
				written = i + 1
			}
			continue
		}
		if !f.done {
			// Slide the tail window and check for the prefix match.
			if f.tailLen < len(f.tail) {
				f.tail[f.tailLen] = b
				f.tailLen++
			} else {
				copy(f.tail[:], f.tail[1:])
				f.tail[len(f.tail)-1] = b
			}
			if f.tailLen >= len(fileTagPrefix) {
				start := f.tailLen - len(fileTagPrefix)
				next := byte(0)
				if start+len(fileTagPrefix) < f.tailLen {
					next = f.tail[start+len(fileTagPrefix)]
				}
				_ = next
				// Match `<file` followed by whitespace, '>', '/', or ':'.
				if bytes.Equal(f.tail[start:start+len(fileTagPrefix)], fileTagPrefix) {
					sep := byte(0)
					if i+1 < len(p) {
						sep = p[i+1]
					}
					if sep == ' ' || sep == '\t' || sep == '\r' || sep == '\n' || sep == '>' {
						// Flush bytes up to and including current; switch to buffering.
						if _, err := f.out.Write(p[written : i+1]); err != nil {
							return written, err
						}
						written = i + 1
						f.inTag = true
						continue
					}
				}
			}
		}
	}
	if !f.inTag && written < len(p) {
		if _, err := f.out.Write(p[written:]); err != nil {
			return written, err
		}
	}
	return len(p), nil
}

// Flush completes any in-progress buffering. Call this at end of
// stream so a `<file` opening that ends in mid-write doesn't get lost.
func (f *fileTagInjector) Flush() error {
	if f.inTag && len(f.buf) > 0 {
		_, err := f.out.Write(f.buf)
		f.buf = nil
		f.inTag = false
		return err
	}
	return nil
}

// injectTargetLanguage modifies a `<file ...>` start tag so it carries
// `target-language="..."`. If the tag already declares one, returns
// the input unchanged. If targetLang is empty, returns input unchanged.
func injectTargetLanguage(tag []byte, targetLang string) []byte {
	if targetLang == "" {
		return tag
	}
	if bytes.Contains(tag, []byte("target-language=")) {
		return tag
	}
	// Insert before the closing '>' (or '/>' for self-closing).
	closeIdx := len(tag) - 1
	if closeIdx >= 1 && tag[closeIdx] == '>' && tag[closeIdx-1] == '/' {
		closeIdx--
	}
	insert := []byte(fmt.Sprintf(` target-language="%s"`, xmlEscapeAttr(targetLang)))
	out := make([]byte, 0, len(tag)+len(insert))
	out = append(out, tag[:closeIdx]...)
	out = append(out, insert...)
	out = append(out, tag[closeIdx:]...)
	return out
}

func (w *Writer) flush() error {
	if w.Output == nil {
		return nil
	}

	targetLang := w.targetLang
	if !w.Locale.IsEmpty() {
		targetLang = w.Locale
	}

	fmt.Fprint(w.Output, xml.Header)
	fmt.Fprintf(w.Output, `<xliff version="1.2" xmlns="urn:oasis:names:tc:xliff:document:1.2">`)
	fmt.Fprintf(w.Output, "\n")

	// Write file envelope
	fmt.Fprintf(w.Output, `  <file original="%s" source-language="%s"`,
		xmlEscapeAttr(w.fileName), xmlEscapeAttr(string(w.sourceLang)))
	if !targetLang.IsEmpty() {
		fmt.Fprintf(w.Output, ` target-language="%s"`, xmlEscapeAttr(string(targetLang)))
	}
	fmt.Fprintf(w.Output, ` datatype="plaintext">`)
	fmt.Fprintf(w.Output, "\n    <body>\n")

	// Write trans-units from collected blocks
	for _, part := range w.parts {
		if part.Type != model.PartBlock {
			continue
		}
		block, ok := part.Resource.(*model.Block)
		if !ok {
			continue
		}

		fmt.Fprintf(w.Output, `      <trans-unit id="%s"`, xmlEscapeAttr(block.ID))
		if !block.Translatable {
			fmt.Fprintf(w.Output, ` translate="no"`)
		}
		if block.PreserveWhitespace {
			fmt.Fprintf(w.Output, ` xml:space="preserve"`)
		}
		if v, ok := block.Properties["approved"]; ok && v == "yes" {
			fmt.Fprintf(w.Output, ` approved="yes"`)
		}
		fmt.Fprintf(w.Output, ">\n")

		// Source
		sourceText := fragmentToXLIFF(block.Source)
		fmt.Fprintf(w.Output, "        <source>%s</source>\n", sourceText)

		// Target
		if block.HasTarget(targetLang) {
			targetText := fragmentToXLIFF(block.Targets[targetLang])
			fmt.Fprintf(w.Output, "        <target>%s</target>\n", targetText)
		}

		// Notes
		for key, ann := range block.Annotations {
			if strings.HasPrefix(key, "note") {
				if note, ok := ann.(*model.NoteAnnotation); ok {
					fmt.Fprintf(w.Output, "        <note")
					if note.From != "" {
						fmt.Fprintf(w.Output, ` from="%s"`, xmlEscapeAttr(note.From))
					}
					if note.Priority > 0 {
						fmt.Fprintf(w.Output, ` priority="%d"`, note.Priority)
					}
					if note.Annotates != "" {
						fmt.Fprintf(w.Output, ` annotates="%s"`, xmlEscapeAttr(note.Annotates))
					}
					fmt.Fprintf(w.Output, ">%s</note>\n", xmlEscapeText(note.Text))
				}
			}
		}

		// Alt-trans
		for key, ann := range block.Annotations {
			if strings.HasPrefix(key, "alt-translation") {
				if alt, ok := ann.(*model.AltTranslation); ok {
					fmt.Fprintf(w.Output, "        <alt-trans")
					if alt.CombinedScore > 0 {
						fmt.Fprintf(w.Output, ` match-quality="%.0f"`, alt.CombinedScore)
					}
					if alt.Origin != "" {
						fmt.Fprintf(w.Output, ` origin="%s"`, xmlEscapeAttr(alt.Origin))
					}
					fmt.Fprintf(w.Output, ">\n")
					if len(alt.Source) > 0 {
						fmt.Fprintf(w.Output, "          <source>%s</source>\n", xmlEscapeText(model.FlattenRuns(alt.Source)))
					}
					if len(alt.Target) > 0 {
						fmt.Fprintf(w.Output, `          <target xml:lang="%s">%s</target>`+"\n",
							xmlEscapeAttr(string(targetLang)), xmlEscapeText(model.FlattenRuns(alt.Target)))
					}
					fmt.Fprintf(w.Output, "        </alt-trans>\n")
				}
			}
		}

		fmt.Fprintf(w.Output, "      </trans-unit>\n")
	}

	fmt.Fprintf(w.Output, "    </body>\n  </file>\n</xliff>")
	return nil
}

// fragmentToXLIFF converts segments to XLIFF inline content.
func fragmentToXLIFF(segs []*model.Segment) string {
	var buf strings.Builder
	for _, seg := range segs {
		if len(seg.Runs) > 0 {
			// Simple case: just use the text
			buf.WriteString(xmlEscapeText(seg.Text()))
		}
	}
	return buf.String()
}
