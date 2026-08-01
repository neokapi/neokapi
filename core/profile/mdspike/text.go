package mdspike

import "strings"

// paragraphs splits markdown prose into paragraphs on blank lines and
// normalizes each one: the hard-wrapped lines a human authors are joined with
// single spaces and runs of whitespace collapse.
//
// This is what makes a hard-wrapped markdown paragraph and a YAML folded scalar
// (`>-`) produce the same string, and it is why the equivalence test in this
// package can compare descriptions and guidelines byte for byte.
func paragraphs(text string) []string {
	var out []string
	var cur []string
	flush := func() {
		if len(cur) == 0 {
			return
		}
		out = append(out, strings.Join(cur, " "))
		cur = nil
	}
	for line := range strings.SplitSeq(text, "\n") {
		if strings.TrimSpace(line) == "" {
			flush()
			continue
		}
		cur = append(cur, strings.Join(strings.Fields(line), " "))
	}
	flush()
	return out
}

// joinParagraphs rejoins normalized paragraphs with a blank line between them.
// A single-paragraph section — the common case for a description or a set of
// guidelines — comes back as one flat line.
func joinParagraphs(paras []string) string {
	return strings.Join(paras, "\n\n")
}
