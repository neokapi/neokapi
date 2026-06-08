package tools

import "github.com/neokapi/neokapi/core/model"

// Typed facet payloads for the analytic results tools produce. These replace
// the former opaque Block.Properties scalars: a tool that declares a Produces
// facet now writes that facet's typed payload rather than stuffing strings into
// Properties (which is pass-through metadata only). Each payload registers a
// constructor so the wire/store layers can rehydrate it by facet type.
//
// Block-scoped payloads are stored under the facet type as the key
// (block.SetAnno(string(FacetX), &XFacet{…})) and read with
// model.AnnoAs[*XFacet](block, string(FacetX)).

func init() {
	model.RegisterPayload(string(model.AnnoWordCount), func() any { return &WordCountFacet{} })
	model.RegisterPayload(string(model.AnnoCharCount), func() any { return &CharCountFacet{} })
	model.RegisterPayload(string(model.AnnoSegCount), func() any { return &SegCountFacet{} })
	model.RegisterPayload(string(model.AnnoTMMatch), func() any { return &TMMatchFacet{} })
	model.RegisterPayload(string(model.AnnoRepetition), func() any { return &RepetitionFacet{} })
}

// WordCountFacet carries source and per-locale target word counts (word-count tool).
type WordCountFacet struct {
	Source  int                    `json:"source"`
	Targets map[model.LocaleID]int `json:"targets,omitempty"`
}

// AnnotationType reports the facet type for registry/wire discrimination.
func (*WordCountFacet) AnnotationType() string { return string(model.AnnoWordCount) }

// CharCountFacet carries source and per-locale target character counts, with
// and without whitespace (char-count tool).
type CharCountFacet struct {
	Source         int                    `json:"source"`
	SourceNoSpace  int                    `json:"sourceNoSpace"`
	Targets        map[model.LocaleID]int `json:"targets,omitempty"`
	TargetsNoSpace map[model.LocaleID]int `json:"targetsNoSpace,omitempty"`
}

// AnnotationType reports the facet type.
func (*CharCountFacet) AnnotationType() string { return string(model.AnnoCharCount) }

// SegCountFacet carries source and target segment counts (segment-count tool).
type SegCountFacet struct {
	Source int `json:"source"`
	Target int `json:"target,omitempty"`
}

// AnnotationType reports the facet type.
func (*SegCountFacet) AnnotationType() string { return string(model.AnnoSegCount) }

// TMMatchFacet carries the best TM match score/type for a block, plus the
// segment-level "matched/total" summary when leveraged per segment (tm-leverage).
type TMMatchFacet struct {
	Score          int    `json:"score"`                    // 0-100
	Type           string `json:"type"`                     // "exact","fuzzy","segmented-exact",…
	SegmentMatches string `json:"segmentMatches,omitempty"` // "3/5"
}

// AnnotationType reports the facet type.
func (*TMMatchFacet) AnnotationType() string { return string(model.AnnoTMMatch) }

// RepetitionFacet carries a block's repetition classification (repetition-analysis).
type RepetitionFacet struct {
	Status string `json:"status"` // "unique","first-occurrence","repetition"
	Group  string `json:"group"`  // hash key linking repeated segments
	Count  int    `json:"count"`  // total occurrences of this text
	Index  int    `json:"index"`  // 1-based index within the group
}

// AnnotationType reports the facet type.
func (*RepetitionFacet) AnnotationType() string { return string(model.AnnoRepetition) }
