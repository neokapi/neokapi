package arb

import (
	"fmt"
	"strings"
)

// rewriteCatalog re-tokenizes the original document and writes it back, leaving
// every byte intact except message value strings whose key matches an entry in
// repl with a changed value. The result is byte-identical to the input when no
// values changed.
//
// ARB is a flat JSON object, so the rewriter only walks the top level: each
// "<key>" : "<value>" pair whose key is a message key (not "@…"/"@@…") and has
// a replacement gets its value string substituted. Attribute objects ("@<id>")
// and global metadata ("@@<name>") are copied verbatim.
func rewriteCatalog(original []byte, repl *replacements) ([]byte, error) {
	sc := newScanner(original)
	tokens, err := sc.scan()
	if err != nil {
		return nil, err
	}
	rw := &rewriter{tokens: tokens, repl: repl}
	rw.walkTop()
	if rw.err != nil {
		return nil, rw.err
	}
	// Trailing whitespace lives on the EOF token's prefix.
	if rw.pos < len(tokens) && tokens[rw.pos].typ == tokEOF {
		rw.out.WriteString(tokens[rw.pos].prefix)
	}
	return []byte(rw.out.String()), nil
}

// rewriter walks the token stream, copying tokens verbatim and substituting
// message value strings as directed by repl.
type rewriter struct {
	tokens []token
	pos    int
	out    strings.Builder
	repl   *replacements
	err    error
}

func (r *rewriter) emit(t token) {
	r.out.WriteString(t.prefix)
	r.out.WriteString(t.raw)
}

func (r *rewriter) emitReplacedString(t token, newValue string) {
	r.out.WriteString(t.prefix)
	r.out.WriteString(encodeJSONString(newValue))
}

func (r *rewriter) fail(format string, args ...any) {
	if r.err == nil {
		r.err = fmt.Errorf("arb rewrite: "+format, args...)
	}
}

func (r *rewriter) cur() token {
	if r.pos < len(r.tokens) {
		return r.tokens[r.pos]
	}
	return token{typ: tokEOF}
}

// walkTop walks the flat top-level object of the ARB document.
func (r *rewriter) walkTop() {
	t := r.cur()
	if t.typ != tokObjectStart {
		r.fail("expected top-level object")
		return
	}
	r.emit(t)
	r.pos++
	for r.err == nil {
		t := r.cur()
		if t.typ == tokObjectEnd {
			r.emit(t)
			r.pos++
			return
		}
		if t.typ == tokComma {
			r.emit(t)
			r.pos++
			continue
		}
		if t.typ != tokString {
			r.fail("expected key in top object, got %v", t.typ)
			return
		}
		key := t.value
		r.emit(t)
		r.pos++
		r.emitColon()

		// Only plain message keys carry a translatable string value we may
		// substitute. "@…" and "@@…" keys are copied verbatim.
		if !strings.HasPrefix(key, "@") {
			r.maybeReplaceValue(key)
		} else {
			r.copyValue()
		}
	}
}

// maybeReplaceValue copies the value of a message key, substituting it when a
// replacement is present and the value actually changed. Non-string values
// (which would be invalid ARB) are copied verbatim defensively.
func (r *rewriter) maybeReplaceValue(key string) {
	valTok := r.cur()
	rep, hasRep := r.repl.lookup(key)
	if valTok.typ == tokString && hasRep && rep.set && valTok.value != rep.value {
		r.emitReplacedString(valTok, rep.value)
		r.pos++
		return
	}
	r.copyValue()
}

// emitColon copies the ':' separator token.
func (r *rewriter) emitColon() {
	t := r.cur()
	if t.typ != tokColon {
		r.fail("expected ':', got %v", t.typ)
		return
	}
	r.emit(t)
	r.pos++
}

// copyValue copies an arbitrary JSON value (scalar/object/array) verbatim,
// keeping nested structure balanced.
func (r *rewriter) copyValue() {
	t := r.cur()
	switch t.typ {
	case tokObjectStart, tokArrayStart:
		r.emit(t)
		r.pos++
		depth := 1
		for depth > 0 && r.err == nil {
			t := r.cur()
			if t.typ == tokEOF {
				r.fail("unexpected EOF while copying value")
				return
			}
			r.emit(t)
			r.pos++
			switch t.typ {
			case tokObjectStart, tokArrayStart:
				depth++
			case tokObjectEnd, tokArrayEnd:
				depth--
			}
		}
	default:
		r.emit(t)
		r.pos++
	}
}
