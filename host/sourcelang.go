package host

import (
	"github.com/neokapi/neokapi/core/locale"
	"github.com/neokapi/neokapi/core/model"
	"github.com/spf13/pflag"
)

// The source language a run works in has exactly three sources, and they rank:
// an explicit --source-lang beats the recipe's `defaults.source_language`, which
// beats DefaultSourceLang. That order is owned here, by one function, so no
// surface can hold a different one.
//
// It is owned here rather than at each call site because a flag default cannot
// express it. pflag writes a flag's default into the bound field at
// REGISTRATION, not at parse, so a literal default on --source-lang would sit in
// the bound field before any command had parsed anything: the field is never
// empty, every fallback that tests it for empty before adopting a recipe is
// unreachable, and a project that declares en-GB converges at "en" — every
// content-memory lookup missing, every unit re-drafted over approved wording
// (#2074, and #2064 as the instance that surfaced it).
//
// So the flag registers EMPTY, and empty is meaningful: it is the field saying
// nobody named a source language. AddSourceLangFlag is the only registration,
// ResolveSourceLang adopts the recipe where a project's context is resolved, and
// SourceLocale supplies the built-in default at the point of use — which is why
// an ad-hoc `kapi translate notes.md --target-lang fr` outside any project, with
// no recipe to adopt, still reads its input as English.

// DefaultSourceLang is the source language a run settles on when neither the
// command line nor a recipe names one.
const DefaultSourceLang = "en"

// sourceLangFlag is the flag the resolution is keyed to.
const sourceLangFlag = "source-lang"

// sourceLangUsage is the one description every command shows for the flag.
const sourceLangUsage = "source language (e.g. en, en-US; defaults to the project's source_language, else " + DefaultSourceLang + ")"

// ResolveSourceLocale settles a source language from the three sources in
// precedence order: named (an explicit --source-lang, or a caller's explicit
// argument) wins, then the recipe's `defaults.source_language`, then
// DefaultSourceLang. Either input is empty when its source is silent — a run
// with no project passes an empty recipe.
//
// The answer is canonical BCP-47 whatever style it was named in, so a
// `--source-lang nb_NO` on the command line and an `nb-NO` in the recipe are
// the same source language to everything downstream. A tag that names no
// language is left as typed: this resolution has no error to return, and the
// command it feeds fails better than a locale helper can.
func ResolveSourceLocale(named string, recipe model.LocaleID) string {
	if named != "" {
		return string(locale.Normalize(model.LocaleID(named)))
	}
	if recipe != "" {
		return string(locale.Normalize(recipe))
	}
	return DefaultSourceLang
}

// AddSourceLangFlag registers --source-lang on f, bound to the App's source
// language. Every command that offers the flag registers it through here, so the
// empty default the resolution depends on has exactly one spelling.
func (a *App) AddSourceLangFlag(f *pflag.FlagSet) {
	f.StringVar(&a.SourceLang, sourceLangFlag, "", sourceLangUsage)
}

// SourceLocale is the source language this run works in: what the command line
// or a recipe named, and DefaultSourceLang when neither did. Never empty, so it
// is the read every consumer of the source language uses. The SourceLang field
// behind it is the raw record of what was named, which is what lets
// ResolveSourceLang tell "named" from "unset".
func (a *App) SourceLocale() string {
	return ResolveSourceLocale(a.SourceLang, "")
}

// ResolveSourceLang adopts recipe as the run's source language when nothing was
// named on the command line, and returns the language the run works in. Called
// where a project's context is resolved, so everything downstream — the
// convergence workers, the plan, the checks, the report — reads one answer.
func (a *App) ResolveSourceLang(recipe model.LocaleID) string {
	a.SourceLang = ResolveSourceLocale(a.SourceLang, recipe)
	return a.SourceLang
}

// scopeSourceLang bounds a resolved source language to one run: the App is
// long-lived in the desktop and the MCP server, where a second project must not
// inherit the first project's recipe. Call as `defer a.scopeSourceLang()()`.
func (a *App) scopeSourceLang() func() {
	named := a.SourceLang
	return func() { a.SourceLang = named }
}
