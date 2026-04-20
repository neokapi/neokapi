package i18n

import "embed"

// builtinFS carries the compiled MO catalogs shipped inside the binary.
// Populated by the l10n pipeline: each `<locale>.mo` under catalogs/ is
// the output of `kapi convert -i core/i18n/klf/<locale>/ --to mo`. The
// `all:` prefix keeps the directory in-tree while it's empty (the
// .gitkeep placeholder is only visible to embed under that prefix).
//
//go:embed all:catalogs
var builtinFS embed.FS
