package i18n

import (
	"github.com/leonelquinteros/gotext"

	corei18n "github.com/neokapi/neokapi/core/i18n"
	loc "github.com/neokapi/neokapi/core/locale"
	"github.com/neokapi/neokapi/core/model"
)

// Catalog returns the embedded CLI MO catalog for the given locale, or nil
// when no catalog has been compiled for it or any of its fallbacks. Lookup
// follows locale.Fallbacks — exact tag first, then the CLDR-minimal form, then
// the bare language — matching core/i18n's embedded catalogs, so a user whose
// config still says "nb-NO" gets the "nb" catalog instead of English.
func Catalog(locale model.LocaleID) *gotext.Mo {
	for _, cand := range loc.Fallbacks(locale) {
		data, err := catalogFS.ReadFile("catalogs/" + string(cand) + ".mo")
		if err != nil {
			continue
		}
		mo := gotext.NewMo()
		mo.Parse(data)
		return mo
	}
	return nil
}

// Resolve builds the Translator for a CLI binary: it runs the framework's
// locale-precedence chain (core/i18n.Resolve) and merges in the CLI
// module's own embedded catalog for the resolved locale. The CLI catalog's
// scopes (cli.commands.*, cli.output.*) are disjoint from the framework's
// (tools.*, formats.*), so merge order is immaterial; misses still fall
// back to the English source.
func Resolve(opts corei18n.ResolveOptions) corei18n.Translator {
	base := corei18n.Resolve(opts)
	mo := Catalog(base.Locale())
	if mo == nil {
		return base
	}
	opts.PluginCatalogs = append([]*gotext.Mo{mo}, opts.PluginCatalogs...)
	return corei18n.Resolve(opts)
}
