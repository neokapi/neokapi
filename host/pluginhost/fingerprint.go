package pluginhost

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
)

// Fingerprint digests what decides the plugin's output beyond its version
// string: its whole manifest (the version, which for okapi-bridge is the Okapi
// release it runs, and each model's identifier and revision) and the
// modification time of its install directory, which an upgrade or a fresh
// install changes. It is "" when the plugin has no manifest.
func (p *Plugin) Fingerprint() string {
	if p == nil || p.Manifest == nil {
		return ""
	}
	m, err := json.Marshal(p.Manifest)
	if err != nil {
		return ""
	}
	h := sha256.New()
	h.Write(m)
	fmt.Fprintf(h, "\x00%d", dirMtime(p.Dir))
	return hex.EncodeToString(h.Sum(nil))[:16]
}
