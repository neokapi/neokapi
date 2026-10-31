package screens

import (
	"crypto/sha256"
	"encoding/hex"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/neokapi/neokapi/core/model"
)

// CheckedFinding is one finding of a check over a file's blocks, in the
// terms the Reader places it by. The desktop and the server each fill one
// from their own check runs.
type CheckedFinding struct {
	BlockID string
	// Field is "source" or "target", and Locale the language of the side
	// the finding is about.
	Field  string
	Locale string
	// Check is the family that raised the finding, as `kapi check` names it:
	// terms, voice, placeholder, dnt, or a comment check's name.
	Check        string
	Category     string
	Rule         string
	OriginalText string
	Message      string
	// Replacement is the wording a term rule asks for, and Term the word the
	// rule is about.
	Replacement string
	Term        string
	Fails       bool
	Suggested   bool
	Position    model.Anchor
	// Fix is the change operation that applies Replacement, kapi.change/v1
	// JSON; empty for a finding with no fix.
	Fix string
}

// ReaderPlacement is a file's blocks with each finding set on its block, and
// the fixes the findings carry.
type ReaderPlacement struct {
	Blocks   []ReaderBlockDTO
	Document []ReaderFindingDTO
	Fixable  int
	// Fixes maps a finding's id to its fix; Order lists the fixable findings
	// in file order.
	Fixes map[string]string
	Order []string
}

// PlaceFindings sets each finding on the block it names, in the order the
// findings are given, and keeps each one's fix. A finding that names no
// block of the file is a document finding.
func PlaceFindings(blocks []*model.Block, findings []CheckedFinding) ReaderPlacement {
	out := ReaderPlacement{
		Blocks:   make([]ReaderBlockDTO, 0, len(blocks)),
		Document: []ReaderFindingDTO{},
		Fixes:    map[string]string{},
	}
	index := make(map[string]int, len(blocks))
	for _, b := range blocks {
		index[b.ID] = len(out.Blocks)
		out.Blocks = append(out.Blocks, ReaderBlockDTO{ID: b.ID, Runs: b.SourceRuns(), Findings: []ReaderFindingDTO{}})
	}
	seen := map[string]int{}
	for _, cf := range findings {
		f := ReaderFindingDTO{
			BlockID:   cf.BlockID,
			Position:  cf.Position,
			Kind:      ReaderKind(cf),
			Label:     ReaderLabel(cf),
			Fails:     cf.Fails,
			Suggested: cf.Suggested,
			Fixable:   cf.Fix != "",
		}
		if cf.Field == "target" {
			f.Locale = cf.Locale
		}
		key := FindingKey(cf)
		seen[key]++
		f.ID = key + "-" + strconv.Itoa(seen[key])
		if f.Fixable {
			out.Fixes[f.ID] = cf.Fix
			out.Fixable++
		}
		i, ok := index[cf.BlockID]
		if !ok || cf.BlockID == "" {
			out.Document = append(out.Document, f)
			continue
		}
		// A finding on a translation is anchored to the source runs too
		// (core/check), at the words the translation lost or changed.
		f.Span = ReaderSpanOf(out.Blocks[i].Runs, cf.Position)
		out.Blocks[i].Findings = append(out.Blocks[i].Findings, f)
	}
	for i := range out.Blocks {
		slices.SortStableFunc(out.Blocks[i].Findings, func(x, y ReaderFindingDTO) int {
			return spanStart(x.Span) - spanStart(y.Span)
		})
		for _, f := range out.Blocks[i].Findings {
			if f.Fixable {
				out.Order = append(out.Order, f.ID)
			}
		}
	}
	return out
}

// FindingKey identifies a finding by what it is about, not by where in the
// block it sits, so the same finding keeps its id while a fix elsewhere in
// the block moves its words.
func FindingKey(cf CheckedFinding) string {
	sum := sha256.Sum256([]byte(strings.Join([]string{
		cf.BlockID, cf.Field, cf.Locale, cf.Check, cf.Category, cf.Rule, cf.OriginalText, cf.Message,
	}, "\x00")))
	return hex.EncodeToString(sum[:6])
}

// spanStart orders findings with a span by where it starts, and the others
// after them.
func spanStart(s *ReaderSpan) int {
	if s == nil {
		return 1 << 30
	}
	return s.Start
}

// ReaderSpanOf is the code-point range of the block's own text that a run
// range over its top-level runs covers. A range inside a plural or a select,
// or one the runs do not hold, has none.
func ReaderSpanOf(runs []model.Run, a model.Anchor) *ReaderSpan {
	if a.Kind != model.AnchorRange || len(a.Path) > 0 || !a.InBounds(runs) {
		return nil
	}
	start, end := textOffset(runs, a.Start), textOffset(runs, a.End)
	if end <= start {
		return nil
	}
	return &ReaderSpan{Start: start, End: end}
}

// textOffset is the code-point offset of p into the text the runs hold
// themselves, in which every run but a text run has no width.
func textOffset(runs []model.Run, p model.RunPos) int {
	n := 0
	for i := 0; i < p.Run && i < len(runs); i++ {
		if runs[i].Text != nil {
			n += utf8.RuneCountInString(runs[i].Text.Text)
		}
	}
	if p.Run < len(runs) && runs[p.Run].Text != nil {
		n += p.Offset
	}
	return n
}

// ReaderKind marks a finding by the check that raised it: a term rule about
// a spelling is a name, any other term rule a word, a voice rule tone, and
// every other check its own family.
func ReaderKind(cf CheckedFinding) string {
	switch cf.Check {
	case "terms":
		if cf.Replacement != "" && cf.Term != "" && SpellingKey(cf.Replacement) == SpellingKey(cf.Term) {
			return "name"
		}
		return "word"
	case "voice":
		return "tone"
	case "":
		return cf.Category
	}
	return cf.Check
}

// ReaderLabel is the line under a finding: the wording a term rule asks for
// and the word it found, or the check's message.
func ReaderLabel(cf CheckedFinding) string {
	if cf.Check == "terms" && cf.Replacement != "" {
		found := cf.OriginalText
		if found == "" {
			found = cf.Term
		}
		if found != "" {
			return cf.Replacement + ", not " + found
		}
	}
	return cf.Message
}

// SpellingKey folds a form to compare spellings: lower case, with spaces,
// hyphens, underscores and dots taken out.
func SpellingKey(form string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(form) {
		switch r {
		case ' ', '-', '_', '.', ' ':
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}
