package model

// Block is the primary modifiable content unit: the text a tool reads,
// rewrites, checks, or translates. Its content is a set of peer editions, each
// a flat []Run under an EditionKey (a language, optionally with a tone or a
// channel): the edition the document is written in and every translation or
// other edition a reader or a tool attached. Segmentation, terminology,
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
// A struct copy (cp := *b) shares the edition storage with b, the edition the
// block was read in included: a write to any edition of the copy, its source
// and the source's status among them, is a write to b. A copy that will be
// written takes CopyEditionSet (its own set of editions and its own source
// entry) or CopyEditions (every edition's runs copied as well).
//
// Role boundary: the raw Block is the wire/storage DTO — exported fields,
// direct serialization, no encapsulation. Tool-facing code goes through
// tool.BlockView / tool.VariantView, the capability-scoped boundary; do not
// hand raw *Block to new tool-facing APIs.
type Block struct {
	ID   string
	Name string
	// Key is the block's DURABLE identity: the key a decision, a translation
	// and a history entry are filed under, and the key a venue stores as a
	// block's source id.
	//
	// It is not the same thing as Name. A name is what the format says, a
	// structural address like `install/p#2` or a message key, and it is the
	// right thing for a reader to report and the wrong thing to record a
	// decision against, because for a positional format it follows position:
	// delete the first paragraph of a section and every name below it shifts.
	// A key is what survives that, because it is MATCHED rather than named
	// (core/reconcile).
	//
	// Empty until something resolves it, and BlockKey falls back to Name, so a
	// reader is under no obligation to fill it in and a format with a natural
	// key needs nothing more than the name it already has.
	//
	// It is a field rather than a property on purpose: properties are folded
	// into the context hash that reconciliation MATCHES on, so a key written
	// there would change the very signal that produced it.
	Key      string
	Type     string
	MimeType string
	// Translatable marks the block as content eligible for modification or
	// extraction — a parse-time classification the reader sets to separate
	// authored content from the surrounding non-content structure. Blocks left
	// unmarked stay in the skeleton, untouched by tools that edit, check, or
	// translate.
	Translatable bool
	SourceLocale LocaleID // locale of the source runs (set by reader)
	Skeleton     *Skeleton
	// Editions holds the block's content, one edition per key. The edition
	// the block was read in, the first native edition, is filed under the zero
	// key whatever language SourceLocale names; every other edition (a
	// translation, a tone, a channel) is filed under its canonical key. The
	// accessors in edition.go resolve a key in the source language to the
	// edition the block was read in unless the block holds a same-language
	// edition of its own, as a bilingual file from en to en does. Read and
	// write editions through them; a test that must plant a state no accessor
	// produces (a key that is not canonical) writes this map directly.
	Editions map[EditionKey]*Edition
	// Native lists the editions the document's bytes hold, by the keys they
	// are filed under in Editions: the edition the block was read in first,
	// under the zero key, then each other edition the format writes into the
	// same document, such as the target of an XLIFF unit or each localization
	// of an xcstrings entry. An empty list means the document holds the
	// edition it was read in and no other.
	Native             []EditionKey
	Overlays           []Overlay          // positional, run-anchored stand-off layers (segmentation, term, entity, qa, alignment), each on the edition it names
	Annotations        map[string]Payload // block-scoped typed metadata (notes, alt-translations, analysis results), keyed by type
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

	// unlabelled is a translation a reader filed under no language: a KBF
	// bundle's "" target, an xcstrings localization keyed by the empty string,
	// or the translation of a Qt TS file that names no language read with no
	// source locale. The zero key names the edition the block was read in, so
	// such a translation sits apart from Editions. TargetEdition("") and the
	// other target accessors given the empty locale read and write it, and
	// EachTargetEdition yields it under the zero key; EditionKeys and
	// EachEdition leave it out.
	unlabelled *Edition

	// readSource is the source the reader produced, kept by the first edit
	// (EditSourceRuns, EditSourceText). A writer compares the two to tell
	// the document's own spelling from wording an edit supplied. It belongs
	// to this process: no wire form, store or hash carries it.
	readSource []Run
	sourceKept bool
}

// ResourceID returns the Block's unique identifier.
func (b *Block) ResourceID() string { return b.ID }

// SourceText returns the plain text of the source runs (TextRun content
// only — inline-code runs contribute nothing).
func (b *Block) SourceText() string {
	return RunsText(b.sourceRuns())
}

// SetSourceText replaces the source content with a single TextRun.
func (b *Block) SetSourceText(text string) {
	b.SetSourceRuns([]Run{{Text: &TextRun{Text: text}}})
}

// HasTarget returns true if a committed target exists for the given locale.
func (b *Block) HasTarget(locale LocaleID) bool {
	t := b.target(Variant(locale))
	return t != nil && len(t.Runs) > 0
}

// TargetText returns the plain text of the target runs for the given locale.
func (b *Block) TargetText(locale LocaleID) string {
	if t := b.target(Variant(locale)); t != nil {
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
		return len(b.sourceRuns()) > 0
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
func (b *Block) SourceRuns() []Run { return b.sourceRuns() }

// TargetRuns returns the Block's target content for a locale, or nil.
func (b *Block) TargetRuns(locale LocaleID) []Run {
	if t := b.target(Variant(locale)); t != nil {
		return t.Runs
	}
	return nil
}

// SetSourceRuns replaces the Block's source content.
func (b *Block) SetSourceRuns(runs []Run) {
	b.writeSource(func(e *Edition) { e.Runs = runs })
}

// EditSourceRuns replaces the source with an edit: wording a tool, an agent
// or a person supplied for a block that was read from a document. The first
// edit keeps the source the reader produced, which SourceAsRead returns, so a
// writer can encode the edit's wording for its format and still write every
// unedited block exactly as it was read. A reader building a block uses
// SetSourceRuns instead.
func (b *Block) EditSourceRuns(runs []Run) {
	if !b.sourceKept {
		b.readSource, b.sourceKept = b.sourceRuns(), true
	}
	b.SetSourceRuns(runs)
}

// EditSourceText is EditSourceRuns for an edit that is a single text run.
func (b *Block) EditSourceText(text string) {
	b.EditSourceRuns([]Run{{Text: &TextRun{Text: text}}})
}

// SourceAsRead returns the source the reader produced for this block and
// reports whether an edit has changed it since: whether the current source
// renders to different bytes than the source as read. A block no edit has
// touched returns its current source and false.
func (b *Block) SourceAsRead() (runs []Run, edited bool) {
	if !b.sourceKept {
		return b.sourceRuns(), false
	}
	return b.readSource, RenderRunsWithData(b.readSource) != RenderRunsWithData(b.sourceRuns())
}

// SetTargetRuns sets the target runs for a locale, preserving any existing
// status and provenance on that target. The source language files a target in
// that language and leaves the edition the block was read in as it is. The
// empty locale files a translation under no language, which never writes the
// edition the block was read in either: TargetRuns("") reads it back and
// EachTargetEdition yields it under the zero key.
func (b *Block) SetTargetRuns(locale LocaleID, runs []Run) {
	key := Variant(locale)
	if t := b.target(key); t != nil {
		t.Runs = runs
		return
	}
	b.putTarget(key, &Edition{Runs: runs})
}

// TargetEdition returns the target filed under locale as an edition (its runs,
// status, origin, score and derivation) and whether the block holds one. It
// reads the target TargetRuns and TargetText read, so it never returns the
// edition the block was read in. The source language names a target only when
// the block holds one, as a bilingual file in one language does. The empty
// locale names a translation a reader filed under no language, as the KBF
// reader files a bundle's "" target and the Qt TS reader files the translation
// of a file with no language attribute read with no source locale. Edition,
// EditionKeys and EachEdition read the zero key as the edition the block was
// read in, so a walk over every text a writer can emit reads such a
// translation here.
func (b *Block) TargetEdition(locale LocaleID) (Edition, bool) {
	t := b.target(Variant(locale))
	if t == nil {
		return Edition{}, false
	}
	return *t, true
}

// SetTargetEdition files e as the target under k (its runs, status, origin,
// score and derivation), creating the target when the block holds none there.
// The edition the block was read in stays as it is: a key in the source
// language files a target in that language, and the zero key files a
// translation under no language. SetEdition writes the edition the block was
// read in for the zero key, and for the source language while the block holds
// no target in it. An existing target is updated in place.
func (b *Block) SetTargetEdition(k EditionKey, e Edition) {
	key := k.Canonical()
	if t := b.target(key); t != nil {
		*t = e
		return
	}
	b.putTarget(key, &e)
}

// StampTargetProvenance records how a locale's committed target was produced —
// its lifecycle status and origin — without touching its runs. It is a no-op
// when no target exists for the locale, so producers can set the text and stamp
// provenance in two steps. A producer (AI/MT/recycle/…) calls this so coverage
// and ship gates can see how far each unit has progressed.
func (b *Block) StampTargetProvenance(locale LocaleID, status TargetStatus, origin Origin) {
	if t := b.target(Variant(locale)); t != nil {
		t.Status = Status(status)
		t.Origin = origin
	}
}

// TargetLocales returns the distinct locales that have a committed target:
// the language of every edition other than the one the block was read in,
// and the empty locale for a translation filed under no language.
func (b *Block) TargetLocales() []LocaleID {
	seen := make(map[LocaleID]bool, len(b.Editions))
	out := make([]LocaleID, 0, len(b.Editions))
	for k := range b.Editions {
		if k.IsZero() {
			continue
		}
		if !seen[k.Locale] {
			seen[k.Locale] = true
			out = append(out, k.Locale)
		}
	}
	if b.unlabelled != nil {
		out = append(out, "")
	}
	return out
}

// NewBlock creates a translatable Block with plain source text.
func NewBlock(id, text string) *Block {
	return NewRunsBlock(id, []Run{{Text: &TextRun{Text: text}}})
}

// NewRunsBlock creates a translatable Block whose source is the given Run
// sequence.
func NewRunsBlock(id string, runs []Run) *Block {
	b := &Block{
		ID:           id,
		Translatable: true,
		Editions:     make(map[EditionKey]*Edition),
		Properties:   make(map[string]string),
	}
	b.SetSourceRuns(runs)
	return b
}
