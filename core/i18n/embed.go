package i18n

import "embed"

// builtinFS carries the compiled MO catalogs shipped inside the binary — one
// per locale, compiled by `go run ./core/i18n/gen/catalogs` from the committed
// translated JSON beside them (catalogs/<locale>.json). The MO is build
// output and is not in version control, so the pattern matches only `*.mo`:
// the committed JSON stays out of the binary, and a build that skipped the
// compile step fails here instead of silently shipping English.
//
//go:embed catalogs/*.mo
var builtinFS embed.FS
