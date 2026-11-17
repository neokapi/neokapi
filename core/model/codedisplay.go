package model

import (
	"regexp"
	"strings"
)

// A code's native Data and its Equiv are for tools: Data is the source slice
// kept for the round trip, and an extractor synthesizes Equiv when the source
// has no name (the React extractor names an element "=m0"). Text a person
// reads names a code only by a human name, or words its kind. This mirrors
// packages/kapi-format/src/code-display.ts, so the server's display strings
// and the clients' chips say the same thing.

// humanCodeNameRe is a name a person can read: it starts with a letter and
// holds only letters, digits, spaces, '_', '.' and '-', at most 40 characters.
var humanCodeNameRe = regexp.MustCompile(`^\pL[\pL\pN _.\-]{0,39}$`)

// wrappedCodeName are the delimiters formats wrap a variable's name in: a
// vocabulary's [B], {{name}} and {{.Name}}, ${name}, {name}, %(name)s and
// :name. Only the name inside is kept, and it still has to pass as a name.
var wrappedCodeName = []*regexp.Regexp{
	regexp.MustCompile(`^\[/?(.+)\]$`),
	regexp.MustCompile(`^\{\{\s*\.?(.+?)\s*\}\}$`),
	regexp.MustCompile(`^\$\{(.+)\}$`),
	regexp.MustCompile(`^\{(.+)\}$`),
	regexp.MustCompile(`^%\((.+)\)[a-z]$`),
	regexp.MustCompile(`^:(.+)$`),
}

// HumanCodeName returns the first candidate that is a human name, with the
// delimiters around a name dropped, or "" when none is.
func HumanCodeName(candidates ...string) string {
	for _, c := range candidates {
		raw := strings.TrimSpace(c)
		if raw == "" {
			continue
		}
		name := raw
		for _, re := range wrappedCodeName {
			if m := re.FindStringSubmatch(raw); m != nil {
				name = m[1]
				break
			}
		}
		if humanCodeNameRe.MatchString(name) {
			return name
		}
	}
	return ""
}

// CodeKind is what a code is, for a surface to word.
type CodeKind string

const (
	CodeKindLink     CodeKind = "link"
	CodeKindBold     CodeKind = "bold"
	CodeKindItalic   CodeKind = "italic"
	CodeKindCode     CodeKind = "code"
	CodeKindBreak    CodeKind = "break"
	CodeKindImage    CodeKind = "image"
	CodeKindVariable CodeKind = "variable"
	CodeKindContent  CodeKind = "content"
	CodeKindElement  CodeKind = "element"
	CodeKindUnknown  CodeKind = "unknown"
)

var codeTagKinds = map[string]CodeKind{
	"a": CodeKindLink, "link": CodeKindLink,
	"strong": CodeKindBold, "b": CodeKindBold,
	"em": CodeKindItalic, "i": CodeKindItalic,
	"code": CodeKindCode, "kbd": CodeKindCode, "samp": CodeKindCode,
	"br":  CodeKindBreak,
	"img": CodeKindImage, "image": CodeKindImage,
}

var codeTypeKinds = []struct {
	re   *regexp.Regexp
	kind CodeKind
}{
	{regexp.MustCompile(`(^|:)(var|variable|param|placeholder)$`), CodeKindVariable},
	{regexp.MustCompile(`(^|:)node$`), CodeKindContent},
	{regexp.MustCompile(`(^|:)(link|hyperlink)$`), CodeKindLink},
	{regexp.MustCompile(`(^|:)(bold|strong)$`), CodeKindBold},
	{regexp.MustCompile(`(^|:)(italic|emphasis)$`), CodeKindItalic},
	{regexp.MustCompile(`(^|:)(break|lb|linebreak)$`), CodeKindBreak},
	{regexp.MustCompile(`(^|:)(image|img)$`), CodeKindImage},
}

// CodeKindOf returns the kind of a code from its vocabulary type and subtype.
func CodeKindOf(typ, subType string) CodeKind {
	tag := subType
	if tag == "" {
		tag = typ
		if _, after, ok := strings.CutLast(typ, ":"); ok {
			tag = after
		}
	}
	if k, ok := codeTagKinds[strings.ToLower(tag)]; ok {
		return k
	}
	for _, t := range codeTypeKinds {
		if t.re.MatchString(typ) {
			return t.kind
		}
	}
	return CodeKindElement
}

// CodeKindWords are the English words for a code with no human name.
var CodeKindWords = map[CodeKind]string{
	CodeKindLink:     "link",
	CodeKindBold:     "bold",
	CodeKindItalic:   "italic",
	CodeKindCode:     "code",
	CodeKindBreak:    "line break",
	CodeKindImage:    "image",
	CodeKindVariable: "value",
	CodeKindContent:  "content",
	CodeKindElement:  "styled text",
	CodeKindUnknown:  "code",
}

// PlaceholderWord is what a reader sees for a placeholder: its human name
// (from Disp, Equiv or the name inside its native form), else the word for
// its kind. Never its native form or a synthesized identifier.
func PlaceholderWord(p *PlaceholderRun) string {
	// A placeholder with no type stands for a value.
	kind := CodeKindVariable
	if p.Type != "" || p.SubType != "" {
		kind = CodeKindOf(p.Type, p.SubType)
	}
	if name := HumanCodeName(p.Disp, p.Equiv, p.Data); name != "" && codeTagKinds[strings.ToLower(name)] != kind {
		return name
	}
	return CodeKindWords[kind]
}
