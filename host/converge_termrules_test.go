package host

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/neokapi/neokapi/core/flow"
	"github.com/neokapi/neokapi/core/model"
	coreprofile "github.com/neokapi/neokapi/core/profile"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/neokapi/neokapi/core/schema"
	"github.com/neokapi/neokapi/core/tool"
	"github.com/neokapi/neokapi/terms/ktb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A convergence fans out over locales, and a term rule is per locale: it pairs a
// source term with the wording approved for one target. Resolving the run's
// bindings before the fan-out left the producers with the do-not-translate
// concepts and none of the project's preferred renderings, while the staleness
// gate resolved per locale and so recomputed a fingerprint over rules no
// producer had. These tests hold the two ends together.

// termRulesRecorder is a flow step that records the terminology its config
// carried, keyed by the locale the tool was built for. It declares
// RequiresTerms, which is what makes a run hand it the term rules
// (applyBindings), and passes every part through, so the flow around it runs as
// it would without it.
type termRulesRecorder struct {
	mu    sync.Mutex
	rules map[string][]coreprofile.TermRule
}

func newTermRulesRecorder() *termRulesRecorder {
	return &termRulesRecorder{rules: map[string][]coreprofile.TermRule{}}
}

func (r *termRulesRecorder) record(config map[string]any, targetLang string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	rules, _ := config["term_rules"].([]coreprofile.TermRule)
	r.rules[targetLang] = rules
}

func (r *termRulesRecorder) rulesFor(locale string) []coreprofile.TermRule {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.rules[locale]
}

func (r *termRulesRecorder) Name() string                      { return "term-recorder" }
func (r *termRulesRecorder) Description() string               { return "records the terminology a run carries" }
func (r *termRulesRecorder) Config() tool.ToolConfig           { return nil }
func (r *termRulesRecorder) SetConfig(_ tool.ToolConfig) error { return nil }

func (r *termRulesRecorder) Process(_ context.Context, in <-chan *model.Part, out chan<- *model.Part) error {
	for p := range in {
		out <- p
	}
	return nil
}

// registerTermRecorder puts the recorder in the registry as a step a project
// flow can name.
func registerTermRecorder(reg *registry.ToolRegistry, rec *termRulesRecorder) {
	reg.RegisterWithSchema("term-recorder", func() tool.Tool { return rec }, &schema.ComponentSchema{
		ID:    "term-recorder",
		Title: "Term Recorder",
		ToolMeta: &schema.ToolMeta{
			ID:          "term-recorder",
			DisplayName: "Term Recorder",
			Requires:    []string{schema.RequiresTerms},
		},
	})
	reg.SetConfigFactory("term-recorder", func(config map[string]any, targetLang string) (tool.Tool, error) {
		rec.record(config, targetLang)
		return rec, nil
	})
}

// newTermRulesConvergeProject writes a two-locale project bound to a voice
// profile and a committed terms source whose one concept is approved
// differently in each locale, over a converge flow that translates with the
// deterministic demo provider and then records what its steps were given.
func newTermRulesConvergeProject(t *testing.T) (*App, *EnvCommand, string, string, *termRulesRecorder) {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)

	require.NoError(t, os.WriteFile(filepath.Join(root, "voice.yaml"), []byte(unitVoiceYAML), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "src"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "src", "en.json"),
		[]byte(`{"greeting":"Utilize the content memory","farewell":"Goodbye now"}`), 0o644))
	writeUnitTerms(t, root)

	proj := &project.KapiProject{
		Version: project.CurrentVersion,
		Name:    "TermRulesConverge",
		Defaults: project.Defaults{
			SourceLanguage:  "en",
			TargetLanguages: []model.LocaleID{"nb", "fr"},
			Flow:            "converge",
			Voice:           &project.VoiceBinding{ProfileFile: "voice.yaml"},
			TermsSource:     project.RelStatePath(ktb.ConventionalName),
		},
		Collections: []project.Collection{
			{Name: "docs", Path: "src/en.json", Target: "src/{lang}.json"},
		},
		Flows: map[string]*flow.StepsSpec{
			"converge": {Steps: []flow.FlowStep{
				{Tool: "translate", Config: map[string]any{"provider": "demo"}},
				{Tool: "term-recorder"},
			}},
		},
	}
	recipe := filepath.Join(root, project.RecipeFileName)
	require.NoError(t, project.Save(recipe, proj))
	require.NoError(t, os.MkdirAll(filepath.Join(root, project.StateDirName), 0o755))
	t.Chdir(root)

	a := &App{}
	a.InitRegistries()
	a.SourceLang = "en"
	rec := newTermRulesRecorder()
	registerTermRecorder(a.ToolReg, rec)

	cmd := NewEnvCommand(context.Background(), "up")
	a.AddFlowRunFlags(cmd)
	return a, cmd, recipe, root, rec
}

// TestConverge_ResolvesTermRulesPerLocale is the defect the fix closes: a
// convergence hands each locale's producers the wording approved for that
// locale. Resolved once for the run, before any locale was known, the rules
// were empty for every locale.
func TestConverge_ResolvesTermRulesPerLocale(t *testing.T) {
	a, cmd, recipe, _, rec := newTermRulesConvergeProject(t)
	runConverge(t, a, cmd, recipe)

	assert.Equal(t, map[string]string{"content memory": "innholdsminne"},
		coreprofile.TermRuleMap(rec.rulesFor("nb")),
		"nb is given the rendering approved for nb")
	assert.Equal(t, map[string]string{"content memory": "mémoire de contenu"},
		coreprofile.TermRuleMap(rec.rulesFor("fr")),
		"fr is given its own")
}

// TestConverge_ProducerAndStalenessGateResolveOneContext closes the loop the
// gate depends on: the context a producer is given for a locale has to be the
// one the gate recomputes for it. Resolved before the fan-out, the producer's
// half held no preferred term for any locale while the gate's held each
// locale's own, so every produced target read as stale against a context nobody
// had touched.
func TestConverge_ProducerAndStalenessGateResolveOneContext(t *testing.T) {
	a, cmd, recipe, root, rec := newTermRulesConvergeProject(t)
	runConverge(t, a, cmd, recipe)

	proj, err := project.Load(recipe)
	require.NoError(t, err)

	units, err := a.UnitsFromProject(proj, root, "")
	require.NoError(t, err)
	require.NotEmpty(t, units)

	// The producer's half: the groups a convergence partitions its sources into,
	// with each group's bindings resolved for the locale of the pass.
	groups, err := a.groupInputsByBinding(cmd, proj, root, []string{filepath.Join(root, "src", "en.json")})
	require.NoError(t, err)
	require.Len(t, groups, 1)
	bindings := a.newLocaleBindings(cmd, proj, recipe)

	// The gate's half, through its own resolver.
	fps, err := newContextFingerprints(a, cmd, proj, root)
	require.NoError(t, err)
	defer fps.close()

	byLocale := map[string]string{}
	for _, u := range units {
		b, berr := bindings.at(groups[0].Point, u.Locale)
		require.NoError(t, berr)
		require.NotNil(t, b)
		require.Equal(t, coreprofile.TermRuleMap(b.termRules), coreprofile.TermRuleMap(rec.rulesFor(u.Locale)),
			"the run handed its %s tools the rules this set carries", u.Locale)
		_, _, produced := coreprofile.GovernanceContext(b.profile, b.termRules)

		want, ferr := fps.at(a.unitGovernancePoint(root, u), u.Locale)
		require.NoError(t, ferr)
		require.NotEmpty(t, want.fingerprint)
		assert.Equal(t, want.fingerprint, produced,
			"the gate and the producer fold one context for %s", u.Locale)
		byLocale[u.Locale] = produced
	}

	require.Len(t, byLocale, 2, "the fixture has two locales to tell apart")
	assert.NotEqual(t, byLocale["nb"], byLocale["fr"],
		"each locale's own vocabulary is folded into its fingerprint")
}

// TestConverge_SingleLocaleRunIsUnchanged: a run pinned to one target locale
// resolves the same rules through the same path, so the fix costs the
// single-locale verbs (`kapi translate`, `kapi run --target-lang`) nothing.
func TestConverge_SingleLocaleRunIsUnchanged(t *testing.T) {
	a, cmd, recipe, _, _ := newTermRulesConvergeProject(t)
	proj, err := project.Load(recipe)
	require.NoError(t, err)

	a.TargetLang = "nb"
	direct, err := a.resolveProjectBindings(cmd, proj, recipe, a.GovernancePointFor("", ""))
	require.NoError(t, err)
	require.NotNil(t, direct)

	viaResolver, err := a.newLocaleBindings(cmd, proj, recipe).at(a.GovernancePointFor("", ""), "nb")
	require.NoError(t, err)
	require.NotNil(t, viaResolver)

	assert.Equal(t, coreprofile.TermRuleMap(direct.termRules), coreprofile.TermRuleMap(viaResolver.termRules))
	assert.Equal(t, map[string]string{"content memory": "innholdsminne"},
		coreprofile.TermRuleMap(viaResolver.termRules))
	assert.Equal(t, direct.profile, viaResolver.profile)
	assert.Equal(t, direct.point, viaResolver.point)
}

// TestLocaleBindings_ResolvesOncePerPointAndLocale: the cache answers a repeat
// ask with the same set, so a run with many groups over many locales resolves
// one binding set per pair rather than one per group per pass.
func TestLocaleBindings_ResolvesOncePerPointAndLocale(t *testing.T) {
	a, cmd, recipe, _, _ := newTermRulesConvergeProject(t)
	proj, err := project.Load(recipe)
	require.NoError(t, err)

	bindings := a.newLocaleBindings(cmd, proj, recipe)
	point := a.GovernancePointFor("", "")

	nb, err := bindings.at(point, "nb")
	require.NoError(t, err)
	again, err := bindings.at(point, "nb")
	require.NoError(t, err)
	assert.Same(t, nb, again, "the same pair is resolved once")

	fr, err := bindings.at(point, "fr")
	require.NoError(t, err)
	assert.NotSame(t, nb, fr, "a second locale gets its own resolution")
	assert.Equal(t, map[string]string{"content memory": "mémoire de contenu"},
		coreprofile.TermRuleMap(fr.termRules))
}
