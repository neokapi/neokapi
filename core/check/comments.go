package check

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/neokapi/neokapi/core/comment"
	"github.com/neokapi/neokapi/core/model"
)

// CommentStyleID is the source-side checker that holds the comments in source
// files to the limits a voice profile sets for them.
const CommentStyleID = "comment-style"

// The categories the comment-style checker reports. `kapi check` reports them
// under the `comment` family, as `comment.sentence-length` and
// `comment.length`.
const (
	// CategorySentenceLength is a sentence that holds more words than the limit.
	CategorySentenceLength = "sentence-length"
	// CategoryCommentLength is a comment that holds more words than the limit
	// for what it documents.
	CategoryCommentLength = "length"
)

// CommentLimits are the word counts the comment-style checker holds comments
// to. A word is a stretch of text between whitespace and inline codes that
// holds a letter or a digit, so a code span, a reference or a link is never a
// word, and neither is a dash standing alone.
type CommentLimits struct {
	// SentenceMinor and SentenceMajor are the word counts above which a
	// sentence is a minor finding and a major one.
	SentenceMinor int
	SentenceMajor int
	// CommentWords is the most words a comment that documents no declaration
	// may hold.
	CommentWords int
	// DocWords is the most words a declaration's doc comment may hold.
	DocWords int
	// PackageDocWords is the most words the doc comment of a package or module
	// may hold (comment.PackageDoc).
	PackageDocWords int
}

// DefaultCommentLimits are the limits that apply where a voice profile asks for
// comment limits and names no number of its own.
func DefaultCommentLimits() CommentLimits {
	return CommentLimits{SentenceMinor: 50, SentenceMajor: 70, CommentWords: 100, DocWords: 150, PackageDocWords: 300}
}

// SentenceBreak finds the sentences in a run sequence, as the UAX #29 engine
// registered with core/segment does.
type SentenceBreak interface {
	Segment(ctx context.Context, runs []model.Run, loc model.LocaleID) ([]model.Span, error)
}

// Sentence is one sentence of a comment's prose.
type Sentence struct {
	// Start and End are the sentence's half-open byte span in the block's
	// hygiene flattening (model.HygieneView).
	Start, End int
	// Words is how many words the sentence holds.
	Words int
}

// CommentSentences divides the prose of a comment into sentences with seg, a
// sentence break such as UAX #29. Each unit of prose goes to seg with its line
// breaks read as spaces: a comment wraps a sentence over lines, and a sentence
// break ends a sentence at every line break.
//
// A unit ends at a blank line, at a line that holds only inline codes, such as
// a code block, and at a divider line, such as `── Section ──`, whose title is
// a unit of its own. A heading is a unit of its own too, and a line that opens
// a list item or a documentation tag starts a unit. A list item opens with a marker written as
// text, or with a placeholder of type comment.TypeListItem where the provider's
// parser consumed the marker. A documentation tag, such as JSDoc's `@param`, is
// a placeholder whose subtype ends in `:tag`.
func CommentSentences(ctx context.Context, seg SentenceBreak, v *model.HygieneView, loc model.LocaleID) ([]Sentence, error) {
	text := v.Text()
	var out []Sentence
	for _, u := range commentUnits(v) {
		unit := strings.ReplaceAll(text[u[0]:u[1]], "\n", " ")
		spans, err := seg.Segment(ctx, []model.Run{model.TextR(unit)}, loc)
		if err != nil {
			return nil, err
		}
		offsets := runeOffsets(unit)
		runes := len(offsets) - 1
		at := func(p model.RunPos) int {
			if p.Run > 0 || p.Offset > runes {
				return offsets[runes]
			}
			return offsets[p.Offset]
		}
		for _, sp := range spans {
			start, end := trimSpace(text, u[0]+at(sp.Range.Start), u[0]+at(sp.Range.End))
			if words := proseWords(text[start:end]); words > 0 {
				out = append(out, Sentence{Start: start, End: end, Words: words})
			}
		}
	}
	return out, nil
}

// CommentSentenceFindings reports each sentence of a comment block that holds
// more words than limits allow: a minor finding above SentenceMinor, and a
// major one above SentenceMajor. It reports nothing on a block the comment
// layer did not build.
func CommentSentenceFindings(ctx context.Context, seg SentenceBreak, b *model.Block, limits CommentLimits, loc model.LocaleID) ([]Finding, error) {
	if !comment.IsBlock(b) {
		return nil, nil
	}
	v := model.NewHygieneView(b.SourceRuns())
	sentences, err := CommentSentences(ctx, seg, v, loc)
	if err != nil {
		return nil, err
	}
	var findings []Finding
	for _, s := range sentences {
		severity, limit := SeverityMajor, limits.SentenceMajor
		switch {
		case s.Words > limits.SentenceMajor:
		case s.Words > limits.SentenceMinor:
			severity, limit = SeverityMinor, limits.SentenceMinor
		default:
			continue
		}
		findings = append(findings, Finding{
			Category:     CategorySentenceLength,
			Severity:     severity,
			Message:      fmt.Sprintf("Sentence has %d words, over the limit of %d", s.Words, limit),
			Position:     v.Range(s.Start, s.End),
			OriginalText: readText(v, s.Start, s.End),
			Metadata:     map[string]string{"words": strconv.Itoa(s.Words), "limit": strconv.Itoa(limit)},
		})
	}
	return findings, nil
}

// CommentLengthFindings reports a comment block that holds more words than the
// limit for what it documents: the package or module its file belongs to, a
// declaration, or nothing. The finding is major. It reports nothing on a block
// the comment layer did not build.
func CommentLengthFindings(b *model.Block, limits CommentLimits) []Finding {
	if !comment.IsBlock(b) {
		return nil
	}
	kind, limit := "Comment", limits.CommentWords
	switch {
	case b.Properties[comment.PropPackageDoc] == "true":
		kind, limit = "Package doc comment", limits.PackageDocWords
	case b.Properties[comment.PropDoc] == "true":
		kind, limit = "Doc comment", limits.DocWords
	}
	words := proseWords(model.RunsHygieneText(b.SourceRuns()))
	if words <= limit {
		return nil
	}
	return []Finding{{
		Category: CategoryCommentLength,
		Severity: SeverityMajor,
		Message:  fmt.Sprintf("%s has %d words, over the limit of %d", kind, words, limit),
		Metadata: map[string]string{"words": strconv.Itoa(words), "limit": strconv.Itoa(limit)},
	}}
}

// CommentSentenceCanaries are the comments the sentence-length check must flag
// under limits: a sentence one word past the minor limit, and one a word past
// the major limit wrapped over three lines, which a check that ends a sentence
// at each line break reads as three short ones.
func CommentSentenceCanaries(limits CommentLimits) []Canary {
	return []Canary{
		{
			Name:     fmt.Sprintf("a sentence of %d words", limits.SentenceMinor+1),
			Block:    commentCanary("func/Canary", true, canarySentence(limits.SentenceMinor+1, 1)),
			Expect:   CategorySentenceLength,
			Severity: SeverityMinor,
		},
		{
			Name:     fmt.Sprintf("a sentence of %d words wrapped over three lines", limits.SentenceMajor+1),
			Block:    commentCanary("func/Canary", true, canarySentence(limits.SentenceMajor+1, 3)),
			Expect:   CategorySentenceLength,
			Severity: SeverityMajor,
		},
	}
}

// CommentLengthCanaries are the comments the comment-length check must flag
// under limits: one a word past the limit for each kind of comment.
func CommentLengthCanaries(limits CommentLimits) []Canary {
	return []Canary{
		{
			Name:   fmt.Sprintf("a comment of %d words", limits.CommentWords+1),
			Block:  commentCanary("func/Canary/comment", false, canarySentence(limits.CommentWords+1, 4)),
			Expect: CategoryCommentLength,
		},
		{
			Name:   fmt.Sprintf("a doc comment of %d words", limits.DocWords+1),
			Block:  commentCanary("func/Canary", true, canarySentence(limits.DocWords+1, 6)),
			Expect: CategoryCommentLength,
		},
		{
			Name:   fmt.Sprintf("a package doc comment of %d words", limits.PackageDocWords+1),
			Block:  commentCanary("package", true, canarySentence(limits.PackageDocWords+1, 12)),
			Expect: CategoryCommentLength,
		},
	}
}

// commentCanary is a block the comment layer builds from one comment holding
// text.
func commentCanary(subject string, doc bool, text string) *model.Block {
	f := comment.File{Language: "canary", Comments: []comment.Comment{{
		Subject: subject, Doc: doc, Style: comment.StyleLine, Runs: []model.Run{model.TextR(text)},
	}}}
	return f.Blocks()[0]
}

// canarySentence is one sentence of n words wrapped over lines lines.
func canarySentence(n, lines int) string {
	var b strings.Builder
	perLine := max(1, (n+lines-1)/lines)
	for i := range n {
		switch {
		case i == 0:
			b.WriteString("Canary")
		case i%perLine == 0:
			b.WriteString("\nword")
		default:
			b.WriteString(" word")
		}
	}
	b.WriteString(".")
	return b.String()
}

var (
	// dividerRule is a rule of box-drawing characters, or three or more of
	// - = ~ _ * #.
	dividerRule = regexp.MustCompile(`[─━═┄┈]{2,}|[-=~_*#]{3,}`)
	// listItemMarker opens a list item written as text.
	listItemMarker = regexp.MustCompile(`^(?:[-*+•]|\d+[.)])\s`)
	// headingMarker opens a Markdown heading.
	headingMarker = regexp.MustCompile(`^#{1,6}\s`)
)

// commentUnits divides a comment's hygiene flattening into the units of prose
// CommentSentences describes, each a half-open byte span that starts and ends
// on a character that is not a space or a tab.
func commentUnits(v *model.HygieneView) [][2]int {
	text := v.Text()
	var units [][2]int
	var cur [2]int
	open := false
	flush := func() {
		if open {
			units = append(units, cur)
			open = false
		}
	}
	for start := 0; ; {
		end := len(text)
		if i := strings.IndexByte(text[start:], '\n'); i >= 0 {
			end = start + i
		}
		first, last := trimSpace(text, start, end)
		line := text[first:last]
		switch {
		case onlyCodes(line):
			flush()
		case isDivider(line):
			flush()
			if t0, t1, ok := dividerTitle(text, first, last); ok {
				units = append(units, [2]int{t0, t1})
			}
		case headingMarker.MatchString(line):
			flush()
			units = append(units, [2]int{first, last})
		case opensUnit(v, first, line):
			flush()
			cur, open = [2]int{first, last}, true
		case open:
			cur[1] = last
		default:
			cur, open = [2]int{first, last}, true
		}
		if end == len(text) {
			break
		}
		start = end + 1
	}
	flush()
	return units
}

// onlyCodes reports a line that holds no text: nothing, or only inline codes.
func onlyCodes(line string) bool {
	for _, r := range line {
		if r != model.ObjectReplacement && !unicode.IsSpace(r) {
			return false
		}
	}
	return true
}

// isDivider reports a line that opens or closes with a rule.
func isDivider(line string) bool {
	loc := dividerRule.FindStringIndex(line)
	return loc != nil && (loc[0] == 0 || loc[1] == len(line))
}

// dividerTitle returns the span of the title between the rules of the divider
// line text[first:last], and false for a divider with no title.
func dividerTitle(text string, first, last int) (int, int, bool) {
	line := []byte(text[first:last])
	for _, loc := range dividerRule.FindAllIndex(line, -1) {
		for i := loc[0]; i < loc[1]; i++ {
			line[i] = ' '
		}
	}
	t0, t1 := trimSpace(string(line), 0, len(line))
	if t0 == t1 {
		return 0, 0, false
	}
	return first + t0, first + t1, true
}

// opensUnit reports a line, starting at byte offset at of the flattening, that
// opens a list item or a documentation tag.
func opensUnit(v *model.HygieneView, at int, line string) bool {
	if listItemMarker.MatchString(line) {
		return true
	}
	if !strings.HasPrefix(line, string(model.ObjectReplacement)) {
		return false
	}
	run, ok := v.Code(at)
	return ok && run.Ph != nil && (run.Ph.Type == comment.TypeListItem || strings.HasSuffix(run.Ph.SubType, ":tag"))
}

// proseWords counts the words in a stretch of a hygiene flattening, as
// CommentLimits defines a word.
func proseWords(text string) int {
	n, counted := 0, false
	for _, r := range text {
		switch {
		case unicode.IsSpace(r) || r == model.ObjectReplacement:
			counted = false
		case !counted && (unicode.IsLetter(r) || unicode.IsDigit(r)):
			n++
			counted = true
		}
	}
	return n
}

// readText returns a stretch of a hygiene flattening the way a reader sees it:
// each inline code as the source text it stands for, and each line break as a
// space.
func readText(v *model.HygieneView, start, end int) string {
	text := v.Text()
	var b strings.Builder
	for i := start; i < end; {
		r, size := utf8.DecodeRuneInString(text[i:])
		switch {
		case r == model.ObjectReplacement:
			if run, ok := v.Code(i); ok {
				b.WriteString(strings.ReplaceAll(codeData(run), "\n", " "))
			}
		case r == '\n':
			b.WriteByte(' ')
		default:
			b.WriteRune(r)
		}
		i += size
	}
	return b.String()
}

// codeData is the source text an inline code stands for: a placeholder's data,
// or the data of either half of a paired code. A subflow reference carries no
// text of its own, and reads as nothing.
func codeData(r model.Run) string {
	switch {
	case r.Ph != nil:
		return r.Ph.Data
	case r.PcOpen != nil:
		return r.PcOpen.Data
	case r.PcClose != nil:
		return r.PcClose.Data
	}
	return ""
}

// runeOffsets returns the byte offset of each rune of s, followed by len(s).
func runeOffsets(s string) []int {
	offsets := make([]int, 0, len(s)+1)
	for i := range s {
		offsets = append(offsets, i)
	}
	return append(offsets, len(s))
}

// trimSpace narrows the span [start, end) of text past the spaces, tabs and
// line breaks at either edge.
func trimSpace(text string, start, end int) (int, int) {
	for start < end && strings.IndexByte(" \t\r\n", text[start]) >= 0 {
		start++
	}
	for end > start && strings.IndexByte(" \t\r\n", text[end-1]) >= 0 {
		end--
	}
	return start, end
}
