package model

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"

	"golang.org/x/text/language"
)

// CanonicalLocale is the one normalization every locale crosses on its way into
// neokapi: it takes a locale written in any accepted style and returns the
// canonical BCP-47 form the rest of the system holds.
//
// It accepts what people and file formats actually write: POSIX separators
// ("nb_NO"), a codeset or modifier suffix ("en_US.UTF-8", "nb@bokmal"), and any
// case ("NB-no"), and answers "nb-NO", "en-US", "nb". Everything downstream of an
// ingress boundary can then compare and key locales as plain strings, which is
// what it already does; the styles exist only at the boundary.
//
// It errors on a locale that is not a locale. Without a gate a typo becomes an
// identity nothing rejects, and a target language nobody can spell is
// indistinguishable from one nobody has translated yet.
//
// A well-formed tag whose subtags CLDR does not know is kept, with those subtags
// intact, because two different things are being asked and only the first is a
// defect:
//
//   - "qps-Ploc" names a real pseudo-locale on a known primary subtag. x/text
//     canonicalizes it to "qps", silently dropping the very subtag that says
//     which pseudo-locale it is, so the tag keeps its own shape instead.
//   - "xx-YY" names no language at all. Its PRIMARY subtag is unknown, so it is
//     rejected; this is the typo case the gate exists for.
//
// It lives beside LocaleID rather than in core/locale so that the content model
// can key a target by it (Variant); core/locale.Canonical is this function.
func CanonicalLocale(s string) (LocaleID, error) {
	cleaned := cleanLocaleInput(s)
	if cleaned == "" {
		return "", fmt.Errorf("invalid locale %q: empty", s)
	}

	tag, err := language.Parse(cleaned)
	if err == nil {
		return LocaleID(tag.String()), nil
	}

	// A subtag CLDR has never heard of is still a subtag. Keep the tag whole
	// unless the unknown part is the language itself.
	if ve, ok := errors.AsType[language.ValueError](err); ok {
		if strings.EqualFold(ve.Subtag(), primarySubtag(cleaned)) {
			return "", fmt.Errorf("invalid locale %q: %w", s, err)
		}
		return LocaleID(canonicalShape(cleaned)), nil
	}
	return "", fmt.Errorf("invalid locale %q: %w", s, err)
}

// NormalizeLocale is the lenient form of CanonicalLocale for a storage or lookup
// boundary: the canonical tag when id is a locale, and id unchanged when it is
// empty or not a locale at all.
//
// A store keyed by a locale wants a miss for a value that is not one, never an
// error, so this is what every store write, every store lookup and every
// Block target accessor applies to the locale it is handed. Two spellings of
// one locale then key the same row and the same variant, whichever one a
// caller wrote.
//
// Results are memoized: the set of locales a process meets is small, and the
// target accessors call this on every block.
func NormalizeLocale(id LocaleID) LocaleID {
	if id == "" {
		return id
	}
	if v, ok := normalizedLocales.Load(id); ok {
		return v.(LocaleID)
	}
	norm, err := CanonicalLocale(string(id))
	if err != nil {
		norm = id
	}
	if normalizedLocaleCount.Load() < normalizedLocaleCap {
		if _, loaded := normalizedLocales.LoadOrStore(id, norm); !loaded {
			normalizedLocaleCount.Add(1)
		}
	}
	return norm
}

// normalizedLocales memoizes NormalizeLocale. The cap keeps a caller that feeds
// arbitrary strings through the boundary (a fuzz test, a hostile file) from
// growing it without bound; past the cap every call computes and nothing is
// remembered, which is correct and merely slower.
var (
	normalizedLocales     sync.Map
	normalizedLocaleCount atomic.Int64
)

const normalizedLocaleCap = 4096

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
