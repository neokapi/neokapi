package tool

import (
	"crypto/sha256"
	"encoding/hex"
	"maps"
	"slices"

	"github.com/neokapi/neokapi/core/partjoin"
)

// OverlayConfigFingerprint returns a short, stable hash of the config inputs
// that shape a tool's per-block output. The session overlay cache stores it
// alongside each cached result, and a tool reuses a cached result only when this
// fingerprint matches its current config. So changing a model, prompt, voice
// profile, or terminology re-runs the tool (the old output would be stale), while an
// unchanged config still skips the work, the "never run unneeded processing,
// never serve stale processing" contract of the project's overlay cache.
//
// Pass every output-affecting setting as a part; order matters, so callers must
// build the parts deterministically (e.g. sort map entries).
func OverlayConfigFingerprint(parts ...string) string {
	h := sha256.New()
	partjoin.WriteNUL(h, parts...)
	return hex.EncodeToString(h.Sum(nil))[:16]
}

// ContextFingerprint hashes the governing context as it reached a producer: the
// rendered voice guidance, the terminology it was given, and the terms it was
// told to keep verbatim. It backs model.Origin.ContextFingerprint, which says
// whether the governing inputs moved since a target was produced.
//
// It is narrower than an engine's OverlayConfigFingerprint, which also covers
// provider, model and prompt wording, so a model swap does not read as a
// governance change. Every translation producer computes it the same way, so
// targets written under one context share a fingerprint and fall stale
// together.
//
// The terms map's keys are sorted before hashing, so identical terminology
// hashes alike on every run; the caller sorts keepVerbatim. It returns "" when
// there is no governing context, so an ungoverned run reads as ungoverned.
//
// An empty keepVerbatim contributes nothing to the hash, so a project that
// marks no concept do-not-translate keeps the fingerprints it already has.
func ContextFingerprint(voiceGuide string, terms map[string]string, keepVerbatim []string) string {
	if voiceGuide == "" && len(terms) == 0 && len(keepVerbatim) == 0 {
		return ""
	}
	parts := make([]string, 0, len(terms)*2+len(keepVerbatim)+2)
	parts = append(parts, voiceGuide)
	for _, src := range slices.Sorted(maps.Keys(terms)) {
		parts = append(parts, src, terms[src])
	}
	if len(keepVerbatim) > 0 {
		parts = append(parts, "keep-verbatim")
		parts = append(parts, keepVerbatim...)
	}
	return OverlayConfigFingerprint(parts...)
}
