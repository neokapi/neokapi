// Package i18n localizes backend-sourced metadata (tool / format / plugin
// DisplayName, Description, parameter titles/descriptions, enum labels,
// group labels) at API boundaries. The runtime format is gettext MO, keyed by
// (msgctxt, msgid) = (scope, English source). Each catalog is compiled at
// build time by `go run ./core/i18n/gen/catalogs` from the committed
// translated JSON under catalogs/, which is what a reviewer reads and what the
// dogfood recipe writes. See
// [AD-016](../../web/docs/contribute/architecture/016-metadata-i18n.md).
//
// The package exposes:
//
//   - Scope / Translator — runtime lookup keyed by a (scope, source) pair
//     mapped onto gettext msgctxt + msgid.
//   - LocalizeComponentSchema / LocalizeCapability — point-of-egress helpers
//     that rewrite English leaves on a schema or capability to the active
//     locale without mutating the registry.
//   - Resolve — picks the active locale from flag / env / config / POSIX
//     fallback chain and assembles a merged Translator spanning the
//     embedded builtin catalog and any plugin-provided catalogs.
//
//go:generate go run ./gen/cmd -out ./builtins
package i18n
