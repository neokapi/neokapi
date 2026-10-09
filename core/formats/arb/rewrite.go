package arb

import (
	"fmt"
	"strings"

	"github.com/neokapi/neokapi/core/internal/jsonscan"
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
// scanPrefix names this format in the shared JSON scanner's error messages.
const scanPrefix = "arb scanner"

func rewriteCatalog(original []byte, repl *replacements) ([]byte, error) {
	sc := jsonscan.New(original, scanPrefix)
	tokens, err := sc.Scan()
	if err != nil {
		return nil, err
	}
	rw := &rewriter{tokens: tokens, repl: repl}
	rw.walkTop()
	if rw.err != nil {
		return nil, rw.err
	}
	// Trailing whitespace lives on the EOF token's prefix.
	if rw.pos < len(tokens) && tokens[rw.pos].Type == jsonscan.EOF {
		rw.out.WriteString(tokens[rw.pos].Prefix)
	}
	return []byte(rw.out.String()), nil
}

// rewriter walks the token stream, copying tokens verbatim and substituting
// message value strings as directed by repl.
type rewriter struct {
	tokens []jsonscan.Token
	pos    int
	out    strings.Builder
	repl   *replacements
	err    error
}

func (r *rewriter) emit(t jsonscan.Token) {
	r.out.WriteString(t.Prefix)
	r.out.WriteString(t.Raw)
}

func (r *rewriter) emitReplacedString(t jsonscan.Token, newValue string) {
	r.out.WriteString(t.Prefix)
	r.out.WriteString(encodeJSONString(newValue))
}

func (r *rewriter) fail(format string, args ...any) {
	if r.err == nil {
		r.err = fmt.Errorf("arb rewrite: "+format, args...)
	}
}

func (r *rewriter) cur() jsonscan.Token {
	if r.pos < len(r.tokens) {
		return r.tokens[r.pos]
	}
	return jsonscan.Token{Type: jsonscan.EOF}
}

// walkTop walks the flat top-level object of the ARB document.
func (r *rewriter) walkTop() {
	t := r.cur()
	if t.Type != jsonscan.ObjectStart {
		r.fail("expected top-level object")
		return
	}
	r.emit(t)
	r.pos++
	for r.err == nil {
		t := r.cur()
		if t.Type == jsonscan.ObjectEnd {
			r.emit(t)
			r.pos++
			return
		}
		if t.Type == jsonscan.Comma {
			r.emit(t)
			r.pos++
			continue
		}
		if t.Type != jsonscan.String {
			r.fail("expected key in top object, got %v", t.Type)
			return
		}
		key := t.Value
		r.emit(t)
		r.pos++
		r.emitColon()

		// Only plain message keys carry a translatable string value we may
		// substitute. "@…" and "@@…" keys are copied verbatim.
		if !strings.HasPrefix(key, "@") {
			r.maybeReplaceValue(key)
		} else if v := r.cur(); key == "@@locale" && r.repl.locale != nil && v.Type == jsonscan.String {
			r.emitReplacedString(v, *r.repl.locale)
			r.pos++
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
	if valTok.Type == jsonscan.String && hasRep && rep.set && valTok.Value != rep.value {
		r.emitReplacedString(valTok, rep.value)
		r.pos++
		return
	}
	r.copyValue()
}

// emitColon copies the ':' separator token.
func (r *rewriter) emitColon() {
	t := r.cur()
	if t.Type != jsonscan.Colon {
		r.fail("expected ':', got %v", t.Type)
		return
	}
	r.emit(t)
	r.pos++
}

// copyValue copies an arbitrary JSON value (scalar/object/array) verbatim,
// keeping nested structure balanced.
func (r *rewriter) copyValue() {
	t := r.cur()
	switch t.Type {
	case jsonscan.ObjectStart, jsonscan.ArrayStart:
		r.emit(t)
		r.pos++
		depth := 1
		for depth > 0 && r.err == nil {
			t := r.cur()
			if t.Type == jsonscan.EOF {
				r.fail("unexpected EOF while copying value")
				return
			}
			r.emit(t)
			r.pos++
			switch t.Type {
			case jsonscan.ObjectStart, jsonscan.ArrayStart:
				depth++
			case jsonscan.ObjectEnd, jsonscan.ArrayEnd:
				depth--
			}
		}
	default:
		r.emit(t)
		r.pos++
	}
}
