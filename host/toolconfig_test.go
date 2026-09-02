package host

import (
	"context"
	"maps"
	"os"
	"path/filepath"
	"testing"
	"time"

	aitools "github.com/neokapi/neokapi/core/ai/tools"
	corememory "github.com/neokapi/neokapi/core/memory"
	"github.com/neokapi/neokapi/core/model"
	coreprofile "github.com/neokapi/neokapi/core/profile"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/registry"
	libtools "github.com/neokapi/neokapi/core/tools"
	"github.com/neokapi/neokapi/memory"
	"github.com/neokapi/neokapi/terms"
	"github.com/neokapi/neokapi/terms/ktb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The gap this file closes: a surface that runs one tool over one unit used to
// hand-write that tool's config, so a Review retranslation was built with a
// source locale, an instruction and nothing else. No voice profile, no term
// rules, no point, no content memory, while a run of the same tool over the
// same unit got all of them. The two configs are one assembly, and these tests
// hold them against each other.

const unitVoiceYAML = `id: house
name: House Style
tone:
  formality: formal
vocabulary:
  preferred_terms:
    - term: utilize
      replacement: use
`

// unitProject writes a recipe carrying every context a translate step can take:
// a voice profile, a committed terms source, a tool preset holding the DNT list
// and the context window, and a per-locale preset that overrides part of it.
func unitProject(t *testing.T) (a *App, recipe, root, srcFile string) {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	root = dir

	require.NoError(t, os.WriteFile(filepath.Join(root, "voice.yaml"), []byte(unitVoiceYAML), 0o644))

	proj := &project.KapiProject{
		Version: project.CurrentVersion,
		Name:    "UnitConfig",
		Defaults: project.Defaults{
			SourceLanguage:  "en",
			TargetLanguages: []model.LocaleID{"nb", "fr"},
			Voice:           &project.VoiceBinding{ProfileFile: "voice.yaml"},
			TermsSource:     project.RelStatePath(ktb.ConventionalName),
			Tools: map[string]map[string]any{
				"translate": {
					"dnt":           []any{"Bowrain", "kapi"},
					"context":       "neighbours",
					"contextWindow": 3,
					"reuse":         "prior",
				},
			},
			Locales: map[string]project.LocaleDefaults{
				"nb": {Tools: map[string]map[string]any{
					"translate": {"contextWindow": 5},
				}},
			},
		},
		Collections: []project.Collection{
			{Name: "docs", Content: []project.ContentItem{{Path: "docs/**/*.md"}}},
		},
	}
	recipe = filepath.Join(root, project.RecipeFileName)
	require.NoError(t, project.Save(recipe, proj))

	require.NoError(t, os.MkdirAll(filepath.Join(root, "docs"), 0o755))
	srcFile = "docs/guide.md"
	require.NoError(t, os.WriteFile(filepath.Join(root, filepath.FromSlash(srcFile)),
		[]byte("Utilize the content memory.\n"), 0o644))

	writeUnitTerms(t, root)

	a = &App{ToolReg: builtinToolReg()}
	a.SourceLang = "en"
	// A corpus the grant can hand to a tool that accepts one, seeded so the two
	// paths can be asked the same question and compared on the answer.
	backend := memory.NewInMemoryStore()
	require.NoError(t, backend.Add(context.Background(), memory.Entry{
		ID:          "unit:1",
		HintSrcLang: "en",
		Variants: map[model.LocaleID][]model.Run{
			"en": {{Text: &model.TextRun{Text: "Utilize the content memory."}}},
			"nb": {{Text: &model.TextRun{Text: "Bruk innholdsminnet."}}},
		},
	}))
	a.MemoryBackend = backend
	return a, recipe, root, srcFile
}

// writeUnitTerms writes a committed terms bundle with one preferred rendering
// per locale, so the resolved term rules differ between nb and fr.
func writeUnitTerms(t *testing.T, root string) {
	t.Helper()
	stamp := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	concepts := []terms.Concept{{
		ID:     "term:content-memory",
		Source: terms.TermSourceTerminology,
		Terms: []terms.Term{
			{Text: "content memory", Locale: "en", Status: model.TermPreferred},
			{Text: "innholdsminne", Locale: "nb", Status: model.TermPreferred},
			{Text: "mémoire de contenu", Locale: "fr", Status: model.TermPreferred},
		},
		CreatedAt: stamp,
		UpdatedAt: stamp,
	}}
	data, err := ktb.Marshal(ktb.FromConcepts(concepts))
	require.NoError(t, err)
	path := filepath.Join(root, project.RelStatePath(ktb.ConventionalName))
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, data, 0o644))
}

func builtinToolReg() *registry.ToolRegistry {
	reg := registry.NewToolRegistry()
	libtools.RegisterAll(reg)
	aitools.RegisterAll(reg)
	return reg
}

// contextBearing is the set of translate config keys that carry context: the
// fields AITranslateConfig reads governance from. A run and a per-unit surface
// have to agree on every one of them; the defect was that they agreed on none.
var contextBearing = []string{
	"term_rules",
	"profile",
	corememory.ConfigKey,
	"point",
	"reuse",
	"dnt",
	"context",
	"contextWindow",
}

// TestToolConfigForUnit_MatchesTheFlowRunner is the assertion the split exists
// for: the config a Review action builds for a unit and the config a flow step
// building the same tool for the same unit would get are the same config.
func TestToolConfigForUnit_MatchesTheFlowRunner(t *testing.T) {
	a, recipe, _, srcFile := unitProject(t)
	proj, err := project.LoadWithOptions(recipe, project.LoadOptions{SkipRequiresCheck: true})
	require.NoError(t, err)
	ctx := t.Context()

	cases := []struct {
		name string
		unit UnitRef
		base map[string]any
	}{
		{
			name: "the locale with its own preset",
			unit: UnitRef{Path: srcFile, TargetLang: "nb"},
		},
		{
			name: "a locale the recipe presets nothing for",
			unit: UnitRef{Path: srcFile, TargetLang: "fr"},
		},
		{
			name: "named by its collection",
			unit: UnitRef{Collection: "docs", Path: srcFile, TargetLang: "nb"},
		},
		{
			name: "with the review action's own keys on top",
			unit: UnitRef{Path: srcFile, TargetLang: "nb"},
			base: map[string]any{"instruction": "make it informal", "batchSize": 1},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			runCfg, runRelease, rerr := a.stepConfigForUnit(ctx, proj, recipe, "translate", tc.unit, cloneConfig(tc.base))
			require.NoError(t, rerr)
			defer runRelease()

			unitCfg, unitRelease, uerr := a.ToolConfigForUnit(ctx, proj, recipe, "translate", tc.unit, cloneConfig(tc.base))
			require.NoError(t, uerr)
			defer unitRelease()

			for _, key := range contextBearing {
				if key == corememory.ConfigKey {
					assertSameCorpus(t, runCfg, unitCfg, tc.unit.TargetLang)
					continue
				}
				assert.Equal(t, runCfg[key], unitCfg[key],
					"%q differs between a run and the per-unit assembly", key)
			}
			// And the caller's own keys survive the assembly.
			for k, v := range tc.base {
				assert.Equal(t, v, unitCfg[k], "the caller's %q is kept", k)
			}
		})
	}
}

// TestToolConfigForUnit_CarriesTheGovernance pins what the assembly actually
// resolves, so a change that made both paths equally empty could not pass.
func TestToolConfigForUnit_CarriesTheGovernance(t *testing.T) {
	a, recipe, _, srcFile := unitProject(t)
	proj, err := project.LoadWithOptions(recipe, project.LoadOptions{SkipRequiresCheck: true})
	require.NoError(t, err)

	cfg, release, cerr := a.ToolConfigForUnit(t.Context(), proj, recipe, "translate",
		UnitRef{Path: srcFile, TargetLang: "nb"},
		map[string]any{"instruction": "make it informal", "batchSize": 1})
	require.NoError(t, cerr)
	defer release()

	profile, ok := cfg["profile"].(*coreprofile.VoiceProfile)
	require.True(t, ok, "the governing voice profile reaches the tool")
	assert.Equal(t, "House Style", profile.Name)

	rules, ok := cfg["term_rules"].([]coreprofile.TermRule)
	require.True(t, ok, "the governing term rules reach the tool")
	assert.Equal(t, map[string]string{"content memory": "innholdsminne"}, coreprofile.TermRuleMap(rules),
		"the rules are resolved for the unit's own locale")

	assert.Equal(t, []any{"Bowrain", "kapi"}, cfg["dnt"], "the recipe's do-not-translate list")
	assert.Equal(t, "neighbours", cfg["context"])
	assert.Equal(t, 5, cfg["contextWindow"], "the nb preset wins over the project one")
	assert.Equal(t, "prior", cfg["reuse"])
	assert.NotNil(t, cfg[corememory.ConfigKey], "a corpus the tool can read a prior version from")
	assert.Equal(t, "en", cfg[corememory.SourceLocaleKey])

	// The action's own keys are layered on top rather than replaced by the
	// assembly.
	assert.Equal(t, "make it informal", cfg["instruction"])
	assert.Equal(t, 1, cfg["batchSize"])
}

// TestToolConfigForUnit_GovernsTheJudge: the AI pre-review judge scores against
// the voice and vocabulary in force, which it can only do if the review tool is
// handed them the way translate is.
func TestToolConfigForUnit_GovernsTheJudge(t *testing.T) {
	a, recipe, _, srcFile := unitProject(t)
	proj, err := project.LoadWithOptions(recipe, project.LoadOptions{SkipRequiresCheck: true})
	require.NoError(t, err)

	cfg, release, cerr := a.ToolConfigForUnit(t.Context(), proj, recipe, "review",
		UnitRef{Path: srcFile, TargetLang: "nb"}, map[string]any{"sourceLocale": "en"})
	require.NoError(t, cerr)
	defer release()

	profile, ok := cfg["profile"].(*coreprofile.VoiceProfile)
	require.True(t, ok, "the judge is given the voice it is judging against")
	assert.Equal(t, "House Style", profile.Name)

	rules, ok := cfg["term_rules"].([]coreprofile.TermRule)
	require.True(t, ok, "and the vocabulary")
	assert.Equal(t, map[string]string{"content memory": "innholdsminne"}, coreprofile.TermRuleMap(rules))

	assert.NotContains(t, cfg, corememory.ConfigKey,
		"review neither requires nor accepts a corpus, so the grant opens nothing for it")
}

// TestToolConfigForUnit_TermRuleMapIsUnchanged: the context fingerprint the
// staleness gate recomputes folds through profile.TermRuleMap, so what the
// assembly puts under "term_rules" has to project to exactly what a run's does.
func TestToolConfigForUnit_TermRuleMapIsUnchanged(t *testing.T) {
	a, recipe, _, srcFile := unitProject(t)
	proj, err := project.LoadWithOptions(recipe, project.LoadOptions{SkipRequiresCheck: true})
	require.NoError(t, err)
	ctx := t.Context()
	unit := UnitRef{Path: srcFile, TargetLang: "nb"}

	runCfg, runRelease, rerr := a.stepConfigForUnit(ctx, proj, recipe, "translate", unit, nil)
	require.NoError(t, rerr)
	defer runRelease()
	unitCfg, unitRelease, uerr := a.ToolConfigForUnit(ctx, proj, recipe, "translate", unit, nil)
	require.NoError(t, uerr)
	defer unitRelease()

	runRules, _ := runCfg["term_rules"].([]coreprofile.TermRule)
	unitRules, _ := unitCfg["term_rules"].([]coreprofile.TermRule)
	require.NotEmpty(t, runRules)

	runProfile, _ := runCfg["profile"].(*coreprofile.VoiceProfile)
	unitProfile, _ := unitCfg["profile"].(*coreprofile.VoiceProfile)

	_, _, runFP := coreprofile.GovernanceContext(runProfile, runRules)
	_, _, unitFP := coreprofile.GovernanceContext(unitProfile, unitRules)
	assert.Equal(t, runFP, unitFP,
		"a Review proposal stamps the same governing context a run stamps")
}

// TestToolConfigForUnit_NoProjectIsNotAnError: a surface with no recipe in scope
// gets its own keys back rather than a failure.
func TestToolConfigForUnit_NoProjectIsNotAnError(t *testing.T) {
	// The in-repo isolation contract: without this the upward walk finds the
	// dogfood recipe and the assembly binds to it.
	t.Setenv("KAPI_NO_PROJECT", "1")
	a := &App{ToolReg: builtinToolReg()}
	base := map[string]any{"sourceLocale": "en"}

	cfg, release, err := a.ToolConfigForUnit(t.Context(), nil, "", "translate",
		UnitRef{Path: "docs/guide.md", TargetLang: "nb"}, base)
	require.NoError(t, err)
	defer release()

	assert.Equal(t, "en", cfg["sourceLocale"])
	assert.NotContains(t, cfg, "profile")
	assert.NotSame(t, &base, &cfg, "the caller's map is not the one that comes back")
}

// assertSameCorpus compares the granted content memory by what it answers: the
// grant wraps the store in a fresh provider per call, so identity is the wrong
// question and the lookup result is the right one.
func assertSameCorpus(t *testing.T, runCfg, unitCfg map[string]any, targetLang string) {
	t.Helper()
	runCorpus, runOK := runCfg[corememory.ConfigKey].(corememory.Provider)
	unitCorpus, unitOK := unitCfg[corememory.ConfigKey].(corememory.Provider)
	require.Equal(t, runOK, unitOK, "both paths grant a corpus, or neither does")
	if !runOK {
		return
	}
	require.NotNil(t, runCorpus)
	require.NotNil(t, unitCorpus)

	req := corememory.Request{
		Text:   "Utilize the content memory.",
		Source: "en",
		Target: model.LocaleID(targetLang),
	}
	runMatch, runFound := runCorpus.Lookup(t.Context(), req)
	unitMatch, unitFound := unitCorpus.Lookup(t.Context(), req)
	assert.Equal(t, runFound, unitFound, "one corpus, one answer")
	assert.Equal(t, runMatch, unitMatch)
}

func cloneConfig(in map[string]any) map[string]any {
	if in == nil {
		return nil
	}
	out := make(map[string]any, len(in))
	maps.Copy(out, in)
	return out
}
