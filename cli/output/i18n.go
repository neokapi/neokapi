package output

import (
	"maps"
	"sync/atomic"

	"github.com/neokapi/neokapi/core/i18n"
)

// sources is the table of localizable chrome in this package: list-command
// headings, table headers, and total/status lines. The Go literals here are
// the source of truth; FormatText methods render them through T so a
// Translator built from the embedded cli catalogs (cli/i18n) can localize
// them. The i18n generator (cli/i18n/gen) exports this table into
// cli/i18n/commands.json under the cli.output.* scopes, which is what the
// l10n pipeline extracts and leverages.
//
// Keys are dot-separated paths; the full msgctxt is "cli.output.<key>".
var sources = map[string]string{
	// tools list
	"tools.available":                "Available tools:",
	"tools.none":                     "No tools available.",
	"tools.total":                    "Total: %d tool(s)",
	"tools.category.translation":     "Translation",
	"tools.category.quality":         "Quality",
	"tools.category.analysis":        "Analysis",
	"tools.category.text-processing": "Text Processing",
	"tools.category.other":           "Other",

	// formats list
	"formats.available":         "Available formats:",
	"formats.total":             "Total: %d format(s)",
	"formats.header.format":     "FORMAT",
	"formats.header.name":       "NAME",
	"formats.header.read":       "READ",
	"formats.header.write":      "WRITE",
	"formats.header.source":     "SOURCE",
	"formats.header.extensions": "EXTENSIONS",
	"formats.header.mimeTypes":  "MIME TYPES",

	// flows list
	"flows.available": "Available flows:",
	"flows.none":      "No flows defined.",
	"flows.total":     "Total: %d flow(s)",

	// plugins list
	"plugins.installed":      "Installed plugins:",
	"plugins.none":           "No plugins installed.",
	"plugins.total":          "Total: %d plugin(s)",
	"plugins.header.name":    "NAME",
	"plugins.header.version": "VERSION",
	"plugins.header.type":    "TYPE",
	"plugins.header.status":  "STATUS",
	"plugins.header.formats": "FORMATS",

	// registry list
	"registries.none":            "No registries configured.",
	"registries.total":           "Total: %d registry(ies)",
	"registries.header.name":     "NAME",
	"registries.header.url":      "URL",
	"registries.header.channels": "CHANNELS",
}

// translator holds the active i18n.Translator. Default is a passthrough;
// the CLI App installs the real one in Init via SetTranslator.
var translator atomic.Value

// SetTranslator installs the Translator used to localize the chrome strings
// in this package. Safe for concurrent use; a nil t resets to passthrough.
func SetTranslator(t i18n.Translator) {
	if t == nil {
		t = i18n.NoopTranslator{}
	}
	translator.Store(&t)
}

// T returns the localized string for the given table key, falling back to
// the English source on catalog misses and to the key itself when the key
// is not in the table (a programming error surfaced visibly, not silently).
func T(key string) string {
	src, ok := sources[key]
	if !ok {
		return key
	}
	v, _ := translator.Load().(*i18n.Translator)
	if v == nil {
		return src
	}
	return (*v).T(i18n.Scope("cli.output."+key), src)
}

// Catalog returns a copy of the localizable string table (key → English
// source). Consumed by the cli/i18n generator.
func Catalog() map[string]string {
	return maps.Clone(sources)
}
