package comments

import (
	"bytes"
	"regexp"
	"sync"

	ts "github.com/tree-sitter/go-tree-sitter"
	tsjava "github.com/tree-sitter/tree-sitter-java/bindings/go"

	"github.com/neokapi/neokapi/core/comment"
)

// javaLanguages is Java. A `/** */` block directly before a declaration is its
// Javadoc, whose block tags, inline tags and HTML are placeholders.
func javaLanguages() []*Language {
	return []*Language{{
		Name:        "java",
		DisplayName: "Java",
		Extensions:  []string{".java"},
		Markers:     comment.Markers{Line: []string{"//"}, Block: []comment.BlockMarker{{Open: "/*", Close: "*/"}}},
		grammar:     tsjava.Language,
		syntax:      javaSyntax,
		once:        &sync.Once{},
		Canary: comment.Canary{
			Name:   "a Javadoc comment with a doubled word, below an inspection suppression and above a string holding a comment marker",
			Source: []byte("//noinspection unused\n/** Parses the the input. */\npublic class Parser {\n    String marker = \"// not a comment\";\n}\n"),
			Block:  "class/Parser",
		},
	}}
}

var javaSyntax = &syntax{
	unit:       javaUnit,
	directives: javaDirectives,
	decl:       javaDecl,
	container:  javaContainer,
	docTags:    true,
	markup:     true,
}

// javaUnit classifies a Java comment node. As javac reads them, a block that
// opens with `/**` is Javadoc unless it is `/**/`, and a line comment ends
// before its line break.
func javaUnit(n *ts.Node, src []byte) (unit, bool) {
	start, end := int(n.StartByte()), int(n.EndByte())
	switch n.Kind() {
	case "line_comment":
		for end > start && (src[end-1] == '\n' || src[end-1] == '\r') {
			end--
		}
		return unit{start: start, end: end, kind: kindLine, open: 2}, true
	case "block_comment":
		if bytes.HasPrefix(src[start:end], []byte("/**")) && end-start >= 5 {
			return unit{start: start, end: end, kind: kindDocBlock, open: 3, close: 2}, true
		}
		return unit{start: start, end: end, kind: kindBlock, open: 2, close: 2}, true
	}
	return unit{}, false
}

var (
	// nosonarMarker is SonarQube's suppression, read anywhere in a comment.
	nosonarMarker = regexp.MustCompile(`\bNOSONAR\b`)
	// nopmdMarker is PMD's suppression, read anywhere in a comment.
	nopmdMarker = regexp.MustCompile(`\bNOPMD\b`)
)

// javaDirectives are the comments a Java tool reads: IntelliJ's inspection
// suppressions, Checkstyle's switches, Sonar's and PMD's suppressions, the
// formatter switches Eclipse, IntelliJ and Spotless read, Eclipse's
// externalized-string markers, and the fall-through marker Checkstyle and Error
// Prone accept.
var javaDirectives = []directiveForm{
	regexpForm("noinspection", "noinspection", []unitKind{kindLine}, regexp.MustCompile(`^noinspection\b`)),
	regexpForm("checkstyle", "CHECKSTYLE", nil, regexp.MustCompile(`^CHECKSTYLE[:.]`)),
	regexpForm("formatter", "@formatter", nil, regexp.MustCompile(`^@formatter:(on|off)\b`)),
	regexpForm("spotless", "spotless", nil, regexp.MustCompile(`^spotless:(on|off)\b`)),
	regexpForm("non-nls", "$NON-NLS", []unitKind{kindLine}, regexp.MustCompile(`^\$NON-NLS-\d+\$`)),
	{name: "nosonar", match: func(t commentText) (string, bool) { return "NOSONAR", nosonarMarker.MatchString(t.raw) }},
	{name: "nopmd", match: func(t commentText) (string, bool) { return "NOPMD", nopmdMarker.MatchString(t.raw) }},
	regexpForm("fallthrough", "fall through", nil, regexp.MustCompile(`(?i)^falls?[ -]?thr(u|ough)\b`)),
}

// javaContainer reports the nodes whose direct children are declarations: a
// file, a type declaration, and the body of a class, record, interface, enum or
// annotation type.
func javaContainer(kind, parent string) bool {
	switch kind {
	case "program", "class_declaration", "interface_declaration", "enum_declaration", "record_declaration", "annotation_type_declaration":
		return true
	case "class_body":
		return parent == "class_declaration" || parent == "record_declaration"
	case "interface_body":
		return parent == "interface_declaration"
	case "annotation_type_body":
		return parent == "annotation_type_declaration"
	case "enum_body":
		return parent == "enum_declaration"
	case "enum_body_declarations":
		return parent == "enum_body"
	}
	return false
}

// javaDecl names a declaration: `class/Parser`, `interface/Visitor`,
// `enum/Kind`, `record/Point` or `annotation/Marker` for a type at any depth, a
// member's own name for a method, constructor, field, enum constant or
// annotation element, and `package` for the package declaration.
func javaDecl(n *ts.Node, _ string, src []byte) (string, bool) {
	named := func(prefix string, name *ts.Node) (string, bool) {
		if name == nil {
			return "", false
		}
		return prefix + nodeName(name, src), true
	}
	switch n.Kind() {
	case "package_declaration":
		return "package", true
	case "class_declaration":
		return named("class/", n.ChildByFieldName("name"))
	case "interface_declaration":
		return named("interface/", n.ChildByFieldName("name"))
	case "enum_declaration":
		return named("enum/", n.ChildByFieldName("name"))
	case "record_declaration":
		return named("record/", n.ChildByFieldName("name"))
	case "annotation_type_declaration":
		return named("annotation/", n.ChildByFieldName("name"))
	case "method_declaration", "constructor_declaration", "annotation_type_element_declaration", "enum_constant":
		return named("", n.ChildByFieldName("name"))
	case "field_declaration", "constant_declaration":
		if d := n.ChildByFieldName("declarator"); d != nil {
			return named("", d.ChildByFieldName("name"))
		}
	}
	return "", false
}
