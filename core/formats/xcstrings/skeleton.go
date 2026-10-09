package xcstrings

import (
	"strconv"

	"github.com/neokapi/neokapi/core/internal/jsonscan"
)

// This file implements the reader-side byte-exact skeleton emission for Apple
// String Catalogs. It re-tokenizes the original document and walks the token
// stream with the exact same structure as the writer's rewriter (rewrite.go),
// emitting every non-translatable byte as skeleton Text and each translatable
// leaf "value" string as a skeleton Ref keyed by the block's ID.
//
// The block IDs assigned here ("tu"+counter, in document order, with the same
// stale-entry skip as the reader's emitLeaf path) line up exactly with the
// blocks the reader emits, so the writer can splice each block's (possibly
// translated) value back into the captured skeleton — the contract `kapi merge`
// depends on. An identity roundtrip (no translation) is byte-for-byte exact.

// emitSkeleton drives the skeleton token walk over the original document bytes.
// It is only called when a skeleton store is wired. Tokenization errors are
// silently ignored: parseCatalog already succeeded on the same bytes, so the
// scanner will not fail here; if it somehow did, the writer falls back to the
// no-skeleton path (which also has no original to splice, producing scratch
// output) — never a panic.
func (r *Reader) emitSkeleton(content []byte) {
	sc := jsonscan.New(content, scanPrefix)
	tokens, err := sc.Scan()
	if err != nil {
		return
	}
	sw := &skelWalker{r: r, tokens: tokens}
	sw.walkTop()
	// Trailing whitespace lives on the EOF token's prefix.
	if sw.pos < len(tokens) && tokens[sw.pos].Type == jsonscan.EOF {
		r.skelText(tokens[sw.pos].Prefix)
	}
	r.skelFlush()
}

// skelWalker walks the token stream emitting skeleton entries. It mirrors
// rewriter (rewrite.go) field-for-field; the only difference is that at each
// translatable leaf "value" it emits a Ref (block ID) instead of copying the
// raw value bytes.
type skelWalker struct {
	r       *Reader
	tokens  []jsonscan.Token
	pos     int
	counter int  // block-ID counter, advanced in lockstep with emitLeaf
	skip    bool // true while walking a stale-skipped entry's leaves
}

func (s *skelWalker) cur() jsonscan.Token {
	if s.pos < len(s.tokens) {
		return s.tokens[s.pos]
	}
	return jsonscan.Token{Type: jsonscan.EOF}
}

// tok copies the current token to the skeleton buffer and advances.
func (s *skelWalker) tok() {
	s.r.skelToken(s.cur())
	s.pos++
}

func (s *skelWalker) walkTop() {
	if s.cur().Type != jsonscan.ObjectStart {
		// Not the expected shape — copy everything verbatim so output is still
		// byte-exact (no Refs, but identity holds).
		s.copyRest()
		return
	}
	s.tok() // {
	for {
		t := s.cur()
		switch t.Type {
		case jsonscan.ObjectEnd:
			s.tok()
			return
		case jsonscan.Comma:
			s.tok()
			continue
		case jsonscan.String:
			key := t.Value
			s.tok() // key
			s.colon()
			if key == "strings" {
				s.walkStrings()
			} else {
				s.copyValue()
			}
		case jsonscan.EOF:
			return
		default:
			s.tok()
		}
	}
}

func (s *skelWalker) walkStrings() {
	if s.cur().Type != jsonscan.ObjectStart {
		s.copyValue()
		return
	}
	s.tok() // {
	for {
		t := s.cur()
		switch t.Type {
		case jsonscan.ObjectEnd:
			s.tok()
			return
		case jsonscan.Comma:
			s.tok()
			continue
		case jsonscan.String:
			entryKey := t.Value
			s.tok() // entry key
			s.colon()
			s.walkEntry(entryKey)
		case jsonscan.EOF:
			return
		default:
			s.tok()
		}
	}
}

func (s *skelWalker) walkEntry(entryKey string) {
	if s.cur().Type != jsonscan.ObjectStart {
		s.copyValue()
		return
	}
	// Decide staleness for this entry up front so leaf emission matches the
	// reader's stale-skip exactly, regardless of field order. When skipped, the
	// entry's bytes are still copied verbatim but no Refs/IDs are produced.
	s.skip = !s.r.cfg.ExtractStale && s.entryIsStale(s.pos)
	s.tok() // {
	for {
		t := s.cur()
		switch t.Type {
		case jsonscan.ObjectEnd:
			s.tok()
			s.skip = false
			return
		case jsonscan.Comma:
			s.tok()
			continue
		case jsonscan.String:
			field := t.Value
			s.tok() // field key
			s.colon()
			if field == "localizations" {
				s.walkLocalizations(valueRef{Key: entryKey})
			} else {
				s.copyValue()
			}
		case jsonscan.EOF:
			s.skip = false
			return
		default:
			s.tok()
		}
	}
}

// entryIsStale scans an entry object (starting at the '{' at objStart) for an
// "extractionState":"stale" pair without consuming tokens. It balances nested
// braces so it only inspects the entry's own top-level fields.
func (s *skelWalker) entryIsStale(objStart int) bool {
	if objStart >= len(s.tokens) || s.tokens[objStart].Type != jsonscan.ObjectStart {
		return false
	}
	depth := 0
	for i := objStart; i < len(s.tokens); i++ {
		t := s.tokens[i]
		switch t.Type {
		case jsonscan.ObjectStart, jsonscan.ArrayStart:
			depth++
		case jsonscan.ObjectEnd, jsonscan.ArrayEnd:
			depth--
			if depth == 0 {
				return false
			}
		case jsonscan.String:
			// Only inspect fields at the entry's own depth (depth == 1).
			if depth == 1 && t.Value == "extractionState" {
				// next non-colon token is the value
				if i+2 < len(s.tokens) && s.tokens[i+1].Type == jsonscan.Colon &&
					s.tokens[i+2].Type == jsonscan.String {
					return s.tokens[i+2].Value == "stale"
				}
			}
		}
	}
	return false
}

func (s *skelWalker) walkLocalizations(base valueRef) {
	if s.cur().Type != jsonscan.ObjectStart {
		s.copyValue()
		return
	}
	s.tok() // {
	for {
		t := s.cur()
		switch t.Type {
		case jsonscan.ObjectEnd:
			s.tok()
			return
		case jsonscan.Comma:
			s.tok()
			continue
		case jsonscan.String:
			lang := t.Value
			s.tok() // lang key
			s.colon()
			vr := base
			vr.Lang = lang
			s.walkLocalization(vr)
		case jsonscan.EOF:
			return
		default:
			s.tok()
		}
	}
}

func (s *skelWalker) walkLocalization(base valueRef) {
	if s.cur().Type != jsonscan.ObjectStart {
		s.copyValue()
		return
	}
	s.tok() // {
	for {
		t := s.cur()
		switch t.Type {
		case jsonscan.ObjectEnd:
			s.tok()
			return
		case jsonscan.Comma:
			s.tok()
			continue
		case jsonscan.String:
			field := t.Value
			s.tok() // field key
			s.colon()
			switch field {
			case "stringUnit":
				vr := base
				vr.Kind = kindStringUnit
				s.walkStringUnit(vr)
			case "variations":
				s.walkVariations(base, "")
			default:
				s.copyValue()
			}
		case jsonscan.EOF:
			return
		default:
			s.tok()
		}
	}
}

func (s *skelWalker) walkVariations(base valueRef, sub string) {
	if s.cur().Type != jsonscan.ObjectStart {
		s.copyValue()
		return
	}
	s.tok() // {
	for {
		t := s.cur()
		switch t.Type {
		case jsonscan.ObjectEnd:
			s.tok()
			return
		case jsonscan.Comma:
			s.tok()
			continue
		case jsonscan.String:
			field := t.Value
			s.tok() // field key
			s.colon()
			switch field {
			case "plural":
				kind := kindPlural
				if sub != "" {
					kind = kindSubstitutionPlural
				}
				s.walkCategoryMap(base, kind, sub)
			case "device":
				kind := kindDevice
				if sub != "" {
					kind = kindSubstitutionDevice
				}
				s.walkCategoryMap(base, kind, sub)
			case "substitutions":
				s.walkSubstitutions(base)
			default:
				s.copyValue()
			}
		case jsonscan.EOF:
			return
		default:
			s.tok()
		}
	}
}

func (s *skelWalker) walkCategoryMap(base valueRef, kind valueKind, sub string) {
	if s.cur().Type != jsonscan.ObjectStart {
		s.copyValue()
		return
	}
	s.tok() // {
	for {
		t := s.cur()
		switch t.Type {
		case jsonscan.ObjectEnd:
			s.tok()
			return
		case jsonscan.Comma:
			s.tok()
			continue
		case jsonscan.String:
			category := t.Value
			s.tok() // category key
			s.colon()
			s.walkCategoryBody(base, kind, sub, category)
		case jsonscan.EOF:
			return
		default:
			s.tok()
		}
	}
}

func (s *skelWalker) walkCategoryBody(base valueRef, kind valueKind, sub, category string) {
	if s.cur().Type != jsonscan.ObjectStart {
		s.copyValue()
		return
	}
	s.tok() // {
	for {
		t := s.cur()
		switch t.Type {
		case jsonscan.ObjectEnd:
			s.tok()
			return
		case jsonscan.Comma:
			s.tok()
			continue
		case jsonscan.String:
			field := t.Value
			s.tok() // field key
			s.colon()
			if field == "stringUnit" {
				vr := base
				vr.Kind = kind
				vr.Sub = sub
				vr.Category = category
				s.walkStringUnit(vr)
			} else {
				s.copyValue()
			}
		case jsonscan.EOF:
			return
		default:
			s.tok()
		}
	}
}

func (s *skelWalker) walkSubstitutions(base valueRef) {
	if s.cur().Type != jsonscan.ObjectStart {
		s.copyValue()
		return
	}
	s.tok() // {
	for {
		t := s.cur()
		switch t.Type {
		case jsonscan.ObjectEnd:
			s.tok()
			return
		case jsonscan.Comma:
			s.tok()
			continue
		case jsonscan.String:
			name := t.Value
			s.tok() // substitution name
			s.colon()
			s.walkSubstitution(base, name)
		case jsonscan.EOF:
			return
		default:
			s.tok()
		}
	}
}

func (s *skelWalker) walkSubstitution(base valueRef, name string) {
	if s.cur().Type != jsonscan.ObjectStart {
		s.copyValue()
		return
	}
	s.tok() // {
	for {
		t := s.cur()
		switch t.Type {
		case jsonscan.ObjectEnd:
			s.tok()
			return
		case jsonscan.Comma:
			s.tok()
			continue
		case jsonscan.String:
			field := t.Value
			s.tok() // field key
			s.colon()
			if field == "variations" {
				s.walkVariations(base, name)
			} else {
				s.copyValue()
			}
		case jsonscan.EOF:
			return
		default:
			s.tok()
		}
	}
}

// walkStringUnit handles { "state": "...", "value": "..." }. The "value" leaf
// is the translatable one: when this entry is not stale-skipped, it consumes a
// block ID and emits a Ref instead of the raw value. "state" and any other
// fields are copied verbatim, matching the reader (which carries state as a
// block property, not a translatable block).
func (s *skelWalker) walkStringUnit(vr valueRef) {
	if s.cur().Type != jsonscan.ObjectStart {
		s.copyValue()
		return
	}
	// Reserve the block ID for this leaf up front (in document order), unless
	// this entry is stale-skipped — then the reader's emitLeaf is never called
	// for it, so no ID is consumed here either.
	var blockID string
	if !s.skip {
		s.counter++
		blockID = "tu" + strconv.Itoa(s.counter)
	}
	s.tok() // {
	for {
		t := s.cur()
		switch t.Type {
		case jsonscan.ObjectEnd:
			s.tok()
			return
		case jsonscan.Comma:
			s.tok()
			continue
		case jsonscan.String:
			field := t.Value
			s.tok() // field key
			s.colon()
			valTok := s.cur()
			if field == "value" && !s.skip && valTok.Type == jsonscan.String {
				// Emit Ref in place of the raw value; its prefix (whitespace
				// before the value jsonscan.Token) is preserved as Text by skelRef.
				s.r.skelRef(valTok.Prefix, blockID)
				s.pos++
			} else {
				s.copyValue()
			}
		case jsonscan.EOF:
			return
		default:
			s.tok()
		}
	}
}

// colon copies the ':' separator token.
func (s *skelWalker) colon() {
	if s.cur().Type == jsonscan.Colon {
		s.tok()
	}
}

// copyValue copies an arbitrary JSON value (scalar/object/array) verbatim,
// keeping nested structure balanced — identical to rewriter.copyValue but
// writing to the skeleton buffer.
func (s *skelWalker) copyValue() {
	t := s.cur()
	switch t.Type {
	case jsonscan.ObjectStart, jsonscan.ArrayStart:
		s.tok()
		depth := 1
		for depth > 0 {
			t := s.cur()
			if t.Type == jsonscan.EOF {
				return
			}
			s.tok()
			switch t.Type {
			case jsonscan.ObjectStart, jsonscan.ArrayStart:
				depth++
			case jsonscan.ObjectEnd, jsonscan.ArrayEnd:
				depth--
			}
		}
	default:
		s.tok()
	}
}

// copyRest copies all remaining tokens verbatim (used when the top-level shape
// is unexpected, guaranteeing byte-exact identity even off the happy path).
func (s *skelWalker) copyRest() {
	for s.pos < len(s.tokens) && s.cur().Type != jsonscan.EOF {
		s.tok()
	}
}
