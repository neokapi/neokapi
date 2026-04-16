package klz

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/neokapi/neokapi/core/klf"
	"github.com/neokapi/neokapi/core/klz/internal/db"
)

// BlockByID returns the Block with the given id. When the klzcache
// build tag is set and a cache entry exists (or can be built), the
// lookup is O(ms) through the SQLite index; otherwise it falls back
// to a linear scan over every archived document.
func (r *Reader) BlockByID(ctx context.Context, id string) (*klf.Block, error) {
	if block, hit := r.cacheBlockByID(ctx, id); hit {
		return block, nil
	}
	docs, err := r.Documents()
	if err != nil {
		return nil, err
	}
	for _, doc := range docs {
		for i := range doc.Documents {
			for j := range doc.Documents[i].Blocks {
				if doc.Documents[i].Blocks[j].ID == id {
					return &doc.Documents[i].Blocks[j], nil
				}
			}
		}
	}
	return nil, nil
}

// cacheBlockByID returns (block, true) when the cache satisfies the
// lookup; (_, false) when the cache is unavailable or missing the
// block so the caller can fall back to the iteration side.
func (r *Reader) cacheBlockByID(ctx context.Context, id string) (*klf.Block, bool) {
	cache, err := r.openCache(ctx)
	if err != nil || cache == nil {
		return nil, false
	}
	defer cache.Close()

	raw, err := cache.BlockByID(ctx, id)
	if err != nil {
		// Cache isn't built yet — try to warm it once and retry.
		var notImpl *db.ErrNotImplemented
		if errors.As(err, &notImpl) {
			if buildErr := r.WarmCache(ctx); buildErr != nil {
				return nil, false
			}
			c2, err := r.openCache(ctx)
			if err != nil || c2 == nil {
				return nil, false
			}
			defer c2.Close()
			raw, err = c2.BlockByID(ctx, id)
			if err != nil || raw == nil {
				return nil, false
			}
		} else {
			return nil, false
		}
	}
	if raw == nil {
		return nil, false
	}
	var runs []klf.Run
	if err := json.Unmarshal(raw, &runs); err != nil {
		return nil, false
	}
	// The cache stores Block.source runs only. Fill in the metadata
	// by consulting the live archive so callers get a complete Block.
	docs, err := r.Documents()
	if err != nil {
		return nil, false
	}
	for _, doc := range docs {
		for i := range doc.Documents {
			for j := range doc.Documents[i].Blocks {
				if doc.Documents[i].Blocks[j].ID == id {
					out := doc.Documents[i].Blocks[j]
					out.Source = runs
					return &out, true
				}
			}
		}
	}
	return nil, false
}

// SimilarSources returns up to `limit` Blocks whose source text is
// most similar to `query`. Ranked by FTS5 bm25() when the cache is
// available; returns an empty list otherwise.
func (r *Reader) SimilarSources(ctx context.Context, query, locale string, limit int) ([]*klf.Block, error) {
	cache, err := r.openCache(ctx)
	if err != nil || cache == nil {
		return nil, nil
	}
	defer cache.Close()

	ids, err := cache.SimilarSources(ctx, query, locale, limit)
	if err != nil {
		var notImpl *db.ErrNotImplemented
		if errors.As(err, &notImpl) {
			if buildErr := r.WarmCache(ctx); buildErr != nil {
				return nil, nil
			}
			c2, err := r.openCache(ctx)
			if err != nil || c2 == nil {
				return nil, nil
			}
			defer c2.Close()
			ids, err = c2.SimilarSources(ctx, query, locale, limit)
			if err != nil {
				return nil, nil
			}
		} else {
			return nil, fmt.Errorf("klz: similar sources: %w", err)
		}
	}
	out := make([]*klf.Block, 0, len(ids))
	for _, id := range ids {
		b, err := r.BlockByID(ctx, id)
		if err != nil || b == nil {
			continue
		}
		out = append(out, b)
	}
	return out, nil
}

// TM returns a query handle for looking up TM matches against the
// archive's embedded target overlays. Returns nil when the cache is
// unavailable (non-klzcache builds).
func (r *Reader) TM() TMQuerier {
	return &tmQuerier{reader: r}
}

// TMQuerier is the interface tools use to query embedded target
// overlays for TM matches.
type TMQuerier interface {
	Match(ctx context.Context, sourceText string, locale string, limit int) ([]TMMatch, error)
}

// TMMatch is one TM lookup result.
type TMMatch struct {
	SourceText string
	TargetRuns []klf.Run
	Locale     string
	Score      float64
	BlockID    string
}

// tmQuerier routes TM lookups through the runtime cache's FTS5
// similarity search, then stitches back the target runs from the
// cache's targets table.
type tmQuerier struct {
	reader *Reader
}

// Match returns up to `limit` TMMatch entries whose source text is
// similar to sourceText, restricted to the given target locale.
// Consults both in-block Targets maps and separate target overlay
// parts (targets/<locale>/*.klf) so archives that split sources and
// targets into separate parts are handled correctly.
func (t *tmQuerier) Match(ctx context.Context, sourceText, locale string, limit int) ([]TMMatch, error) {
	if limit <= 0 {
		limit = 10
	}
	similar, err := t.reader.SimilarSources(ctx, sourceText, t.reader.Manifest().Project.SourceLocale, limit)
	if err != nil {
		return nil, err
	}

	// Build a { blockID → targetRuns } index from any separate
	// target overlay parts so we don't force the caller to fold
	// overlays into the source blocks themselves.
	overlays, err := t.reader.Targets(locale)
	if err != nil {
		return nil, err
	}
	overlayTargets := make(map[string][]klf.Run)
	for _, f := range overlays {
		for _, d := range f.Documents {
			for i := range d.Blocks {
				if runs, ok := d.Blocks[i].Targets[locale]; ok {
					overlayTargets[d.Blocks[i].ID] = runs
				}
			}
		}
	}

	out := make([]TMMatch, 0, len(similar))
	for _, b := range similar {
		runs, ok := b.Targets[locale]
		if !ok {
			runs, ok = overlayTargets[b.ID]
		}
		if !ok {
			continue
		}
		out = append(out, TMMatch{
			SourceText: flattenRuns(b.Source),
			TargetRuns: runs,
			Locale:     locale,
			BlockID:    b.ID,
		})
	}
	return out, nil
}
