package i18n

import "embed"

// catalogFS carries the compiled MO catalogs for the CLI module's own
// strings (command help, output chrome), embedded into any binary that
// builds on the shared CLI base. Populated by the l10n pipeline:
// `kapi recycle cli/i18n/commands.json -f json -o
// cli/i18n/catalogs/<locale>.mo` (Makefile target l10n-cli). Kept separate
// from core/i18n's catalogs — separate modules, separate embeds. The `all:`
// prefix keeps the directory in-tree while it's empty (the .gitkeep
// placeholder is only visible to embed under that prefix).
//
//go:embed all:catalogs
var catalogFS embed.FS
