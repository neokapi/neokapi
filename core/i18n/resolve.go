package i18n

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/leonelquinteros/gotext"

	loc "github.com/neokapi/neokapi/core/locale"
	"github.com/neokapi/neokapi/core/model"
)

// ResolveOptions feeds Resolve's locale-selection chain. Populate from
// App flags / config at CLI startup (or from a request header on the
// server side).
type ResolveOptions struct {
	// Flag is the explicit `--lang` value when set, otherwise "".
	Flag string

	// ConfigLanguage is the `language` key from the user's kapi config.
	ConfigLanguage string

	// PluginCatalogs is the set of MO catalogs contributed by installed
	// plugins for the resolved locale. With the legacy plugin loader
	// gone (#438 phase 9), nothing currently populates this — kept on
	// the type so a future manifest-aware catalog merge can drop into
	// place without a Resolve API change.
	PluginCatalogs []*gotext.Mo
}

// Resolve picks the active locale using the precedence chain defined in
// the i18n AD — flag > KAPI_LANG > config > LC_ALL > LANG > "en" — and
// returns a Translator merging the embedded builtin catalog and any
// plugin catalogs supplied via opts. When the resolved locale has no
// matching MO catalog (because no one has translated the app into it yet),
// the Translator degrades to NoopTranslator and every lookup returns the
// English source.
//
// Catalog lookup follows the locale's fallback chain (locale.Fallbacks), so a
// request for "nb-NO" or LANG=nb_NO.UTF-8 finds the "nb" catalog rather than
// silently degrading to English.
func Resolve(opts ResolveOptions) Translator {
	locale := resolveLocale(opts)
	// English needs no catalog: the message IDs *are* the English source.
	// Compare in minimal form so "en-US" and "en_US.UTF-8" short-circuit too,
	// while a genuinely distinct "en-GB" still gets a catalog lookup.
	if locale.IsEmpty() || loc.Minimal(locale) == "en" {
		return NoopTranslator{}
	}

	var catalogs []*gotext.Mo
	if mo := loadEmbeddedCatalog(locale); mo != nil {
		catalogs = append(catalogs, mo)
	}
	for _, pc := range opts.PluginCatalogs {
		if pc != nil {
			catalogs = append(catalogs, pc)
		}
	}

	return NewTranslator(locale, catalogs...)
}

// resolveLocale walks the precedence chain. First non-empty wins; LC_ALL /
// LANG fallbacks strip the codeset/modifier suffix (e.g. "en_US.UTF-8" →
// "en-US") and normalize POSIX "_" separators to BCP-47 "-".
func resolveLocale(opts ResolveOptions) model.LocaleID {
	if opts.Flag != "" {
		return canonicalOrRaw(opts.Flag)
	}
	if env := os.Getenv("KAPI_LANG"); env != "" {
		return canonicalOrRaw(env)
	}
	if opts.ConfigLanguage != "" {
		return canonicalOrRaw(opts.ConfigLanguage)
	}
	for _, env := range []string{"LC_ALL", "LC_MESSAGES", "LANG"} {
		if v := os.Getenv(env); v != "" {
			return canonicalOrRaw(v)
		}
	}
	return "en"
}

// canonicalOrRaw canonicalizes a requested UI locale, keeping the request as
// written when it names no locale.
//
// Every source is treated the same. `--lang nb_NO`, KAPI_LANG and a config
// `language:` used to skip the POSIX conversion that only the LC_ALL/LANG
// branch performed, so the same locale asked for two ways resolved two ways.
//
// Asking for a catalog is not the place to refuse a bad locale: "C", "POSIX"
// and a typo all mean the same thing here, which is that no catalog matches and
// the caller falls back to the source text. Canonical's error is the gate for a
// recipe, not for a UI preference.
func canonicalOrRaw(v string) model.LocaleID {
	if id, err := loc.Canonical(v); err == nil {
		return id
	}
	return model.LocaleID(normalizePOSIXLocale(v))
}

// normalizePOSIXLocale turns POSIX-style locale IDs ("en_US.UTF-8",
// "fr_CA@euro") into BCP-47 form ("en-US", "fr-CA"). It does NOT alias
// language codes — if LANG is "C" or "POSIX", that's what callers get,
// and Resolve will treat it as an unknown locale and degrade gracefully.
func normalizePOSIXLocale(v string) string {
	// Strip .codeset
	if i := strings.IndexByte(v, '.'); i >= 0 {
		v = v[:i]
	}
	// Strip @modifier
	if i := strings.IndexByte(v, '@'); i >= 0 {
		v = v[:i]
	}
	return strings.ReplaceAll(v, "_", "-")
}

// loadEmbeddedCatalog returns the Mo for the given locale from the embedded
// builtin catalogs, or nil if no catalog exists for the locale or any of its
// fallbacks. The exact tag is tried first, so a future "nb-NO" catalog would
// win over the "nb" one.
func loadEmbeddedCatalog(locale model.LocaleID) *gotext.Mo {
	for _, cand := range loc.Fallbacks(locale) {
		data, err := builtinFS.ReadFile("catalogs/" + string(cand) + ".mo")
		if err != nil {
			continue
		}
		mo := gotext.NewMo()
		mo.Parse(data)
		return mo
	}
	return nil
}

// LoadPluginCatalog loads a plugin-provided MO catalog from disk. Returns
// nil (no error) if the file doesn't exist — absence of a translation is
// normal. Returns a non-nil error only on a real filesystem or parse
// failure. Callers aggregate successful returns into ResolveOptions.
func LoadPluginCatalog(path string) (*gotext.Mo, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	mo := gotext.NewMo()
	mo.Parse(data)
	return mo, nil
}

// PluginCatalogPath returns the conventional path for a plugin's MO catalog
// given the plugin's version directory and the target locale. It returns the
// first path in the locale's fallback chain that exists on disk, falling back
// to the exact-tag path when none does (so the caller's "not found" handling
// reports the tag the user asked for).
func PluginCatalogPath(pluginVersionDir string, locale model.LocaleID, i18nDir string) string {
	if i18nDir == "" {
		i18nDir = "i18n"
	}
	exact := filepath.Join(pluginVersionDir, i18nDir, string(locale)+".mo")
	for _, cand := range loc.Fallbacks(locale) {
		p := filepath.Join(pluginVersionDir, i18nDir, string(cand)+".mo")
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return exact
}

// Ensure fs.FS interface compatibility — keeps the go:embed declaration honest.
var _ fs.FS = builtinFS
