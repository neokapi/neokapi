package icu

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"
)

// NodeType classifies parsed MessageFormat nodes.
type NodeType int

const (
	NodeText          NodeType = iota // Plain text
	NodeArg                           // Simple argument: {name} or {name, type} or {name, type, style}
	NodePlural                        // {name, plural, ...}
	NodeSelect                        // {name, select, ...}
	NodeSelectOrdinal                 // {name, selectordinal, ...}
	NodeHash                          // # (number placeholder in plural/selectordinal)
)

// Picker reports whether the node type is one that chooses between
// sub-messages: plural, select or selectordinal.
func (t NodeType) Picker() bool {
	return t == NodePlural || t == NodeSelect || t == NodeSelectOrdinal
}

// Node represents a parsed element in a MessageFormat pattern.
type Node struct {
	Type     NodeType
	Text     string   // For NodeText: the text content; for NodeArg: the full argument expression
	ArgName  string   // The argument name (e.g., "count", "0", "name")
	ArgType  string   // "plural", "select", "selectordinal", or format type
	ArgStyle string   // Style for simple arguments (e.g., "short", "integer")
	Offset   string   // Offset value for plural (e.g., "1")
	Branches []Branch // For plural/select: the branches
}

// Branch represents one branch in a plural/select pattern.
type Branch struct {
	Keyword string // e.g., "one", "other", "=0", "male", "female"
	Body    []Node // The parsed content of this branch
}

// Parse parses an ICU MessageFormat pattern string into a list of nodes.
func Parse(pattern string) ([]Node, error) {
	p := &parser{input: pattern}
	return p.parsePattern(0, false)
}

// HasPicker reports whether the node list contains a plural, select or
// selectordinal anywhere, including inside a branch body. A message with a
// picker carries sub-messages a translator must rewrite; a message without one
// is a frame of text around placeholders.
func HasPicker(nodes []Node) bool {
	for _, n := range nodes {
		if n.Type.Picker() {
			return true
		}
		for _, br := range n.Branches {
			if HasPicker(br.Body) {
				return true
			}
		}
	}
	return false
}

type parser struct {
	input string
	pos   int
}

func (p *parser) parsePattern(depth int, inBranch bool) ([]Node, error) {
	var nodes []Node
	var textBuf strings.Builder

	flushText := func() {
		if textBuf.Len() > 0 {
			nodes = append(nodes, Node{Type: NodeText, Text: textBuf.String()})
			textBuf.Reset()
		}
	}

	for p.pos < len(p.input) {
		r, size := utf8.DecodeRuneInString(p.input[p.pos:])

		switch r {
		case '\'':
			start := p.pos
			SkipQuoted(p.input, &p.pos)
			textBuf.WriteString(Unquote(p.input[start:p.pos]))

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
				nodes = append(nodes, Node{Type: NodeHash, Text: "#"})
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

func (p *parser) parseArgument(depth int) (Node, error) {
	// Read argument name
	argName := p.readUntil(',', '}')
	argName = strings.TrimSpace(argName)

	if p.pos >= len(p.input) {
		return Node{}, fmt.Errorf("unterminated argument at position %d", p.pos)
	}

	r, size := utf8.DecodeRuneInString(p.input[p.pos:])

	if r == '}' {
		// Simple argument: {name}
		p.pos += size
		return Node{
			Type:    NodeArg,
			ArgName: argName,
			Text:    "{" + argName + "}",
		}, nil
	}

	// r == ','
	p.pos += size

	// Read argument type
	argType := p.readUntil(',', '}')
	argType = strings.TrimSpace(argType)

	if p.pos >= len(p.input) {
		return Node{}, fmt.Errorf("unterminated argument at position %d", p.pos)
	}

	r, size = utf8.DecodeRuneInString(p.input[p.pos:])

	switch strings.ToLower(argType) {
	case "plural", "select", "selectordinal":
		if r != ',' {
			return Node{}, fmt.Errorf("expected ',' after %s type at position %d", argType, p.pos)
		}
		p.pos += size
		return p.parsePluralOrSelect(argName, argType, depth)

	case "choice":
		return Node{}, errors.New("choice format is deprecated and is not supported")

	default:
		// Simple typed argument: {name, type} or {name, type, style}
		if r == '}' {
			p.pos += size
			return Node{
				Type:    NodeArg,
				ArgName: argName,
				ArgType: argType,
				Text:    "{" + argName + ", " + argType + "}",
			}, nil
		}
		// Has style: {name, type, style}
		p.pos += size // skip ','
		style := p.readArgStyle()
		return Node{
			Type:     NodeArg,
			ArgName:  argName,
			ArgType:  argType,
			ArgStyle: style,
			Text:     "{" + argName + ", " + argType + ", " + style + "}",
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

func (p *parser) parsePluralOrSelect(argName, argType string, depth int) (Node, error) {
	n := Node{
		ArgName: argName,
		ArgType: argType,
	}
	switch strings.ToLower(argType) {
	case "plural":
		n.Type = NodePlural
	case "select":
		n.Type = NodeSelect
	case "selectordinal":
		n.Type = NodeSelectOrdinal
	}

	// Skip whitespace
	p.skipWhitespace()

	// Check for offset (plural only)
	if n.Type == NodePlural || n.Type == NodeSelectOrdinal {
		if strings.HasPrefix(p.input[p.pos:], "offset:") {
			p.pos += len("offset:")
			offsetVal := p.readUntil(' ', '\t', '\n', '\r')
			n.Offset = strings.TrimSpace(offsetVal)
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
			return Node{}, fmt.Errorf("expected keyword in %s at position %d", argType, p.pos)
		}

		p.skipWhitespace()

		if p.pos >= len(p.input) {
			return Node{}, fmt.Errorf("unterminated %s branch at position %d", argType, p.pos)
		}

		r, size := utf8.DecodeRuneInString(p.input[p.pos:])
		if r != '{' {
			return Node{}, fmt.Errorf("expected '{' for branch body, got '%c' at position %d", r, p.pos)
		}
		p.pos += size

		body, err := p.parsePattern(depth+1, true)
		if err != nil {
			return Node{}, fmt.Errorf("in %s branch '%s': %w", argType, keyword, err)
		}

		n.Branches = append(n.Branches, Branch{
			Keyword: keyword,
			Body:    body,
		})
	}

	return Node{}, fmt.Errorf("unterminated %s expression", argType)
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
