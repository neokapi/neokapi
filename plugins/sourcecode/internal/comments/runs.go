package comments

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/neokapi/neokapi/core/model"
)

// Placeholder types and subtypes for the parts of a comment that are not prose.
const (
	phCode     = "code"
	phLink     = "link:hyperlink"
	subCode    = "comment:code"
	subURL     = "comment:url"
	subTag     = "jsdoc:tag"
	subLink    = "jsdoc:link"
	subExample = "jsdoc:example"
	subMarkup  = "doc:markup"
)

var (
	// inlineRe finds the inline parts of a comment line that are not prose: a
	// code span, a JSDoc inline tag such as {@link Parser}, and a URL.
	inlineRe = regexp.MustCompile("``[^`]+``|`[^`]+`|\\{@[A-Za-z][^}]*\\}|https?://[^\\s<>\"'`)\\]}]+")
	// tagRe is a JSDoc block tag at the start of a line.
	tagRe = regexp.MustCompile(`^@([A-Za-z][\w-]*)`)
	// typeRe is a JSDoc type expression after a tag. One level of nested braces
	// covers record types such as {{a: string}}.
	typeRe = regexp.MustCompile(`^\s*\{(?:[^{}]|\{[^{}]*\})*\}`)
	// nameRe is a JSDoc parameter or property name, optionally in brackets with
	// a default, followed by an optional hyphen.
	nameRe = regexp.MustCompile(`^\s*(\[[^\]]*\]|[\w$.]+)(\s+-(\s|$))?`)
	// markupRe is an element in a documentation comment written in HTML or XML: a
	// code element with its content, such as <c>x</c> or <code>x</code>, or any
	// other tag, such as <summary> or <see cref="Parser"/>.
	markupRe = regexp.MustCompile(`(?i)(<(?:c|code|tt|pre)\b[^<>]*>.*?</(?:c|code|tt|pre)\s*>)|</?[a-z][\w:.-]*(?:\s[^<>]*)?/?>`)
	// markupBlockRe opens a code element that holds lines of its own.
	markupBlockRe = regexp.MustCompile(`(?i)^\s*<(code|pre)\b[^<>]*>`)
)

// namedTags take a type and a name before their description.
var namedTags = map[string]bool{
	"param": true, "arg": true, "argument": true, "prop": true, "property": true,
	"template": true, "typedef": true, "callback": true,
}

// valueTags hold a value rather than prose, so the whole line is a placeholder.
var valueTags = map[string]bool{
	"type": true, "satisfies": true, "enum": true, "this": true, "since": true, "version": true,
	"author": true, "license": true, "copyright": true, "module": true, "namespace": true,
	"memberof": true, "alias": true, "augments": true, "extends": true, "implements": true,
	"mixes": true, "lends": true, "borrows": true, "access": true, "kind": true, "default": true,
	"defaultvalue": true, "import": true, "category": true, "group": true, "internal": true,
	"public": true, "private": true, "protected": true, "readonly": true, "override": true,
	"abstract": true, "static": true, "async": true, "generator": true, "function": true,
	"func": true, "method": true, "class": true, "constructor": true, "interface": true,
	"inheritdoc": true, "hideconstructor": true, "ignore": true, "package": true, "sealed": true,
	"virtual": true, "experimental": true, "beta": true, "alpha": true, "hidden": true,
	"overload": true, "label": true, "event": true, "fires": true, "emits": true, "listens": true,
	"requires": true, "external": true, "host": true, "name": true, "member": true, "var": true,
	"constant": true, "const": true,
}

// buildRuns turns a comment's content lines into runs. Prose is text. Code
// spans and fenced code blocks, URLs and JSDoc inline tags are placeholders. In
// a documentation comment whose tags are structured, a block tag with its type
// and name is a placeholder, a tag that holds a value is one whole, and an
// @example section is one. In a documentation comment written in HTML or XML,
// each tag is a placeholder and a code element is one with its content. It also
// reports whether a @deprecated tag is present.
func buildRuns(lines []string, docTags, markup bool) ([]model.Run, bool) {
	var b runBuilder
	deprecated := false
	for i := 0; i < len(lines); i++ {
		if i > 0 {
			b.text("\n")
		}
		line := lines[i]
		if markup {
			if end, ok := markupBlockEnd(lines, i); ok {
				b.placeholder(phCode, subCode, strings.Join(lines[i:end+1], "\n"))
				i = end
				continue
			}
		}
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			end := i + 1
			for end < len(lines) && !strings.HasPrefix(strings.TrimSpace(lines[end]), "```") {
				end++
			}
			if end == len(lines) {
				end--
			}
			b.placeholder(phCode, subCode, strings.Join(lines[i:end+1], "\n"))
			i = end
			continue
		}
		if docTags {
			if m := tagRe.FindStringSubmatch(line); m != nil {
				tag := strings.ToLower(m[1])
				if tag == "deprecated" {
					deprecated = true
				}
				if tag == "example" {
					end := i + 1
					for end < len(lines) && !tagRe.MatchString(lines[end]) {
						end++
					}
					b.placeholder(phCode, subExample, strings.Join(lines[i:end], "\n"))
					i = end - 1
					continue
				}
				// Javadoc's @see holds a reference, and its @throws names the
				// exception before the description.
				if valueTags[tag] || markup && tag == "see" {
					b.placeholder(phCode, subTag, line)
					continue
				}
				prefix := len(m[0])
				if loc := typeRe.FindStringIndex(line[prefix:]); loc != nil {
					prefix += loc[1]
				}
				if namedTags[tag] || markup && (tag == "throws" || tag == "exception") {
					if loc := nameRe.FindStringIndex(line[prefix:]); loc != nil {
						prefix += loc[1]
					}
				}
				for prefix < len(line) && (line[prefix] == ' ' || line[prefix] == '\t') {
					prefix++
				}
				b.placeholder(phCode, subTag, line[:prefix])
				line = line[prefix:]
			}
		}
		if markup {
			b.markup(line)
			continue
		}
		b.inline(line)
	}
	return b.runs, deprecated
}

// markupBlockEnd reports the line that closes a code element opening line i and
// closing on a later line, such as a <pre> block.
func markupBlockEnd(lines []string, i int) (int, bool) {
	m := markupBlockRe.FindStringSubmatch(lines[i])
	if m == nil {
		return 0, false
	}
	closer := "</" + strings.ToLower(m[1])
	if strings.Contains(strings.ToLower(lines[i]), closer) {
		return 0, false
	}
	for end := i + 1; end < len(lines); end++ {
		if strings.Contains(strings.ToLower(lines[end]), closer) {
			return end, true
		}
	}
	return 0, false
}

// markup appends one line of a documentation comment written in HTML or XML:
// its tags and code elements are placeholders, and the text between them is
// read as inline prose.
func (b *runBuilder) markup(line string) {
	for {
		loc := markupRe.FindStringSubmatchIndex(line)
		if loc == nil {
			b.inline(line)
			return
		}
		b.inline(line[:loc[0]])
		sub := subMarkup
		if loc[2] >= 0 {
			sub = subCode
		}
		b.placeholder(phCode, sub, line[loc[0]:loc[1]])
		line = line[loc[1]:]
	}
}

type runBuilder struct {
	runs []model.Run
	ids  int
}

// inline appends one line of prose, with its inline code, tags and URLs as
// placeholders. Punctuation that ends a sentence after a URL stays prose.
func (b *runBuilder) inline(line string) {
	for {
		loc := inlineRe.FindStringIndex(line)
		if loc == nil {
			b.text(line)
			return
		}
		match := line[loc[0]:loc[1]]
		b.text(line[:loc[0]])
		switch {
		case strings.HasPrefix(match, "`"):
			b.placeholder(phCode, subCode, match)
			line = line[loc[1]:]
		case strings.HasPrefix(match, "{@"):
			b.placeholder(phCode, subLink, match)
			line = line[loc[1]:]
		default:
			url := strings.TrimRight(match, ".,;:!?")
			b.placeholder(phLink, subURL, url)
			line = line[loc[0]+len(url):]
		}
	}
}

// text appends prose, joining it to a text run already at the end.
func (b *runBuilder) text(s string) {
	if s == "" {
		return
	}
	if n := len(b.runs); n > 0 && b.runs[n-1].Text != nil {
		b.runs[n-1].Text.Text += s
		return
	}
	b.runs = append(b.runs, model.TextR(s))
}

func (b *runBuilder) placeholder(typ, subType, data string) {
	b.ids++
	b.runs = append(b.runs, model.PhR(model.PlaceholderRun{
		ID:      "c" + strconv.Itoa(b.ids),
		Type:    typ,
		SubType: subType,
		Data:    data,
	}))
}
