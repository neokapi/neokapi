package redaction

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/neokapi/neokapi/core/model"
)

// DefaultPlaceholder is the rendering template used when none is configured.
// It supports two slots: {category} (title-cased category) and {n} (the
// 1-based occurrence number within the block).
const DefaultPlaceholder = "[REDACTED:{category}]"

// DefaultTokenPrefix prefixes the stable placeholder IDs (rdx1, rdx2, …)
// that key the vault and survive a translation roundtrip via the
// PlaceholderRun ID.
const DefaultTokenPrefix = "rdx"

// RedactOptions configures how matches are rendered into placeholders.
type RedactOptions struct {
	// Placeholder is the visible stand-in template shown to translators and
	// models. Empty means DefaultPlaceholder.
	Placeholder string
	// TokenPrefix prefixes the per-occurrence Ph ID. Empty means
	// DefaultTokenPrefix.
	TokenPrefix string
}

func (o RedactOptions) placeholder() string {
	if o.Placeholder == "" {
		return DefaultPlaceholder
	}
	return o.Placeholder
}

func (o RedactOptions) tokenPrefix() string {
	if o.TokenPrefix == "" {
		return DefaultTokenPrefix
	}
	return o.TokenPrefix
}

// Redacted records one replacement made within a block: the stable token
// (also the PlaceholderRun ID), the category, the visible placeholder string
// (Disp), and the original sensitive text. Callers persist these to a
// [Vault]; they are the only place the original survives.
type Redacted struct {
	Token    string
	Category string
	Disp     string
	Original string
}

// Redact rewrites a run sequence, replacing each match with a protected
// redaction PlaceholderRun. It returns the new runs, the replacements made, and
// the edit set applied to the flattened source text (RunEdits, in rune offsets) —
// the latter lets callers rebase surviving run-anchored overlays onto the
// redacted runs (see model.RemapOverlays). Each redaction replaces matched text
// with an inline placeholder run, which contributes nothing to the text
// flattening, so every edit has NewLen 0. Matches are taken relative to the
// flattened text of the sequence's TextRuns (the same coordinate space as
// [TextOf] and model.Block.SourceText), so detector offsets line up directly.
//
// A match whose span crosses a non-text (inline-code) run is skipped rather
// than risk dropping that code; in practice sensitive spans sit within plain
// text. Matches are normalized defensively before application.
func Redact(runs []model.Run, matches []Match, opts RedactOptions) ([]model.Run, []Redacted, []model.RunEdit) {
	matches = NormalizeMatches(matches)
	if len(runs) == 0 || len(matches) == 0 {
		return runs, nil, nil
	}

	tmpl := opts.placeholder()
	prefix := opts.tokenPrefix()

	// Count occurrences per category so the visible placeholder is unique
	// within the block: a category that appears once renders cleanly
	// ("[REDACTED:Person]"); a repeated category gets a per-category index
	// ("[REDACTED:Person]" + "#1", "#2"). Per-block uniqueness lets a
	// flattened (plain-text) roundtrip be restored by string match.
	catTotal := map[string]int{}
	for _, m := range matches {
		catTotal[m.Category]++
	}
	rd := &redactor{tmpl: tmpl, prefix: prefix, catTotal: catTotal, catSeen: map[string]int{}}

	var (
		out      []model.Run
		records  []Redacted
		applied  [][2]int // byte spans [start,end) of applied matches in flattened text
		globalAt int      // byte offset into the flattened text
		mi       int      // index into matches
		counter  int      // token counter
	)

	// Group consecutive TextRuns so a span split across adjacent text runs
	// (with no inline code between) is still matched. Inline runs flush the
	// current group and pass through verbatim.
	var (
		groupText  strings.Builder
		groupStart int
		groupOpen  bool
	)

	flush := func() {
		if !groupOpen {
			return
		}
		text := groupText.String()
		emitted, recs, spans, used := carveGroup(text, groupStart, matches[mi:], rd, &counter)
		out = append(out, emitted...)
		records = append(records, recs...)
		applied = append(applied, spans...)
		mi += used
		groupText.Reset()
		groupOpen = false
	}

	for _, r := range runs {
		if r.Text != nil {
			if !groupOpen {
				groupStart = globalAt
				groupOpen = true
			}
			groupText.WriteString(r.Text.Text)
			globalAt += len(r.Text.Text)
			continue
		}
		// Inline run: flush any pending text group, then pass through.
		flush()
		out = append(out, r)
	}
	flush()

	return out, records, editsFromSpans(model.RunsText(runs), applied)
}

// editsFromSpans converts the applied matches' byte spans in the flattened text
// into RunEdits in rune-offset coordinates. Each redacted span is replaced by an
// inline placeholder that contributes nothing to the flattening, so NewLen is 0.
func editsFromSpans(flat string, spans [][2]int) []model.RunEdit {
	if len(spans) == 0 {
		return nil
	}
	edits := make([]model.RunEdit, 0, len(spans))
	for _, sp := range spans {
		edits = append(edits, model.RunEdit{
			Start: utf8.RuneCountInString(byteSlice(flat, sp[0])),
			End:   utf8.RuneCountInString(byteSlice(flat, sp[1])),
		})
	}
	return edits
}

// byteSlice returns flat[:n] clamped to valid bounds.
func byteSlice(flat string, n int) string {
	if n <= 0 {
		return ""
	}
	if n >= len(flat) {
		return flat
	}
	return flat[:n]
}

// carveGroup splits one contiguous text group around the matches that fall
// entirely within it. start is the group's byte offset in the flattened
// text. It returns the emitted runs (text + placeholders), the records for
// replacements made, the flattened-text byte spans [start,end) of the matches it
// replaced, and how many leading entries of matches it consumed.
func carveGroup(text string, start int, matches []Match, rd *redactor, counter *int) ([]model.Run, []Redacted, [][2]int, int) {
	end := start + len(text)
	var (
		out     []model.Run
		records []Redacted
		applied [][2]int
		cursor  = start // byte offset in flattened text
		used    int
	)
	for _, m := range matches {
		if m.Start >= end {
			break // this and later matches belong to a later group
		}
		used++
		// Skip a match that starts before the cursor (already consumed) or
		// extends past this group (would cross an inline run boundary).
		if m.Start < cursor || m.End > end {
			continue
		}
		if m.Start > cursor {
			out = append(out, textRun(text[cursor-start:m.Start-start]))
		}
		token, disp := rd.next(m.Category, counter)
		out = append(out, redactionRun(token, m.Category, disp))
		records = append(records, Redacted{Token: token, Category: m.Category, Disp: disp, Original: m.Original})
		applied = append(applied, [2]int{m.Start, m.End})
		cursor = m.End
	}
	if cursor < end {
		out = append(out, textRun(text[cursor-start:]))
	}
	return out, records, applied, used
}

// redactor generates a token and a visible placeholder per match, keeping
// the visible string unique within a block.
type redactor struct {
	tmpl     string
	prefix   string
	catTotal map[string]int // total matches per category in the block
	catSeen  map[string]int // running count per category
}

func (rd *redactor) next(category string, counter *int) (token, disp string) {
	*counter++
	token = fmt.Sprintf("%s%d", rd.prefix, *counter)
	rd.catSeen[category]++
	disp = renderPlaceholder(rd.tmpl, category, *counter)
	// Disambiguate repeated categories unless the template already varies by
	// occurrence number.
	if !strings.Contains(rd.tmpl, "{n}") && rd.catTotal[category] > 1 {
		disp = fmt.Sprintf("%s#%d", disp, rd.catSeen[category])
	}
	return token, disp
}

// RestoreText restores originals into a run sequence by string-replacing each
// entry's visible placeholder (Disp) with its Original in TextRuns. This is
// the fallback for formats that flatten the redaction PlaceholderRun to text
// on write (e.g. JSON, and XLIFF for unknown inline types) — the structure is
// gone, but the unique visible token survives and can be matched. It returns
// the rewritten runs and the count of placeholders restored.
func RestoreText(runs []model.Run, entries []RedactedValue) ([]model.Run, int) {
	if len(runs) == 0 || len(entries) == 0 {
		return runs, 0
	}
	restored := 0
	out := make([]model.Run, len(runs))
	copy(out, runs)
	for i := range out {
		if out[i].Text == nil {
			continue
		}
		s := out[i].Text.Text
		for _, e := range entries {
			if e.Disp == "" {
				continue
			}
			if c := strings.Count(s, e.Disp); c > 0 {
				s = strings.ReplaceAll(s, e.Disp, e.Original)
				restored += c
			}
		}
		if s != out[i].Text.Text {
			out[i] = textRun(s)
		}
	}
	return out, restored
}

// Restore replaces redaction placeholders in runs with TextRuns carrying
// their original values, looked up by the placeholder ID (token) via get.
// The vault is the authority: any placeholder whose ID resolves to a stored
// original is restored, whether or not its Type still carries the
// "redaction:" prefix (an XLIFF roundtrip may drop the Type but always
// preserves the <ph> id). Placeholders not found in the vault are left
// untouched. It returns the rewritten runs and the count restored.
func Restore(runs []model.Run, get func(token string) (string, bool)) ([]model.Run, int) {
	if len(runs) == 0 {
		return runs, 0
	}
	out := make([]model.Run, 0, len(runs))
	restored := 0
	for _, r := range runs {
		if r.Ph != nil {
			if original, found := get(r.Ph.ID); found {
				out = append(out, textRun(original))
				restored++
				continue
			}
		}
		out = append(out, r)
	}
	return out, restored
}

// RestorePlan restores originals into a run sequence in one walk — by
// placeholder ID (via get, for structure-preserving carriers) and by visible
// token text (via entries' Disp, for carriers that flattened the placeholder
// on write) — and returns the rewritten runs, the count restored, and the
// structured edits applied to the flattened text (model.RunEdit, rune
// offsets, ascending). The edits let the framework applier rebase surviving
// run-anchored overlays across the restore (AD-006): a placeholder restore is
// a pure insertion at the placeholder's position (it contributed nothing to
// the flattening), a token restore replaces the token's span.
func RestorePlan(runs []model.Run, get func(token string) (string, bool), entries []RedactedValue) ([]model.Run, int, []model.RunEdit) {
	if len(runs) == 0 {
		return runs, 0, nil
	}
	var (
		out      []model.Run
		edits    []model.RunEdit
		restored int
		at       int // rune offset into the old flattened text
	)
	for _, r := range runs {
		switch {
		case r.Ph != nil:
			if get != nil {
				if original, found := get(r.Ph.ID); found {
					out = append(out, textRun(original))
					edits = append(edits, model.RunEdit{Start: at, End: at, NewLen: utf8.RuneCountInString(original)})
					restored++
					continue
				}
			}
			out = append(out, r)
		case r.Text != nil:
			text := r.Text.Text
			var nb strings.Builder
			cursor := 0 // byte offset into text
			for cursor < len(text) {
				b, e, original, ok := nextTokenMatch(text, cursor, entries)
				if !ok {
					break
				}
				nb.WriteString(text[cursor:b])
				nb.WriteString(original)
				edits = append(edits, model.RunEdit{
					Start:  at + utf8.RuneCountInString(text[:b]),
					End:    at + utf8.RuneCountInString(text[:e]),
					NewLen: utf8.RuneCountInString(original),
				})
				restored++
				cursor = e
			}
			if cursor == 0 {
				out = append(out, r)
			} else {
				nb.WriteString(text[cursor:])
				out = append(out, textRun(nb.String()))
			}
			at += utf8.RuneCountInString(text)
		default:
			out = append(out, r)
		}
	}
	if restored == 0 {
		return runs, 0, nil
	}
	return out, restored, edits
}

// nextTokenMatch finds the earliest occurrence at or after byte offset from of
// any entry's visible token (Disp) in text, returning its byte span and the
// original to restore. Ties at the same position prefer the longest token.
func nextTokenMatch(text string, from int, entries []RedactedValue) (start, end int, original string, ok bool) {
	best := -1
	for _, e := range entries {
		if e.Disp == "" {
			continue
		}
		i := strings.Index(text[from:], e.Disp)
		if i < 0 {
			continue
		}
		pos := from + i
		if best == -1 || pos < best || (pos == best && len(e.Disp) > end-start) {
			best, start, end, original = pos, pos, pos+len(e.Disp), e.Original
		}
	}
	return start, end, original, best != -1
}

// TextOf returns the flattened plain text of a run sequence using only
// TextRun content — the coordinate space detectors and [Redact] share. It
// mirrors model.Block.SourceText for a single run slice.
func TextOf(runs []model.Run) string {
	var b strings.Builder
	for _, r := range runs {
		if r.Text != nil {
			b.WriteString(r.Text.Text)
		}
	}
	return b.String()
}

func textRun(s string) model.Run {
	return model.Run{Text: &model.TextRun{Text: s}}
}

// redactionRun builds a protected placeholder for a redacted span. The
// original text is deliberately absent: Data and Equiv carry only the
// visible stand-in, and the constraints forbid deletion or duplication so a
// translator (human or model) keeps it intact.
func redactionRun(token, category, disp string) model.Run {
	return model.Run{Ph: &model.PlaceholderRun{
		ID:    token,
		Type:  PlaceholderType(category),
		Data:  disp,
		Equiv: disp,
		Disp:  disp,
		Constraints: &model.RunConstraints{
			Deletable:   false,
			Cloneable:   false,
			Reorderable: true,
		},
	}}
}

// renderPlaceholder expands the template's {category} (title-cased) and {n}
// (occurrence number) slots.
func renderPlaceholder(tmpl, category string, n int) string {
	out := strings.ReplaceAll(tmpl, "{category}", titleCase(category))
	out = strings.ReplaceAll(out, "{n}", strconv.Itoa(n))
	return out
}

func titleCase(s string) string {
	if s == "" {
		return s
	}
	r := []rune(s)
	r[0] = unicode.ToUpper(r[0])
	return string(r)
}
