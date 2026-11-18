package tools_test

import (
	"testing"

	"github.com/neokapi/neokapi/core/model"
	coreprofile "github.com/neokapi/neokapi/core/profile"
	"github.com/neokapi/neokapi/core/tools"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func workspaceRule(target model.LocaleID, replacement string) *tools.TermCheckConfig {
	return &tools.TermCheckConfig{
		TermRules:    []coreprofile.TermRule{{Term: "workspace", Replacement: replacement, ConceptID: "c-ws"}},
		SourceLocale: model.LocaleEnglish,
		TargetLocale: target,
	}
}

func plural(forms map[model.PluralForm]string) model.Run {
	p := model.PluralRun{Pivot: "count", Forms: map[model.PluralForm][]model.Run{}}
	for k, v := range forms {
		p.Forms[k] = []model.Run{model.TextR(v)}
	}
	return model.PluralR(p)
}

// "{count, plural, one {# workspace} other {# workspaces}}"
func enWorkspaces() []model.Run {
	return []model.Run{plural(map[model.PluralForm]string{
		model.PluralOne: "# workspace", model.PluralOther: "# workspaces",
	})}
}

func TestTermCheckRunViolations_EachFormIsChecked(t *testing.T) {
	t.Parallel()
	cfg := workspaceRule(norwegian, "arbeidsområde")
	// The flat text reads only `other`, which renders the term; `one` drops it.
	tgt := []model.Run{plural(map[model.PluralForm]string{
		model.PluralOne: "# rom", model.PluralOther: "# arbeidsområder",
	})}

	flat, _ := tools.TermCheckViolations(cfg, model.RunsText(enWorkspaces()), model.RunsText(tgt))
	assert.Empty(t, flat, "the flat text sees only the other form")

	errs, _ := tools.TermCheckRunViolations(cfg, enWorkspaces(), tgt)
	require.Len(t, errs, 1)
	assert.Contains(t, errs[0], `term "workspace" found in source but required translation "arbeidsområde" missing in target`)
	assert.Contains(t, errs[0], "(in the one form)")
}

func TestTermCheckRunViolations_AFormTheSourceLacksReadsItsOther(t *testing.T) {
	t.Parallel()
	polish := model.LocaleID("pl")
	cfg := workspaceRule(polish, "obszar roboczy")
	tgt := []model.Run{plural(map[model.PluralForm]string{
		model.PluralOne:   "# obszar roboczy",
		model.PluralFew:   "# obszary",
		model.PluralMany:  "# obszarów roboczych",
		model.PluralOther: "# obszaru roboczego",
	})}
	// English has no `few` or `many`: those forms are held to its `other`, and
	// a containment match on the bare rendering reaches none of the inflected
	// forms but `one`. Declared forms are what would satisfy them.
	errs, _ := tools.TermCheckRunViolations(cfg, enWorkspaces(), tgt)
	require.Len(t, errs, 3)
	assert.Contains(t, errs[0], "(in the other form)")
	assert.Contains(t, errs[1], "(in the few form)")
	assert.Contains(t, errs[2], "(in the many form)")
}

func TestTermCheckRunViolations_ATranslationWithoutFormsMeetsEverySourceForm(t *testing.T) {
	t.Parallel()
	cfg := workspaceRule(norwegian, "arbeidsområde")
	src := []model.Run{plural(map[model.PluralForm]string{
		model.PluralOne: "Your workspace", model.PluralOther: "Your projects",
	})}
	errs, _ := tools.TermCheckRunViolations(cfg, src, []model.Run{model.TextR("Prosjektene dine")})
	require.Len(t, errs, 1)
	assert.Contains(t, errs[0], "(in the one form)")
}

func TestTermCheckRunViolations_NoFormsReadsAsTheFlatText(t *testing.T) {
	t.Parallel()
	cfg := serverRule()
	src := []model.Run{model.TextR("Can't reach your server")}
	tgt := []model.Run{model.TextR("Får ikke kontakt med serveren")}
	errs, _ := tools.TermCheckRunViolations(cfg, src, tgt)
	flat, _ := tools.TermCheckViolations(cfg, model.RunsText(src), model.RunsText(tgt))
	assert.Equal(t, flat, errs)
	assert.Len(t, errs, 1)
}

func TestTermCheckOverlays_AnchorsEachForm(t *testing.T) {
	t.Parallel()
	cfg := workspaceRule(norwegian, "arbeidsområde")
	tgt := []model.Run{plural(map[model.PluralForm]string{
		model.PluralOne: "# rom", model.PluralOther: "# arbeidsområder",
	})}
	ovs := tools.TermCheckOverlays(cfg, enWorkspaces(), tgt)
	require.Len(t, ovs, 2)

	src := ovs[0]
	require.Len(t, src.Spans, 2, "the term in each source form")
	forms := map[model.PluralForm]string{}
	for _, s := range src.Spans {
		require.Len(t, s.Range.Path, 2)
		forms[s.Range.Path[1].PluralForm] = s.Range.TextIn(enWorkspaces())
		assert.Equal(t, "false", s.Props["satisfied"])
		assert.Equal(t, "one", s.Props["missing_forms"])
	}
	assert.Equal(t, map[model.PluralForm]string{model.PluralOne: "workspace", model.PluralOther: "workspaces"}, forms)

	require.Len(t, ovs[1].Spans, 1)
	span := ovs[1].Spans[0]
	assert.Equal(t, model.PluralOther, span.Range.Path[1].PluralForm)
	assert.Equal(t, "arbeidsområde", span.Range.TextIn(tgt))
}
