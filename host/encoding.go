package host

import "github.com/spf13/pflag"

// The encoding a run reads its inputs in follows the same three-source
// precedence as the source language, for the same reason and by the same
// construction: an explicit --encoding beats the recipe's `defaults.encoding`,
// which beats DefaultEncoding, and the flag therefore registers EMPTY so that
// "unset" is expressible. See the note in host/sourcelang.go for why a literal
// pflag default cannot express this.
//
// The asymmetry this closes: the WRITE side has always been recipe-governed
// (project.ConfigureWriterFor calls SetEncoding with the recipe's value), so a
// project declaring `defaults.encoding: ISO-8859-1` was decoded as UTF-8 and
// re-encoded as Latin-1 by every verb that read the flag's own default.

// DefaultEncoding is the encoding a run reads in when neither the command line
// nor a recipe names one.
const DefaultEncoding = "UTF-8"

// encodingFlag is the flag the resolution is keyed to.
const encodingFlag = "encoding"

// ResolveEncodingName settles an encoding from the three sources in precedence
// order: named (an explicit --encoding) wins, then the recipe's
// `defaults.encoding`, then DefaultEncoding.
func ResolveEncodingName(named, recipe string) string {
	if named != "" {
		return named
	}
	if recipe != "" {
		return recipe
	}
	return DefaultEncoding
}

// AddEncodingFlag registers --encoding on f, bound to the App's encoding.
// shorthand may be empty; usage differs between the verbs that only read a file
// and the ones that write one back.
func (a *App) AddEncodingFlag(f *pflag.FlagSet, shorthand, usage string) {
	f.StringVarP(&a.Encoding, encodingFlag, shorthand, "", usage)
}

// InputEncoding is the encoding this run reads in: what the command line or a
// recipe named, and DefaultEncoding when neither did. Never empty, so it is the
// read every consumer uses; the Encoding field behind it is the raw record of
// what was named.
func (a *App) InputEncoding() string {
	return ResolveEncodingName(a.Encoding, "")
}

// ResolveEncoding adopts recipe as the run's encoding when nothing was named on
// the command line, and returns the encoding the run reads in. Called where a
// project's context is resolved.
func (a *App) ResolveEncoding(recipe string) string {
	a.Encoding = ResolveEncodingName(a.Encoding, recipe)
	return a.Encoding
}
