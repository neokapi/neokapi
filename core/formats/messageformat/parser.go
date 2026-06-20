package messageformat

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"
)

// nodeType classifies parsed MessageFormat nodes.
type nodeType int

const (
	nodeText      nodeType = iota // Plain text
	nodeArg                       // Simple argument: {name} or {name, type} or {name, type, style}
	nodePlural                    // {name, plural, ...}
	nodeSelect                    // {name, select, ...}
	nodeSelectOrd                 // {name, selectordinal, ...}
	nodeHash                      // # (number placeholder in plural/selectordinal)
)

// node represents a parsed element in a MessageFormat pattern.
type node struct {
	typ      nodeType
	text     string   // For nodeText: the text content; for nodeArg: the full argument expression
	argName  string   // The argument name (e.g., "count", "0", "name")
	argType  string   // "plural", "select", "selectordinal", or format type
	argStyle string   // Style for simple arguments (e.g., "short", "integer")
	offset   string   // Offset value for plural (e.g., "1")
	branches []branch // For plural/select: the branches
}

// branch represents one branch in a plural/select pattern.
type branch struct {
	keyword string // e.g., "one", "other", "=0", "male", "female"
	body    []node // The parsed content of this branch
}

// errPrefix is the error prefix used to match the Okapi bridge's error messages.
// The capitalization matches Okapi Framework's convention.
const errPrefix = "Error reading Message Format String"

// parse parses an ICU MessageFormat pattern string into a list of nodes.
func parse(pattern string) ([]node, error) {
	p := &parser{input: pattern}
	nodes, err := p.parsePattern(0, false)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", errPrefix, err)
	}
	return nodes, nil
}

type parser struct {
	input string
	pos   int
}

func (p *parser) parsePattern(depth int, inBranch bool) ([]node, error) {
	var nodes []node
	var textBuf strings.Builder

	flushText := func() {
		if textBuf.Len() > 0 {
			nodes = append(nodes, node{typ: nodeText, text: textBuf.String()})
			textBuf.Reset()
		}
	}

	for p.pos < len(p.input) {
		r, size := utf8.DecodeRuneInString(p.input[p.pos:])

		switch r {
		case '\'':
			// ICU quote handling
			p.pos += size
			if p.pos >= len(p.input) {
				// Trailing single quote: literal apostrophe
				textBuf.WriteRune('\'')
				continue
			}
			next, nextSize := utf8.DecodeRuneInString(p.input[p.pos:])
			if next == '\'' {
				// '' → literal single quote
				textBuf.WriteRune('\'')
				p.pos += nextSize
			} else if next == '{' || next == '}' || next == '#' {
				// '{', '}', '#' → literal char, read until closing quote
				textBuf.WriteRune(next)
				p.pos += nextSize
				// Read until closing single quote or end
				for p.pos < len(p.input) {
					r2, s2 := utf8.DecodeRuneInString(p.input[p.pos:])
					if r2 == '\'' {
						p.pos += s2
						// Check for '' inside quoted string
						if p.pos < len(p.input) {
							r3, s3 := utf8.DecodeRuneInString(p.input[p.pos:])
							if r3 == '\'' {
								textBuf.WriteRune('\'')
								p.pos += s3
								continue
							}
						}
						break
					}
					textBuf.WriteRune(r2)
					p.pos += s2
				}
			} else {
				// 'text' → quoted literal text (until closing quote)
				for p.pos < len(p.input) {
					r2, s2 := utf8.DecodeRuneInString(p.input[p.pos:])
					if r2 == '\'' {
						p.pos += s2
						// Check for '' inside quoted string
						if p.pos < len(p.input) {
							r3, s3 := utf8.DecodeRuneInString(p.input[p.pos:])
							if r3 == '\'' {
								textBuf.WriteRune('\'')
								p.pos += s3
								continue
							}
						}
						break
					}
					textBuf.WriteRune(r2)
					p.pos += s2
				}
			}

		case '{':
			flushText()
			p.pos += size
			argNode, err := p.parseArgument(depth)
			if err != nil {
				return nil, err
			}
			nodes = append(nodes, argNode)

		case '}':
			if inBranch {
				flushText()
				p.pos += size // consume the closing brace
				return nodes, nil
			}
			// Stray closing brace at top level
			return nil, fmt.Errorf("unexpected '}' at position %d", p.pos)

		case '#':
			if depth > 0 {
				// # is a number reference in plural/selectordinal context
				flushText()
				nodes = append(nodes, node{typ: nodeHash, text: "#"})
				p.pos += size
			} else {
				textBuf.WriteRune(r)
				p.pos += size
			}

		default:
			textBuf.WriteRune(r)
			p.pos += size
		}
	}

	flushText()
	return nodes, nil
}

func (p *parser) parseArgument(depth int) (node, error) {
	// Read argument name
	argName := p.readUntil(',', '}')
	argName = strings.TrimSpace(argName)

	if p.pos >= len(p.input) {
		return node{}, fmt.Errorf("unterminated argument at position %d", p.pos)
	}

	r, size := utf8.DecodeRuneInString(p.input[p.pos:])

	if r == '}' {
		// Simple argument: {name}
		p.pos += size
		return node{
			typ:     nodeArg,
			argName: argName,
			text:    "{" + argName + "}",
		}, nil
	}

	// r == ','
	p.pos += size

	// Read argument type
	argType := p.readUntil(',', '}')
	argType = strings.TrimSpace(argType)

	if p.pos >= len(p.input) {
		return node{}, fmt.Errorf("unterminated argument at position %d", p.pos)
	}

	r, size = utf8.DecodeRuneInString(p.input[p.pos:])

	switch strings.ToLower(argType) {
	case "plural", "select", "selectordinal":
		if r != ',' {
			return node{}, fmt.Errorf("expected ',' after %s type at position %d", argType, p.pos)
		}
		p.pos += size
		return p.parsePluralOrSelect(argName, argType, depth)

	case "choice":
		return node{}, errors.New("choice format is deprecated and is not supported")

	default:
		// Simple typed argument: {name, type} or {name, type, style}
		if r == '}' {
			p.pos += size
			return node{
				typ:     nodeArg,
				argName: argName,
				argType: argType,
				text:    "{" + argName + ", " + argType + "}",
			}, nil
		}
		// Has style: {name, type, style}
		p.pos += size // skip ','
		style := p.readArgStyle()
		return node{
			typ:      nodeArg,
			argName:  argName,
			argType:  argType,
			argStyle: style,
			text:     "{" + argName + ", " + argType + ", " + style + "}",
		}, nil
	}
}

// readArgStyle reads the style portion of an argument, handling nested braces.
func (p *parser) readArgStyle() string {
	var buf strings.Builder
	braceDepth := 0

	for p.pos < len(p.input) {
		r, size := utf8.DecodeRuneInString(p.input[p.pos:])
		if r == '{' {
			braceDepth++
			buf.WriteRune(r)
			p.pos += size
		} else if r == '}' {
			if braceDepth == 0 {
				p.pos += size
				return strings.TrimSpace(buf.String())
			}
			braceDepth--
			buf.WriteRune(r)
			p.pos += size
		} else {
			buf.WriteRune(r)
			p.pos += size
		}
	}
	return strings.TrimSpace(buf.String())
}

func (p *parser) parsePluralOrSelect(argName, argType string, depth int) (node, error) {
	n := node{
		argName: argName,
		argType: argType,
	}
	switch strings.ToLower(argType) {
	case "plural":
		n.typ = nodePlural
	case "select":
		n.typ = nodeSelect
	case "selectordinal":
		n.typ = nodeSelectOrd
	}

	// Skip whitespace
	p.skipWhitespace()

	// Check for offset (plural only)
	if n.typ == nodePlural || n.typ == nodeSelectOrd {
		if strings.HasPrefix(p.input[p.pos:], "offset:") {
			p.pos += len("offset:")
			offsetVal := p.readUntil(' ', '\t', '\n', '\r')
			n.offset = strings.TrimSpace(offsetVal)
			p.skipWhitespace()
		}
	}

	// Parse branches: keyword {body} keyword {body} ...
	for p.pos < len(p.input) {
		p.skipWhitespace()

		r, _ := utf8.DecodeRuneInString(p.input[p.pos:])
		if r == '}' {
			p.pos++ // consume closing brace of the plural/select
			return n, nil
		}

		// Read keyword (e.g., "one", "other", "=0", "male")
		keyword := p.readUntil('{', '}', ' ', '\t')
		keyword = strings.TrimSpace(keyword)
		if keyword == "" {
			return node{}, fmt.Errorf("expected keyword in %s at position %d", argType, p.pos)
		}

		p.skipWhitespace()

		if p.pos >= len(p.input) {
			return node{}, fmt.Errorf("unterminated %s branch at position %d", argType, p.pos)
		}

		r, size := utf8.DecodeRuneInString(p.input[p.pos:])
		if r != '{' {
			return node{}, fmt.Errorf("expected '{' for branch body, got '%c' at position %d", r, p.pos)
		}
		p.pos += size

		body, err := p.parsePattern(depth+1, true)
		if err != nil {
			return node{}, fmt.Errorf("in %s branch '%s': %w", argType, keyword, err)
		}

		n.branches = append(n.branches, branch{
			keyword: keyword,
			body:    body,
		})
	}

	return node{}, fmt.Errorf("unterminated %s expression", argType)
}

func (p *parser) readUntil(stops ...rune) string {
	var buf strings.Builder
	for p.pos < len(p.input) {
		r, size := utf8.DecodeRuneInString(p.input[p.pos:])
		if slices.Contains(stops, r) {
			return buf.String()
		}
		buf.WriteRune(r)
		p.pos += size
	}
	return buf.String()
}

func (p *parser) skipWhitespace() {
	for p.pos < len(p.input) {
		r, size := utf8.DecodeRuneInString(p.input[p.pos:])
		if r != ' ' && r != '\t' && r != '\n' && r != '\r' {
			return
		}
		p.pos += size
	}
}

// segment represents a translatable segment extracted from the pattern.
type segment struct {
	path string // Dot-delimited path (e.g., "count.one", "gender.male.count.other")
	text string // The plain text content of this segment
	hash bool   // Whether the segment contains # references
}

// extractSegments recursively extracts translatable segments from parsed nodes.
func extractSegments(nodes []node, pathPrefix string) []segment {
	var segments []segment

	// Check if this node list contains any plural/select patterns
	hasBranching := false
	for _, n := range nodes {
		if n.typ == nodePlural || n.typ == nodeSelect || n.typ == nodeSelectOrd {
			hasBranching = true
			break
		}
	}

	if !hasBranching {
		// This is a leaf: extract as a single translatable segment
		text, hasHash := nodesToText(nodes)
		text = strings.TrimSpace(text)
		if text != "" {
			segments = append(segments, segment{
				path: pathPrefix,
				text: text,
				hash: hasHash,
			})
		}
		return segments
	}

	// We have branching: process mixed content
	// Collect text before/after/between branching nodes and recurse into branches
	for _, n := range nodes {
		switch n.typ {
		case nodePlural, nodeSelect, nodeSelectOrd:
			for _, br := range n.branches {
				branchPath := pathPrefix
				if branchPath != "" {
					branchPath += "."
				}
				branchPath += n.argName + "." + br.keyword
				subSegments := extractSegments(br.body, branchPath)
				segments = append(segments, subSegments...)
			}
		}
	}

	return segments
}

// extractLiteralSiblings returns the literal text nodes that sit beside a
// plural/select node — the sentence frame around a branch (e.g. "You have " and
// " in your cart." in "You have {count, plural, …} in your cart."). These are
// dropped by extractSegments (which only recurses into branches), so they are
// surfaced separately as non-translatable content. Whitespace-only siblings are
// skipped: they carry no prose and are preserved verbatim in the skeleton. The
// walk descends into branch bodies so framing text inside nested pickers (e.g.
// "He has " in a branch that wraps another plural) is also collected.
func extractLiteralSiblings(nodes []node) []string {
	hasBranching := false
	for _, n := range nodes {
		if n.typ == nodePlural || n.typ == nodeSelect || n.typ == nodeSelectOrd {
			hasBranching = true
			break
		}
	}
	if !hasBranching {
		// Leaf node list: its text is captured as a translatable segment by
		// extractSegments, not as a framing sibling.
		return nil
	}

	var out []string
	for _, n := range nodes {
		switch n.typ {
		case nodeText:
			if strings.TrimSpace(n.text) != "" {
				out = append(out, n.text)
			}
		case nodePlural, nodeSelect, nodeSelectOrd:
			for _, br := range n.branches {
				out = append(out, extractLiteralSiblings(br.body)...)
			}
		}
	}
	return out
}

// nodesToText converts a list of nodes to plain text for extraction.
// Returns the text and whether any # references were found.
func nodesToText(nodes []node) (string, bool) {
	var buf strings.Builder
	hasHash := false
	for _, n := range nodes {
		switch n.typ {
		case nodeText:
			buf.WriteString(n.text)
		case nodeHash:
			buf.WriteString("#")
			hasHash = true
		case nodeArg:
			// Placeholder - will be represented as inline span
			buf.WriteString(n.text)
		}
	}
	return buf.String(), hasHash
}

// nodesHavePlaceholders checks if a node list contains argument references.
func nodesHavePlaceholders(nodes []node) bool {
	for _, n := range nodes {
		if n.typ == nodeArg || n.typ == nodeHash {
			return true
		}
	}
	return false
}
