package model

import "strings"

// This file defines the variant-keyed target model (AD-002). A Block's
// committed translations are first-class Target records keyed by a VariantKey
// rather than bare locale→runs slots. Locale is the only required variant
// dimension; tone and channel are optional, so locale-only code carries no
// extra ceremony. Candidate/alternative translations stay as stand-off
// alt-translation overlays; a Target is the chosen one.

// VariantKey identifies a target variant. Locale is required; Tone and
// Channel are optional (empty = unspecified). The zero-extension form is a
// valid Go map key, so map[VariantKey]*Target keyed by a locale-only key is
// the common case.
type VariantKey struct {
	Locale  LocaleID `json:"locale"`
	Tone    string   `json:"tone,omitempty"`
	Channel string   `json:"channel,omitempty"`
}

// Variant returns a locale-only VariantKey — the common case.
func Variant(locale LocaleID) VariantKey { return VariantKey{Locale: locale} }

// IsZero reports whether the key is the zero value.
func (k VariantKey) IsZero() bool { return k == VariantKey{} }

// MarshalText encodes a VariantKey as text so it can serve as a JSON/YAML map
// key. A locale-only key encodes as the bare locale ("fr-FR"); optional
// dimensions append as ";tone=…" / ";channel=…".
func (k VariantKey) MarshalText() ([]byte, error) {
	s := string(k.Locale)
	if k.Tone != "" {
		s += ";tone=" + k.Tone
	}
	if k.Channel != "" {
		s += ";channel=" + k.Channel
	}
	return []byte(s), nil
}

// UnmarshalText decodes a VariantKey produced by MarshalText.
func (k *VariantKey) UnmarshalText(b []byte) error {
	parts := strings.Split(string(b), ";")
	*k = VariantKey{Locale: LocaleID(parts[0])}
	for _, p := range parts[1:] {
		name, val, ok := strings.Cut(p, "=")
		if !ok {
			continue
		}
		switch name {
		case "tone":
			k.Tone = val
		case "channel":
			k.Channel = val
		}
	}
	return nil
}

// TargetStatus is the lifecycle state of a committed translation.
type TargetStatus string

const (
	TargetStatusNew        TargetStatus = ""
	TargetStatusDraft      TargetStatus = "draft"
	TargetStatusTranslated TargetStatus = "translated"
	TargetStatusReviewed   TargetStatus = "reviewed"
	TargetStatusSignedOff  TargetStatus = "signed-off"
)

// Origin records how content was produced. On a Target it records how the
// committed translation was made; on a Block's source it records how a
// *recognized* source was extracted (ocr, asr) — source and target provenance
// are the same record on two sides of the Block.
type Origin struct {
	Kind      string `json:"kind,omitempty"`      // human | tm | mt | ai | ocr | asr
	Engine    string `json:"engine,omitempty"`    // MT/AI/OCR/ASR engine name
	Tool      string `json:"tool,omitempty"`      // tool id that produced it
	Reference string `json:"reference,omitempty"` // batch id, TM entry, etc.
	Timestamp string `json:"timestamp,omitempty"` // RFC 3339
	// Confidence is the recognizer's confidence in [0,1] for content produced by
	// extraction (ocr, asr); 0 = unset/not applicable. A confidence-gated
	// refinement step reads this to decide which units to re-examine.
	Confidence float64 `json:"confidence,omitempty"`
}

// Origin Kind values. The translation kinds (human, tm, mt, ai) describe how a
// Target was produced; the extraction kinds (ocr, asr) describe how a recognized
// source was produced.
const (
	OriginHuman = "human"
	OriginTM    = "tm"
	OriginMT    = "mt"
	OriginAI    = "ai"
	OriginOCR   = "ocr"
	OriginASR   = "asr"
)

// AnnoSourceOrigin is the block-scoped annotation key carrying a Block's source
// *Origin — how its source content was produced when it was extracted rather
// than parsed (the source-side counterpart of Target.Origin). Absent for content
// read losslessly from a text format.
const AnnoSourceOrigin = "source-origin"

// TypeName implements Payload, so an *Origin can ride the block annotation map as
// the source-provenance facet.
func (*Origin) TypeName() string { return AnnoSourceOrigin }

// SourceOrigin returns the block's source Origin (recognition provenance), or
// (nil, false) for content that was parsed rather than recognized.
func (b *Block) SourceOrigin() (*Origin, bool) {
	return AnnoAs[*Origin](b, AnnoSourceOrigin)
}

// SetSourceOrigin stores the block's source Origin.
func (b *Block) SetSourceOrigin(o *Origin) { b.SetAnno(AnnoSourceOrigin, o) }

// Target is the committed translation for one variant: the content plus its
// lifecycle and provenance.
type Target struct {
	Runs   []Run        `json:"runs"`
	Status TargetStatus `json:"status,omitempty"`
	Origin Origin       `json:"origin,omitzero"`
	Score  float64      `json:"score,omitempty"`
}

// NewTarget builds a Target from a Run sequence with the given status.
func NewTarget(runs []Run, status TargetStatus) *Target {
	return &Target{Runs: runs, Status: status}
}
