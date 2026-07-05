// Package gen (see gen.go) regenerates the CLI help-string inventory that
// the host module embeds (host/i18n/commands.json). The go:generate
// directive below runs from this directory in the cli module — the
// generator needs the cobra factories, which live here, while the emitted
// inventory ships with the cobra-free host/i18n package.
package gen

//go:generate go run ./cmd ../../host/i18n/commands.json
