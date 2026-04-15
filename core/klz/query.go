package klz

import (
	"context"

	"github.com/neokapi/neokapi/core/klf"
)

// BlockByID walks the archive's documents in manifest order and
// returns the first Block whose ID matches. This is the linear-scan
// fallback; Phase 4 wires this same entry point through an internal
// SQLite cache for O(1) lookup keyed by the manifest hash.
func (r *Reader) BlockByID(_ context.Context, id string) (*klf.Block, error) {
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

// SimilarSources is a placeholder for the Phase-4 FTS-backed
// similarity search. In Phase 1 it returns an empty result — the
// public API is shaped now so tools can import klz.Reader with the
// final signature before the cache layer lands.
//
// Callers should not depend on this returning useful results until
// Phase 4.
func (r *Reader) SimilarSources(_ context.Context, _ string, _ string, _ int) ([]*klf.Block, error) {
	return nil, nil
}

// TM is a placeholder for the Phase-4 TM query interface. Returns
// nil in Phase 1.
func (r *Reader) TM() TMQuerier { return nil }

// TMQuerier is the interface tools will use to look up TM matches
// against a .klz's embedded target overlays. Phase 4 wires this
// through the internal SQLite cache; Phase 1 leaves the interface
// in place so consuming tools can type-check against the final
// shape.
type TMQuerier interface {
	Match(ctx context.Context, sourceText string, locale string, limit int) ([]TMMatch, error)
}

// TMMatch is one TM lookup result.
type TMMatch struct {
	SourceText string
	TargetRuns []klf.Run
	Locale     string
	Score      float64
}
