package model

// This file defines the facet vocabulary (AD-002 / tool-data-model redesign).
// A *facet* is any typed, stand-off interpretation that rides on a Block: the
// positional ones (segmentation, term, entity, qa, alignment) and the
// block-scoped ones (alt-translation, note, analysis results, …). Facets are
// the single carrier for stand-off block data — there is no separate annotation
// interface. Each facet has a stable FacetType, registered with a payload
// constructor so the wire and store layers can rehydrate the typed Value.

// FacetType names a kind of facet. Built-in content facets have stable
// constants below; formats and plugins may use any string (registered via
// RegisterFacet) for their own stand-off state.
type FacetType string

const (
	// Positional, run-anchored content facets.
	FacetSegmentation FacetType = "segmentation"
	FacetTerm         FacetType = "term"
	FacetEntity       FacetType = "entity"
	FacetQA           FacetType = "qa"
	FacetAlignment    FacetType = "alignment"

	// Block-scoped content facets.
	FacetAltTranslation FacetType = "alt-translation"
	FacetNote           FacetType = "note"
	FacetTMMatch        FacetType = "tm-match"
	FacetWordCount      FacetType = "word-count"
	FacetCharCount      FacetType = "char-count"
	FacetSegCount       FacetType = "seg-count"
	FacetComparison     FacetType = "comparison"
	FacetScopingReport  FacetType = "scoping-report"
	FacetRepetition     FacetType = "repetition"
	FacetBrandVoice     FacetType = "brand-voice"
	FacetTermCandidate  FacetType = "term-candidate"
	FacetEntityMapping  FacetType = "entity-mapping"
	FacetTermEnforce    FacetType = "term-enforcement"

	// Pseudo-facets for the flow IO contract (AD-006): produced/consumed
	// outputs that are not stored as stand-off facets but participate in
	// data-flow validation. FacetTarget is the committed Target; FacetSource is
	// a rewritten source.
	FacetTarget FacetType = "target"
	FacetSource FacetType = "source"
)

// FacetSide names which run sequence of a Block a facet pertains to.
type FacetSide int

const (
	// SideSource: the facet pertains to Block.Source.
	SideSource FacetSide = iota
	// SideTarget: the facet pertains to a target variant (see Facet.Variant).
	SideTarget
)

// String renders the side as the wire token used in facet metadata.
func (s FacetSide) String() string {
	switch s {
	case SideTarget:
		return "target"
	default:
		return "source"
	}
}

// MarshalText encodes the side as its string token so facet metadata is
// human-readable on the wire and in the flow editor.
func (s FacetSide) MarshalText() ([]byte, error) { return []byte(s.String()), nil }

// UnmarshalText decodes the string token form ("source"/"target").
func (s *FacetSide) UnmarshalText(b []byte) error {
	if string(b) == "target" {
		*s = SideTarget
	} else {
		*s = SideSource
	}
	return nil
}

// IsPositional reports whether the facet type is one of the built-in
// run-anchored positional interpretations (segmentation, term, entity, qa,
// alignment). Block-scoped facets — the former annotations, keyed by an
// arbitrary type string — are non-positional. The distinction lets the single
// facet carrier hold both kinds while keeping positional iteration and
// block-scoped (annotation) lookup separate.
func (t FacetType) IsPositional() bool {
	switch t {
	case FacetSegmentation, FacetTerm, FacetEntity, FacetQA, FacetAlignment:
		return true
	default:
		return false
	}
}
