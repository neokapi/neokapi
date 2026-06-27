package json

import "io"

// tokenStream is the token source the JSON walk consumes. It abstracts over a
// fully-materialised token slice (the buffered path) and the bounded-memory
// streamScanner (the streaming path), so the same forward, ancestor-only walk
// drives both byte-identically.
//
//   - cur() returns the current token; at end it returns the EOF token (carrying
//     the document's trailing whitespace/comment bytes as its prefix).
//   - next() consumes the current token.
//   - done() reports that the EOF token has itself been consumed (the analogue of
//     the buffered walk's `pos >= len(tokens)`).
//   - err() surfaces a streaming scan error (always nil for the slice source).
type tokenStream interface {
	cur() token
	next()
	done() bool
	err() error
}

// sliceStream walks a pre-scanned token slice (which ends with a tokenEOF). It is
// the buffered path, byte-identical to the historical index-based walk.
type sliceStream struct {
	toks []token
	pos  int
}

func (s *sliceStream) cur() token {
	if s.pos < len(s.toks) {
		return s.toks[s.pos]
	}
	return token{typ: tokenEOF}
}

func (s *sliceStream) next() {
	if s.pos < len(s.toks) {
		s.pos++
	}
}

func (s *sliceStream) done() bool { return s.pos >= len(s.toks) }

func (s *sliceStream) err() error { return nil }

// streamTokenStream pulls tokens one at a time from a streamScanner, holding only
// the current token — the bounded-memory path. A scan error is captured and
// surfaced via err(); the walk then terminates as if at EOF (the caller checks
// err() afterwards and emits it).
type streamTokenStream struct {
	sc       *streamScanner
	curTok   token
	loaded   bool
	consumed bool
	scanErr  error
}

func newStreamTokenStream(r io.Reader) *streamTokenStream {
	return &streamTokenStream{sc: newStreamScanner(r)}
}

func (s *streamTokenStream) load() {
	if s.loaded || s.consumed {
		return
	}
	t, err := s.sc.next()
	if err != nil {
		s.scanErr = err
		t = token{typ: tokenEOF}
	}
	s.curTok = t
	s.loaded = true
}

func (s *streamTokenStream) cur() token {
	s.load()
	return s.curTok
}

func (s *streamTokenStream) next() {
	s.load()
	if s.curTok.typ == tokenEOF {
		s.consumed = true
		return
	}
	s.loaded = false
}

func (s *streamTokenStream) done() bool { return s.consumed }

func (s *streamTokenStream) err() error { return s.scanErr }
