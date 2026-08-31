package tools

import (
	"errors"
	"slices"
	"strings"

	"github.com/neokapi/neokapi/core/check"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/tool"
)

// Zero-width characters that can be removed.
const (
	zeroWidthSpace     = '\u200B'
	zeroWidthNonJoiner = '\u200C'
	zeroWidthJoiner    = '\u200D'
	zeroWidthNoBreak   = '\uFEFF' // BOM / zero-width no-break space
)

// WhitespaceCorrectConfig holds configuration for the whitespace correction tool.
type WhitespaceCorrectConfig struct {
	TargetLocale          model.LocaleID `json:"targetLocale,omitempty"          schema:"title=Target Locale,description=Target locale for processing"`                                             // Required
	NormalizeSpaces       bool           `json:"normalizeSpaces,omitempty"       schema:"title=Normalize Spaces,description=Collapse multiple spaces to a single space,default=true"`               // Collapse multiple spaces to single (default: true)
	TrimLeading           bool           `json:"trimLeading,omitempty"           schema:"title=Trim Leading Whitespace,description=Remove leading whitespace from target text"`                     // Remove leading whitespace (default: false)
	TrimTrailing          bool           `json:"trimTrailing,omitempty"          schema:"title=Trim Trailing Whitespace,description=Remove trailing whitespace from target text"`                   // Remove trailing whitespace (default: false)
	MatchSourceWhitespace bool           `json:"matchSourceWhitespace,omitempty" schema:"title=Match Source Whitespace,description=Copy source leading/trailing whitespace to target,default=true"` // Copy source leading/trailing whitespace to target (default: true)
	RemoveZeroWidthChars  bool           `json:"removeZeroWidthChars,omitempty"  schema:"title=Remove Zero-Width Characters,description=Remove zero-width spaces and joiners,default=true"`         // Remove zero-width spaces/joiners (default: true)

	// Punctuation-specific correction toggles (CJK full-width/ASCII conversion).
	CorrectFullStop    bool `json:"correctFullStop,omitempty"    schema:"title=Full Stop,description=Correct whitespace after full stops (periods),default=true,group=punctuation"`
	CorrectComma       bool `json:"correctComma,omitempty"       schema:"title=Comma,description=Correct whitespace after commas,default=true,group=punctuation"`
	CorrectExclamation bool `json:"correctExclamation,omitempty" schema:"title=Exclamation Point,description=Correct whitespace after exclamation marks,default=true,group=punctuation"`
	CorrectQuestion    bool `json:"correctQuestion,omitempty"    schema:"title=Question Mark,description=Correct whitespace after question marks,default=true,group=punctuation"`

	// Whitespace type toggles for which whitespace characters to remove.
	IncludeVerticalWS   bool `json:"includeVerticalWS,omitempty"   schema:"title=Vertical White Space,description=Include vertical whitespace (line feeds and carriage returns) in corrections,default=true,group=whitespace-types"`
	IncludeHorizontalWS bool `json:"includeHorizontalWS,omitempty" schema:"title=Horizontal Tabs,description=Include horizontal tab characters in corrections,default=true,group=whitespace-types"`
}

// ToolName returns the tool name this config applies to.
func (c *WhitespaceCorrectConfig) ToolName() string { return "whitespace-correct" }

// Reset restores default values.
func (c *WhitespaceCorrectConfig) Reset() {
	c.TargetLocale = ""
	c.NormalizeSpaces = true
	c.TrimLeading = false
	c.TrimTrailing = false
	c.MatchSourceWhitespace = true
	c.RemoveZeroWidthChars = true
	c.CorrectFullStop = true
	c.CorrectComma = true
	c.CorrectExclamation = true
	c.CorrectQuestion = true
	c.IncludeVerticalWS = true
	c.IncludeHorizontalWS = true
}

// Validate checks configuration validity.
func (c *WhitespaceCorrectConfig) Validate() error {
	if c.TargetLocale.IsEmpty() {
		return errors.New("whitespace-correct: TargetLocale is required")
	}
	return nil
}

// NewWhitespaceCorrectConfig creates a WhitespaceCorrectConfig at its
// documented defaults for the given target locale.
func NewWhitespaceCorrectConfig(targetLocale model.LocaleID) *WhitespaceCorrectConfig {
	cfg := &WhitespaceCorrectConfig{}
	cfg.Reset()
	cfg.TargetLocale = targetLocale
	return cfg
}

// NewWhitespaceCorrectFromConfig creates a whitespace-correct tool from a
// config map. Without this the registry has only the zero-arg factory, and
// NewToolWithConfig then throws the step's config away without a word — the
// tool declares eleven knobs and honoured none of them, on a locale pinned to
// English regardless of what the run was for.
func NewWhitespaceCorrectFromConfig(config map[string]any, targetLang string) (tool.Tool, error) {
	cfg := NewWhitespaceCorrectConfig(model.LocaleID(targetLang))
	if err := applyStepConfig("whitespace-correct", config, cfg, targetLang, setWhitespaceCorrectLocale); err != nil {
		return nil, err
	}
	return NewWhitespaceCorrectTool(cfg), nil
}

func setWhitespaceCorrectLocale(c *WhitespaceCorrectConfig, loc model.LocaleID) {
	c.TargetLocale = loc
}

// NewWhitespaceCorrectTool creates a tool that normalizes and fixes whitespace
// issues in target translations.
func NewWhitespaceCorrectTool(cfg *WhitespaceCorrectConfig) *tool.BaseTool {
	t := &tool.BaseTool{
		ToolName:        "whitespace-correct",
		ToolDescription: "Normalizes and fixes whitespace issues in translations",
		Cfg:             cfg,
	}
	// Translate: whitespace-correct fixes target content; source is read-only.
	//
	// Every correction below is applied to the target's *runs*, one text run at a
	// time, and the inline codes are copied through untouched. The tool used to
	// round-trip the target through TargetText() / SetTargetText(), which replaced
	// its whole run sequence with a single text run: every placeholder, paired
	// code and plural construct in the target was destroyed by a tool asked only
	// to tidy whitespace. That is the recycle-path loss of #1456 as a *write*, and
	// nothing downstream could tell the difference — the codes were simply gone.
	t.Produce = func(v tool.VariantView) error {
		if !v.Translatable() {
			return nil
		}

		conf := t.Cfg.(*WhitespaceCorrectConfig)
		if conf.TargetLocale.IsEmpty() || !v.HasTarget(conf.TargetLocale) {
			return nil
		}

		runs := v.TargetRuns(conf.TargetLocale)

		if conf.RemoveZeroWidthChars {
			runs = mapTextRuns(runs, removeZeroWidthChars)
		}

		if conf.NormalizeSpaces {
			runs = normalizeSpacesRuns(runs)
		}

		if conf.TrimLeading {
			runs = trimLeadingRuns(runs)
		}

		if conf.TrimTrailing {
			runs = trimTrailingRuns(runs)
		}

		if conf.MatchSourceWhitespace {
			runs = matchSourceWhitespaceRuns(v.SourceRuns(), runs)
		}

		// Punctuation-specific whitespace correction: remove trailing whitespace
		// after specific punctuation marks (useful for CJK targets).
		runs = correctPunctuationWhitespaceRuns(runs, conf)

		v.SetTargetRuns(conf.TargetLocale, runs)
		return nil
	}
	return t
}

// edgeWhitespace is the set of whitespace characters this tool trims and copies.
const edgeWhitespace = " \t\n\r"

// mapTextRuns returns a copy of runs with fn applied to each text run's text.
// Inline codes are copied through untouched; Plural / Select branches are mapped
// recursively, since each branch is text of its own. The view hands back the
// block's live run slice, so nothing here mutates in place.
//
// fn must be position-independent — it sees one text run at a time and nothing
// of its neighbours. A correction whose meaning depends on what sits next to
// what needs [textRunScan] instead.
func mapTextRuns(runs []model.Run, fn func(string) string) []model.Run {
	out := make([]model.Run, len(runs))
	for i, r := range runs {
		switch r.Kind() {
		case model.RunKindText:
			out[i] = model.TextR(fn(r.Text.Text))
		case model.RunKindPlural:
			forms := make(map[model.PluralForm][]model.Run, len(r.Plural.Forms))
			for f, form := range r.Plural.Forms {
				forms[f] = mapTextRuns(form, fn)
			}
			p := *r.Plural
			p.Forms = forms
			out[i] = model.Run{Plural: &p}
		case model.RunKindSelect:
			cases := make(map[string][]model.Run, len(r.Select.Cases))
			for k, c := range r.Select.Cases {
				cases[k] = mapTextRuns(c, fn)
			}
			s := *r.Select
			s.Cases = cases
			out[i] = model.Run{Select: &s}
		default:
			out[i] = r
		}
	}
	return out
}

// textRunScan applies a *stateful* text correction across a Run sequence. The
// state carries across adjacent text runs — nothing separates those, so they are
// one stretch of text — and resets at every inline code, because a code is a real
// boundary (#1465). It resets at each Plural / Select branch too, each branch
// being an independent stretch.
//
// Both halves of that matter. Carrying state across the join is what lets
// `[text "Bonjour "][text " monde"]` collapse to one space, a defect the runs
// genuinely contain. Resetting at a code is what stops
// `[text "Bonjour "][ph][text " monde"]` from collapsing, where the two spaces
// separate a word from the placeholder between them and neither is redundant.
func textRunScan(runs []model.Run, newState func() func(string) string) []model.Run {
	fn := newState()
	out := make([]model.Run, len(runs))
	for i, r := range runs {
		switch r.Kind() {
		case model.RunKindText:
			out[i] = model.TextR(fn(r.Text.Text))
		case model.RunKindPlural:
			forms := make(map[model.PluralForm][]model.Run, len(r.Plural.Forms))
			for f, form := range r.Plural.Forms {
				forms[f] = textRunScan(form, newState)
			}
			p := *r.Plural
			p.Forms = forms
			out[i] = model.Run{Plural: &p}
			fn = newState()
		case model.RunKindSelect:
			cases := make(map[string][]model.Run, len(r.Select.Cases))
			for k, c := range r.Select.Cases {
				cases[k] = textRunScan(c, newState)
			}
			s := *r.Select
			s.Cases = cases
			out[i] = model.Run{Select: &s}
			fn = newState()
		default:
			out[i] = r
			fn = newState() // an inline code is a boundary: start a fresh stretch
		}
	}
	return out
}

// removeZeroWidthChars strips zero-width Unicode characters from text.
func removeZeroWidthChars(text string) string {
	var b strings.Builder
	b.Grow(len(text))
	for _, r := range text {
		switch r {
		case zeroWidthSpace, zeroWidthNonJoiner, zeroWidthJoiner, zeroWidthNoBreak:
			continue
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// normalizeSpacesRuns collapses runs of multiple spaces into a single space
// across the target's runs. A space either side of an inline code survives: the
// code sits between them, so they separate a word from a placeholder rather than
// doubling each other. Two spaces spanning the join of adjacent text runs do
// collapse — nothing separates those. Whitespace that indents a line
// survives whole: it continues the source's line structure, and collapsing a
// wrapped list item's two-space continuation indent rewrites the markup around
// the translation. That is the same boundary rule `hygiene.double-spaces`
// reports on (#1465), so the rule and the correction agree on what a double
// space is.
func normalizeSpacesRuns(runs []model.Run) []model.Run {
	return textRunScan(runs, func() func(string) string {
		prevSpace := false
		indent := false
		return func(text string) string {
			var b strings.Builder
			b.Grow(len(text))
			for _, r := range text {
				switch r {
				case ' ':
					if prevSpace && !indent {
						continue
					}
					prevSpace = true
				case '\n':
					prevSpace = false
					indent = true
				default:
					prevSpace = false
					indent = false
				}
				b.WriteRune(r)
			}
			return b.String()
		}
	})
}

// trimLeadingRuns removes whitespace from the content's leading edge. It drops
// whole leading text runs that are nothing but whitespace and trims the first run
// that has content; a leading inline code stops it, because a code *is* the
// content's leading edge and the whitespace after it is a separator, not stray
// edge whitespace.
func trimLeadingRuns(runs []model.Run) []model.Run {
	for i, r := range runs {
		if r.Kind() != model.RunKindText {
			return runs[i:]
		}
		trimmed := strings.TrimLeft(r.Text.Text, edgeWhitespace)
		if trimmed == "" {
			continue // whitespace-only run at the edge: drop it and keep looking
		}
		if i == 0 && trimmed == r.Text.Text {
			return runs // nothing to trim
		}
		out := make([]model.Run, 0, len(runs)-i)
		out = append(out, model.TextR(trimmed))
		return append(out, runs[i+1:]...)
	}
	return nil // every run was whitespace-only text
}

// trimTrailingRuns is the trailing-edge counterpart of [trimLeadingRuns].
func trimTrailingRuns(runs []model.Run) []model.Run {
	for i, r := range slices.Backward(runs) {

		if r.Kind() != model.RunKindText {
			return runs[:i+1]
		}
		trimmed := strings.TrimRight(r.Text.Text, edgeWhitespace)
		if trimmed == "" {
			continue
		}
		if i == len(runs)-1 && trimmed == r.Text.Text {
			return runs
		}
		out := make([]model.Run, 0, i+1)
		out = append(out, runs[:i]...)
		return append(out, model.TextR(trimmed))
	}
	return nil
}

// correctPunctuationWhitespaceRuns removes whitespace immediately following
// specific punctuation marks, based on configuration. This is primarily
// useful when translating from space-delimited to non-space-delimited languages.
//
// The "immediately following" carries across the join of adjacent text runs and
// resets at an inline code: in `[text "Hello."][ph][text " world"]` the space
// follows the placeholder, not the full stop, so it stays.
func correctPunctuationWhitespaceRuns(runs []model.Run, conf *WhitespaceCorrectConfig) []model.Run {
	punctSet := configuredPunctuation(conf)
	if len(punctSet) == 0 {
		return runs
	}
	return textRunScan(runs, func() func(string) string {
		// skipping is true while consuming the whitespace that follows a
		// configured punctuation mark.
		skipping := false
		return func(text string) string {
			var b strings.Builder
			b.Grow(len(text))
			for _, r := range text {
				if skipping {
					if isConfiguredWhitespace(r, conf) {
						continue
					}
					skipping = false
				}
				b.WriteRune(r)
				if punctSet[r] {
					skipping = true
				}
			}
			return b.String()
		}
	})
}

// configuredPunctuation returns the set of punctuation marks whose following
// whitespace the config asks to remove, ASCII and fullwidth alike.
func configuredPunctuation(conf *WhitespaceCorrectConfig) map[rune]bool {
	var punctChars []rune
	if conf.CorrectFullStop {
		punctChars = append(punctChars, '.', '\uFF0E') // ASCII + fullwidth
	}
	if conf.CorrectComma {
		punctChars = append(punctChars, ',', '\uFF0C')
	}
	if conf.CorrectExclamation {
		punctChars = append(punctChars, '!', '\uFF01')
	}
	if conf.CorrectQuestion {
		punctChars = append(punctChars, '?', '\uFF1F')
	}
	if len(punctChars) == 0 {
		return nil
	}
	punctSet := make(map[rune]bool, len(punctChars))
	for _, r := range punctChars {
		punctSet[r] = true
	}
	return punctSet
}

// isConfiguredWhitespace checks if a rune is whitespace that the config
// includes for removal.
func isConfiguredWhitespace(r rune, conf *WhitespaceCorrectConfig) bool {
	switch r {
	case ' ':
		return true
	case '\t':
		return conf.IncludeHorizontalWS
	case '\n', '\r', '\v', '\f':
		return conf.IncludeVerticalWS
	default:
		return false
	}
}

// matchSourceWhitespaceRuns copies the source's leading and trailing whitespace
// onto the target, preserving the target's runs.
//
// Which whitespace the source *has* is a content-boundary question, so it reads
// the run-aware flattening (check.HygieneText) and not SourceText(). Under
// SourceText() the founder's `[ph{p.price}][text " each"]` read as a leading
// space and this tool copied that phantom space onto every target — the same
// false judgement as the `hygiene.leading-whitespace` report #1455 fixed, except
// written into the content instead of printed. On the shape flattening the
// leading edge is the placeholder, so there is nothing to copy.
func matchSourceWhitespaceRuns(sourceRuns, targetRuns []model.Run) []model.Run {
	shape := check.HygieneText(sourceRuns)
	leading := check.LeadingWhitespace(shape)
	trailing := check.TrailingWhitespace(shape)

	out := trimTrailingRuns(trimLeadingRuns(targetRuns))
	return padEdgeWhitespace(out, leading, trailing)
}

// padEdgeWhitespace prepends leading and appends trailing whitespace to a Run
// sequence: merged into the edge text run when there is one, and as a new text
// run when the edge is an inline code — so an edge code stays a code.
func padEdgeWhitespace(runs []model.Run, leading, trailing string) []model.Run {
	if leading == "" && trailing == "" {
		return runs
	}
	out := slices.Clone(runs)
	if leading != "" {
		if len(out) > 0 && out[0].Kind() == model.RunKindText {
			out[0] = model.TextR(leading + out[0].Text.Text)
		} else {
			out = append([]model.Run{model.TextR(leading)}, out...)
		}
	}
	if trailing != "" {
		if n := len(out); n > 0 && out[n-1].Kind() == model.RunKindText {
			out[n-1] = model.TextR(out[n-1].Text.Text + trailing)
		} else {
			out = append(out, model.TextR(trailing))
		}
	}
	return out
}
