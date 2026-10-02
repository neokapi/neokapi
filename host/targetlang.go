package host

import (
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/pflag"

	"github.com/neokapi/neokapi/core/model"
)

const (
	targetLangFlag  = "target-lang"
	targetLangUsage = "target language (e.g. fr, de-DE); one per run"
)

// AddTargetLangFlag registers --target-lang, the one language a translate,
// pseudo-translate, run or exec command writes, bound to the App's TargetLang.
func (a *App) AddTargetLangFlag(f *pflag.FlagSet) {
	f.Var(&targetLangValue{dst: &a.TargetLang}, targetLangFlag, targetLangUsage)
}

// errTargetLangList refuses a list. A run writes one output per input for one
// language, and the language names that output (guide_fr.md), so "fr,de" would
// name a file guide_fr,de.md and translate into neither language.
var errTargetLangList = errors.New("takes one language: run the command once per language, " +
	"or use 'kapi up' to bring a project's files up to date in every language it declares")

// targetLangValue is the --target-lang value: one language tag, checked as it
// is parsed. The value is kept as written, since it names output files.
type targetLangValue struct{ dst *string }

func (v *targetLangValue) String() string {
	if v.dst == nil {
		return ""
	}
	return *v.dst
}

func (v *targetLangValue) Set(s string) error {
	s = strings.TrimSpace(s)
	if strings.ContainsAny(s, ", \t") {
		return errTargetLangList
	}
	if s != "" {
		if _, err := model.CanonicalLocale(s); err != nil {
			return fmt.Errorf("%q is not a language tag (for example fr, de-DE or pt-BR)", s)
		}
	}
	*v.dst = s
	return nil
}

// Type is "string" so the flag reads as one in help and GetString reads it.
func (v *targetLangValue) Type() string { return "string" }
