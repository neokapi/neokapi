package host

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
)

// ShipEntry is one locale's standing in the picker manifest: whether it clears
// its ship gate (safe to offer) and whether it clears its verified gate (a
// person reviewed or signed off — no AI badge). The two are independent.
type ShipEntry struct {
	Shippable bool `json:"shippable"`
	Verified  bool `json:"verified"`
}

// ShipManifest is the minimal, stable picker manifest `kapi status --ship`
// emits: locale → {shippable, verified}. A language picker consumes it to offer
// only shippable locales and to badge the shippable-but-unverified ones "AI".
// It is a deliberately tiny projection of the richer StatusOutput --json, so a
// build can emit it (kapi status --ship --emit ship.json) and ship it next to
// the app. Keys are the target locales; a build redirects or writes the file.
type ShipManifest map[string]ShipEntry

// BuildShipManifest projects the per-(collection, locale) coverage rows to the
// per-locale picker manifest. When a locale spans several collection scopes it
// is shippable only if every scope is shippable, and verified only if every
// scope is verified (a locale is no stronger than its weakest collection).
func BuildShipManifest(locales []LocaleCoverage) ShipManifest {
	m := ShipManifest{}
	for _, lc := range locales {
		e, seen := m[lc.Locale]
		if !seen {
			e = ShipEntry{Shippable: true, Verified: true}
		}
		e.Shippable = e.Shippable && lc.Shippable
		e.Verified = e.Verified && lc.Verified
		m[lc.Locale] = e
	}
	return m
}

// emitShipManifest writes the picker manifest as pretty JSON to the --emit path
// (a one-line note to stderr), or to stdout when no path is given so a build can
// redirect it. Keys are sorted for a deterministic, diff-friendly file.
func (a *App) emitShipManifest(cmd Command, m ShipManifest) error {
	// json.Marshal sorts map keys, but marshal through an ordered view so the
	// intent (stable output) is explicit and independent of encoder behavior.
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var buf []byte
	buf = append(buf, '{')
	for i, k := range keys {
		if i > 0 {
			buf = append(buf, ',')
		}
		entry, err := json.Marshal(m[k])
		if err != nil {
			return fmt.Errorf("encode ship manifest: %w", err)
		}
		buf = append(buf, "\n  "...)
		key, _ := json.Marshal(k)
		buf = append(buf, key...)
		buf = append(buf, ": "...)
		buf = append(buf, entry...)
	}
	if len(keys) > 0 {
		buf = append(buf, '\n')
	}
	buf = append(buf, '}', '\n')

	if path, _ := cmd.Flags().GetString("emit"); path != "" {
		if err := os.WriteFile(path, buf, 0o644); err != nil {
			return fmt.Errorf("write ship manifest: %w", err)
		}
		if !a.Quiet {
			fmt.Fprintf(cmd.ErrOrStderr(), "wrote %s (%d locales)\n", path, len(keys))
		}
		return nil
	}
	_, err := cmd.OutOrStdout().Write(buf)
	return err
}
