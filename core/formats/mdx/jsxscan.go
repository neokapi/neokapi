package mdx

import "github.com/neokapi/neokapi/core/translatability"

// jsxTagToken classifies the next structural JSX event for tag-depth
// balancing in scanJSX.
type jsxTagToken int

const (
	jsxOther         jsxTagToken = iota // consumed text / expression / whitespace
	jsxStartTag                         // <Tag …>  or  <Tag … />
	jsxEndTag                           // </Tag>
	jsxFragmentOpen                     // <>
	jsxFragmentClose                    // </>
	jsxEOF
)

// attrValue is the byte range of one attribute's quoted value, relative to the
// start of the tag it was found in.
type attrValue struct{ start, end int }

// jsxTranslatableAttrValues returns the quoted values in tag whose attribute
// names carry user-visible text on this element, in source order.
//
// Only literal quoted values. An `{expression}` value is code: its bytes are
// not a string a translator can be handed, and rewriting them would change the
// program.
func jsxTranslatableAttrValues(tag []byte, element string) []attrValue {
	var out []attrValue
	i := 0
	// Skip `<` and the element name.
	for i < len(tag) && tag[i] != ' ' && tag[i] != '\t' && tag[i] != '\n' && tag[i] != '>' {
		i++
	}
	for i < len(tag) {
		for i < len(tag) && (tag[i] == ' ' || tag[i] == '\t' || tag[i] == '\n' || tag[i] == '\r') {
			i++
		}
		if i >= len(tag) || tag[i] == '>' || tag[i] == '/' {
			break
		}
		nameStart := i
		for i < len(tag) && (isTagNameByte(tag[i]) || tag[i] == '-') {
			i++
		}
		name := string(tag[nameStart:i])
		if name == "" {
			i++
			continue
		}
		for i < len(tag) && (tag[i] == ' ' || tag[i] == '\t') {
			i++
		}
		if i >= len(tag) || tag[i] != '=' {
			continue // a bare attribute carries no value
		}
		i++
		for i < len(tag) && (tag[i] == ' ' || tag[i] == '\t') {
			i++
		}
		if i >= len(tag) {
			break
		}
		switch q := tag[i]; q {
		case '"', '\'':
			i++
			valStart := i
			for i < len(tag) && tag[i] != q {
				i++
			}
			if i >= len(tag) {
				return out
			}
			if valStart < i && translatability.IsTranslatableAttribute(name, element) {
				out = append(out, attrValue{start: valStart, end: i})
			}
			i++
		case '{':
			js := &jsScanner{body: tag, pos: i + 1}
			js.skipBraces()
			if js.pos <= i {
				return out
			}
			i = js.pos
		default:
			for i < len(tag) && tag[i] != ' ' && tag[i] != '>' {
				i++
			}
		}
	}
	return out
}

// isTagNameByte reports whether b can appear in an element or attribute name.
func isTagNameByte(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') ||
		(b >= '0' && b <= '9') || b == '_' || b == '.' || b == '$' || b == ':'
}

// jsxTagNameAt returns the element name of the tag starting at pos (which must
// be a '<'), or "" for a fragment or a non-tag. The scanner reports tag events
// for depth balancing and does not need the name; translatability does,
// because the W3C table answers per element.
//
// The name keeps its source spelling. Classification lowercases it, but a
// diagnostic has to name the element the author actually wrote: <TabItem>
// lowercased is a component nobody can find.
func jsxTagNameAt(body []byte, pos int) string {
	i := pos + 1
	if i < len(body) && body[i] == '/' {
		i++
	}
	start := i
	for i < len(body) {
		c := body[i]
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') ||
			c == '-' || c == '_' || c == '.' || c == '$' {
			i++
			continue
		}
		break
	}
	if i == start {
		return ""
	}
	return string(body[start:i])
}

// jsxScanner walks block-level JSX, reporting tag-open/close events so the
// caller can balance element nesting. Inside the scan it skips:
//
//   - `{ … }` expression containers (attribute values and children),
//     using the JS scanner's balanced-brace logic so braces and angle
//     brackets nested in JS don't desync the tag depth;
//   - quoted attribute-value strings;
//   - JS-style comments;
//   - Markdown code in the children — fenced blocks and inline code spans
//     (see fence.go), whose angle brackets are code, not tags;
//   - JSX text between tags.
//
// It is deliberately permissive: it recognises tag *shape* (`<name …>`,
// `</name>`, `<… />`, `<>`, `</>`) rather than validating JSX grammar,
// which is enough to find where a top-level element/fragment closes.
type jsxScanner struct {
	body []byte
	pos  int
}

func newJSXScanner(body []byte, start int) *jsxScanner {
	return &jsxScanner{body: body, pos: start}
}

// nextTag advances to and consumes the next JSX tag, returning its class
// and (for start tags) whether it was self-closing. Non-tag bytes between
// tags are consumed and reported as jsxOther so the caller can detect
// stray content. Expression containers `{ … }` and the children's Markdown
// code — fenced blocks and inline code spans — are skipped wholesale.
func (s *jsxScanner) nextTag() (jsxTagToken, bool) {
	for s.pos < len(s.body) {
		// Markdown children may hold fenced code, and a fence is opaque to
		// tag depth: `<binary> version` in a shell transcript is not a tag
		// that leaves the element unbalanced.
		if end, ok := fenceRegionAt(s.body, s.pos); ok {
			s.pos = end
			continue
		}
		c := s.body[s.pos]
		switch c {
		case '<':
			return s.consumeTag()
		case '`':
			// An inline code span in the children is opaque for the same
			// reason a fence is.
			if end, ok := codeSpanEnd(s.body, s.pos); ok {
				s.pos = end
				continue
			}
			s.pos++
		case '{':
			// Skip a JSX expression container as opaque content.
			s.pos++
			js := &jsScanner{body: s.body, pos: s.pos}
			js.skipBraces()
			s.pos = js.pos
			return jsxOther, false
		default:
			s.pos++
			// Report a benign "other" only at a structural boundary to let
			// the caller observe progress; here we just keep consuming.
		}
	}
	return jsxEOF, false
}

// consumeTag consumes a tag beginning at the current `<`. It classifies
// fragments (`<>`, `</>`), end tags (`</name>`), and start/self-closing
// tags (`<name …>`, `<name … />`), skipping quoted attribute values and
// `{ … }` attribute expressions so `>` characters inside them don't end
// the tag prematurely.
func (s *jsxScanner) consumeTag() (jsxTagToken, bool) {
	// s.body[s.pos] == '<'
	if s.pos+1 >= len(s.body) {
		s.pos++
		return jsxOther, false
	}
	next := s.body[s.pos+1]

	// Fragments.
	if next == '>' {
		s.pos += 2
		return jsxFragmentOpen, false
	}
	if next == '/' {
		// `</>` fragment close, or `</name>` end tag.
		i := s.pos + 2
		for i < len(s.body) && (s.body[i] == ' ' || s.body[i] == '\t') {
			i++
		}
		if i < len(s.body) && s.body[i] == '>' {
			s.pos = i + 1
			return jsxFragmentClose, false
		}
		// End tag: consume to the closing '>'.
		s.scanToTagEnd(s.pos + 2)
		return jsxEndTag, false
	}

	// A start tag must begin with a letter, `_`, or `$` (component or HTML
	// element). Anything else (e.g. `< 3` in prose) is not a JSX tag —
	// consume the `<` as ordinary content.
	if !isTagNameStart(next) {
		s.pos++
		return jsxOther, false
	}
	selfClosing := s.scanToTagEnd(s.pos + 1)
	return jsxStartTag, selfClosing
}

// scanToTagEnd advances from p (just past `<` or `</`) to the byte after
// the tag's closing `>`, skipping quoted strings and `{ … }` expression
// values. Returns true when the tag self-closes (`… />`). Updates s.pos.
func (s *jsxScanner) scanToTagEnd(p int) bool {
	prevSlash := false
	for p < len(s.body) {
		c := s.body[p]
		switch c {
		case '>':
			s.pos = p + 1
			return prevSlash
		case '"', '\'':
			js := &jsScanner{body: s.body, pos: p}
			js.skipString(c)
			p = js.pos
			prevSlash = false
			continue
		case '{':
			js := &jsScanner{body: s.body, pos: p + 1}
			js.skipBraces()
			p = js.pos
			prevSlash = false
			continue
		case '/':
			prevSlash = true
			p++
			continue
		case ' ', '\t', '\n', '\r':
			p++
			continue
		default:
			prevSlash = false
			p++
		}
	}
	s.pos = p
	return false
}

// isTagNameStart reports whether c may begin a JSX tag name.
func isTagNameStart(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || c == '_' || c == '$'
}

// isJSXStart reports whether position p (a `<`) plausibly begins a
// block-level JSX element or fragment: `<>`, `</…`, or `<name…`. A `<`
// followed by anything else (a space, a digit, `=`, …) is treated as
// ordinary Markdown text rather than JSX, matching how MDX rejects
// `< 3` etc. as non-JSX.
func isJSXStart(body []byte, p int) bool {
	if p+1 >= len(body) {
		return false
	}
	next := body[p+1]
	if next == '>' || next == '/' {
		return true
	}
	return isTagNameStart(next)
}
