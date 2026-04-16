package model

import "strings"

// Block is the primary translatable content unit.
// Source and target segments live directly on the Block.
type Block struct {
	ID                 string
	Name               string
	Type               string
	MimeType           string
	Translatable       bool
	SourceLocale       LocaleID // locale of the source segments (set by reader)
	Skeleton           *Skeleton
	Source             []*Segment
	Targets            map[LocaleID][]*Segment
	Properties         map[string]string
	Annotations        map[string]Annotation
	Identity           *BlockIdentity // Content-addressable hash for deduplication
	ContentRef         *ContentRef    // Link to external connector source
	DisplayHint        *DisplayHint   // UI rendering guidance
	PreserveWhitespace bool           // Whether whitespace is significant in this block
	IsReferent         bool           // Whether this block is referenced by a skeleton
}

// ResourceID returns the Block's unique identifier.
func (b *Block) ResourceID() string { return b.ID }

// SourceText returns the plain text of all source segments
// concatenated (TextRun content only — inline-code runs contribute
// nothing).
func (b *Block) SourceText() string {
	var buf strings.Builder
	for _, seg := range b.Source {
		buf.WriteString(seg.Text())
	}
	return buf.String()
}

// SetSourceText replaces all source content with a single
// unsegmented TextRun.
func (b *Block) SetSourceText(text string) {
	b.Source = []*Segment{{ID: "s1", Runs: []Run{{Text: &TextRun{Text: text}}}}}
}

// HasTarget returns true if target segments exist for the given locale.
func (b *Block) HasTarget(locale LocaleID) bool {
	segs, ok := b.Targets[locale]
	return ok && len(segs) > 0
}

// TargetText returns the plain text of all target segments for the given locale.
func (b *Block) TargetText(locale LocaleID) string {
	segs, ok := b.Targets[locale]
	if !ok {
		return ""
	}
	var buf strings.Builder
	for _, seg := range segs {
		buf.WriteString(seg.Text())
	}
	return buf.String()
}

// SetTargetText sets the target text for a locale as a single
// unsegmented TextRun.
func (b *Block) SetTargetText(locale LocaleID, text string) {
	if b.Targets == nil {
		b.Targets = make(map[LocaleID][]*Segment)
	}
	b.Targets[locale] = []*Segment{{ID: "s1", Runs: []Run{{Text: &TextRun{Text: text}}}}}
}

// Text returns the plain text for a locale. If the locale matches SourceLocale,
// returns the source text. Otherwise returns the target text for that locale.
// Returns empty string if the locale has no segments. This provides uniform
// access regardless of whether a locale is source or target.
func (b *Block) Text(locale LocaleID) string {
	if locale == b.SourceLocale && b.SourceLocale != "" {
		return b.SourceText()
	}
	return b.TargetText(locale)
}

// SetText writes text for a locale. If the locale matches SourceLocale,
// writes to source. Otherwise writes to targets.
func (b *Block) SetText(locale LocaleID, text string) {
	if locale == b.SourceLocale && b.SourceLocale != "" {
		b.SetSourceText(text)
		return
	}
	b.SetTargetText(locale, text)
}

// HasLocale reports whether the Block has segments for a locale,
// checking both source and targets.
func (b *Block) HasLocale(locale LocaleID) bool {
	if locale == b.SourceLocale && b.SourceLocale != "" {
		return len(b.Source) > 0
	}
	return b.HasTarget(locale)
}

// WordCount returns the number of words in the source text.
// Words are sequences of non-whitespace characters. Coded text
// markers are stripped automatically by SourceText().
func (b *Block) WordCount() int {
	text := strings.TrimSpace(b.SourceText())
	if text == "" {
		return 0
	}
	return len(strings.Fields(text))
}

// NewBlock creates a new translatable Block with the given ID and
// plain source text. Produces a single TextRun segment.
func NewBlock(id, text string) *Block {
	return &Block{
		ID:           id,
		Translatable: true,
		Source:       []*Segment{{ID: "s1", Runs: []Run{{Text: &TextRun{Text: text}}}}},
		Targets:      make(map[LocaleID][]*Segment),
		Properties:   make(map[string]string),
		Annotations:  make(map[string]Annotation),
	}
}

// NewRunsBlock creates a Block whose source is a single segment
// populated from the given Run sequence. Companion to NewBlock for
// readers that emit Runs natively.
func NewRunsBlock(id string, runs []Run) *Block {
	return &Block{
		ID:           id,
		Translatable: true,
		Source:       []*Segment{NewRunsSegment("s1", runs)},
		Targets:      make(map[LocaleID][]*Segment),
		Properties:   make(map[string]string),
		Annotations:  make(map[string]Annotation),
	}
}

// SourceRuns returns the Block's source content as a flat Run
// sequence (concatenated across segments).
func (b *Block) SourceRuns() []Run {
	var out []Run
	for _, s := range b.Source {
		out = append(out, s.Runs...)
	}
	return out
}

// TargetRuns returns the Block's target content for a given locale
// as a flat Run sequence.
func (b *Block) TargetRuns(locale LocaleID) []Run {
	segs, ok := b.Targets[locale]
	if !ok {
		return nil
	}
	var out []Run
	for _, s := range segs {
		out = append(out, s.Runs...)
	}
	return out
}

// SetSourceRuns replaces the Block's source with a single segment
// carrying the given Run sequence. Companion to SetSourceText.
func (b *Block) SetSourceRuns(runs []Run) {
	b.Source = []*Segment{NewRunsSegment("s1", runs)}
}

// SetTargetRuns replaces the Block's target content for a locale
// with a single segment carrying the given Run sequence.
func (b *Block) SetTargetRuns(locale LocaleID, runs []Run) {
	if b.Targets == nil {
		b.Targets = make(map[LocaleID][]*Segment)
	}
	b.Targets[locale] = []*Segment{NewRunsSegment("s1", runs)}
}

// AsCodedText flattens the Block's source into the legacy PUA-marker
// coded string + Span list form. Hot-path helper for tools that need
// O(1) substring operations on a flat string; never persist the
// output, always derive on demand. Matches the RFC 0001 §Phase 2
// acceptance criterion.
func (b *Block) AsCodedText() (string, []*Span) {
	return AsCodedText(b.SourceRuns())
}

// FirstSegment returns the first source segment or nil if the block
// has no source content.
func (b *Block) FirstSegment() *Segment {
	if len(b.Source) == 0 {
		return nil
	}
	return b.Source[0]
}

// FirstFragment materializes the block's first source segment as a
// Fragment (CodedText + Span list) — the PUA-marker representation
// used by writers and assertions that operate on a flat string. Runs
// are the canonical form; the Fragment is derived on demand from
// Source[0].Runs and should never be persisted.
func (b *Block) FirstFragment() *Fragment {
	if len(b.Source) == 0 {
		return nil
	}
	return RunsToFragment(b.Source[0].Runs)
}

// SetTargetFragment installs a single-segment target for a locale
// from a Fragment. The Fragment is translated to Runs — callers that
// already hold Runs should prefer SetTargetRuns directly.
func (b *Block) SetTargetFragment(locale LocaleID, frag *Fragment) {
	b.SetTargetRuns(locale, FragmentToRuns(frag))
}
