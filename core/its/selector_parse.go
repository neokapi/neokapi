package its

import (
	"errors"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

// ParseSelector parses an XPath subset selector string into a
// Selector tree. `nsMap` resolves prefixes to namespace URIs; it
// must include the binding for every prefix used in the selector
// (typically populated from the in-scope namespaces of the
// <its:rules> element).
//
// Returns a non-nil error for unsupported XPath features so callers
// can surface authoring mistakes instead of silently mis-matching.
func ParseSelector(s string, nsMap map[string]string) (*Selector, error) {
	if strings.TrimSpace(s) == "" {
		return nil, errors.New("its: empty selector")
	}
	parser := &selectorParser{src: s, nsMap: nsMap}
	return parser.parse()
}

type selectorParser struct {
	src   string
	pos   int
	nsMap map[string]string
}

func (p *selectorParser) parse() (*Selector, error) {
	sel := &Selector{}
	for {
		alt, err := p.parseAlternate()
		if err != nil {
			return nil, err
		}
		sel.Alternates = append(sel.Alternates, alt)
		p.skipWS()
		if p.eof() {
			break
		}
		if p.peek() != '|' {
			return nil, fmt.Errorf("its: unexpected %q in selector %q at offset %d", p.peek(), p.src, p.pos)
		}
		p.advance(1)
		p.skipWS()
	}
	return sel, nil
}

func (p *selectorParser) parseAlternate() (Alternate, error) {
	var alt Alternate
	p.skipWS()

	// Determine first step axis.
	if p.eof() {
		return alt, fmt.Errorf("its: unexpected end-of-input parsing alternate in %q", p.src)
	}

	firstDescendant := false
	if p.peek() == '/' {
		p.advance(1)
		if !p.eof() && p.peek() == '/' {
			p.advance(1)
			firstDescendant = true
		} else {
			firstDescendant = false
		}
	} else {
		// No leading `/` — the first step is implicitly absolute on
		// the current node. Real-world ITS selectors always start
		// with `/` or `//`, so anything else is an authoring error.
		return alt, fmt.Errorf("its: selector %q must start with `/` or `//`", p.src)
	}

	// Parse the rest of the path.
	descendant := firstDescendant
	for {
		// Attribute step terminates a path. `//@attr` (no element
		// step between `//` and `@`) is shorthand for `//*/@attr` —
		// the descendant-axis must land on some element before the
		// attribute step makes sense. Insert that implicit `*` step
		// so matchSteps has something to align with the path.
		if p.peek() == '@' {
			if len(alt.Steps) == 0 {
				alt.Steps = append(alt.Steps, Step{Descendant: descendant, Name: NameMatch{Local: "*"}})
			}
			p.advance(1)
			name, err := p.parseName()
			if err != nil {
				return alt, err
			}
			alt.Attribute = &name
			break
		}

		name, err := p.parseName()
		if err != nil {
			return alt, err
		}
		alt.Steps = append(alt.Steps, Step{Descendant: descendant, Name: name})

		// Parse predicates that follow this step.
		for !p.eof() && p.peek() == '[' {
			pred, err := p.parsePredicate()
			if err != nil {
				return alt, err
			}
			alt.Predicates = append(alt.Predicates, pred)
		}

		if p.eof() {
			break
		}
		if p.peek() == '/' {
			p.advance(1)
			if !p.eof() && p.peek() == '/' {
				p.advance(1)
				descendant = true
			} else {
				descendant = false
			}
			continue
		}
		// Anything else terminates this alternate (likely '|' or end).
		break
	}
	// Predicates on attribute step.
	for !p.eof() && p.peek() == '[' {
		pred, err := p.parsePredicate()
		if err != nil {
			return alt, err
		}
		alt.Predicates = append(alt.Predicates, pred)
	}
	return alt, nil
}

func (p *selectorParser) parseName() (NameMatch, error) {
	var nm NameMatch
	if p.eof() {
		return nm, fmt.Errorf("its: expected name in selector %q at offset %d", p.src, p.pos)
	}
	if p.peek() == '*' {
		p.advance(1)
		nm.Local = "*"
		return nm, nil
	}
	first, err := p.readNCName()
	if err != nil {
		return nm, err
	}
	// Optional prefix.
	if !p.eof() && p.peek() == ':' {
		p.advance(1)
		// Allow `*` after prefix (e.g. `xs:*`).
		if p.peek() == '*' {
			p.advance(1)
			nm.Local = "*"
		} else {
			local, err := p.readNCName()
			if err != nil {
				return nm, err
			}
			nm.Local = local
		}
		uri, ok := p.nsMap[first]
		if !ok {
			return nm, fmt.Errorf("its: undeclared namespace prefix %q in selector %q", first, p.src)
		}
		nm.NamespaceURI = uri
	} else {
		nm.Local = first
	}
	return nm, nil
}

func (p *selectorParser) parsePredicate() (Predicate, error) {
	var pred Predicate
	if p.peek() != '[' {
		return pred, fmt.Errorf("its: expected `[` for predicate in %q at offset %d", p.src, p.pos)
	}
	p.advance(1)
	p.skipWS()
	if p.eof() {
		return pred, fmt.Errorf("its: unterminated predicate in %q", p.src)
	}
	if p.peek() == '@' {
		p.advance(1)
		name, err := p.parseName()
		if err != nil {
			return pred, err
		}
		pred.AttrName = name
		p.skipWS()
		if !p.eof() && p.peek() == '=' {
			p.advance(1)
			p.skipWS()
			val, err := p.parseStringLiteral()
			if err != nil {
				return pred, err
			}
			pred.Kind = PredAttrEquals
			pred.AttrValue = val
		} else {
			pred.Kind = PredAttrExists
		}
	} else if strings.HasPrefix(p.src[p.pos:], "ancestor::") {
		p.advance(len("ancestor::"))
		name, err := p.parseName()
		if err != nil {
			return pred, err
		}
		pred.Kind = PredAncestor
		pred.AncestorName = name
	} else {
		return pred, fmt.Errorf("its: unsupported predicate at offset %d in %q", p.pos, p.src)
	}
	p.skipWS()
	if p.eof() || p.peek() != ']' {
		return pred, fmt.Errorf("its: expected `]` to close predicate in %q at offset %d", p.src, p.pos)
	}
	p.advance(1)
	return pred, nil
}

func (p *selectorParser) parseStringLiteral() (string, error) {
	if p.eof() {
		return "", fmt.Errorf("its: expected string literal in %q", p.src)
	}
	q := p.peek()
	if q != '\'' && q != '"' {
		return "", fmt.Errorf("its: expected `'` or `\"` opening string literal at offset %d in %q", p.pos, p.src)
	}
	p.advance(1)
	end := strings.IndexByte(p.src[p.pos:], q)
	if end < 0 {
		return "", fmt.Errorf("its: unterminated string literal in %q", p.src)
	}
	val := p.src[p.pos : p.pos+end]
	p.advance(end + 1)
	return val, nil
}

// readNCName reads an XML NCName (no `:`). NCName ::= NameStartChar
// (NameChar)*; we approximate with letter-or-underscore start +
// letter/digit/-/_/. continuation, plus any non-ASCII letter.
func (p *selectorParser) readNCName() (string, error) {
	if p.eof() {
		return "", fmt.Errorf("its: expected name in %q", p.src)
	}
	start := p.pos
	r, size := utf8.DecodeRuneInString(p.src[p.pos:])
	if !isNameStart(r) {
		return "", fmt.Errorf("its: invalid name start %q at offset %d in %q", r, p.pos, p.src)
	}
	p.pos += size
	for !p.eof() {
		r, size := utf8.DecodeRuneInString(p.src[p.pos:])
		if !isNameChar(r) {
			break
		}
		p.pos += size
	}
	return p.src[start:p.pos], nil
}

func isNameStart(r rune) bool {
	if r == '_' || unicode.IsLetter(r) {
		return true
	}
	return false
}

func isNameChar(r rune) bool {
	if isNameStart(r) {
		return true
	}
	if r == '-' || r == '.' || unicode.IsDigit(r) {
		return true
	}
	return false
}

func (p *selectorParser) skipWS() {
	for !p.eof() {
		r, size := utf8.DecodeRuneInString(p.src[p.pos:])
		if !unicode.IsSpace(r) {
			break
		}
		p.pos += size
	}
}

func (p *selectorParser) eof() bool     { return p.pos >= len(p.src) }
func (p *selectorParser) peek() byte    { return p.src[p.pos] }
func (p *selectorParser) advance(n int) { p.pos += n }
