package check

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/set"
)

// This file is the one home of the content-*shape* predicates: where the content
// begins and ends, what sits next to what, whether anything is there at all.
// Every venue that judges shape — the source-side `kapi check` hygiene family,
// the bilingual `qa.*` family, and the editor/lab check overlays — calls these, so
// a rule cannot mean one thing in the CLI and another in a preview.
//
// They all read a [HygieneText] flattening rather than a plain SourceText,
// because dropping inline code changes the shape of the content being judged.
// See [HygieneText]; the same defect had four separate instances precisely
// because the rules were implemented four times (#1441).

// HygieneText is the flattening every content-*shape* rule must inspect: the
// block's runs with each inline-code run standing as one sentinel rune
// (model.RunsHygieneText).
//
// A shape rule asks where the content begins and ends, what sits next to what,
// and whether there is anything there at all. `SourceText()` cannot answer any
// of those, because it drops inline code and so silently changes the shape:
//
//	[ph{p.price}][text " each"]  →  " each"   →  "leading whitespace" (false)
//
// The space there separates a placeholder from a word. It is not whitespace on
// the content's leading edge — the placeholder is. Evaluating against run-aware
// boundaries, where a leading or trailing placeholder counts as content, is what
// makes that distinction expressible at all.
//
// A rule that reports a range, not just a verdict, needs the offsets as well as
// the string: use model.NewHygieneView, which pairs this flattening with the
// mapping back to run-anchored spans (see [HygieneOverlay]).
func HygieneText(runs []model.Run) string {
	return model.RunsHygieneText(runs)
}

// LeadingWhitespace returns the leading whitespace of a [HygieneText]
// flattening. On a plain text it would report a placeholder's separator space as
// whitespace on the content's leading edge.
func LeadingWhitespace(s string) string {
	trimmed := strings.TrimLeft(s, " \t\n\r")
	return s[:len(s)-len(trimmed)]
}

// TrailingWhitespace returns the trailing whitespace of a [HygieneText]
// flattening — the trailing-edge counterpart of [LeadingWhitespace]. It is the
// whole edge, which is what a rule comparing two sides reads; a rule judging one
// text on its own reads [StrayTrailingWhitespace].
func TrailingWhitespace(s string) string {
	trimmed := strings.TrimRight(s, " \t\n\r")
	return s[len(trimmed):]
}

// StrayTrailingWhitespace returns the trailing whitespace an author left on a
// [HygieneText] flattening: [TrailingWhitespace] with one terminating line break
// discounted.
//
// A block's last line break is often the format's rather than the author's. A
// YAML block scalar keeps exactly one under clip chomping, which is what `|` and
// `>` mean — `|-` and `>-` are how an author asks for none, and a plain scalar
// carries none either way. The break terminates the content instead of trailing
// off it, so grading it reports the scalar's style, on a character the author
// cannot remove without changing what the document says.
//
// Everything else stands, because nothing structural puts it there: trailing
// spaces and tabs, spaces or tabs before the break, whitespace after it, and a
// second break — a blank line the author kept, which is what `|+` is for.
//
// A rule that compares a source against its target reads the whole edge
// ([TrailingWhitespace]) instead: a terminator both sides carry cancels, and a
// target that dropped one differs from its source. The same is true of the
// whitespace-correct tool, which copies the source's edge onto the target and
// must carry the terminator with it.
func StrayTrailingWhitespace(s string) string {
	ws := TrailingWhitespace(s)
	switch {
	case strings.HasSuffix(ws, "\r\n"):
		return ws[:len(ws)-len("\r\n")]
	case strings.HasSuffix(ws, "\n"):
		return ws[:len(ws)-len("\n")]
	}
	return ws
}

// DoubleSpaces reports whether a [HygieneText] flattening contains two or more
// consecutive spaces between two pieces of content on the same line.
//
// Across an inline code it deliberately does not fire: in `[text "Hello "][ph]
// [text " world"]` each space separates a word from the placeholder between
// them, and the placeholder is content sitting between the two — there are no
// consecutive spaces in the content, only in a flattening that dropped what was
// between them. Two spaces inside one text run, or spanning two adjacent text
// runs (nothing separates those), are a real double space and do fire.
//
// Across a line break it does not fire either — see [doubleSpaceSpans] for why
// the whitespace after a newline is the source's line structure rather than
// spacing the author put between two words.
func DoubleSpaces(text string) bool { return len(doubleSpaceSpans(text)) > 0 }

// DoubledWord returns the first immediately-repeated word in a [HygieneText]
// flattening, or "" when there is none. Comparison is case-insensitive;
// exceptions is a semicolon-separated list of words allowed to repeat (some
// languages legitimately double a pronoun).
//
// An inline code between two instances of a word is not a doubled word:
// `the {name} the cat` repeats "the" either side of a placeholder, which is odd
// phrasing but not the redundant token this rule is about. The code counts as a
// token of its own, so it separates the two without being mistaken for either —
// and without gluing itself onto the word beside it, which would hide the
// genuine double in `{name}the the cat`. `the the cat` does report, including
// when it arrives as two adjacent text runs, since nothing separates those.
func DoubledWord(text, exceptions string) string {
	spans := doubledWordSpans(text, exceptions)
	if len(spans) == 0 {
		return ""
	}
	return text[spans[0][0]:spans[0][1]]
}

// HygieneOverlay returns the shape findings that carry a *range* — double spaces
// and doubled words — as a source-anchored check overlay, one span per occurrence,
// or nil when the content is clean. It is the overlay form of the same rules
// `kapi check` reports as `hygiene.double-spaces` / `hygiene.doubled-word`, and
// backs the desktop and lab annotators so a preview highlight and a CLI finding
// can never disagree about what counts as a defect.
//
// Spans are anchored through model.HygieneView.Range, which maps the offsets the
// rules found in the hygiene flattening back to [model.RunsText] coordinates.
// Judging the shape on one string and anchoring to another is how a highlight
// ends up shifted by the width of every preceding placeholder.
func HygieneOverlay(runs []model.Run) *model.Overlay {
	v := model.NewHygieneView(runs)
	text := v.Text()

	var spans []model.Span
	for _, r := range doubleSpaceSpans(text) {
		spans = append(spans, model.Span{
			Range: v.Range(r[0], r[1]),
			Props: map[string]string{
				"category": "double-spaces",
				"severity": string(SeverityMinor),
				"message":  "Source contains double spaces",
			},
		})
	}
	for _, r := range doubledWordSpans(text, "") {
		spans = append(spans, model.Span{
			Range: v.Range(r[0], r[1]),
			Props: map[string]string{
				"category": "doubled-word",
				"severity": string(SeverityMinor),
				"message":  fmt.Sprintf("Doubled word: %q", text[r[0]:r[1]]),
			},
		})
	}
	if len(spans) == 0 {
		return nil
	}
	return &model.Overlay{Type: model.OverlayQA, Spans: spans}
}

// doubleSpaceSpans returns the byte range of every maximal run of two or more
// consecutive spaces in a hygiene flattening, skipping any run that indents a
// line. It is the single scanner behind both the verdict ([DoubleSpaces]) and
// the ranges ([HygieneOverlay]).
//
// A block's text spans as many lines as the source wrote it on: a wrapped
// paragraph or list item arrives with its literal line breaks, each followed by
// whatever prefix the format uses to continue the line. That prefix is line
// structure, not spacing between words — CommonMark indents a list item's
// continuation lines to the marker's content column, so every conventionally
// wrapped bullet carries a two-space run after each newline. A rule that reads
// those as a typographic double space fires on the indent rather than the prose,
// on content the author cannot correct without breaking the markup.
//
// Only a run that immediately follows a newline is discounted. A run at the
// text's own leading edge is not: a block need not begin at a line start (it may
// follow a list marker or a heading's `# `), so the check cannot know that
// whitespace to be indentation — and [LeadingWhitespace] already judges the
// content's edge. Spaces before a newline stay reportable for the same reason:
// nothing structural puts them there.
func doubleSpaceSpans(text string) [][2]int {
	var out [][2]int
	for i := 0; i+1 < len(text); {
		if text[i] == ' ' && text[i+1] == ' ' {
			start := i
			for i < len(text) && text[i] == ' ' {
				i++
			}
			if start > 0 && text[start-1] == '\n' {
				continue // the line's indent, not spacing inside a sentence
			}
			out = append(out, [2]int{start, i})
			continue
		}
		i++
	}
	return out
}

// doubledWordSpans returns the byte range of the *second* occurrence of every
// immediately-repeated word in a hygiene flattening, so a caller can highlight
// exactly the redundant token. It is the single scanner behind both the verdict
// ([DoubledWord]) and the ranges ([HygieneOverlay]).
//
// Only a token that holds a word repeats reportably — see [isWordToken].
func doubledWordSpans(text, exceptions string) [][2]int {
	words := hygieneWords(text)
	if len(words) < 2 {
		return nil
	}
	excSet := set.New[string]()
	for w := range strings.SplitSeq(exceptions, ";") {
		if w = strings.TrimSpace(w); w != "" {
			excSet.Add(strings.ToLower(w))
		}
	}

	var out [][2]int
	for i := 1; i < len(words); i++ {
		if words[i].code {
			continue // two adjacent codes are not a doubled word
		}
		prev := strings.ToLower(words[i-1].text)
		curr := strings.ToLower(words[i].text)
		if prev == curr && !excSet.Contains(curr) && isWordToken(curr) {
			out = append(out, [2]int{words[i].start, words[i].end})
		}
	}
	return out
}

// isWordToken reports whether a hygiene token holds a word: a letter or a digit
// somewhere in it. Tokenization is by whitespace, so a token need not be a word
// at all — a shade glyph standing for a pseudo-translation wrapper
// (`▒ ▒ Utility ▒ ▒`), a row of dashes, a repeated bullet — and those repeat
// exactly the way a word does. A repetition of them is the notation the author
// meant to show, not the redundant word this rule is about; reporting it makes
// the writer choose between a correct example and a clean gate.
func isWordToken(s string) bool {
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return true
		}
	}
	return false
}

// wordSpan is one token of a hygiene flattening and its byte range in the text:
// either a whitespace-delimited word, or a single inline code (code=true).
type wordSpan struct {
	text       string
	start, end int
	code       bool
}

// hygieneWords tokenizes a hygiene flattening: whitespace-delimited words, plus
// one token per inline code. Whitespace is Unicode whitespace, matching
// strings.Fields, so "word" means the same thing to a rule that reports a verdict
// and one that reports a range.
//
// An inline code is a token of its own — not whitespace, and not part of the word
// beside it. Both halves of that matter to an adjacency rule: as a token it
// separates the words either side of it (`the {name} the cat` holds no doubled
// word), and by not merging into its neighbour it cannot hide one
// (`{name}the the cat` does).
func hygieneWords(text string) []wordSpan {
	var out []wordSpan
	start := -1
	flush := func(end int) {
		if start >= 0 {
			out = append(out, wordSpan{text: text[start:end], start: start, end: end})
			start = -1
		}
	}
	for i, r := range text {
		switch {
		case r == model.ObjectReplacement:
			flush(i)
			out = append(out, wordSpan{
				text:  text[i : i+len(objectReplacement)],
				start: i,
				end:   i + len(objectReplacement),
				code:  true,
			})
		case unicode.IsSpace(r):
			flush(i)
		default:
			if start < 0 {
				start = i
			}
		}
	}
	flush(len(text))
	return out
}

// objectReplacement is model.ObjectReplacement as a string — one inline code's
// worth of the hygiene flattening.
const objectReplacement = string(model.ObjectReplacement)
