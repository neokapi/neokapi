package model

import "strings"

// Block is the primary translatable content unit. Its content is a flat
// []Run per locale: Source for the source language and Targets for each
// committed translation variant. Segmentation, terminology, entities, and
// other interpretations ride as stand-off Overlays (see overlay.go); there is
// no structural segment type.
type Block struct {
	ID                 string
	Name               string
	Type               string
	MimeType           string
	Translatable       bool
	SourceLocale       LocaleID // locale of the source runs (set by reader)
	Skeleton           *Skeleton
	Source             []Run                  // source content
	Targets            map[VariantKey]*Target // committed translations, keyed by variant
	Overlays           []Overlay              // opt-in stand-off interpretations (usually none)
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

// SourceText returns the plain text of the source runs (TextRun content
// only — inline-code runs contribute nothing).
func (b *Block) SourceText() string {
	return RunsText(b.Source)
}

// SetSourceText replaces the source content with a single TextRun.
func (b *Block) SetSourceText(text string) {
	b.Source = []Run{{Text: &TextRun{Text: text}}}
}

// HasTarget returns true if a committed target exists for the given locale.
func (b *Block) HasTarget(locale LocaleID) bool {
	t, ok := b.Targets[Variant(locale)]
	return ok && t != nil && len(t.Runs) > 0
}

// TargetText returns the plain text of the target runs for the given locale.
func (b *Block) TargetText(locale LocaleID) string {
	if t, ok := b.Targets[Variant(locale)]; ok && t != nil {
		return RunsText(t.Runs)
	}
	return ""
}

// SetTargetText sets the target text for a locale as a single TextRun.
func (b *Block) SetTargetText(locale LocaleID, text string) {
	b.SetTargetRuns(locale, []Run{{Text: &TextRun{Text: text}}})
}

// Text returns the plain text for a locale. If the locale matches
// SourceLocale, returns the source text; otherwise the target text. Provides
// uniform access regardless of whether a locale is source or target.
func (b *Block) Text(locale LocaleID) string {
	if locale == b.SourceLocale && b.SourceLocale != "" {
		return b.SourceText()
	}
	return b.TargetText(locale)
}

// SetText writes text for a locale. Source if it matches SourceLocale,
// otherwise a target.
func (b *Block) SetText(locale LocaleID, text string) {
	if locale == b.SourceLocale && b.SourceLocale != "" {
		b.SetSourceText(text)
		return
	}
	b.SetTargetText(locale, text)
}

// HasLocale reports whether the Block has content for a locale (source or
// target).
func (b *Block) HasLocale(locale LocaleID) bool {
	if locale == b.SourceLocale && b.SourceLocale != "" {
		return len(b.Source) > 0
	}
	return b.HasTarget(locale)
}

// WordCount returns the number of words in the source text. Words are
// sequences of non-whitespace characters; inline codes are stripped by
// SourceText().
func (b *Block) WordCount() int {
	text := strings.TrimSpace(b.SourceText())
	if text == "" {
		return 0
	}
	return len(strings.Fields(text))
}

// SourceRuns returns the Block's source content as a Run sequence.
func (b *Block) SourceRuns() []Run { return b.Source }

// TargetRuns returns the Block's target content for a locale, or nil.
func (b *Block) TargetRuns(locale LocaleID) []Run {
	if t, ok := b.Targets[Variant(locale)]; ok && t != nil {
		return t.Runs
	}
	return nil
}

// SetSourceRuns replaces the Block's source content.
func (b *Block) SetSourceRuns(runs []Run) { b.Source = runs }

// SetTargetRuns sets the target runs for a locale, preserving any existing
// status/provenance on that variant's Target.
func (b *Block) SetTargetRuns(locale LocaleID, runs []Run) {
	key := Variant(locale)
	if b.Targets == nil {
		b.Targets = make(map[VariantKey]*Target)
	}
	if t, ok := b.Targets[key]; ok && t != nil {
		t.Runs = runs
		return
	}
	b.Targets[key] = &Target{Runs: runs}
}

// Target returns the committed target for a locale variant, or nil.
func (b *Block) Target(locale LocaleID) *Target { return b.Targets[Variant(locale)] }

// TargetVariant returns the committed target for a full variant key, or nil.
func (b *Block) TargetVariant(key VariantKey) *Target { return b.Targets[key] }

// SetTarget stores a committed target for a locale variant.
func (b *Block) SetTarget(locale LocaleID, t *Target) { b.SetTargetVariant(Variant(locale), t) }

// SetTargetVariant stores a committed target for a full variant key.
func (b *Block) SetTargetVariant(key VariantKey, t *Target) {
	if b.Targets == nil {
		b.Targets = make(map[VariantKey]*Target)
	}
	b.Targets[key] = t
}

// TargetLocales returns the distinct locales that have a committed target.
func (b *Block) TargetLocales() []LocaleID {
	seen := make(map[LocaleID]bool, len(b.Targets))
	out := make([]LocaleID, 0, len(b.Targets))
	for k := range b.Targets {
		if !seen[k.Locale] {
			seen[k.Locale] = true
			out = append(out, k.Locale)
		}
	}
	return out
}

// NewBlock creates a translatable Block with plain source text.
func NewBlock(id, text string) *Block {
	return &Block{
		ID:           id,
		Translatable: true,
		Source:       []Run{{Text: &TextRun{Text: text}}},
		Targets:      make(map[VariantKey]*Target),
		Properties:   make(map[string]string),
		Annotations:  make(map[string]Annotation),
	}
}

// NewRunsBlock creates a translatable Block whose source is the given Run
// sequence.
func NewRunsBlock(id string, runs []Run) *Block {
	return &Block{
		ID:           id,
		Translatable: true,
		Source:       runs,
		Targets:      make(map[VariantKey]*Target),
		Properties:   make(map[string]string),
		Annotations:  make(map[string]Annotation),
	}
}
