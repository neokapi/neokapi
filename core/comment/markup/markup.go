// Package markup holds what every `<!-- -->` comment shares, whichever markup
// language's provider located it: the prose a comment holds, and the directive
// forms tools read in more than one markup language.
//
// Where a comment starts and ends is each language's own business, since XML
// and HTML close a comment by different rules. A provider hands this package
// what sits between the markers.
package markup

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/neokapi/neokapi/core/model"
)

// The markers that open and close a markup comment.
const (
	Open  = "<!--"
	Close = "-->"
)

// Placeholder type and subtype for a line of markup inside a comment.
const (
	PhCode    = "code"
	SubMarkup = "markup"
)

// Blank reports whether a comment holds nothing but whitespace.
func Blank(inner string) bool { return strings.TrimSpace(inner) == "" }

// Runs projects what a comment holds into runs. Each line is trimmed and blank
// lines at either edge are dropped. A line that is itself markup, such as an
// element commented out, becomes a placeholder, so no check reads it as a
// sentence.
func Runs(inner string) []model.Run {
	lines := strings.Split(inner, "\n")
	for i := range lines {
		lines[i] = strings.TrimSpace(lines[i])
	}
	first, last := 0, len(lines)
	for first < last && lines[first] == "" {
		first++
	}
	for last > first && lines[last-1] == "" {
		last--
	}
	var runs []model.Run
	text := func(s string) {
		if n := len(runs); n > 0 && runs[n-1].Text != nil {
			runs[n-1].Text.Text += s
			return
		}
		runs = append(runs, model.TextR(s))
	}
	ids := 0
	for i, line := range lines[first:last] {
		if i > 0 {
			text("\n")
		}
		if !isMarkup(line) {
			if line != "" {
				text(line)
			}
			continue
		}
		ids++
		runs = append(runs, model.PhR(model.PlaceholderRun{
			ID: "m" + strconv.Itoa(ids), Type: PhCode, SubType: SubMarkup, Data: line,
		}))
	}
	return runs
}

// isMarkup reports a line that opens with a tag and closes with one.
func isMarkup(line string) bool {
	if len(line) < 3 || line[0] != '<' || line[len(line)-1] != '>' {
		return false
	}
	c := line[1]
	return c == '/' || c == '?' || c == '!' || (c|0x20 >= 'a' && c|0x20 <= 'z')
}

// DirectiveForm is one kind of comment a tool reads as an instruction.
type DirectiveForm struct {
	// Name is recorded as the exclusion's form.
	Name string
	// Match receives what the comment holds, trimmed.
	Match func(body string) bool
}

// Classify returns the name of the first form the comment matches.
func Classify(inner string, forms []DirectiveForm) (string, bool) {
	body := strings.TrimSpace(inner)
	for _, f := range forms {
		if f.Match(body) {
			return f.Name, true
		}
	}
	return "", false
}

// PrettierIgnore is Prettier's instruction to leave what follows as written:
// `prettier-ignore`, and the `prettier-ignore-start`, `prettier-ignore-end` and
// `prettier-ignore-attribute` forms.
var PrettierIgnore = DirectiveForm{Name: "prettier-ignore", Match: func(b string) bool {
	return b == "prettier-ignore" || strings.HasPrefix(b, "prettier-ignore-")
}}

// FormatterToggle is the JetBrains IDEs' `@formatter:off` and `@formatter:on`,
// which turn the formatter off and on around what they enclose.
var FormatterToggle = DirectiveForm{Name: "formatter", Match: func(b string) bool {
	return b == "@formatter:off" || b == "@formatter:on"
}}

// suppressRe is a JetBrains suppression: the keyword and a comma-separated list
// of inspection ids, which are written in upper camel case.
var suppressRe = regexp.MustCompile(`^(suppress|noinspection)\s+[A-Z][A-Za-z0-9]*(\s*,\s*[A-Z][A-Za-z0-9]*)*$`)

// Suppress is the JetBrains IDEs' `suppress` and `noinspection`, which silence
// the named inspections, as in `<!--suppress AndroidLintUnusedResources -->`.
var Suppress = DirectiveForm{Name: "suppress", Match: suppressRe.MatchString}

// resharperRe is ReSharper's `disable` or `restore`, optionally `once`.
var resharperRe = regexp.MustCompile(`^ReSharper (disable|restore)\b`)

// ReSharper is ReSharper's instruction to disable or restore inspections.
var ReSharper = DirectiveForm{Name: "ReSharper", Match: resharperRe.MatchString}
