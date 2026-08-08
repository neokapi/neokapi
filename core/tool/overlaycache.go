package tool

import (
	"crypto/sha256"
	"encoding/hex"
	"maps"
	"slices"
)

// OverlayConfigFingerprint returns a short, stable hash of the config inputs
// that shape a tool's per-block output. The session overlay cache stores it
// alongside each cached result, and a tool reuses a cached result only when this
// fingerprint matches its current config. So changing a model, prompt, brand
// voice, or glossary re-runs the tool (the old output would be stale), while an
// unchanged config still skips the work — the "never run unneeded processing,
// never serve stale processing" contract of the project's overlay cache.
//
// Pass every output-affecting setting as a part; order matters, so callers must
// build the parts deterministically (e.g. sort map entries).
func OverlayConfigFingerprint(parts ...string) string {
	h := sha256.New()
	for _, p := range parts {
		h.Write([]byte(p))
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))[:16]
}

// ContextFingerprint hashes the governing context as it reached a producer: the
// rendered voice guidance and the terminology it was given. It backs
// model.Origin.ContextFingerprint — a change detector that answers "have the
// governing inputs moved since this target was produced?" and nothing more.
//
// It is deliberately narrower than an engine's OverlayConfigFingerprint, which
// also covers provider, model and prompt wording: folding those in would make a
// model swap read as a governance change and destroy the field's meaning. Every
// translation producer computes it the same way, so targets written under one
// context share a fingerprint regardless of which engine produced them and fall
// stale together when the context moves.
//
// The glossary is a map, so its keys are sorted before hashing: an unsorted walk
// would hash identical terminology differently on each run and report drift that
// never happened. Returns "" when there is no governing context at all, so an
// ungoverned run reads as ungoverned rather than as the constant hash of two
// empty strings.
func ContextFingerprint(voiceGuide string, glossary map[string]string) string {
	if voiceGuide == "" && len(glossary) == 0 {
		return ""
	}
	parts := make([]string, 0, len(glossary)*2+1)
	parts = append(parts, voiceGuide)
	for _, src := range slices.Sorted(maps.Keys(glossary)) {
		parts = append(parts, src, glossary[src])
	}
	return OverlayConfigFingerprint(parts...)
}
