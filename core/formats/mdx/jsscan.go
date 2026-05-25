package mdx

// jsToken classifies a lexical event the JS scanner reports while walking
// ESM/expression content for bracket balancing.
type jsToken int

const (
	tokOther   jsToken = iota // ordinary byte(s) consumed
	tokOpen                   // one of ( { [
	tokClose                  // one of ) } ]
	tokNewline                // a physical line break at top scan level
	tokEOF                    // end of input
)

// jsScanner is a minimal JavaScript-flavoured lexer used only to balance
// brackets for ESM statements and `{ … }` expressions. It does NOT build
// an AST; it merely advances byte-by-byte while skipping the interiors of
// string literals, template literals, and comments so that brackets and
// newlines occurring inside them do not affect balancing or line
// termination. Regex literals are not specially handled — they are rare
// at the top level of MDX ESM/expression nodes and a stray `/` is treated
// as an ordinary byte, which is safe for balancing.
type jsScanner struct {
	body []byte
	pos  int
}

func newJSScanner(body []byte, start int) *jsScanner {
	return &jsScanner{body: body, pos: start}
}

// next advances the scanner past the next lexical unit and returns its
// token class. The scanner consumes string/template/comment interiors
// internally so callers only ever see structural tokens.
func (s *jsScanner) next() jsToken {
	if s.pos >= len(s.body) {
		return tokEOF
	}
	c := s.body[s.pos]
	switch c {
	case '\n':
		s.pos++
		return tokNewline
	case '(', '{', '[':
		s.pos++
		return tokOpen
	case ')', '}', ']':
		s.pos++
		return tokClose
	case '"', '\'':
		s.skipString(c)
		return tokOther
	case '`':
		s.skipTemplate()
		return tokOther
	case '/':
		if s.pos+1 < len(s.body) {
			switch s.body[s.pos+1] {
			case '/':
				s.skipLineComment()
				return tokOther
			case '*':
				s.skipBlockComment()
				return tokOther
			}
		}
		s.pos++
		return tokOther
	default:
		s.pos++
		return tokOther
	}
}

// skipString consumes a single- or double-quoted string literal,
// including the closing quote, honouring backslash escapes. Stops at EOF
// if the string is unterminated.
func (s *jsScanner) skipString(quote byte) {
	s.pos++ // opening quote
	for s.pos < len(s.body) {
		c := s.body[s.pos]
		if c == '\\' {
			s.pos += 2
			continue
		}
		s.pos++
		if c == quote {
			return
		}
	}
}

// skipTemplate consumes a template literal. Template interpolations
// (`${ … }`) are skipped wholesale via balanced-brace counting so braces
// inside them don't leak into the outer balance. Honours backslash
// escapes.
func (s *jsScanner) skipTemplate() {
	s.pos++ // opening backtick
	for s.pos < len(s.body) {
		c := s.body[s.pos]
		switch {
		case c == '\\':
			s.pos += 2
		case c == '`':
			s.pos++
			return
		case c == '$' && s.pos+1 < len(s.body) && s.body[s.pos+1] == '{':
			s.pos += 2
			s.skipBraces()
		default:
			s.pos++
		}
	}
}

// skipBraces consumes a balanced `{ … }` interior (the opening `{` has
// already been consumed by the caller), recursing through nested strings,
// templates, comments, and braces.
func (s *jsScanner) skipBraces() {
	depth := 1
	for s.pos < len(s.body) && depth > 0 {
		c := s.body[s.pos]
		switch c {
		case '{':
			depth++
			s.pos++
		case '}':
			depth--
			s.pos++
		case '"', '\'':
			s.skipString(c)
		case '`':
			s.skipTemplate()
		case '/':
			if s.pos+1 < len(s.body) && s.body[s.pos+1] == '/' {
				s.skipLineComment()
			} else if s.pos+1 < len(s.body) && s.body[s.pos+1] == '*' {
				s.skipBlockComment()
			} else {
				s.pos++
			}
		default:
			s.pos++
		}
	}
}

// skipLineComment consumes a `// …` comment up to (not including) the LF.
func (s *jsScanner) skipLineComment() {
	for s.pos < len(s.body) && s.body[s.pos] != '\n' {
		s.pos++
	}
}

// skipBlockComment consumes a `/* … */` comment including the closing
// delimiter, or to EOF if unterminated.
func (s *jsScanner) skipBlockComment() {
	s.pos += 2 // opening /*
	for s.pos < len(s.body) {
		if s.body[s.pos] == '*' && s.pos+1 < len(s.body) && s.body[s.pos+1] == '/' {
			s.pos += 2
			return
		}
		s.pos++
	}
}
