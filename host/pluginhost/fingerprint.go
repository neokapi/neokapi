package pluginhost

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
)

// Fingerprint names the plugin's bytes, for the parse cache key: the same
// plugin installed again gives the same fingerprint, and a different build
// gives another, whatever its version string says. It digests the whole
// manifest (the version, which for okapi-bridge is the Okapi release it runs,
// and each model's identifier and revision) with one of:
//
//   - the sha256 of the archive `kapi plugins install` installed it from
//     (installed.json), which a reinstall of the same release repeats, so a
//     fresh install on every CI run still finds the cache;
//   - for a plugin installed another way (a development build, a package
//     manager), the size and modification time of its executable. A rebuild
//     changes them, so its parses are made again; a copy that keeps them
//     (a package manager preserving the archive's times) keeps the cache.
//
// It is "" when the plugin has no manifest or neither is known.
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
	switch {
	case p.Archive != "":
		fmt.Fprintf(h, "\x00archive:%s", p.Archive)
	case p.BinaryModTime != 0:
		fmt.Fprintf(h, "\x00binary:%d:%d", p.BinarySize, p.BinaryModTime)
	default:
		return ""
	}
	return hex.EncodeToString(h.Sum(nil))[:16]
}
