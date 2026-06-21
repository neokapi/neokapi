// Package i18n carries the CLI module's own localization surface: the
// generated inventory of cobra command help strings (commands.json), the
// compiled MO catalogs embedded into binaries built on the shared CLI base,
// and the resolver that merges those catalogs with the framework's builtin
// catalogs (core/i18n).
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
// derives from the document's key path, so `kapi recycle commands.json
// -f json -o catalogs/<lang>.mo` produces MO entries whose msgctxt matches
// the runtime lookups in cli.LocalizeCommandHelp and cli/output.T. See the
// l10n-cli Makefile target and AD-016.
package i18n

//go:generate go run ./gen/cmd
