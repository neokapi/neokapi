package xcstrings

import (
	"fmt"
	"strings"

	"github.com/neokapi/neokapi/core/internal/jsonscan"
)

// rewriteCatalog re-tokenizes the original document and writes it back, leaving
// every byte intact except leaf "value" / "state" strings whose location
// matches an entry in repl with a changed value. The result is byte-identical
// to the input when no values changed.
// scanPrefix names this format in the shared JSON scanner's error messages.
const scanPrefix = "xcstrings scanner"

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
// leaf value/state strings as directed by repl. It tracks just enough schema
// context (the current valueRef) to identify which leaf it is at.
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
		r.err = fmt.Errorf("xcstrings rewrite: "+format, args...)
	}
}

func (r *rewriter) cur() jsonscan.Token {
	if r.pos < len(r.tokens) {
		return r.tokens[r.pos]
	}
	return jsonscan.Token{Type: jsonscan.EOF}
}

// walkTop walks the top-level object: { sourceLanguage, strings, version }.
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
		if key == "strings" {
			r.walkStrings()
		} else {
			r.copyValue()
		}
	}
}

func (r *rewriter) walkStrings() {
	t := r.cur()
	if t.Type != jsonscan.ObjectStart {
		r.copyValue()
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
			r.fail("expected entry key, got %v", t.Type)
			return
		}
		entryKey := t.Value
		r.emit(t)
		r.pos++
		r.emitColon()
		r.walkEntry(entryKey)
	}
}

func (r *rewriter) walkEntry(entryKey string) {
	t := r.cur()
	if t.Type != jsonscan.ObjectStart {
		r.copyValue()
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
			r.fail("expected entry field key, got %v", t.Type)
			return
		}
		field := t.Value
		r.emit(t)
		r.pos++
		r.emitColon()
		if field == "localizations" {
			r.walkLocalizations(entryKey)
		} else {
			r.copyValue()
		}
	}
}

func (r *rewriter) walkLocalizations(entryKey string) {
	t := r.cur()
	if t.Type != jsonscan.ObjectStart {
		r.copyValue()
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
			r.fail("expected lang key, got %v", t.Type)
			return
		}
		lang := t.Value
		r.emit(t)
		r.pos++
		r.emitColon()
		r.walkLocalization(valueRef{Key: entryKey, Lang: lang})
	}
}

// walkLocalization handles a localization object containing either a
// stringUnit or a variations subtree. The base valueRef carries Key+Lang.
func (r *rewriter) walkLocalization(base valueRef) {
	t := r.cur()
	if t.Type != jsonscan.ObjectStart {
		r.copyValue()
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
			r.fail("expected localization field key, got %v", t.Type)
			return
		}
		field := t.Value
		r.emit(t)
		r.pos++
		r.emitColon()
		switch field {
		case "stringUnit":
			vr := base
			vr.Kind = kindStringUnit
			r.walkStringUnit(vr)
		case "variations":
			r.walkVariations(base, "")
		default:
			r.copyValue()
		}
	}
}

// walkVariations handles a variations object (plural/device/substitutions).
// sub is the substitution argument name when descending into a substitution's
// own variation subtree (empty at the top level).
func (r *rewriter) walkVariations(base valueRef, sub string) {
	t := r.cur()
	if t.Type != jsonscan.ObjectStart {
		r.copyValue()
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
			r.fail("expected variations field key, got %v", t.Type)
			return
		}
		field := t.Value
		r.emit(t)
		r.pos++
		r.emitColon()
		switch field {
		case "plural":
			kind := kindPlural
			if sub != "" {
				kind = kindSubstitutionPlural
			}
			r.walkCategoryMap(base, kind, sub)
		case "device":
			kind := kindDevice
			if sub != "" {
				kind = kindSubstitutionDevice
			}
			r.walkCategoryMap(base, kind, sub)
		case "substitutions":
			r.walkSubstitutions(base)
		default:
			r.copyValue()
		}
	}
}

// walkCategoryMap handles a { "<category>" : { "stringUnit" : {...} } } map.
func (r *rewriter) walkCategoryMap(base valueRef, kind valueKind, sub string) {
	t := r.cur()
	if t.Type != jsonscan.ObjectStart {
		r.copyValue()
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
			r.fail("expected category key, got %v", t.Type)
			return
		}
		category := t.Value
		r.emit(t)
		r.pos++
		r.emitColon()
		// Each category is { "stringUnit" : {...} }.
		r.walkCategoryBody(base, kind, sub, category)
	}
}

func (r *rewriter) walkCategoryBody(base valueRef, kind valueKind, sub, category string) {
	t := r.cur()
	if t.Type != jsonscan.ObjectStart {
		r.copyValue()
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
			r.fail("expected category body key, got %v", t.Type)
			return
		}
		field := t.Value
		r.emit(t)
		r.pos++
		r.emitColon()
		if field == "stringUnit" {
			vr := base
			vr.Kind = kind
			vr.Sub = sub
			vr.Category = category
			r.walkStringUnit(vr)
		} else {
			r.copyValue()
		}
	}
}

func (r *rewriter) walkSubstitutions(base valueRef) {
	t := r.cur()
	if t.Type != jsonscan.ObjectStart {
		r.copyValue()
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
			r.fail("expected substitution name, got %v", t.Type)
			return
		}
		name := t.Value
		r.emit(t)
		r.pos++
		r.emitColon()
		r.walkSubstitution(base, name)
	}
}

func (r *rewriter) walkSubstitution(base valueRef, name string) {
	t := r.cur()
	if t.Type != jsonscan.ObjectStart {
		r.copyValue()
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
			r.fail("expected substitution field key, got %v", t.Type)
			return
		}
		field := t.Value
		r.emit(t)
		r.pos++
		r.emitColon()
		if field == "variations" {
			r.walkVariations(base, name)
		} else {
			r.copyValue()
		}
	}
}

// walkStringUnit handles a { "state" : "...", "value" : "..." } object,
// substituting the value (and state) for the given leaf reference when a
// replacement is present.
func (r *rewriter) walkStringUnit(vr valueRef) {
	t := r.cur()
	if t.Type != jsonscan.ObjectStart {
		r.copyValue()
		return
	}
	r.emit(t)
	r.pos++

	rep, hasRep := r.repl.lookup(vr)

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
			r.fail("expected stringUnit field key, got %v", t.Type)
			return
		}
		field := t.Value
		r.emit(t)
		r.pos++
		r.emitColon()
		valTok := r.cur()
		switch {
		case field == "value" && hasRep && rep.set:
			if valTok.Type == jsonscan.String && valTok.Value != rep.value {
				r.emitReplacedString(valTok, rep.value)
			} else {
				r.emit(valTok)
			}
			r.pos++
		case field == "state" && hasRep && rep.set && rep.state != "":
			if valTok.Type == jsonscan.String && valTok.Value != rep.state {
				r.emitReplacedString(valTok, rep.state)
			} else {
				r.emit(valTok)
			}
			r.pos++
		default:
			r.copyValue()
		}
	}
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
