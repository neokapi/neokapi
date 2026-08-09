package i18n

import "embed"

// catalogFS carries the compiled MO catalogs for the CLI module's own strings
// (command help, output chrome), embedded into any binary that builds on the
// shared CLI base. Each `<locale>.mo` is compiled by
// `go run ./core/i18n/gen/catalogs` from the committed translated JSON beside
// it (catalogs/<locale>.json); the MO is build output and is not in version
// control. Kept separate from core/i18n's catalogs — separate modules,
// separate embeds. The pattern matches only `*.mo` so the committed JSON stays
// out of the binary and a missing compile step fails the build.
//
//go:embed catalogs/*.mo
var catalogFS embed.FS
