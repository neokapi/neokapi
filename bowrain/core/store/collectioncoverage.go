package store

import "context"

// CollectionCoverage is one collection's translatable blocks on a stream and,
// for each locale asked about, how many of them are established in that
// locale. CollectionID is empty for the blocks of items in no collection.
type CollectionCoverage struct {
	CollectionID string
	Blocks       int
	Established  map[string]int
}

// CollectionCoverageStore counts coverage per collection in aggregate
// queries, reading no block.
type CollectionCoverageStore interface {
	// CountCollectionCoverage answers, for each collection of a stream with a
	// translatable block, the translatable blocks and the established ones
	// per locale in locales. A block counts as established in a locale by the
	// same bucket CountBlocks reports as BlockStatusEstablished.
	CountCollectionCoverage(ctx context.Context, projectID, stream string, locales []string) ([]CollectionCoverage, error)
}

// CollectionCoverages finds the coverage store behind a content store,
// looking through any decorator that names what it wraps.
func CollectionCoverages(cs ContentStore) (CollectionCoverageStore, bool) {
	for cs != nil {
		if s, ok := cs.(CollectionCoverageStore); ok {
			return s, true
		}
		inner, ok := cs.(interface{ Unwrap() ContentStore })
		if !ok {
			return nil, false
		}
		cs = inner.Unwrap()
	}
	return nil, false
}
