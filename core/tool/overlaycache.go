package tool

import (
	"crypto/sha256"
	"encoding/hex"
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
