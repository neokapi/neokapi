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

// Variant returns a locale-only VariantKey, the common case.
//
// The locale is normalized (NormalizeLocale), so a target set under "nb_NO" is
// the target read under "nb-NO": the key is where a locale spelling becomes an
// identity, and two spellings of one locale must not become two variants.
func Variant(locale LocaleID) VariantKey { return VariantKey{Locale: NormalizeLocale(locale)} }

// Canonical returns the key with its locale normalized, the form every Targets
// map is keyed by. A key built as a struct literal, or decoded from a wire
// shape that spelled the locale another way, passes through here before it
// addresses a target.
func (k VariantKey) Canonical() VariantKey {
	k.Locale = NormalizeLocale(k.Locale)
	return k
}

// IsZero reports whether the key is the zero value.
func (k VariantKey) IsZero() bool { return k == VariantKey{} }

// MarshalText encodes a VariantKey as text so it can serve as a JSON/YAML map
// key. A locale-only key encodes as the bare locale ("fr"); optional
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

// UnmarshalText decodes a VariantKey produced by MarshalText. The locale is
// normalized on the way in, so a document that spelled it another way still
// keys the variant every reader looks up.
func (k *VariantKey) UnmarshalText(b []byte) error {
	parts := strings.Split(string(b), ";")
	*k = VariantKey{Locale: NormalizeLocale(LocaleID(parts[0]))}
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

// TargetStatusLadder is the lifecycle order, lowest to highest. Membership and
// order define "at least this status" coverage (used by ship gates). New ("") is
// not listed — it means "no committed status yet" and sits below every rung.
func TargetStatusLadder() []TargetStatus {
	return []TargetStatus{
		TargetStatusDraft,
		TargetStatusTranslated,
		TargetStatusReviewed,
		TargetStatusSignedOff,
	}
}

// Rank returns the 0-based position of s on the ladder, or -1 for New ("") or an
// unknown status. A higher rank is a more advanced lifecycle state.
func (s TargetStatus) Rank() int {
	for i, t := range TargetStatusLadder() {
		if t == s {
			return i
		}
	}
	return -1
}

// Origin records how content was produced, and under what context. On a Target
// it records how the committed translation was made; on a Block's source it
// records how a *recognized* source was extracted (ocr, asr) — source and target
// provenance are the same record on two sides of the Block.
//
// The Kind/Engine/Tool/Reference/Timestamp/Confidence group answers *how* it was
// made. The Profile group answers *what governed it*: which named context was in
// force at the moment of production. That second half cannot be reconstructed
// after the fact — a profile is edited in place and a timestamp is only a proxy
// for it, one that breaks for imported content and for anything produced while a
// pilot shadowed a stream. So it is recorded at production time or approximated
// forever.
type Origin struct {
	Kind      string `json:"kind,omitempty"`      // human | memory | mt | ai | ocr | asr
	Engine    string `json:"engine,omitempty"`    // MT/AI/OCR/ASR engine name
	Tool      string `json:"tool,omitempty"`      // tool id that produced it
	Reference string `json:"reference,omitempty"` // batch id, content-memory entry, etc.
	Timestamp string `json:"timestamp,omitempty"` // RFC 3339
	// Confidence is the recognizer's confidence in [0,1] for content produced by
	// extraction (ocr, asr); 0 = unset/not applicable. A confidence-gated
	// refinement step reads this to decide which units to re-examine.
	Confidence float64 `json:"confidence,omitempty"`
	// Profile identifies the context profile that governed production, and
	// ProfileVersion pins the revision of it that was in force. Both are opaque
	// to the model: the producer stamps whatever its resolver handed it, and
	// nothing here parses them. Empty when the producer resolved no profile —
	// an ad-hoc run, or a tool that takes no context (pseudo-translation).
	Profile        string `json:"profile,omitempty"`
	ProfileVersion string `json:"profile_version,omitempty"`
	// ContextFingerprint is a content hash of the governing context as it
	// actually reached the producer — the rendered voice guidance and the
	// terminology it was given. It exists because Profile/ProfileVersion pin
	// only half of that: terminology reaches a producer separately from the
	// profile and carries no version of its own, so a profile stamp alone
	// cannot tell whether the terms have moved since.
	//
	// It answers "have the governing inputs changed since this was produced?"
	// — a change detector, not a snapshot: it cannot reconstruct what the
	// context was, only tell you this target no longer matches it.
	//
	// Deliberately NOT the engine's config fingerprint. That one also covers
	// provider, model and prompt wording, so swapping models would move it and
	// destroy its meaning as a statement about governance. Empty when no
	// context governed production at all.
	ContextFingerprint string `json:"context_fingerprint,omitempty"`
}

// Origin Kind values. The translation kinds (human, tm, mt, ai) describe how a
// Target was produced; the extraction kinds (ocr, asr) describe how a recognized
// source was produced. OriginLLMRefined is a derived extraction kind: a recognized
// source (ocr/asr) that a multimodal LLM re-read and rewrote (media-refine), the
// least-verified recognition tier — distinguished so refined units are queryable
// without parsing the Engine string. The original recognizer's engine is
// preserved in Origin.Engine; the refining tool/provider lives in Tool/Reference.
const (
	OriginHuman      = "human"
	OriginMemory     = "memory"
	OriginMT         = "mt"
	OriginAI         = "ai"
	OriginOCR        = "ocr"
	OriginASR        = "asr"
	OriginLLMRefined = "llm-refined"
)

// IsRecognized reports whether an Origin Kind denotes machine-recognized source
// content (ocr, asr, or an llm-refined recognition) — content a review/refine
// tier treats as provisional, as opposed to losslessly parsed or human-authored
// source.
func IsRecognized(kind string) bool {
	switch kind {
	case OriginOCR, OriginASR, OriginLLMRefined:
		return true
	default:
		return false
	}
}

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
