package place

import (
	"fmt"
	"sort"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// Search finds places by id or name: places whose id starts with query first,
// then places whose name starts with it, then places whose name holds it.
// Matching ignores case and accents, so "grunerlokka" finds Grünerløkka. A
// declared place's names in other locales count as names. Within each tier,
// the place whose id is the query comes first, then wider kinds of place
// (Type.Rank), then ids in order. A limit of
// zero or less returns every match; an empty query returns none.
func (g *Gazetteer) Search(query string, limit int) []Place {
	g = g.or()
	q := fold(strings.TrimSpace(query))
	if q == "" {
		return nil
	}
	var tiers [3][]*Place
	consider := func(p *Place, names []string) {
		if strings.HasPrefix(fold(p.ID), q) {
			tiers[0] = append(tiers[0], p)
			return
		}
		prefix, sub := false, false
		for _, n := range names {
			if strings.HasPrefix(n, q) {
				prefix = true
			} else if strings.Contains(n, q) {
				sub = true
			}
		}
		switch {
		case prefix:
			tiers[1] = append(tiers[1], p)
		case sub:
			tiers[2] = append(tiers[2], p)
		}
	}
	for _, id := range g.ownIDs {
		p := g.own[id]
		names := []string{fold(p.Name)}
		for _, n := range p.Names {
			names = append(names, fold(n))
		}
		consider(p, names)
	}
	for _, id := range g.base.order {
		consider(g.base.places[id], []string{g.base.folded[id]})
	}
	var out []Place
	for _, tier := range tiers {
		sortTier(tier, q)
		for _, p := range tier {
			if limit > 0 && len(out) >= limit {
				return out
			}
			out = append(out, p.clone())
		}
	}
	return out
}

// sortTier orders one tier of matches: the place whose id is the query
// first, then wider kinds of place, then ids in order.
func sortTier(ps []*Place, q string) {
	sort.SliceStable(ps, func(i, j int) bool { return less(ps[i], ps[j], q) })
}

func less(a, b *Place, q string) bool {
	if ea, eb := a.ID == q, b.ID == q; ea != eb {
		return ea
	}
	if ra, rb := a.Type.Rank(), b.Type.Rank(); ra != rb {
		return ra < rb
	}
	return a.ID < b.ID
}

// letterFolds spell the letters that carry no combining mark in Unicode,
// which decomposition alone leaves as they are.
var letterFolds = strings.NewReplacer(
	"ø", "o", "æ", "ae", "œ", "oe", "ß", "ss", "ł", "l", "đ", "d", "ð", "d", "þ", "th", "ı", "i", "ħ", "h",
)

// fold lowers s and strips its accents, for matching.
func fold(s string) string {
	s = letterFolds.Replace(strings.ToLower(s))
	var b strings.Builder
	for _, r := range norm.NFD.String(s) {
		if unicode.Is(unicode.Mn, r) {
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// Hint teaches the spelling of a place id that names no place: the id in
// lower case or with its hyphen when that names one, the closest places by
// name, and how to look one up.
func Hint(g *Gazetteer, value string) string {
	g = g.or()
	lower := strings.ToLower(strings.TrimSpace(value))
	if lower != value && g.Has(lower) {
		return "Write it in lower case: " + lower
	}
	if len(lower) > 2 && lower[2] != '-' {
		if hyphen := lower[:2] + "-" + lower[2:]; g.Has(hyphen) {
			return "Write it with a hyphen: " + hyphen
		}
	}
	look := fmt.Sprintf("Look a place up with `kapi context places %s`", shellWord(value))
	if matches := g.Search(value, 3); len(matches) > 0 {
		names := make([]string, len(matches))
		for i, m := range matches {
			names[i] = fmt.Sprintf("%s (%s)", m.ID, g.Describe(m.ID))
		}
		return "Did you mean " + strings.Join(names, ", ") + "? " + look
	}
	return "A place is world, a UN M49 region (northern-europe), an ISO 3166-1 country in lower case (no), an ISO 3166-2 subdivision (no-03), or a place the recipe declares under `places:`. " + look
}

// shellWord quotes a query that a shell would split.
func shellWord(s string) string {
	if s == "" || strings.ContainsAny(s, " \t'\"$`\\*?[]{}()<>|&;!#~") {
		return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
	}
	return s
}
