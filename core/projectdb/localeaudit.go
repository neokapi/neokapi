package projectdb

import (
	"context"
	"fmt"
	"sort"

	"github.com/neokapi/neokapi/core/blockstore"
	"github.com/neokapi/neokapi/core/model"
)

// Every subsystem in the store keys its rows by a canonical BCP-47 locale
// (model.NormalizeLocale) beside the text, and every lookup asks in the same
// form. A row keyed by any other spelling was written before the stores
// normalized, and no lookup will find it again: the content memory's variant
// rows, the terms store's term rows and the block cache's `targets/<locale>`
// overlays all persist across the digest-gated recompiles, so nothing heals
// them but a rebuild of the store. NonCanonicalLocales is how a command notices
// and says so, rather than reporting the leverage as absent.

// LocaleDrift is one non-canonical locale spelling a subsystem's rows are
// keyed by.
type LocaleDrift struct {
	// Subsystem is the store holding the rows: "content memory", "terms" or
	// "block cache".
	Subsystem string
	// Locale is the spelling the rows carry, and Canonical the form every
	// lookup asks in.
	Locale    string
	Canonical string
	// Rows is how many rows carry the spelling.
	Rows int
}

// String renders the drift the way a warning names it.
func (d LocaleDrift) String() string {
	return fmt.Sprintf("%s: %d row(s) under %q (lookups ask for %q)", d.Subsystem, d.Rows, d.Locale, d.Canonical)
}

// NonCanonicalLocales reports every locale spelling in the store that is not
// the canonical one, per subsystem, ordered by subsystem then spelling. A store
// whose rows are all keyed canonically reports nothing. A build with no
// file-backed store has no rows to audit and reports nothing either.
func (d *DB) NonCanonicalLocales(ctx context.Context) ([]LocaleDrift, error) {
	if d.raw == nil {
		return nil, nil
	}
	var out []LocaleDrift
	probes := []struct {
		subsystem string
		query     string
		canonical func(string) string
	}{
		{"content memory", `SELECT locale, COUNT(*) FROM tm_variants GROUP BY locale`, canonicalLocale},
		{"terms", `SELECT locale, COUNT(*) FROM tb_terms GROUP BY locale`, canonicalLocale},
		{"block cache", `SELECT kind, COUNT(*) FROM overlays WHERE kind LIKE 'targets/%' GROUP BY kind`, blockstore.CanonicalOverlayKind},
	}
	for _, p := range probes {
		rows, err := d.raw.QueryContext(ctx, p.query)
		if err != nil {
			return nil, fmt.Errorf("projectdb: audit %s locales: %w", p.subsystem, err)
		}
		for rows.Next() {
			var key string
			var n int
			if err := rows.Scan(&key, &n); err != nil {
				rows.Close()
				return nil, fmt.Errorf("projectdb: audit %s locales: %w", p.subsystem, err)
			}
			if canon := p.canonical(key); canon != key {
				out = append(out, LocaleDrift{Subsystem: p.subsystem, Locale: key, Canonical: canon, Rows: n})
			}
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return nil, fmt.Errorf("projectdb: audit %s locales: %w", p.subsystem, err)
		}
		rows.Close()
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Subsystem != out[j].Subsystem {
			return out[i].Subsystem < out[j].Subsystem
		}
		return out[i].Locale < out[j].Locale
	})
	return out, nil
}

func canonicalLocale(s string) string {
	return string(model.NormalizeLocale(model.LocaleID(s)))
}
