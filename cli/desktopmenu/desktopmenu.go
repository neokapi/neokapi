// Package desktopmenu holds the kapi-desktop native menu labels as localizable
// strings. The Go literals here are the source of truth; the kapi-desktop app
// renders them through a Translator (T) and the i18n generator (cli/i18n/gen)
// exports them under the desktop.menu.* scope into cli/i18n/commands.json, which
// the l10n pipeline extracts and compiles into the embedded MO catalogs.
//
// It lives in the cli module (not apps/kapi-desktop) so the generator — which is
// in the cli module — can enumerate it; kapi-desktop imports it for the labels.
// Standard macOS role items (About, Quit, Services, Hide…) are localized by the
// OS and are not listed here.
package desktopmenu

import (
	"maps"

	"github.com/neokapi/neokapi/core/i18n"
)

// scopePrefix is the msgctxt prefix for every menu label.
const scopePrefix = "desktop.menu."

// sources maps a stable key to its English label. Keys are dot-free; the full
// scope is "desktop.menu.<key>".
var sources = map[string]string{
	"file":                "File",
	"newProject":          "New Project",
	"open":                "Open...",
	"recentProjects":      "Recent Projects",
	"noRecentProjects":    "No Recent Projects",
	"clearRecentProjects": "Clear Recent Projects",
	"save":                "Save",
	"saveAs":              "Save As...",
	"checkForUpdates":     "Check for Updates…",
}

// T returns the localized label for key, falling back to the English source on a
// catalog miss and to the key itself when the key is unknown (a programming
// error surfaced visibly). A nil translator returns the English source.
func T(tr i18n.Translator, key string) string {
	src, ok := sources[key]
	if !ok {
		return key
	}
	if tr == nil {
		return src
	}
	return tr.T(i18n.Scope(scopePrefix+key), src)
}

// Catalog returns a copy of the key→English table. Consumed by the i18n
// generator to emit the desktop.menu.* scopes.
func Catalog() map[string]string {
	return maps.Clone(sources)
}
