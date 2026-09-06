package blockstore

import (
	"context"
	"slices"
	"strings"

	"github.com/neokapi/neokapi/core/model"
)

// SourceLocale is the locale key a block's own text is filed under. It is empty
// on purpose: a block cache holds no opinion about which language its source is
// in — the recipe does. Targets are filed under their own locale id.
const SourceLocale = ""

// BlockText is one block's plain text in one locale.
type BlockText struct {
	// Locale is SourceLocale for the block's source text, or the target
	// locale id.
	Locale string
	// Text is the flattened plain text: placeholders contribute their
	// equivalent, paired codes contribute their content, plural and select
	// constructs contribute their 'other' branch. It is what a reader of the
	// rendered content would see, which is the text a term occurs in.
	Text string
}

// BlockTexts returns the block's plain text per locale — the source first, then
// each target that carries any. Empty texts are omitted: there is nothing to
// index or search in them.
//
// This is the one definition of "the text of a block" the search index, the
// in-memory scan and the occurrence query all share, so a match found by one is
// found by the others.
func BlockTexts(b *Block) []BlockText {
	if b == nil {
		return nil
	}
	out := make([]BlockText, 0, 1+len(b.Targets))
	if src := model.FlattenRuns(b.Source); src != "" {
		out = append(out, BlockText{Locale: SourceLocale, Text: src})
	}
	for locale, runs := range b.Targets {
		if locale == SourceLocale {
			continue // a target filed under the source key would shadow it
		}
		if txt := model.FlattenRuns(runs); txt != "" {
			// Canonical, the form every locale filter asks in.
			out = append(out, BlockText{Locale: string(model.NormalizeLocale(model.LocaleID(locale))), Text: txt})
		}
	}
	return out
}

// TextSearchOptions narrows a block-text search.
type TextSearchOptions struct {
	// Collection restricts the search to one collection. Empty means all.
	Collection string
	// Locales restricts the search to these locale keys, where the empty
	// string means the source text (SourceLocale). Nil means every locale.
	Locales []string
	// Limit caps the number of hits returned. Zero means no cap.
	Limit int
}

func (o TextSearchOptions) wants(locale string) bool {
	if o.Locales == nil {
		return true
	}
	return slices.Contains(o.CanonicalLocales(), locale)
}

// CanonicalLocales returns the locale filter with every locale in canonical
// form, the form BlockTexts files a target's text under. Nil stays nil (no
// filter) and the source key stays the empty string.
func (o TextSearchOptions) CanonicalLocales() []string {
	if o.Locales == nil {
		return nil
	}
	out := make([]string, len(o.Locales))
	for i, l := range o.Locales {
		out[i] = string(model.NormalizeLocale(model.LocaleID(l)))
	}
	return out
}

// TextHit is one block's text in one locale, matched by a text search.
type TextHit struct {
	// Hash is the block's durable content key.
	Hash string
	// Collection is the collection the block was stored under, where the
	// store can derive it. A scanned store reads blocks through the session
	// API, which carries no collection, so it reports only the collection the
	// search was filtered to — empty when it was not filtered.
	Collection string
	// Locale is SourceLocale for source text, or the target locale id.
	Locale string
	// Text is the matched text, verbatim.
	Text string
	// Block is the block itself, so a caller can name and place the hit
	// without a second round trip.
	Block *Block
}

// TextSearcher is the optional capability of finding blocks by the text inside
// them without reading every block. The SQLite store implements it over an FTS5
// trigram index; stores that do not are scanned instead (see SearchText).
//
// It is a CANDIDATE filter, not a matcher. An implementation may return hits
// whose text does not in fact contain the needle — a trigram index matches on
// three-character grams and an index is allowed to be approximate — so callers
// verify the text themselves. What it may never do is miss a block whose text
// contains the needle case-insensitively.
type TextSearcher interface {
	SearchBlockText(ctx context.Context, needle string, opts TextSearchOptions) ([]TextHit, error)
}

// SearchText finds the blocks whose text may contain needle, case-insensitively.
//
// It uses the store's own index when it has one and otherwise scans every block
// in the store — which is what the browser build does, where the corpus is one
// lab document and an index would cost more than it saves. Both paths return the
// same hits for the same corpus; only the work differs.
//
// An empty needle matches nothing, since every text would qualify.
func SearchText(ctx context.Context, store Store, needle string, opts TextSearchOptions) ([]TextHit, error) {
	if needle == "" || store == nil {
		return nil, nil
	}
	if s, ok := store.(TextSearcher); ok {
		return s.SearchBlockText(ctx, needle, opts)
	}
	return scanText(ctx, store, needle, opts)
}

// scanText is the universal fallback: read every block, flatten it, keep the
// texts that contain the needle.
func scanText(ctx context.Context, store Store, needle string, opts TextSearchOptions) ([]TextHit, error) {
	sess, err := store.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer sess.Close()

	lowerNeedle := strings.ToLower(needle)
	var hits []TextHit
	for b, err := range sess.Blocks(BlockFilter{Collection: opts.Collection}) {
		if err != nil {
			return nil, err
		}
		for _, bt := range BlockTexts(b) {
			if !opts.wants(bt.Locale) {
				continue
			}
			if !strings.Contains(strings.ToLower(bt.Text), lowerNeedle) {
				continue
			}
			hits = append(hits, TextHit{
				Hash:       b.Hash,
				Collection: opts.Collection,
				Locale:     bt.Locale,
				Text:       bt.Text,
				Block:      b,
			})
			if opts.Limit > 0 && len(hits) >= opts.Limit {
				return hits, nil
			}
		}
	}
	return hits, nil
}
