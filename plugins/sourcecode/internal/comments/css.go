package comments

import (
	"bytes"
	"regexp"
	"strings"
	"sync"

	ts "github.com/tree-sitter/go-tree-sitter"
	tscss "github.com/tree-sitter/tree-sitter-css/bindings/go"

	"github.com/neokapi/neokapi/core/comment"
)

// cssLanguages is CSS. Its only comment is `/* */`, and a `/*` inside a string
// or a URL is content.
func cssLanguages() []*Language {
	return []*Language{{
		Name:        "css",
		DisplayName: "CSS",
		Extensions:  []string{".css"},
		Markers:     comment.Markers{Block: []comment.BlockMarker{{Open: "/*", Close: "*/"}}},
		grammar:     tscss.Language,
		syntax:      cssSyntax,
		once:        &sync.Once{},
		Canary: comment.Canary{
			Name:   "a CSS comment with a doubled word, below a stylelint directive and above a string holding a comment marker",
			Source: []byte("/* stylelint-disable no-empty-source */\n/* Styles the the header. */\n.header { content: \"/* not a comment */\"; }\n"),
			Block:  "rule/.header",
		},
	}}
}

var cssSyntax = &syntax{
	unit:       cssUnit,
	directives: cssDirectives,
	decl:       cssDecl,
	container:  cssContainer,
}

func cssUnit(n *ts.Node, src []byte) (unit, bool) {
	if n.Kind() != "comment" {
		return unit{}, false
	}
	start, end := int(n.StartByte()), int(n.EndByte())
	if !bytes.HasPrefix(src[start:end], []byte("/*")) || inUnquotedURL(n, src) {
		return unit{}, false
	}
	// A `/** */` block opens with three bytes of marker, so its text starts
	// after the second asterisk. CSS gives it no meaning beyond a comment.
	open := 2
	if bytes.HasPrefix(src[start:end], []byte("/**")) && end-start >= 5 {
		open = 3
	}
	return unit{start: start, end: end, kind: kindBlock, open: open, close: 2}, true
}

// cssDirectives are the comments a stylesheet tool reads.
var cssDirectives = []directiveForm{
	prefixForm("stylelint", "stylelint-disable", "stylelint-enable"),
	prefixForm("prettier", "prettier-ignore"),
	regexpForm("source-map", "sourceMappingURL", nil, regexp.MustCompile(`^[#@]\s?source(Mapping)?URL=`)),
}

// cssContainer reports the nodes whose direct children are structural: the
// stylesheet, a conditional group rule such as @media, and its block.
func cssContainer(kind, parent string) bool {
	switch kind {
	case "stylesheet", "media_statement", "supports_statement", "at_rule":
		return true
	case "block":
		return parent == "media_statement" || parent == "supports_statement" || parent == "at_rule"
	}
	return false
}

// cssDecl names a rule: `rule/.header` for a rule set, named for its selectors,
// and `media`, `supports` or `keyframes/spin` for an at-rule.
func cssDecl(n *ts.Node, _ string, src []byte) (string, bool) {
	switch n.Kind() {
	case "rule_set":
		for i := uint(0); i < n.NamedChildCount(); i++ {
			if c := n.NamedChild(i); c.Kind() == "selectors" {
				return "rule/" + selectorName(c.Utf8Text(src)), true
			}
		}
		return "rule", true
	case "media_statement":
		return "media", true
	case "supports_statement":
		return "supports", true
	case "keyframes_statement":
		for i := uint(0); i < n.NamedChildCount(); i++ {
			if c := n.NamedChild(i); c.Kind() == "keyframes_name" {
				return "keyframes/" + nodeName(c, src), true
			}
		}
		return "keyframes", true
	}
	return "", false
}

// selectorName is a selector list as one path segment: whitespace collapsed,
// and a slash replaced so it cannot split the path.
func selectorName(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	return strings.ReplaceAll(s, "/", ".")
}

// inUnquotedURL reports a `/* */` the grammar finds in the argument of an
// unquoted url(), which CSS reads as part of the URL. The grammar reads url()
// as any function call.
func inUnquotedURL(n *ts.Node, src []byte) bool {
	args := n.Parent()
	if args == nil || args.Kind() != "arguments" {
		return false
	}
	call := args.Parent()
	if call == nil || call.Kind() != "call_expression" {
		return false
	}
	name := call.NamedChild(0)
	return name != nil && name.Kind() == "function_name" && strings.EqualFold(name.Utf8Text(src), "url")
}
