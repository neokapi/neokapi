package importer

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"iter"
	"strings"

	"github.com/neokapi/neokapi/core/blockstore"
	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
)

// ConflictPolicy determines how the importer handles a block+locale
// pair that already has an overlay in the target store.
type ConflictPolicy int

const (
	// ReplaceExisting upserts every incoming pair. The previous
	// overlay payload is overwritten. Default.
	ReplaceExisting ConflictPolicy = iota
	// SkipExisting writes only when the (block, locale) pair has no
	// overlay in the target store. Useful when importing a vendor
	// delivery as a baseline and local edits should win.
	SkipExisting
)

// Options scopes an import run. All fields are optional — zero values
// mean "take the safe default".
type Options struct {
	// OnConflict picks between ReplaceExisting (default) and SkipExisting.
	OnConflict ConflictPolicy
	// Provider, when set, is written into each target overlay's
	// `provider` field so downstream readers can distinguish
	// imports from different upstream sources (e.g. "tmx:legacy",
	// "webhook:deepl").
	Provider string
}

// ImportPair is one block→target row the importer writes. The caller
// supplies either:
//
//   - a SourceText (→ ImportFromFormat hashes + matches against the
//     store), or
//   - a BlockHash directly (→ ImportDirect, no matching needed).
//
// Locale is required; Text is the translated text. Runs, when
// non-nil, preserves full placeholder + inline-code structure — the
// translations table stores both the flat text and the runs
// when available.
type ImportPair struct {
	// BlockHash is the content-addressed block key, if already known
	// by the caller. ImportDirect uses this path.
	BlockHash string
	// SourceText is the plain source text used for hash matching when
	// BlockHash is empty. ImportFromFormat supplies this.
	SourceText string
	// Locale is the target locale (required).
	Locale string
	// Text is the target text (required — may be empty string to
	// indicate an intentional empty target, though most importers
	// will skip empty-target pairs upstream).
	Text string
	// Runs carries the target content as a Run sequence when the source
	// format preserves it. Marshaled into the translations table's
	// runs_json column on write.
	Runs []model.Run
}

// Report summarizes a completed import run.
type Report struct {
	// TotalPairs is the count of ImportPair values seen from the source.
	TotalPairs int
	// Matched is pairs where a target block was found. ImportDirect
	// doesn't do matching (every pair counts as matched).
	Matched int
	// Unmatched is pairs with no corresponding source block in the
	// target store. Silently skipped — review these if an expected
	// row didn't land.
	Unmatched int
	// Written is pairs that produced an overlay write (subject to
	// Options.OnConflict).
	Written int
	// SkippedByPolicy is pairs skipped because OnConflict=SkipExisting
	// and the (block, locale) already had an overlay.
	SkippedByPolicy int
}

// ImportDirect writes overlays for a stream of pairs whose BlockHash
// is already resolved (the webhook case). Never does source-hash
// matching.
func ImportDirect(
	ctx context.Context,
	store blockstore.Store,
	pairs iter.Seq2[ImportPair, error],
	opts Options,
) (*Report, error) {
	sess, err := store.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("importer: begin session: %w", err)
	}
	defer func() { _ = sess.Close() }()

	r := &Report{}
	for p, err := range pairs {
		if err != nil {
			return r, err
		}
		r.TotalPairs++
		if p.BlockHash == "" || p.Locale == "" {
			r.Unmatched++
			continue
		}
		r.Matched++

		skipped, err := skipByPolicy(sess, opts.OnConflict, "targets/"+p.Locale, p.BlockHash)
		if err != nil {
			return r, err
		}
		if skipped {
			r.SkippedByPolicy++
			continue
		}
		if err := writeTargetOverlay(sess, p, opts.Provider); err != nil {
			return r, err
		}
		r.Written++
	}
	if err := sess.Commit(); err != nil {
		return r, fmt.Errorf("importer: commit: %w", err)
	}
	return r, nil
}

// ImportFromFormat drives a `format.DataFormatReader` and lands each
// incoming block's targets as overlays on matching source blocks in
// the target store. Source matching uses a SHA-256 of the normalized
// source text (same normalization as model.ComputeContentHash).
//
// A doc argument is required — it's the RawDocument the reader opens
// (path, encoding, bytes). Construct it as you would for any other
// reader usage.
func ImportFromFormat(
	ctx context.Context,
	store blockstore.Store,
	reader format.DataFormatReader,
	doc *model.RawDocument,
	opts Options,
) (*Report, error) {
	if err := reader.Open(ctx, doc); err != nil {
		return nil, fmt.Errorf("importer: open format reader: %w", err)
	}
	defer reader.Close()

	sess, err := store.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("importer: begin session: %w", err)
	}
	defer func() { _ = sess.Close() }()

	// Build a source-hash → blockHash index from the target store
	// up front. For typical imports (vendor delivery of 10K segments
	// against a 100K-block project), this is one full scan amortized
	// over every match.
	index, err := buildSourceIndex(sess)
	if err != nil {
		return nil, fmt.Errorf("importer: build source index: %w", err)
	}

	r := &Report{}
	results := reader.Read(ctx)
	for res := range results {
		if res.Error != nil {
			return r, fmt.Errorf("importer: read: %w", res.Error)
		}
		if res.Part == nil || res.Part.Type != model.PartBlock {
			continue
		}
		block, ok := res.Part.Resource.(*model.Block)
		if !ok || block == nil || !block.Translatable {
			continue
		}
		srcText := plainSourceText(block)
		if srcText == "" {
			continue
		}
		srcHash := hashSource(srcText)
		blockHash, matched := index[srcHash]

		for _, locale := range block.TargetLocales() {
			runs := block.TargetRuns(locale)
			r.TotalPairs++
			if !matched {
				r.Unmatched++
				continue
			}
			r.Matched++

			text := model.RunsText(runs)
			if text == "" {
				continue
			}

			skipped, err := skipByPolicy(sess, opts.OnConflict, "targets/"+string(locale), blockHash)
			if err != nil {
				return r, err
			}
			if skipped {
				r.SkippedByPolicy++
				continue
			}

			if err := writeTargetOverlay(sess, ImportPair{
				BlockHash: blockHash,
				Locale:    string(locale),
				Text:      text,
				Runs:      runs,
			}, opts.Provider); err != nil {
				return r, err
			}
			r.Written++
		}
	}
	if err := sess.Commit(); err != nil {
		return r, fmt.Errorf("importer: commit: %w", err)
	}
	return r, nil
}

// ─── internals ──────────────────────────────────────────────────

// buildSourceIndex scans the target store once and returns a map of
// source-text-hash → block hash. Returns an empty map on an empty
// store rather than an error.
func buildSourceIndex(sess blockstore.Session) (map[string]string, error) {
	index := map[string]string{}
	for b, err := range sess.Blocks(blockstore.BlockFilter{}) {
		if err != nil {
			return nil, err
		}
		if b == nil || !b.Translatable {
			continue
		}
		text := plainBlockSourceText(b)
		if text == "" {
			continue
		}
		h := hashSource(text)
		// First block wins on hash collision — source-hash collisions
		// within one project are an authoring decision; the importer
		// writes to one of them and surfaces the rest as Matched but
		// doesn't write twice.
		if _, ok := index[h]; ok {
			continue
		}
		key := b.Hash
		if key == "" {
			key = b.ID
		}
		index[h] = key
	}
	return index, nil
}

// skipByPolicy probes whether a (kind, blockHash) overlay already
// exists and returns true when the conflict policy says "leave it
// alone". Returns false for ReplaceExisting (always overwrite) and
// for SkipExisting when no existing overlay was found.
func skipByPolicy(sess blockstore.Session, policy ConflictPolicy, kind, blockHash string) (bool, error) {
	if policy != SkipExisting {
		return false, nil
	}
	_, err := sess.GetOverlay(kind, blockHash)
	switch {
	case err == nil:
		return true, nil
	case errors.Is(err, blockstore.ErrNotFound):
		return false, nil
	default:
		return false, fmt.Errorf("importer: probe existing overlay: %w", err)
	}
}

// writeTargetOverlay persists one pair as a `targets/<locale>`
// overlay on the supplied session. Payload shape is the adapter's
// translationPayload (`{text, provider, segments}`); platform adapters
// can split that across their own first-class columns at dispatch time.
func writeTargetOverlay(sess blockstore.Session, p ImportPair, provider string) error {
	payload := map[string]any{"text": p.Text}
	if provider != "" {
		payload["provider"] = provider
	}
	if len(p.Runs) > 0 {
		payload["runs"] = p.Runs
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("importer: marshal payload: %w", err)
	}
	return sess.PutOverlay(blockstore.Overlay{
		Kind:      "targets/" + p.Locale,
		BlockHash: p.BlockHash,
		Payload:   body,
	})
}

// hashSource mirrors model.ComputeContentHash: trim whitespace +
// SHA-256. Keeps the importer's matching byte-identical with the
// extract pipeline's source-hash computation.
func hashSource(text string) string {
	h := sha256.Sum256([]byte(strings.TrimSpace(text)))
	return hex.EncodeToString(h[:])
}

// plainBlockSourceText extracts the flat text from a klf.Block's
// source runs (the type blockstore.Session returns).
func plainBlockSourceText(b *blockstore.Block) string {
	if b == nil {
		return ""
	}
	var sb strings.Builder
	for _, r := range b.Source {
		if r.Text != nil {
			sb.WriteString(r.Text.Text)
		}
	}
	return sb.String()
}

// plainSourceText extracts the flat text from a model.Block's source runs
// (what format readers produce).
func plainSourceText(b *model.Block) string {
	if b == nil {
		return ""
	}
	return model.RunsText(b.Source)
}
