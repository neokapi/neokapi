// Package i18n localizes backend-sourced metadata (tool / format / plugin
// DisplayName, Description, parameter titles/descriptions, enum labels,
// group labels) at API boundaries. Catalogs are gettext MO files compiled
// from the l10n pipeline described in docs/ad/*-i18n-for-go-surfaces.md —
// KLF is the exchange format in-pipeline; nothing KLF-shaped reaches the
// runtime binary.
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
