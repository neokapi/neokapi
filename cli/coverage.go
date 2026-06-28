package cli

import (
	"context"
	"math"
	"sort"
	"strings"

	"github.com/neokapi/neokapi/core/gate"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
)

// LocaleCoverage is the ship-gate view for one target locale: the state
// distribution of its translatable units, whether it clears its gate, and which
// thresholds are still pending.
type LocaleCoverage struct {
	Locale    string           `json:"locale"`
	Total     int              `json:"total"`
	Pct       map[string]int   `json:"pct"`               // ladder state → "at least" percent (rounded)
	Gated     bool             `json:"gated"`             // a ship gate applies to this locale
	Shippable bool             `json:"shippable"`         // gate satisfied (or no gate)
	Pending   []gate.Shortfall `json:"pending,omitempty"` // unmet gate thresholds
}

// unitState derives a translatable block's lifecycle state for a locale. When a
// committed status is present (set by a producer / a future store-backed read)
// it is authoritative; otherwise a present, non-empty target counts as
// `translated` (the presence baseline) and an absent/empty target is untranslated.
// Lifecycle beyond translated (reviewed/signed-off) surfaces once per-unit state
// is persisted through interchange/transport — see the convergence model.
func unitState(b *model.Block, locale string) string {
	loc := model.LocaleID(locale)
	t := b.Target(loc)
	if t == nil || strings.TrimSpace(b.TargetText(loc)) == "" {
		return "" // untranslated — below every rung
	}
	if t.Status != "" {
		return string(t.Status)
	}
	return string(model.TargetStatusTranslated)
}

// computeShipCoverage rolls up per-locale coverage over the verify units and
// evaluates each locale against its resolved ship gate. Collection-scoped gate
// rules are not yet threaded through verify units, so gates resolve at locale
// scope (collection ""); collection rules are a follow-up (the gate engine
// already supports them).
func (a *App) computeShipCoverage(ctx context.Context, proj *project.KapiProject, units []verifyUnit) ([]LocaleCoverage, error) {
	rs, err := proj.BuildShipGates()
	if err != nil {
		return nil, err
	}

	statesByLocale := map[string][]string{}
	for _, u := range units {
		blocks, missing, berr := a.bilingualBlocks(ctx, u)
		if berr != nil {
			return nil, berr
		}
		if missing {
			// No target file yet — every translatable source unit is untranslated.
			srcs, serr := a.readBlocks(ctx, u.sourcePath, a.SourceLang)
			if serr != nil {
				return nil, serr
			}
			for _, b := range srcs {
				if b.Translatable {
					statesByLocale[u.locale] = append(statesByLocale[u.locale], "")
				}
			}
			continue
		}
		for _, b := range blocks {
			if !b.Translatable {
				continue
			}
			statesByLocale[u.locale] = append(statesByLocale[u.locale], unitState(b, u.locale))
		}
	}

	ladder := gate.TargetLadder()
	locales := make([]string, 0, len(statesByLocale))
	for loc := range statesByLocale {
		locales = append(locales, loc)
	}
	sort.Strings(locales)

	out := make([]LocaleCoverage, 0, len(locales))
	for _, loc := range locales {
		cov := gate.NewCoverage(statesByLocale[loc])
		lc := LocaleCoverage{Locale: loc, Total: cov.Total, Pct: map[string]int{}}
		for _, s := range ladder {
			lc.Pct[s] = int(math.Round(cov.AtLeastPct(ladder, s)))
		}
		if g, ok := rs.Resolve("", loc); ok {
			lc.Gated = true
			res := gate.Evaluate(g, cov, ladder)
			lc.Shippable = res.Pass
			lc.Pending = res.Shortfalls
		} else {
			// No gate matched this locale — nothing to gate on.
			lc.Shippable = true
		}
		out = append(out, lc)
	}
	return out, nil
}
