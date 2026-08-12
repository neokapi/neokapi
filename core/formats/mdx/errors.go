package mdx

import (
	"bytes"
	"fmt"
)

// errors.go holds the MDX reader's structural backstop.
//
// The reader's opaque regions are byte-faithful by construction, so a
// mis-scan never corrupts a document — it makes content DISAPPEAR. An MDX
// construct that opens and never closes takes every following byte into one
// opaque region: the file still round-trips, still exits 0, and still reports
// blocks, but the document's tail is no longer readable content. Reporting a
// fraction of a document as if it were the whole document is worse than
// failing, because nothing downstream can tell the difference.
//
// Both errors below therefore stop the read. They are raised after the scan
// and before any part is emitted, so a failing document produces an error and
// no partial part stream.

// UnterminatedConstructError reports a top-level MDX construct that opened and
// never closed: the scan reached end of document still looking for its closing
// tag or brace, so the rest of the document sits inside the region.
//
// Fenced code is opaque to the scan (see fence.go), so an angle-bracket token
// in a code sample does not raise this — reaching it means the document really
// does hold an unclosed construct, which the MDX compiler rejects too.
type UnterminatedConstructError struct {
	// Kind names the construct in prose: "JSX element", "ESM statement",
	// or "expression".
	Kind string
	// Line is the 1-based line the construct opens on, counted over the
	// whole file (front matter included).
	Line int
	// Offset is the 0-based byte offset of the construct's first byte,
	// after any byte-order mark.
	Offset int
	// Opening is the construct's first line, for the message.
	Opening string
	// Swallowed is how many bytes the unterminated region covers.
	Swallowed int
}

func (e *UnterminatedConstructError) Error() string {
	return fmt.Sprintf(
		"mdx: unterminated %s opened at line %d (byte %d): %q never closes, so the remaining %d bytes of the document are unreadable",
		e.Kind, e.Line, e.Offset, e.Opening, e.Swallowed)
}

// ScanCoverageError reports that the segmenter did not partition the document:
// the segments overlap, run backwards, or stop short of the final byte. Every
// byte must belong to exactly one segment, so this is an internal invariant
// failure rather than a property of the input — and one that would otherwise
// present as a document quietly missing its tail.
type ScanCoverageError struct {
	// Covered is the byte offset the segments account for.
	Covered int
	// Size is the document's length in bytes.
	Size int
}

func (e *ScanCoverageError) Error() string {
	return fmt.Sprintf("mdx: scan covered %d of %d bytes: the segmenter did not partition the document",
		e.Covered, e.Size)
}

// checkSegments enforces the two invariants a completed scan must satisfy
// before the reader emits anything: the segments tile the document exactly,
// and every opaque region closed.
func checkSegments(segs []segment, body []byte) error {
	covered := 0
	for _, seg := range segs {
		if seg.start != covered || seg.end < seg.start || seg.end > len(body) {
			return &ScanCoverageError{Covered: covered, Size: len(body)}
		}
		covered = seg.end
		if seg.unterminated {
			return unterminatedError(seg, body)
		}
	}
	if covered != len(body) {
		return &ScanCoverageError{Covered: covered, Size: len(body)}
	}
	return nil
}

// unterminatedError builds the report for an unterminated opaque region.
func unterminatedError(seg segment, body []byte) *UnterminatedConstructError {
	return &UnterminatedConstructError{
		Kind:      seg.kind.constructName(),
		Line:      1 + bytes.Count(body[:seg.start], []byte("\n")),
		Offset:    seg.start,
		Opening:   openingLine(body, seg.start),
		Swallowed: seg.end - seg.start,
	}
}

// constructName names a segment kind the way an error message should.
func (k segmentKind) constructName() string {
	switch k {
	case segESM:
		return "ESM statement"
	case segJSX:
		return "JSX element"
	case segExpr:
		return "expression"
	default:
		return "region"
	}
}

// openingLine returns the line at start, trimmed of its line ending and
// shortened for a one-line error message.
func openingLine(body []byte, start int) string {
	line := body[start:lineEndAt(body, start)]
	line = bytes.TrimRight(line, "\r")
	const maxLen = 60
	if len(line) > maxLen {
		return string(line[:maxLen]) + "…"
	}
	return string(line)
}
