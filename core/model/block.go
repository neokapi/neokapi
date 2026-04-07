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

// SourceText returns the plain text of all source segments concatenated.
func (b *Block) SourceText() string {
	var buf strings.Builder
	for _, seg := range b.Source {
		buf.WriteString(seg.Content.Text())
	}
	return buf.String()
}

// FirstFragment returns the Fragment of the first source segment.
func (b *Block) FirstFragment() *Fragment {
	if len(b.Source) == 0 {
		return nil
	}
	return b.Source[0].Content
}

// SetSourceText replaces all source content with a single unsegmented Fragment.
func (b *Block) SetSourceText(text string) {
	b.Source = []*Segment{{ID: "s1", Content: NewFragment(text)}}
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
		buf.WriteString(seg.Content.Text())
	}
	return buf.String()
}

// SetTargetText sets the target text for a locale as a single unsegmented Fragment.
func (b *Block) SetTargetText(locale LocaleID, text string) {
	if b.Targets == nil {
		b.Targets = make(map[LocaleID][]*Segment)
	}
	b.Targets[locale] = []*Segment{{ID: "s1", Content: NewFragment(text)}}
}

// SetTargetFragment sets the target for a locale using a pre-built Fragment,
// preserving inline span data instead of creating a plain-text-only fragment.
func (b *Block) SetTargetFragment(locale LocaleID, frag *Fragment) {
	if b.Targets == nil {
		b.Targets = make(map[LocaleID][]*Segment)
	}
	b.Targets[locale] = []*Segment{{ID: "s1", Content: frag}}
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

// NewBlock creates a new translatable Block with the given ID and source text.
func NewBlock(id, text string) *Block {
	return &Block{
		ID:           id,
		Translatable: true,
		Source:       []*Segment{{ID: "s1", Content: NewFragment(text)}},
		Targets:      make(map[LocaleID][]*Segment),
		Properties:   make(map[string]string),
		Annotations:  make(map[string]Annotation),
	}
}
