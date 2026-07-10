package androidxml

import (
	"bufio"
	"context"
	"fmt"
	"strings"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/safeio"
)

// Ensure Reader satisfies StreamingReader: the skeleton round-trip reads
// incrementally from doc.Reader (bounded to one resource entry) — see
// readStreaming. The file-runner uses this to concurrent-feed the reader.
var _ format.StreamingReader = (*Reader)(nil)

// StreamingReader marks the bounded-memory skeleton path.
func (r *Reader) StreamingReader() {}

// readStreaming tokenizes the document incrementally through a bounded bufio
// window. It streams *through* <resources> (emitting its tags verbatim without
// buffering the whole container) and buffers only individual entry subtrees
// (<string>/<string-array>/<plurals>/other), reusing the same emit* handlers as
// the buffered walk. Skeleton is emitted inline exactly as walkSpan does, so an
// untranslated round-trip is byte-identical with peak memory O(read-ahead + one
// entry) instead of the whole document + token slice.
func (r *Reader) readStreaming(ctx context.Context, ch chan<- model.PartResult) {
	locale := r.Doc.SourceLocale
	if locale.IsEmpty() {
		locale = model.LocaleEnglish
	}
	layer := &model.Layer{
		ID:         "doc1",
		Name:       r.Doc.URI,
		Format:     "androidxml",
		Locale:     locale,
		Encoding:   r.Doc.Encoding,
		MimeType:   "application/xml",
		Properties: map[string]string{},
	}
	if !r.emit(ctx, ch, &model.Part{Type: model.PartLayerStart, Resource: layer}) {
		return
	}

	tz := newStreamTokenizer(bufio.NewReader(safeio.DefaultBudget().Reader(r.Doc.Reader)))
	counter := 0
	if !r.streamWalk(ctx, ch, tz, &counter) {
		return
	}
	r.skelFlush()
	r.emit(ctx, ch, &model.Part{Type: model.PartLayerEnd, Resource: layer})
}

// streamWalk pulls tokens one at a time, mirroring walkSpan's dispatch. It
// returns false when the context is cancelled or a token error was emitted.
func (r *Reader) streamWalk(ctx context.Context, ch chan<- model.PartResult, tz *streamTokenizer, counter *int) bool {
	pendingComment := ""
	for {
		t, err := tz.next()
		if err != nil {
			ch <- model.PartResult{Error: fmt.Errorf("androidxml: %w", err)}
			return false
		}
		if t.kind == tokEOF {
			return true
		}

		switch t.kind {
		case tokComment:
			pendingComment = commentText(t.raw)
			r.skelText(t.raw)
		case tokText:
			if strings.TrimSpace(t.raw) != "" {
				pendingComment = ""
			}
			r.skelText(t.raw)
		case tokSelfClose:
			pendingComment = ""
			r.skelText(t.raw)
		case tokStartTag:
			switch t.name {
			case "resources":
				// Stream through the container: emit its start tag verbatim and
				// keep pulling children (its </resources> is handled verbatim by
				// the default end-tag case). Never buffer the whole container.
				pendingComment = ""
				r.skelText(t.raw)
			case "string", "string-array", "plurals":
				entry, ok, cont := r.bufferEntry(ctx, ch, tz, t)
				if !cont {
					return false
				}
				if !ok {
					// Unbalanced: no matching end tag before EOF. Mirror
					// matchEnd < 0 — keep the buffered bytes verbatim.
					r.skelTokens(entry)
					continue
				}
				if !r.dispatchEntry(ctx, ch, entry, &pendingComment, counter) {
					return false
				}
			default:
				// Any other element is non-translatable structure: buffer its
				// subtree and round-trip it verbatim.
				pendingComment = ""
				entry, _, cont := r.bufferEntry(ctx, ch, tz, t)
				if !cont {
					return false
				}
				r.skelTokens(entry)
			}
		default:
			// End tags, CDATA, PIs, prolog/doctype, anything else: verbatim.
			r.skelText(t.raw)
		}
	}
}

// bufferEntry reads the whole subtree of a start token (through its matching end
// tag, depth-tracked on the element name), returning the [start … end] token
// slice. ok is false when EOF arrives before the element closes (unbalanced).
// cont is false only when a token error was emitted.
func (r *Reader) bufferEntry(ctx context.Context, ch chan<- model.PartResult, tz *streamTokenizer, start token) (entry []token, ok bool, cont bool) {
	entry = []token{start}
	depth := 1
	for depth > 0 {
		nt, err := tz.next()
		if err != nil {
			ch <- model.PartResult{Error: fmt.Errorf("androidxml: %w", err)}
			return entry, false, false
		}
		if nt.kind == tokEOF {
			return entry, false, true // unbalanced
		}
		entry = append(entry, nt)
		switch {
		case nt.kind == tokStartTag && nt.name == start.name:
			depth++
		case nt.kind == tokEndTag && nt.name == start.name:
			depth--
		}
	}
	return entry, true, true
}

// dispatchEntry runs the buffered <string>/<string-array>/<plurals> entry
// through the same skeleton + block emission walkSpan uses. entry is
// [start … end]; inner is the span between them.
func (r *Reader) dispatchEntry(ctx context.Context, ch chan<- model.PartResult, entry []token, pendingComment *string, counter *int) bool {
	start := entry[0]
	endTok := entry[len(entry)-1]
	inner := entry[1 : len(entry)-1]

	switch start.name {
	case "string":
		switch {
		case r.isTranslatable(start) && !r.isReferenceValue(inner):
			*counter++
			r.skelText(start.raw)
			r.skelRef(blockID(*counter))
			r.skelText(endTok.raw)
			if !r.emitString(ctx, ch, start, inner, *pendingComment, *counter, true) {
				return false
			}
			*pendingComment = ""
		case !r.isTranslatable(start) && r.cfg.ExtractNonTranslatableContent():
			*counter++
			r.skelText(start.raw)
			r.skelRef(blockID(*counter))
			r.skelText(endTok.raw)
			if !r.emitString(ctx, ch, start, inner, *pendingComment, *counter, false) {
				return false
			}
			*pendingComment = ""
		default:
			r.skelTokens(entry)
		}
	case "string-array":
		switch {
		case r.isTranslatable(start):
			r.skelText(start.raw)
			if !r.emitArray(ctx, ch, start, inner, *pendingComment, counter, true) {
				return false
			}
			r.skelText(endTok.raw)
			*pendingComment = ""
		case r.cfg.ExtractNonTranslatableContent():
			r.skelText(start.raw)
			if !r.emitArray(ctx, ch, start, inner, *pendingComment, counter, false) {
				return false
			}
			r.skelText(endTok.raw)
			*pendingComment = ""
		default:
			r.skelTokens(entry)
		}
	case "plurals":
		switch {
		case r.isTranslatable(start):
			r.skelText(start.raw)
			if !r.emitPlurals(ctx, ch, start, inner, *pendingComment, counter, true) {
				return false
			}
			r.skelText(endTok.raw)
			*pendingComment = ""
		case r.cfg.ExtractNonTranslatableContent():
			r.skelText(start.raw)
			if !r.emitPlurals(ctx, ch, start, inner, *pendingComment, counter, false) {
				return false
			}
			r.skelText(endTok.raw)
			*pendingComment = ""
		default:
			r.skelTokens(entry)
		}
	}
	return true
}
