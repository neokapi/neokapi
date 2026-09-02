package blockstore

import (
	"github.com/neokapi/neokapi/core/model"
)

// TargetOverlayKind names the overlay a locale's committed translations live
// under.
func TargetOverlayKind(locale model.LocaleID) string { return "targets/" + string(locale) }

// TargetOverlay is the payload of a `targets/<locale>` overlay: one block's
// translation as the store holds it, what produced it, and what a later run
// needs to decide whether it can serve it again.
//
// Four writers reach this key. The AI and MT translate tools write it as they
// translate, and the trailing commit-targets step re-affirms it for the merge
// round-trip. commit-targets runs last, so a second shape written there would
// replace the producers' record with one they cannot decode, and the next run
// would re-translate the whole locale at full provider cost while the store held
// its answers (#2356). One shape, written by all of them, keeps each write
// legible to every reader.
//
// Runs are preferred over Text on read so inline markup round-trips; Text and
// the legacy Target alias serve run-free targets.
type TargetOverlay struct {
	Runs   []model.Run `json:"runs,omitempty"`
	Text   string      `json:"text,omitempty"`
	Target string      `json:"target,omitempty"`
	Status string      `json:"status,omitempty"`

	// Provider names the engine that produced the translation, and Config is
	// that producer's fingerprint of everything the answer depended on besides
	// the source: model, prompt, voice profile, term rules, neighbourhood. A
	// producer reuses a stored target only when Config still matches what it
	// would send now, so a changed model or a moved governing context re-drafts.
	Provider string `json:"provider,omitempty"`
	Config   string `json:"config,omitempty"`

	// Source is model.ComputeContentHash of the source text the translation was
	// made from. The overlay key namespaces a block by file and id but not by
	// wording (StoreKey), so an edited sentence keeps the key its old
	// translation sits under; this is what tells the two apart. An overlay
	// carrying no source stamp is treated as no answer, rather than as a
	// promise that the source stood still.
	Source string `json:"source,omitempty"`

	// Origin is the provenance the producer stamped: how the target was made,
	// and the governing context it was made under (#2344). It travels here
	// because most target formats have nowhere to keep it. A plain JSON catalog
	// holds strings and nothing else, so for those the overlay is the only
	// durable record of what governed the answer, and both the staleness gate
	// and the recycle decision read the stamp from here. A pointer, so an
	// unstamped target writes no `origin` key at all and reads back as the
	// absence it is.
	Origin *model.Origin `json:"origin,omitempty"`
}

// OverlayOrigin is the provenance an overlay carries for a target, or nil when
// the producer stamped none.
func OverlayOrigin(o model.Origin) *model.Origin {
	if o == (model.Origin{}) {
		return nil
	}
	return &o
}

// SourceStamp is the value TargetOverlay.Source takes for a given source text.
func SourceStamp(sourceText string) string { return model.ComputeContentHash(sourceText) }

// TargetText renders the overlay's translation: the runs when it has them,
// otherwise the plain text (or the legacy Target alias).
func (o TargetOverlay) TargetText() string {
	if len(o.Runs) > 0 {
		return model.RunsText(o.Runs)
	}
	if o.Text != "" {
		return o.Text
	}
	return o.Target
}

// ContextFingerprint is the governing context the producer stamped, empty when
// it stamped none.
func (o TargetOverlay) ContextFingerprint() string {
	if o.Origin == nil {
		return ""
	}
	return o.Origin.ContextFingerprint
}

// ReusableFor reports whether a producer about to translate sourceText can
// serve this stored target instead of calling out. Three anchors must hold:
//
//   - the same source wording (Source), because the key says nothing about it;
//   - the same producer configuration (Config), which covers the engine, the
//     model and the prompt as well as the governance it sends;
//   - the same governing context (Origin.ContextFingerprint), the stamp every
//     producer computes the same way, so a voice profile or term rule that has
//     moved re-drafts whichever engine made the original.
//
// An overlay carrying neither Config nor a source stamp is not reusable: it was
// written by a producer that recorded neither, and serving it would answer a
// question nobody asked.
func (o TargetOverlay) ReusableFor(sourceText, config, contextFP string) bool {
	if o.Source == "" || o.Config == "" {
		return false
	}
	if o.TargetText() == "" {
		return false
	}
	return o.Source == SourceStamp(sourceText) && o.Config == config && o.ContextFingerprint() == contextFP
}
