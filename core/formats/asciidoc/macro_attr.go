package asciidoc

import (
	"errors"
	"slices"
	"strings"

	"github.com/neokapi/neokapi/core/model"
)

// ErrAltUnwritable is the writer's refusal of an image's alternative text
// that no macro attribute list can hold: a line break, which ends the macro,
// or a quoted value ending in a backslash, which would escape its closing
// quote.
var ErrAltUnwritable = errors.New("its alternative text cannot be written in the image macro")

// macroAttrValue spells text as the first positional attribute of an image
// macro's attribute list, the place an image's alternative text sits.
//
// In the list a comma ends a value, an `=` turns it into a named attribute, a
// leading quote opens a quoted value, and space at either end is trimmed, so
// text holding any of them is written in double quotes unless the document
// already quoted it. Inside double quotes a `"` is written `\"`. The list of
// an inline macro ends at the first `]`, which is written `\]` there; a block
// macro's list runs to the last `]` on its line. A character a backslash
// already escapes is left as it is, so alternative text read with escapes is
// written as read.
func macroAttrValue(text string, quoted, inline bool) (string, error) {
	if strings.ContainsAny(text, "\n\r") {
		return "", ErrAltUnwritable
	}
	addQuotes := !quoted && (strings.ContainsAny(text, ",=") ||
		strings.HasPrefix(text, `"`) || strings.HasPrefix(text, "'") ||
		strings.TrimSpace(text) != text)
	inQuotes := quoted || addQuotes
	if inQuotes && trailingBackslashes(text)%2 == 1 {
		return "", ErrAltUnwritable
	}
	var b strings.Builder
	b.Grow(len(text) + 4)
	if addQuotes {
		b.WriteByte('"')
	}
	for i := 0; i < len(text); i++ {
		c := text[i]
		if c == '\\' && i+1 < len(text) {
			b.WriteByte(c)
			b.WriteByte(text[i+1])
			i++
			continue
		}
		if (c == '"' && inQuotes) || (c == ']' && inline) {
			b.WriteByte('\\')
		}
		b.WriteByte(c)
	}
	if addQuotes {
		b.WriteByte('"')
	}
	return b.String(), nil
}

// trailingBackslashes counts the backslashes text ends with.
func trailingBackslashes(text string) int {
	return len(text) - len(strings.TrimRight(text, `\`))
}

// spellImageAlts returns runs with the alternative text of every inline image
// macro spelled for the macro's attribute list (macroAttrValue). The reader
// reads an inline image macro as a code pair around its alternative text,
// which the pair's opening data ends with a quote for when the document
// quoted it. runs itself is not changed.
func spellImageAlts(runs []model.Run) ([]model.Run, error) {
	var out []model.Run
	for i := range runs {
		open := runs[i].PcOpen
		if open == nil || open.SubType != "asciidoc:image" {
			continue
		}
		end := slices.IndexFunc(runs[i+1:], func(r model.Run) bool {
			return r.PcClose != nil && r.PcClose.ID == open.ID
		})
		if end < 0 {
			continue
		}
		end += i + 1
		var alt strings.Builder
		textOnly := true
		for _, r := range runs[i+1 : end] {
			if r.Text == nil {
				textOnly = false
				break
			}
			alt.WriteString(r.Text.Text)
		}
		if !textOnly {
			continue
		}
		spelled, err := macroAttrValue(alt.String(), strings.HasSuffix(open.Data, `"`), true)
		if err != nil {
			return nil, err
		}
		if spelled == alt.String() {
			continue
		}
		if out == nil {
			out = slices.Clone(runs)
		}
		// The alt text becomes one text run, keeping the first run's flags.
		// out is offset from runs by what earlier replacements removed.
		shift := len(out) - len(runs)
		text := *runs[i+1].Text
		text.Text = spelled
		out = slices.Replace(out, i+1+shift, end+shift, model.Run{Text: &text})
	}
	if out == nil {
		return runs, nil
	}
	return out, nil
}
