package model

// SkeletonStrategy defines how document structure is preserved for reconstruction.
type SkeletonStrategy int

const (
	// SkeletonFragmentBased stores interleaved text/reference fragments.
	SkeletonFragmentBased SkeletonStrategy = iota
	// SkeletonReparse re-reads the source document during writing.
	SkeletonReparse
)

// Skeleton preserves non-translatable document structure for reconstruction.
type Skeleton struct {
	Strategy  SkeletonStrategy
	Parts     []SkeletonPart // Used by fragment-based strategy
	SourceURI string         // Used by re-parse strategy
}

// SkeletonPart is either a literal text fragment or a reference to a Block/Data.
type SkeletonPart interface {
	isSkeletonPart()
}

// SkeletonText is a literal text fragment in the skeleton.
type SkeletonText struct {
	Text string
}

func (st *SkeletonText) isSkeletonPart() {}

// SkeletonRef is a reference to a Block or Data resource in the skeleton.
type SkeletonRef struct {
	ResourceID string
	Property   string // Which property to reference (e.g., "target", "source")
	Locale     string // Target locale for locale-specific references
}

func (sr *SkeletonRef) isSkeletonPart() {}
