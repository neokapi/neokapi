// Package locale provides BCP-47 locale validation, normalization, and display
// name resolution for use throughout neokapi.
package locale

import (
	"cmp"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/neokapi/neokapi/core/model"
	"golang.org/x/text/language"
	"golang.org/x/text/language/display"
)

// Supported locale format identifiers.
const (
	FormatBCP47 = "bcp-47"
	FormatPOSIX = "posix"
)

// LocaleInfo holds a locale code and its English display name.
type LocaleInfo struct {
	Code        string `json:"code"`
	DisplayName string `json:"display_name"`
}

// Parse validates and normalizes a well-formed BCP-47 locale string, returning
// its canonical form ("pt-br" → "pt-BR").
//
// Parse is strict about CLDR membership: a well-formed tag naming a subtag CLDR
// does not know ("qps-Ploc", the Microsoft pseudo-locale) is an error here. It
// also takes the tag as written, so it rejects the styles an operating system
// or a person writes ("en_US.UTF-8", "nb@bokmal").
//
// It is therefore not an ingress boundary. Canonical is the one normalization a
// locale entering neokapi crosses; it accepts those styles, and reaches this
// function for the tags they clean up to.
func Parse(s string) (model.LocaleID, error) {
	tag, err := language.Parse(s)
	if err != nil {
		return "", fmt.Errorf("invalid locale %q: %w", s, err)
	}
	return model.LocaleID(tag.String()), nil
}

// Canonical is the one normalization every locale crosses on its way into
// neokapi: it takes a locale written in any accepted style and returns the
// canonical BCP-47 form the rest of the system holds.
//
// It accepts what people and file formats actually write — POSIX separators
// ("nb_NO"), a codeset or modifier suffix ("en_US.UTF-8", "nb@bokmal"), and any
// case ("NB-no") — and answers "nb-NO", "en-US", "nb". Everything downstream of
// an ingress boundary can then compare and key locales as plain strings, which
// is what it already does; the styles exist only at the boundary.
//
// It errors on a locale that is not a locale. That is the point: without a gate,
// a typo becomes an identity nothing rejects, and a target language nobody can
// spell is indistinguishable from one nobody has translated yet.
//
// A well-formed tag whose subtags CLDR does not know is NOT an error, and is
// returned with those subtags intact. Two different things are being asked, and
// only the first is a defect:
//
//   - "qps-Ploc" names a real pseudo-locale on a known primary subtag. x/text
//     canonicalizes it to "qps", silently dropping the very subtag that says
//     which pseudo-locale it is, so Canonical keeps the tag's own shape instead.
//   - "xx-YY" names no language at all. Its PRIMARY subtag is unknown, so it is
//     rejected — this is the typo case the gate exists for.
func Canonical(s string) (model.LocaleID, error) {
	cleaned := cleanLocaleInput(s)
	if cleaned == "" {
		return "", fmt.Errorf("invalid locale %q: empty", s)
	}

	// The strict attempt is Parse itself, so the form the two return for a tag
	// they both accept cannot drift apart.
	id, err := Parse(cleaned)
	if err == nil {
		return id, nil
	}

	// A subtag CLDR has never heard of is still a subtag. Keep the tag whole
	// unless the unknown part is the language itself.
	var ve language.ValueError
	if errors.As(err, &ve) {
		if strings.EqualFold(ve.Subtag(), primarySubtag(cleaned)) {
			return "", fmt.Errorf("invalid locale %q: %w", s, err)
		}
		return model.LocaleID(canonicalShape(cleaned)), nil
	}
	return "", fmt.Errorf("invalid locale %q: %w", s, err)
}

// CanonicalAll canonicalizes a list, reporting the first locale that is not one.
func CanonicalAll(in []model.LocaleID) ([]model.LocaleID, error) {
	if len(in) == 0 {
		return in, nil
	}
	out := make([]model.LocaleID, len(in))
	for i, id := range in {
		c, err := Canonical(string(id))
		if err != nil {
			return nil, err
		}
		out[i] = c
	}
	return out, nil
}

// cleanLocaleInput strips the parts of a POSIX locale that name an encoding or
// a variant selection rather than a language, and squares the separator.
//
// "en_US.UTF-8" and "nb@bokmal" are locales as an operating system writes them;
// neither is well-formed BCP-47, and x/text rejects both outright rather than
// reporting which part it disliked.
func cleanLocaleInput(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexAny(s, ".@"); i >= 0 {
		s = s[:i]
	}
	return strings.ReplaceAll(s, "_", "-")
}

// primarySubtag returns the language subtag of a cleaned tag.
func primarySubtag(cleaned string) string {
	base, _, _ := strings.Cut(cleaned, "-")
	return base
}

// canonicalShape applies BCP-47's own casing convention to a tag x/text declined
// to canonicalize: lowercase language, Titlecase script, uppercase region, and
// everything after left as written.
func canonicalShape(cleaned string) string {
	parts := strings.Split(cleaned, "-")
	for i, p := range parts {
		switch {
		case i == 0:
			parts[i] = strings.ToLower(p)
		case len(p) == 4:
			parts[i] = strings.ToUpper(p[:1]) + strings.ToLower(p[1:])
		case len(p) == 2:
			parts[i] = strings.ToUpper(p)
		}
	}
	return strings.Join(parts, "-")
}

// MustParse is like Parse but panics on invalid input.
func MustParse(s string) model.LocaleID {
	id, err := Parse(s)
	if err != nil {
		panic(err)
	}
	return id
}

// Normalize returns the canonical BCP-47 form of id (e.g. "pt-br" → "pt-BR",
// "EN" → "en"), falling back to id unchanged when it is empty or not a
// parseable tag. Use it at storage and lookup boundaries so locales that differ
// only in formatting compare and key equal.
func Normalize(id model.LocaleID) model.LocaleID {
	if id == "" {
		return id
	}
	if norm, err := Parse(string(id)); err == nil {
		return norm
	}
	return id
}

// hanRegionScript maps the two Chinese region subtags whose written variant
// CLDR expresses as a script to that script. Minimal rewrites "zh-CN" to
// "zh-Hans" and "zh-TW" to "zh-Hant" so the Simplified/Traditional
// distinction — the thing that actually differs — is carried by the script
// subtag rather than by a region that only implies it. Other Chinese regions
// (HK, MO, SG) are left as written: they are requested as distinct locales in
// their own right, and folding them into a script tag would lose that.
var hanRegionScript = map[string]string{"CN": "Hans", "TW": "Hant"}

// regionAlwaysWritten lists base languages whose region subtag is kept even
// when CLDR would call it redundant. Portuguese is the case that matters:
// CLDR's likely region for "pt" is BR, so a mechanical minimization would
// rewrite "pt-BR" to "pt" — but pt-BR and pt-PT are both in active use as
// separate written variants and a bare "pt" is ambiguous in practice.
var regionAlwaysWritten = map[string]bool{"pt": true}

// Minimal returns the CLDR-minimal form of id: the shortest tag that still
// names the same written variant. A region subtag is dropped when it is the
// CLDR likely region for the language ("nb-NO" → "nb", "fr-FR" → "fr",
// "en-US" → "en"); a region that carries a real distinction is kept
// ("en-GB", "fr-CA", "es-419", "pt-PT"). Unparseable or empty input is
// returned unchanged.
//
// Three deliberate departures from a mechanical CLDR minimization:
//
//   - Script subtags are never dropped. "zh-Hans" stays "zh-Hans" even though
//     Hans is the likely script for "zh", because the script is exactly what
//     distinguishes it from "zh-Hant".
//   - "zh-CN" → "zh-Hans" and "zh-TW" → "zh-Hant" (see hanRegionScript).
//   - Portuguese keeps its region (see regionAlwaysWritten).
//
// Tags carrying variants, extensions or private use ("ca-ES-valencia",
// "en-US-u-ca-gregory") are canonicalized but otherwise returned untouched —
// minimizing them would risk dropping the very subtag the caller cares about.
//
// Minimal changes granularity and is therefore a *display* and *default*
// helper: use it when choosing the tag to write into new configuration or to
// show a user. Do not use it at a storage or lookup boundary where an
// existing tag is the key — Normalize is the tool for that, and Fallbacks is
// the tool for accepting both granularities on lookup.
func Minimal(id model.LocaleID) model.LocaleID {
	if id == "" {
		return id
	}
	tag, err := language.Parse(string(id))
	if err != nil {
		return id
	}
	canon := model.LocaleID(tag.String())

	base, rawScript, rawRegion := tag.Raw()
	b := base.String()
	script := scriptOrEmpty(rawScript)
	region := regionOrEmpty(rawRegion)

	// Only plain base[-script][-region] tags are minimized.
	if compose(b, script, region) != string(canon) {
		return canon
	}
	if region == "" || regionAlwaysWritten[b] {
		return canon
	}

	// A region-only Chinese tag states its written variant as a script.
	if script == "" {
		if s, ok := hanRegionScript[region]; ok && b == "zh" {
			return model.LocaleID(compose(b, s, ""))
		}
	}

	// Drop the region when it is the likely region for the language (plus
	// script, when one is written).
	if likelyRegion(compose(b, script, "")) == region {
		return model.LocaleID(compose(b, script, ""))
	}
	return canon
}

// Fallbacks returns id's lookup chain, most specific first: the canonical
// tag, then its minimal form, then the language+script stem, then the bare
// language. Duplicates are collapsed, so "nb" yields just ["nb"] and "nb-NO"
// yields ["nb-NO", "nb"].
//
// Use it wherever a locale keys an optional resource — a message catalog, a
// per-locale asset — so a project that writes "nb-NO" finds the "nb" resource
// instead of silently getting nothing. Lookups stay exact-first, so a tag
// written verbatim always wins over its fallbacks.
func Fallbacks(id model.LocaleID) []model.LocaleID {
	if id == "" {
		return nil
	}
	tag, err := language.Parse(string(id))
	if err != nil {
		return []model.LocaleID{id}
	}

	candidates := []model.LocaleID{model.LocaleID(tag.String()), Minimal(id)}

	base, rawScript, _ := tag.Raw()
	b := base.String()
	if script := scriptOrEmpty(rawScript); script != "" {
		candidates = append(candidates, model.LocaleID(compose(b, script, "")))
	}
	// A minimal form may have introduced a script ("zh-CN" → "zh-Hans").
	if mBase, mScript, _ := language.Make(string(Minimal(id))).Raw(); scriptOrEmpty(mScript) != "" {
		candidates = append(candidates, model.LocaleID(compose(mBase.String(), scriptOrEmpty(mScript), "")))
	}
	candidates = append(candidates, model.LocaleID(b))

	out := make([]model.LocaleID, 0, len(candidates))
	seen := make(map[model.LocaleID]bool, len(candidates))
	for _, c := range candidates {
		if c == "" || seen[c] {
			continue
		}
		seen[c] = true
		out = append(out, c)
	}
	return out
}

// scriptOrEmpty returns the script subtag, or "" when it is the "unknown"
// placeholder x/text reports for an absent script.
func scriptOrEmpty(s language.Script) string {
	if str := s.String(); str != "Zzzz" {
		return str
	}
	return ""
}

// regionOrEmpty returns the region subtag, or "" when it is the "unknown"
// placeholder x/text reports for an absent region.
func regionOrEmpty(r language.Region) string {
	if str := r.String(); str != "ZZ" {
		return str
	}
	return ""
}

// compose joins the base, script and region subtags present into a BCP-47
// tag. Empty components are skipped.
func compose(base, script, region string) string {
	parts := make([]string, 0, 3)
	for _, p := range []string{base, script, region} {
		if p != "" {
			parts = append(parts, p)
		}
	}
	return strings.Join(parts, "-")
}

// likelyRegion returns the CLDR likely region for a base[-script] stem, or ""
// when the stem does not parse.
func likelyRegion(stem string) string {
	tag, err := language.Parse(stem)
	if err != nil {
		return ""
	}
	r, _ := tag.Region()
	return regionOrEmpty(r)
}

// DisplayName returns the English display name for a locale ID.
// For example, "fr" returns "French", "de" returns "German".
// Falls back to the raw code for unrecognized tags.
func DisplayName(id model.LocaleID) string {
	tag, err := language.Parse(string(id))
	if err != nil {
		return string(id)
	}
	name := display.English.Tags().Name(tag)
	if name == "" {
		return string(id)
	}
	return name
}

// wellKnown is the curated list of common locales for UI dropdowns. It targets
// broad global coverage for a worldwide consumer + developer audience, not a
// niche or heritage set: the major world languages, the commercial regional
// variants localization teams actually request (Latin American Spanish,
// Canadian French, British English, Brazilian/European Portuguese, Simplified/
// Traditional Chinese), and the high-population languages that are commonly
// under-served (Filipino, Punjabi, Amharic, Burmese, Khmer, Sinhala, …).
//
// This is a convenience list only — Parse accepts any valid BCP-47 tag, so the
// long tail is always reachable by typing a code directly.
var wellKnown = []struct {
	code string
}{
	// Western European
	{"ca"}, {"cy"}, {"da"}, {"de"}, {"en"}, {"en-GB"}, {"es"}, {"es-419"},
	{"eu"}, {"fi"}, {"fr"}, {"fr-CA"}, {"ga"}, {"gl"}, {"is"}, {"it"},
	{"nb"}, {"nl"}, {"nn"}, {"pt"}, {"pt-BR"}, {"sv"},
	// Central / Eastern European
	{"bg"}, {"bs"}, {"cs"}, {"el"}, {"et"}, {"hr"}, {"hu"}, {"lt"}, {"lv"},
	{"mk"}, {"pl"}, {"ro"}, {"ru"}, {"sk"}, {"sl"}, {"sq"}, {"sr"}, {"uk"},
	// Middle East / Central Asia / Caucasus
	{"ar"}, {"az"}, {"fa"}, {"he"}, {"hy"}, {"ka"}, {"kk"}, {"tr"}, {"uz"},
	// South Asia
	{"bn"}, {"gu"}, {"hi"}, {"kn"}, {"ml"}, {"mr"}, {"ne"}, {"pa"}, {"si"},
	{"ta"}, {"te"}, {"ur"},
	// Southeast Asia
	{"fil"}, {"id"}, {"km"}, {"lo"}, {"ms"}, {"my"}, {"th"}, {"vi"},
	// East Asia
	{"ja"}, {"ko"}, {"mn"}, {"zh"}, {"zh-Hans"}, {"zh-Hant"},
	// Africa
	{"af"}, {"am"}, {"sw"},
}

// WellKnownLocales returns a curated list of common BCP-47 locales with
// English display names, sorted alphabetically by display name.
func WellKnownLocales() []LocaleInfo {
	result := make([]LocaleInfo, 0, len(wellKnown))
	for _, w := range wellKnown {
		tag := language.MustParse(w.code)
		result = append(result, LocaleInfo{
			Code:        tag.String(),
			DisplayName: display.English.Tags().Name(tag),
		})
	}
	slices.SortFunc(result, func(a, b LocaleInfo) int {
		return cmp.Compare(a.DisplayName, b.DisplayName)
	})
	return result
}

// ToPosix converts a BCP-47 locale code to POSIX format.
// "pt-BR" → "pt_BR", "en" → "en", "zh-Hans" → "zh_Hans".
func ToPosix(code string) string {
	return strings.ReplaceAll(code, "-", "_")
}

// FromPosix converts a POSIX locale code to BCP-47 format.
// "pt_BR" → "pt-BR", "en" → "en", "zh_Hans" → "zh-Hans".
func FromPosix(code string) string {
	return strings.ReplaceAll(code, "_", "-")
}

// FormatCode converts a BCP-47 locale code to the specified format.
// Supported formats: "bcp-47" (default, no-op) and "posix".
func FormatCode(code, format string) string {
	if format == FormatPOSIX {
		return ToPosix(code)
	}
	return code
}

// WellKnownLocalesFormatted returns the curated locale list with codes
// formatted according to the given format string.
func WellKnownLocalesFormatted(format string) []LocaleInfo {
	locales := WellKnownLocales()
	if format == FormatPOSIX {
		for i := range locales {
			locales[i].Code = ToPosix(locales[i].Code)
		}
	}
	return locales
}
