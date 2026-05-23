package xcstrings

import (
	"fmt"
	"strings"
)

// rewriteCatalog re-tokenizes the original document and writes it back, leaving
// every byte intact except leaf "value" / "state" strings whose location
// matches an entry in repl with a changed value. The result is byte-identical
// to the input when no values changed.
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
// leaf value/state strings as directed by repl. It tracks just enough schema
// context (the current valueRef) to identify which leaf it is at.
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
		r.err = fmt.Errorf("xcstrings rewrite: "+format, args...)
	}
}

func (r *rewriter) cur() token {
	if r.pos < len(r.tokens) {
		return r.tokens[r.pos]
	}
	return token{typ: tokEOF}
}

// walkTop walks the top-level object: { sourceLanguage, strings, version }.
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
		if key == "strings" {
			r.walkStrings()
		} else {
			r.copyValue()
		}
	}
}

func (r *rewriter) walkStrings() {
	t := r.cur()
	if t.typ != tokObjectStart {
		r.copyValue()
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
			r.fail("expected entry key, got %v", t.typ)
			return
		}
		entryKey := t.value
		r.emit(t)
		r.pos++
		r.emitColon()
		r.walkEntry(entryKey)
	}
}

func (r *rewriter) walkEntry(entryKey string) {
	t := r.cur()
	if t.typ != tokObjectStart {
		r.copyValue()
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
			r.fail("expected entry field key, got %v", t.typ)
			return
		}
		field := t.value
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
	if t.typ != tokObjectStart {
		r.copyValue()
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
			r.fail("expected lang key, got %v", t.typ)
			return
		}
		lang := t.value
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
	if t.typ != tokObjectStart {
		r.copyValue()
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
			r.fail("expected localization field key, got %v", t.typ)
			return
		}
		field := t.value
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
	if t.typ != tokObjectStart {
		r.copyValue()
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
			r.fail("expected variations field key, got %v", t.typ)
			return
		}
		field := t.value
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
	if t.typ != tokObjectStart {
		r.copyValue()
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
			r.fail("expected category key, got %v", t.typ)
			return
		}
		category := t.value
		r.emit(t)
		r.pos++
		r.emitColon()
		// Each category is { "stringUnit" : {...} }.
		r.walkCategoryBody(base, kind, sub, category)
	}
}

func (r *rewriter) walkCategoryBody(base valueRef, kind valueKind, sub, category string) {
	t := r.cur()
	if t.typ != tokObjectStart {
		r.copyValue()
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
			r.fail("expected category body key, got %v", t.typ)
			return
		}
		field := t.value
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
	if t.typ != tokObjectStart {
		r.copyValue()
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
			r.fail("expected substitution name, got %v", t.typ)
			return
		}
		name := t.value
		r.emit(t)
		r.pos++
		r.emitColon()
		r.walkSubstitution(base, name)
	}
}

func (r *rewriter) walkSubstitution(base valueRef, name string) {
	t := r.cur()
	if t.typ != tokObjectStart {
		r.copyValue()
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
			r.fail("expected substitution field key, got %v", t.typ)
			return
		}
		field := t.value
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
	if t.typ != tokObjectStart {
		r.copyValue()
		return
	}
	r.emit(t)
	r.pos++

	rep, hasRep := r.repl.lookup(vr)

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
			r.fail("expected stringUnit field key, got %v", t.typ)
			return
		}
		field := t.value
		r.emit(t)
		r.pos++
		r.emitColon()
		valTok := r.cur()
		switch {
		case field == "value" && hasRep && rep.set:
			if valTok.typ == tokString && valTok.value != rep.value {
				r.emitReplacedString(valTok, rep.value)
			} else {
				r.emit(valTok)
			}
			r.pos++
		case field == "state" && hasRep && rep.set && rep.state != "":
			if valTok.typ == tokString && valTok.value != rep.state {
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
