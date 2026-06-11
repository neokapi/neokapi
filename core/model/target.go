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

// Origin records how a committed translation was produced.
type Origin struct {
	Kind      string `json:"kind,omitempty"`      // human | tm | mt | ai
	Engine    string `json:"engine,omitempty"`    // MT/AI engine name
	Tool      string `json:"tool,omitempty"`      // tool id that produced it
	Reference string `json:"reference,omitempty"` // batch id, TM entry, etc.
	Timestamp string `json:"timestamp,omitempty"` // RFC 3339
}

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
