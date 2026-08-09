// Package i18n carries the CLI module's own multilingual surface: the
// generated inventory of cobra command help strings (commands.json), the
// per-locale translated catalogs under catalogs/, the compiled MO embedded
// into binaries built on the shared CLI base, and the resolver that merges
// those catalogs with the framework's builtin catalogs (core/i18n).
//
// Source of truth stays the Go literals — Short/Long/Example on the cobra
// commands and the chrome table in cli/output. The //go:generate step below
// reconstructs the full kapi command set from the exported cli factories
// (cli.KapiCommandSet) and emits commands.json with one entry per
// translatable string, scope-keyed as:
//
//	cli.commands.<full.command.path>.{short,long,example}
//	cli.output.<key>
//
// The dot-separated scope is exactly the block name the JSON format reader
// derives from the document's key path. The dogfood recipe's neokapi-cli
// collection writes catalogs/<lang>.json in that same shape, and
// `go run ./core/i18n/gen/catalogs` compiles it into catalogs/<lang>.mo with
// one entry per string, msgctxt = the key path — so the compiled catalog is
// keyed exactly as cli.LocalizeCommandHelp and cli/output.T look it up. See
// AD-016.
package i18n
