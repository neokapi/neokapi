package host

import (
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/neokapi/neokapi/core/graph"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/terms"
)

// Rules held elsewhere in a project, and the places they are held at.
//
// A rename scoped to the help pages leaves the API reference with the old
// name, and the old name is correct there. An answer that only leaves the rule
// out of the API reference's list says nothing an agent can act on: told by a
// person to "rename Workspace to Space", it renames the API reference too. So
// an answer for a file names every rule held at another place of the project
// that does not hold at this one, as the wording that stays as it is here.

// ContextElsewhere is one rule held at other places of the project and not at
// the point an answer is for: the wording that is correct here, and the
// wording the rule asks for where it holds.
type ContextElsewhere struct {
	// Keep are the wordings that stay as they are here.
	Keep []string `json:"keep"`
	// Instead is what the rule asks for where it holds.
	Instead string `json:"instead"`
	// HeldIn names the places the rule holds at: a folder (`help/`), or the
	// pattern of a collection that has no folder of its own.
	HeldIn []string `json:"held_in"`
	// Locale is the language the rule is stated in.
	Locale string `json:"locale,omitempty"`
}

// rulePlace is one place in a project a rule can hold at: the files one
// content item of a collection reads.
type rulePlace struct {
	// Dir is the folder the item's files sit under, slash-separated and empty
	// for the project root.
	Dir string
	// Collection is the collection's name, empty for an unnamed entry.
	Collection string
	// Pattern is the item's pattern, as the recipe matches it.
	Pattern string
	// Sample is a path the item claims, made from its pattern. A point is
	// resolved for a path, and every path the item claims resolves to the
	// same one.
	Sample string
}

// Label names the place as a reader finds it: its folder, or its pattern when
// its files sit at the project root.
func (p rulePlace) Label() string {
	if p.Dir != "" {
		return p.Dir + "/"
	}
	return p.Pattern
}

// rulePlaces lists the places a project's collections read, in recipe order.
// An item whose sample another item claims first is left out, because its
// files resolve at that item's point, and so is an item that reads only the
// comments of its files.
func (a *App) rulePlaces(proj *project.KapiProject, root string) []rulePlace {
	if proj == nil {
		return nil
	}
	var out []rulePlace
	seen := map[string]bool{}
	for i := range proj.Collections {
		coll := &proj.Collections[i]
		for _, item := range coll.EffectiveItems() {
			for _, pattern := range expandBraces(item.Path) {
				sample, ok := globSample(pattern)
				if !ok || !project.MatchGlob(item.Path, sample) || seen[item.Path+"\x00"+sample] {
					continue
				}
				if ProjectIgnores(root, sample) {
					continue
				}
				noReader := a.NoReaderFor(filepath.Join(root, filepath.FromSlash(sample)))
				claimed, ci, claims := proj.ContentItemForPath(sample, noReader)
				if !claims || ci != i || claimed.Path != item.Path {
					continue
				}
				seen[item.Path+"\x00"+sample] = true
				out = append(out, rulePlace{Dir: globDir(pattern), Collection: coll.Name, Pattern: pattern, Sample: sample})
			}
		}
	}
	return out
}

// expandBraces expands the brace alternations in a pattern, so
// `{docs,web}/**/*.md` is two places with a folder each.
func expandBraces(pattern string) []string {
	open := strings.IndexByte(pattern, '{')
	if open < 0 {
		return []string{pattern}
	}
	depth, end := 0, -1
	for i := open; i < len(pattern) && end < 0; i++ {
		switch pattern[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				end = i
			}
		}
	}
	if end < 0 {
		return []string{pattern}
	}
	var alts []string
	depth, from := 0, open+1
	for i := open + 1; i < end; i++ {
		switch pattern[i] {
		case '{':
			depth++
		case '}':
			depth--
		case ',':
			if depth == 0 {
				alts = append(alts, pattern[from:i])
				from = i + 1
			}
		}
	}
	alts = append(alts, pattern[from:end])
	var out []string
	for _, alt := range alts {
		out = append(out, expandBraces(pattern[:open]+alt+pattern[end+1:])...)
	}
	return out
}

// globMeta reports whether a pattern segment holds a wildcard.
func globMeta(seg string) bool { return strings.ContainsAny(seg, "*?[{") }

// globDir is the folder a pattern's files sit under: its segments up to the
// first one holding a wildcard, without the file name.
func globDir(pattern string) string {
	segs := strings.Split(strings.Trim(pattern, "/"), "/")
	k := len(segs) - 1
	for i, seg := range segs {
		if globMeta(seg) {
			k = i
			break
		}
	}
	return strings.Join(segs[:k], "/")
}

// globSample makes a path a pattern matches: `**` matches no folder, and each
// wildcard stands for one letter.
func globSample(pattern string) (string, bool) {
	if pattern == "" || strings.HasPrefix(pattern, "/") || strings.HasPrefix(pattern, "../") {
		return "", false
	}
	var segs []string
	for _, seg := range strings.Split(pattern, "/") {
		if seg == "**" {
			continue
		}
		var b strings.Builder
		for i := 0; i < len(seg); i++ {
			switch c := seg[i]; c {
			case '*', '?':
				b.WriteByte('x')
			case '[':
				j := strings.IndexByte(seg[i:], ']')
				if j < 0 {
					return "", false
				}
				class := strings.TrimLeft(seg[i+1:i+j], "!^")
				if class == "" {
					return "", false
				}
				b.WriteByte(class[0])
				i += j
			case '\\':
				if i+1 < len(seg) {
					i++
					b.WriteByte(seg[i])
				}
			default:
				b.WriteByte(c)
			}
		}
		segs = append(segs, b.String())
	}
	sample := strings.Join(segs, "/")
	return sample, sample != ""
}

// placePoint is a place's coordinates as the terms store reads them, with the
// terms store governing there.
type placePoint struct {
	place       rulePlace
	profile     string
	coordinates map[string]string
	store       string
}

// placePoints resolves where each place sits, at the instant at.
func placePoints(proj *project.KapiProject, places []rulePlace, at time.Time) []placePoint {
	out := make([]placePoint, 0, len(places))
	for _, p := range places {
		point := project.GovernancePoint{Path: p.Sample, At: at}
		rc, err := proj.ResolveGovernanceFor(point)
		if err != nil || rc == nil {
			continue
		}
		store, err := governedTermsPath(rc)
		if err != nil {
			continue
		}
		out = append(out, placePoint{place: p, profile: rc.Profile, coordinates: pointCoordinates(proj, rc, point), store: store})
	}
	return out
}

// rulesElsewhere lists the rules the store holds at other places of the
// project and not at this point. all is every concept in the store governing
// the point, here the concepts that hold at it, and store names that store
// (empty for the project's own), so a place governed by another store is not
// compared with this one.
func rulesElsewhere(proj *project.KapiProject, places []placePoint, store string, all, here []terms.Concept, locale model.LocaleID, at time.Time) []ContextElsewhere {
	if len(all) == 0 || len(places) == 0 {
		return nil
	}
	held := map[string]bool{}
	mentioned := map[string]bool{}
	for _, c := range here {
		held[c.ID] = true
		for _, t := range c.Terms {
			mentioned[fold(t.Text)] = true
		}
	}
	source := ""
	if proj != nil {
		source = baseLanguage(string(proj.Defaults.SourceLanguage))
	}
	var out []ContextElsewhere
	for _, c := range all {
		if held[c.ID] {
			continue
		}
		var in []string
		for _, p := range places {
			if p.store == store && len(terms.AtPoint([]terms.Concept{c}, p.profile, p.coordinates)) > 0 {
				in = append(in, p.place.Label())
			}
		}
		if len(in) == 0 {
			continue
		}
		slices.Sort(in)
		in = slices.Compact(in)
		byLocale := map[model.LocaleID][]string{}
		var locales []model.LocaleID
		for _, t := range c.Terms {
			switch {
			case !t.Status.Discouraged(), mentioned[fold(t.Text)]:
				continue
			case locale != "" && t.Locale != locale:
				continue
			case locale == "" && source != "" && baseLanguage(string(t.Locale)) != source:
				continue
			case !at.IsZero() && !t.Validity.Matches(graph.ScopeAt(at)):
				continue
			}
			if _, ok := byLocale[t.Locale]; !ok {
				locales = append(locales, t.Locale)
			}
			byLocale[t.Locale] = append(byLocale[t.Locale], t.Text)
		}
		for _, loc := range locales {
			instead := preferredTerm(c, loc)
			if instead == "" || mentioned[fold(instead)] {
				continue
			}
			keep := byLocale[loc]
			slices.Sort(keep)
			out = append(out, ContextElsewhere{Keep: keep, Instead: instead, HeldIn: in, Locale: string(loc)})
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if a, b := fold(out[i].Keep[0]), fold(out[j].Keep[0]); a != b {
			return a < b
		}
		return out[i].Locale < out[j].Locale
	})
	return out
}

// baseLanguage is a locale's language subtag, folded.
func baseLanguage(locale string) string {
	lang, _, _ := strings.Cut(strings.ReplaceAll(locale, "_", "-"), "-")
	return strings.ToLower(lang)
}

// elsewhereLine renders one rule held elsewhere as the line an answer and a
// rules file print: the wording that is correct here, plainly, and where the
// rule that would change it holds.
func elsewhereLine(e ContextElsewhere) string {
	verb, pronoun := "is", "it"
	if len(e.Keep) > 1 {
		verb, pronoun = "are", "them"
	}
	return "- " + quotedAll(e.Keep) + " " + verb + " correct here; do not rename " + pronoun + " to \"" + e.Instead +
		"\". That rename holds only in " + joinAnd(e.HeldIn) + "."
}

// quotedAll renders wordings as `"a"`, `"a" and "b"`, or `"a", "b" and "c"`.
func quotedAll(words []string) string {
	q := make([]string, len(words))
	for i, w := range words {
		q[i] = `"` + w + `"`
	}
	return joinAnd(q)
}

// joinAnd joins items as an English list with "and".
func joinAnd(items []string) string {
	switch len(items) {
	case 0:
		return ""
	case 1:
		return items[0]
	}
	return strings.Join(items[:len(items)-1], ", ") + " and " + items[len(items)-1]
}
