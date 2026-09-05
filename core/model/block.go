package model

// Block is the primary modifiable content unit: the text a tool reads,
// rewrites, checks, or translates. Its content is a flat []Run per variant —
// Source for the canonical content and Targets for each committed variant (a
// locale, optionally with tone or channel). Segmentation, terminology,
// entities, and other interpretations ride as stand-off Overlays (see
// overlay.go); there is no structural segment type.
//
// Ownership invariant: a Block (like every Part payload) is SINGLE-OWNER as it
// moves through pipeline channels — exactly one stage holds it at a time, so
// accessors and tools hand back live slices and maps with NO defensive copies
// by design (the zero-copy trade-off that keeps the streaming pipeline cheap).
// A stage that wants to retain a Block past sending it downstream must copy it
// explicitly; the executor's EnforceImmutability backstop catches accidental
// in-place edits from read-only tool tiers in dev/test.
//
// Role boundary: the raw Block is the wire/storage DTO — exported fields,
// direct serialization, no encapsulation. Tool-facing code goes through
// tool.BlockView / tool.VariantView, the capability-scoped boundary; do not
// hand raw *Block to new tool-facing APIs.
type Block struct {
	ID   string
	Name string
	// Unit is the block's DURABLE identity: the key a decision, a translation
	// and a history entry are filed under, and the key a venue stores as a
	// block's source id.
	//
	// It is not the same thing as Name. A name is what the format says — a
	// structural address like `install/p#2`, or a message key — and it is the
	// right thing for a reader to report and the wrong thing to record a
	// decision against, because for a positional format it follows position:
	// delete the first paragraph of a section and every name below it shifts.
	// A unit is what survives that, because it is MATCHED rather than named
	// (core/reconcile).
	//
	// Empty until something resolves it, and BlockKey falls back to Name, so a
	// reader is under no obligation to fill it in and a format with a natural
	// key needs nothing more than the name it already has.
	//
	// It is a field rather than a property on purpose: properties are folded
	// into the context hash that reconciliation MATCHES on, so a unit written
	// there would change the very signal that produced it.
	Unit     string
	Type     string
	MimeType string
	// Translatable marks the block as content eligible for modification or
	// extraction — a parse-time classification the reader sets to separate
	// authored content from the surrounding non-content structure. Blocks left
	// unmarked stay in the skeleton, untouched by tools that edit, check, or
	// translate.
	Translatable bool
	SourceLocale LocaleID // locale of the source runs (set by reader)
	// SourceStatus is the authoring lifecycle state of the source content
	// (authored → checked → approved): the source-side counterpart of
	// Target.Status. New ("") means "no committed status yet" and reads as the
	// authored baseline. A source edit resets it; a clean source check stamps
	// `checked`; an explicit human/agent approval stamps `approved`.
	SourceStatus       SourceStatus
	Skeleton           *Skeleton
	Source             []Run                  // source content
	Targets            map[VariantKey]*Target // committed translations, keyed by variant
	Overlays           []Overlay              // positional, run-anchored stand-off layers (segmentation, term, entity, qa, alignment)
	Annotations        map[string]Payload     // block-scoped typed metadata (notes, alt-translations, analysis results), keyed by type
	Properties         map[string]string
	Identity           *BlockIdentity // Content-addressable hash for deduplication
	ContentRef         *ContentRef    // Link to external connector source
	DisplayHint        *DisplayHint   // UI rendering guidance
	PreserveWhitespace bool           // Whether whitespace is significant in this block
	IsReferent         bool           // Whether this block is referenced by a skeleton

	// structure and geometry are annotations by contract and fields by
	// storage. Every accessor — Anno, SetAnno, DelAnno, Annos, AnnoMap,
	// AnnoAs — presents them exactly as if they sat in Annotations, so the
	// wire, the stores and every consumer see no difference.
	//
	// They are held here because they are the two annotations that are set on
	// nearly every structured block rather than occasionally: a role on any
	// block a format gives structure to, a position on any block that comes
	// from a grid or a page. A map entry costs around 285 bytes, so a
	// spreadsheet of a million cells was paying a quarter of a gigabyte to
	// store one enum and four small integers per cell. Nothing else in the
	// annotation vocabulary is common enough to be worth taking out of the
	// map, and the map stays for all of it.
	structure *StructureAnnotation
	geometry  *GeometryAnnotation
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
	if b.isSourceLocale(locale) {
		return b.SourceText()
	}
	return b.TargetText(locale)
}

// SetText writes text for a locale. Source if it matches SourceLocale,
// otherwise a target.
func (b *Block) SetText(locale LocaleID, text string) {
	if b.isSourceLocale(locale) {
		b.SetSourceText(text)
		return
	}
	b.SetTargetText(locale, text)
}

// HasLocale reports whether the Block has content for a locale (source or
// target).
func (b *Block) HasLocale(locale LocaleID) bool {
	if b.isSourceLocale(locale) {
		return len(b.Source) > 0
	}
	return b.HasTarget(locale)
}

// isSourceLocale reports whether locale names the block's source language,
// whichever way either side spelled it. A block with no source locale owns no
// locale as its source.
func (b *Block) isSourceLocale(locale LocaleID) bool {
	return b.SourceLocale != "" && NormalizeLocale(locale) == NormalizeLocale(b.SourceLocale)
}

// WordCount returns the number of words in the source text. Inline codes are
// stripped by SourceText(); plural/select forms descend into their 'other'
// branch; Private Use Area span markers are treated as word breaks.
func (b *Block) WordCount() int {
	return CountWords(b.SourceText())
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
func (b *Block) TargetVariant(key VariantKey) *Target { return b.Targets[key.Canonical()] }

// StampTargetProvenance records how a locale's committed target was produced —
// its lifecycle status and origin — without touching its runs. It is a no-op
// when no target exists for the locale, so producers can set the text and stamp
// provenance in two steps. A producer (AI/MT/recycle/…) calls this so coverage
// and ship gates can see how far each unit has progressed.
func (b *Block) StampTargetProvenance(locale LocaleID, status TargetStatus, origin Origin) {
	if t := b.Target(locale); t != nil {
		t.Status = status
		t.Origin = origin
	}
}

// SetTarget stores a committed target for a locale variant.
func (b *Block) SetTarget(locale LocaleID, t *Target) { b.SetTargetVariant(Variant(locale), t) }

// SetTargetVariant stores a committed target for a full variant key.
func (b *Block) SetTargetVariant(key VariantKey, t *Target) {
	if b.Targets == nil {
		b.Targets = make(map[VariantKey]*Target)
	}
	b.Targets[key.Canonical()] = t
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
	}
}
