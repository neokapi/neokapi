package mdx

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

// jsxScanner walks block-level JSX, reporting tag-open/close events so the
// caller can balance element nesting. Inside the scan it skips:
//
//   - `{ … }` expression containers (attribute values and children),
//     using the JS scanner's balanced-brace logic so braces and angle
//     brackets nested in JS don't desync the tag depth;
//   - quoted attribute-value strings;
//   - JS-style comments;
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
// stray content. Expression containers `{ … }` are skipped wholesale.
func (s *jsxScanner) nextTag() (jsxTagToken, bool) {
	for s.pos < len(s.body) {
		c := s.body[s.pos]
		switch c {
		case '<':
			return s.consumeTag()
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
