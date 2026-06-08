package tools

import "github.com/neokapi/neokapi/core/model"

// Typed payloads for the analytic results tools produce, stored as block
// annotations rather than opaque Block.Properties scalars: a tool that declares
// a Produces port for one of these writes the typed payload instead of stuffing
// strings into Properties (which is pass-through metadata only). Each payload
// registers a constructor so the wire/store layers can rehydrate it by type.
//
// They are stored under the annotation type as the key
// (block.SetAnno(string(AnnoX), &XAnnotation{…})) and read with
// model.AnnoAs[*XAnnotation](block, string(AnnoX)).

func init() {
	model.RegisterPayload(string(model.AnnoWordCount), func() model.Payload { return &WordCountAnnotation{} })
	model.RegisterPayload(string(model.AnnoCharCount), func() model.Payload { return &CharCountAnnotation{} })
	model.RegisterPayload(string(model.AnnoSegCount), func() model.Payload { return &SegCountAnnotation{} })
	model.RegisterPayload(string(model.AnnoTMMatch), func() model.Payload { return &TMMatchAnnotation{} })
	model.RegisterPayload(string(model.AnnoRepetition), func() model.Payload { return &RepetitionAnnotation{} })
}

// WordCountAnnotation carries source and per-locale target word counts (word-count tool).
type WordCountAnnotation struct {
	Source  int                    `json:"source"`
	Targets map[model.LocaleID]int `json:"targets,omitempty"`
}

// AnnotationType reports the annotation type for registry/wire discrimination.
func (*WordCountAnnotation) TypeName() string { return string(model.AnnoWordCount) }

// CharCountAnnotation carries source and per-locale target character counts, with
// and without whitespace (char-count tool).
type CharCountAnnotation struct {
	Source         int                    `json:"source"`
	SourceNoSpace  int                    `json:"sourceNoSpace"`
	Targets        map[model.LocaleID]int `json:"targets,omitempty"`
	TargetsNoSpace map[model.LocaleID]int `json:"targetsNoSpace,omitempty"`
}

// AnnotationType reports the annotation type for registry/wire discrimination.
func (*CharCountAnnotation) TypeName() string { return string(model.AnnoCharCount) }

// SegCountAnnotation carries source and target segment counts (segment-count tool).
type SegCountAnnotation struct {
	Source int `json:"source"`
	Target int `json:"target,omitempty"`
}

// AnnotationType reports the annotation type for registry/wire discrimination.
func (*SegCountAnnotation) TypeName() string { return string(model.AnnoSegCount) }

// TMMatchAnnotation carries the best TM match score/type for a block, plus the
// segment-level "matched/total" summary when leveraged per segment (tm-leverage).
type TMMatchAnnotation struct {
	Score          int    `json:"score"`                    // 0-100
	Type           string `json:"type"`                     // "exact","fuzzy","segmented-exact",…
	SegmentMatches string `json:"segmentMatches,omitempty"` // "3/5"
}

// AnnotationType reports the annotation type for registry/wire discrimination.
func (*TMMatchAnnotation) TypeName() string { return string(model.AnnoTMMatch) }

// RepetitionAnnotation carries a block's repetition classification (repetition-analysis).
type RepetitionAnnotation struct {
	Status string `json:"status"` // "unique","first-occurrence","repetition"
	Group  string `json:"group"`  // hash key linking repeated segments
	Count  int    `json:"count"`  // total occurrences of this text
	Index  int    `json:"index"`  // 1-based index within the group
}

// AnnotationType reports the annotation type for registry/wire discrimination.
func (*RepetitionAnnotation) TypeName() string { return string(model.AnnoRepetition) }
