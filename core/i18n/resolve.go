package i18n

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/leonelquinteros/gotext"

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
func Resolve(opts ResolveOptions) Translator {
	locale := resolveLocale(opts)
	if locale.IsEmpty() || locale == "en" {
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
		return model.LocaleID(opts.Flag)
	}
	if env := os.Getenv("KAPI_LANG"); env != "" {
		return model.LocaleID(env)
	}
	if opts.ConfigLanguage != "" {
		return model.LocaleID(opts.ConfigLanguage)
	}
	for _, env := range []string{"LC_ALL", "LC_MESSAGES", "LANG"} {
		if v := os.Getenv(env); v != "" {
			return model.LocaleID(normalizePOSIXLocale(v))
		}
	}
	return "en"
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

// loadEmbeddedCatalog returns the Mo for the given locale from the
// embedded builtin catalogs, or nil if no catalog exists.
func loadEmbeddedCatalog(locale model.LocaleID) *gotext.Mo {
	name := string(locale) + ".mo"
	data, err := builtinFS.ReadFile("catalogs/" + name)
	if err != nil {
		return nil
	}
	mo := gotext.NewMo()
	mo.Parse(data)
	return mo
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

// PluginCatalogPath returns the conventional path for a plugin's MO
// catalog given the plugin's version directory and the target locale.
func PluginCatalogPath(pluginVersionDir string, locale model.LocaleID, i18nDir string) string {
	if i18nDir == "" {
		i18nDir = "i18n"
	}
	return filepath.Join(pluginVersionDir, i18nDir, string(locale)+".mo")
}

// Ensure fs.FS interface compatibility — keeps the go:embed declaration
// honest even when the directory is empty (no .mo files committed yet).
var _ fs.FS = builtinFS
