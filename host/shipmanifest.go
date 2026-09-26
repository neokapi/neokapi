package host

import (
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"sort"
)

// ShipEntry is one locale's standing in the picker manifest: whether nothing
// withholds it (safe to offer) and its ship state.
type ShipEntry struct {
	Shippable bool `json:"shippable"`
	// State is the locale's ship state, folded across its scopes as
	// ConvergeLocaleResult.ShipState is: established (no AI badge), translated
	// (badged as AI translation), withheld, or not_gated. Shippable is true for
	// every state but withheld.
	State ShipState `json:"state"`
	// NotGoverned names the dimensions that govern nothing in the locale:
	// "terms" when no terms bound where the locale's content sits answer for
	// it. Neither gate reads it; it is there so a picker or a build never takes
	// an ungoverned locale for a governed one.
	NotGoverned []string `json:"not_governed,omitempty"`
}

// ShipManifest is the minimal, stable picker manifest `kapi status --ship`
// emits: locale → {shippable, state, not_governed}. A language picker consumes
// it to offer only shippable locales and to badge the ones not established "AI".
// It is a deliberately tiny projection of the richer StatusOutput --json, so a
// build can emit it (kapi status --ship --emit ship.json) and ship it next to
// the app. Keys are the target locales; a build redirects or writes the file.
type ShipManifest map[string]ShipEntry

// BuildShipManifest projects the per-(collection, locale) coverage rows to the
// per-locale picker manifest. When a locale spans several collection scopes it
// is shippable only if every scope is shippable, and its State is the weakest of
// its scopes' states (a locale is no stronger than its weakest collection). It names a dimension as not governed
// only when that dimension governs none of the locale's scopes.
func BuildShipManifest(locales []LocaleCoverage) ShipManifest {
	m := ShipManifest{}
	scopes := map[string]int{}
	ungoverned := map[string]map[string]int{}
	for _, lc := range locales {
		e, seen := m[lc.Locale]
		if !seen {
			e = ShipEntry{Shippable: true, State: ShipStateEstablished}
			ungoverned[lc.Locale] = map[string]int{}
		}
		e.Shippable = e.Shippable && lc.Shippable
		e.State = weakerShipState(e.State, lc.ShipState)
		scopes[lc.Locale]++
		for _, d := range lc.NotGoverned {
			ungoverned[lc.Locale][d]++
			if !slices.Contains(e.NotGoverned, d) {
				e.NotGoverned = append(e.NotGoverned, d)
			}
		}
		m[lc.Locale] = e
	}
	for locale, e := range m {
		e.NotGoverned = slices.DeleteFunc(e.NotGoverned, func(d string) bool {
			return ungoverned[locale][d] < scopes[locale]
		})
		if len(e.NotGoverned) == 0 {
			e.NotGoverned = nil
		}
		m[locale] = e
	}
	return m
}

// weakerShipState returns the weaker of two ship states, in the order withheld,
// not_gated, translated, established. A locale takes the weakest state among
// its scopes: one withheld scope withholds it, content no gate matches leaves
// it not gated, and one translated scope keeps it from reading established.
func weakerShipState(a, b ShipState) ShipState {
	rank := func(s ShipState) int {
		switch s {
		case ShipStateWithheld:
			return 0
		case ShipStateNotGated:
			return 1
		case ShipStateTranslated:
			return 2
		default:
			return 3
		}
	}
	if rank(b) < rank(a) {
		return b
	}
	return a
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
