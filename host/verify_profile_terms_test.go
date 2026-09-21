package host

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/check"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/terms"
	"github.com/neokapi/neokapi/terms/ktb"
)

// Where a profile-terms fixture binds its vocabulary.
const (
	termsOnProfileStore  = "profile termstore database"
	termsOnDefaults      = "defaults terms_source"
	termsNowhere         = "nowhere"
)

// profileTermsBindings is how a profile binds terms of its own: `termstore:`
// naming a store. A bundle under `.kapi/` is an export artifact that an import
// reads into the project vocabulary, so it governs everywhere rather than at
// one point.
var profileTermsBindings = []string{termsOnProfileStore}

// profileTermsFixture describes a project with a `press` profile on the `docs`
// channel and French targets. The vocabulary approves Enregistrer for Save.
type profileTermsFixture struct {
	binding string
	// fixed writes the approved term into the press document's target.
	fixed bool
	// noDocument leaves out the readable press document.
	noDocument bool
	// catalog adds a press catalog whose target no reader opens.
	catalog bool
	// elsewhere adds a collection at the project's default point whose target
	// would violate the vocabulary if it governed there.
	elsewhere bool
	// brand adds a term the vocabulary keeps unchanged in French, and keeps it
	// unchanged in the press document's target.
	brand bool
}

var brandConcept = terms.Concept{
	ID: "brand",
	Terms: []terms.Term{
		{Text: "KapiMart", Locale: model.LocaleEnglish, Status: model.TermPreferred},
		{Text: "KapiMart", Locale: model.LocaleFrench, Status: model.TermPreferred},
	},
}

var saveConcept = terms.Concept{
	ID: "save",
	Terms: []terms.Term{
		{Text: "Save", Locale: model.LocaleEnglish, Status: model.TermPreferred},
		{Text: "Enregistrer", Locale: model.LocaleFrench, Status: model.TermPreferred},
	},
}

func writeProfileTermsProject(t *testing.T, f profileTermsFixture) string {
	t.Helper()
	isolateCheckExecution(t)
	root := t.TempDir()

	var recipe strings.Builder
	recipe.WriteString("version: v1\nname: profileterms\ndefaults:\n  source_language: en\n  target_languages: [fr]\n")
	if f.binding == termsOnDefaults {
		recipe.WriteString("  terms_source: .kapi/terms.json\n")
	}
	recipe.WriteString("profiles:\n  press:\n    channels: [docs]\n")
	switch f.binding {
	case termsOnProfileStore:
		recipe.WriteString("    termstore: vocab/press.db\n")
	}
	recipe.WriteString("collections:\n")
	if !f.noDocument {
		recipe.WriteString("  - name: press-docs\n    channel: press/docs\n    path: press/en/app.json\n    target: \"press/{lang}/app.json\"\n")
	}
	if f.catalog {
		recipe.WriteString("  - name: press-catalog\n    channel: press/docs\n    path: press/en/catalog.json\n    target: \"press/{lang}/catalog.bin\"\n")
	}
	if f.elsewhere {
		recipe.WriteString("  - name: app\n    path: app/en/app.json\n    target: \"app/{lang}/app.json\"\n")
	}
	recipe.WriteString("ship_gate: { translated: 0 }\n")
	writeFixtureFile(t, root, "kapi.yaml", recipe.String())

	pressTarget := "Sauvegarder"
	if f.fixed {
		pressTarget = "Enregistrer"
	}
	concepts := []terms.Concept{saveConcept}
	brandKey, brandTarget := "", ""
	if f.brand {
		concepts = append(concepts, brandConcept)
		brandKey, brandTarget = `, "brand": "KapiMart"`, `, "brand": "KapiMart"`
	}
	if !f.noDocument {
		writeFixtureFile(t, root, "press/en/app.json", `{"save": "Save"`+brandKey+`}`)
		writeFixtureFile(t, root, "press/fr/app.json", `{"save": "`+pressTarget+`"`+brandTarget+`}`)
	}
	if f.catalog {
		writeFixtureFile(t, root, "press/en/catalog.json", `{"save": "Save"}`)
		writeFixtureFile(t, root, "press/fr/catalog.bin", "\x00compiled catalog")
	}
	if f.elsewhere {
		writeFixtureFile(t, root, "app/en/app.json", `{"save": "Save"}`)
		writeFixtureFile(t, root, "app/fr/app.json", `{"save": "Sauvegarder"}`)
	}

	switch f.binding {
	case termsOnProfileStore:
		require.NoError(t, os.MkdirAll(filepath.Join(root, "vocab"), 0o755))
		store, err := terms.NewSQLiteStore(filepath.Join(root, "vocab", "press.db"))
		require.NoError(t, err)
		for _, c := range concepts {
			require.NoError(t, store.AddConcept(t.Context(), c))
		}
		require.NoError(t, store.Close())
	case termsOnDefaults:
		writeConceptsBundle(t, filepath.Join(root, project.RelStatePath(ktb.ConventionalName)), concepts)
	}
	readProjectContext(t, root)
	return root
}

func writeFixtureFile(t *testing.T, root, rel, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
}

func writeConceptsBundle(t *testing.T, path string, concepts []terms.Concept) {
	t.Helper()
	data, err := ktb.Marshal(ktb.FromConcepts(concepts))
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, data, 0o644))
}

// shipCheck runs `kapi check --ship` over the project, with extra flags.
func shipCheck(t *testing.T, root string, flags map[string]string) verifyOutput {
	t.Helper()
	cmd := sourceShipCommand(t, root)
	for k, v := range flags {
		require.NoError(t, cmd.Flags().Set(k, v))
	}
	out, err := (&App{}).computeVerify(cmd, nil)
	require.NoError(t, err)
	return out
}

// coverageCounts reads a gate's coverage as its JSON consumers do.
func coverageCounts(t *testing.T, g verifyGateResult) map[string]int {
	t.Helper()
	require.NotNil(t, g.Coverage, "gate %s reports no coverage", g.Gate)
	data, err := json.Marshal(g.Coverage)
	require.NoError(t, err)
	counts := map[string]int{}
	require.NoError(t, json.Unmarshal(data, &counts))
	return counts
}

func findingFiles(g verifyGateResult) []string {
	var files []string
	for _, f := range g.Findings {
		files = append(files, f.File)
	}
	return files
}

// profileTermsStatus runs `kapi status --json` over the project.
func profileTermsStatus(t *testing.T, root string) StatusOutput {
	t.Helper()
	cmd := NewEnvCommand(t.Context(), "status")
	AddProjectFlag(cmd)
	AddStatusFlags(cmd)
	require.NoError(t, cmd.Flags().Set("project", filepath.Join(root, "kapi.yaml")))
	require.NoError(t, cmd.Flags().Set("json", "true"))
	out, err := captureStdout(t, func() error { return (&App{}).RunStatus(cmd, nil) })
	require.NoError(t, err)
	var parsed StatusOutput
	require.NoError(t, json.Unmarshal([]byte(out), &parsed), "status must emit valid JSON: %s", out)
	return parsed
}

// profileTermsManifest runs `kapi status --ship` over the project and reads
// the ship.json it prints.
func profileTermsManifest(t *testing.T, root string) ShipManifest {
	t.Helper()
	cmd := NewEnvCommand(t.Context(), "status")
	AddProjectFlag(cmd)
	AddStatusFlags(cmd)
	require.NoError(t, cmd.Flags().Set("project", filepath.Join(root, "kapi.yaml")))
	require.NoError(t, cmd.Flags().Set("ship", "true"))
	out, err := captureStdout(t, func() error { return (&App{}).RunStatus(cmd, nil) })
	require.NoError(t, err)
	var manifest ShipManifest
	require.NoError(t, json.Unmarshal([]byte(out), &manifest), "stdout must be the ship manifest: %s", out)
	return manifest
}

func scopeCoverage(t *testing.T, o StatusOutput, collection, locale string) LocaleCoverage {
	t.Helper()
	for _, lc := range o.Locales {
		if lc.Collection == collection && lc.Locale == locale {
			return lc
		}
	}
	require.Failf(t, "no coverage row", "%s/%s in %+v", locale, collection, o.Locales)
	return LocaleCoverage{}
}

// TestShip_ProfileTermsFailTheTerminologyGate binds terms only on a profile
// and puts a violation in content at that profile's channel. The terminology
// gate runs and fails on it, and the ship gate's checks see the same failure.
func TestShip_ProfileTermsFailTheTerminologyGate(t *testing.T) {
	for _, binding := range profileTermsBindings {
		t.Run(binding, func(t *testing.T) {
			root := writeProfileTermsProject(t, profileTermsFixture{binding: binding})

			out := shipCheck(t, root, nil)
			g, ok := gateByName(out, gateTerms)
			require.True(t, ok, "terms bound on a profile schedule the terminology gate: %+v", out.Gates)
			assert.Equal(t, check.VerdictFailed, g.Verdict)
			require.NotEmpty(t, g.Findings)
			assert.Contains(t, g.Findings[0].Message, "Enregistrer")
			assert.Equal(t, "press/fr/app.json", g.Findings[0].File)
			counts := coverageCounts(t, g)
			assert.Equal(t, 1, counts["files"])
			assert.Positive(t, counts["blocks"])

			ship, ok := gateByName(out, gateShip)
			require.True(t, ok)
			assert.Equal(t, check.VerdictFailed, ship.Verdict, "the ship gate's checks apply the profile's terms too")
			assert.Equal(t, check.VerdictFailed, out.Verdict)

			cmd := sourceShipCommand(t, root)
			var buf bytes.Buffer
			cmd.SetOut(&buf)
			require.ErrorIs(t, (&App{}).RunVerify(cmd, nil), ErrQualityGate)

			press := scopeCoverage(t, profileTermsStatus(t, root), "press-docs", "fr")
			assert.Equal(t, 1, press.FailingChecks, "status reads the profile's terms at the unit's point")
			assert.Empty(t, press.NotGoverned)
		})
	}
}

// TestShip_ProfileTermsPassWhenContentFollowsThem is the same project with the
// approved term in place: the gate passes, having checked content.
func TestShip_ProfileTermsPassWhenContentFollowsThem(t *testing.T) {
	for _, binding := range profileTermsBindings {
		t.Run(binding, func(t *testing.T) {
			root := writeProfileTermsProject(t, profileTermsFixture{binding: binding, fixed: true})

			out := shipCheck(t, root, nil)
			g, ok := gateByName(out, gateTerms)
			require.True(t, ok, "terms bound on a profile schedule the terminology gate: %+v", out.Gates)
			assert.Equal(t, check.VerdictPassed, g.Verdict, "%+v", g)
			assert.Empty(t, g.Findings)
			counts := coverageCounts(t, g)
			assert.Equal(t, 1, counts["files"])
			assert.Positive(t, counts["blocks"], "a pass must have checked content")

			ship, ok := gateByName(out, gateShip)
			require.True(t, ok)
			assert.Equal(t, check.VerdictPassed, ship.Verdict)

			press := scopeCoverage(t, profileTermsStatus(t, root), "press-docs", "fr")
			assert.Empty(t, press.NotGoverned, "the profile's terms govern the press content")
			assert.True(t, press.Shippable)
			assert.Empty(t, profileTermsManifest(t, root)["fr"].NotGoverned)
		})
	}
}

// TestShip_ProfileTermsLeaveContentElsewhereNotGoverned mixes content at the
// bound profile with content at a point that binds no terms. The first is
// checked. The second is counted as not governed and never fails, although
// its target would violate the profile's vocabulary.
func TestShip_ProfileTermsLeaveContentElsewhereNotGoverned(t *testing.T) {
	for _, fixed := range []bool{false, true} {
		t.Run(map[bool]string{false: "violation", true: "clean"}[fixed], func(t *testing.T) {
			root := writeProfileTermsProject(t, profileTermsFixture{binding: termsOnProfileStore, fixed: fixed, elsewhere: true})

			out := shipCheck(t, root, nil)
			g, ok := gateByName(out, gateTerms)
			require.True(t, ok, "%+v", out.Gates)
			assert.NotContains(t, findingFiles(g), "app/fr/app.json", "content no terms govern is never checked against them")
			counts := coverageCounts(t, g)
			assert.Equal(t, 1, counts["files"], "only the press document is checked")
			assert.Equal(t, 1, counts["not_governed"], "the app document is counted as not governed")
			if fixed {
				assert.Equal(t, check.VerdictPassed, g.Verdict, "%+v", g)
			} else {
				assert.Equal(t, check.VerdictFailed, g.Verdict)
				assert.Contains(t, findingFiles(g), "press/fr/app.json")
			}

			status := profileTermsStatus(t, root)
			assert.Empty(t, scopeCoverage(t, status, "press-docs", "fr").NotGoverned)
			app := scopeCoverage(t, status, "app", "fr")
			assert.Equal(t, []string{"terms"}, app.NotGoverned)
			assert.Zero(t, app.FailingChecks)
			assert.Empty(t, profileTermsManifest(t, root)["fr"].NotGoverned,
				"terms govern part of fr, so ship.json does not call fr ungoverned")
		})
	}
}

// TestShip_ProfileTermsOverUnreadableContentDidNotRun binds terms on a profile
// whose only content has a target no reader opens. Nothing the terms govern
// was checked, so the gate did not run, although readable content sits
// elsewhere.
func TestShip_ProfileTermsOverUnreadableContentDidNotRun(t *testing.T) {
	root := writeProfileTermsProject(t, profileTermsFixture{binding: termsOnProfileStore, noDocument: true, catalog: true, elsewhere: true})

	out := shipCheck(t, root, nil)
	g, ok := gateByName(out, gateTerms)
	require.True(t, ok, "%+v", out.Gates)
	assert.Equal(t, check.VerdictDidNotRun, g.Verdict)
	assert.Equal(t, check.CauseContentNotChecked, g.DidNotRunCause)
	assert.Contains(t, strings.Join(g.DidNotRun, "; "), "catalog")
	assert.False(t, g.Pass)
	counts := coverageCounts(t, g)
	assert.Zero(t, counts["blocks"])
	assert.Equal(t, 1, counts["not_checked"])
	assert.Equal(t, 1, counts["not_governed"])
	assert.NotEqual(t, check.VerdictPassed, out.Verdict)

	catalog := scopeCoverage(t, profileTermsStatus(t, root), "press-catalog", "fr")
	assert.Positive(t, catalog.TermsNotChecked)
	assert.False(t, catalog.Shippable)
}

// TestShip_UnreadableGovernedContentWithholdsTheTerminologyGate checks the press
// document cleanly, and cannot read the press catalog. The gate does not pass
// on the half it read, and the ship gate names the unchecked units.
func TestShip_UnreadableGovernedContentWithholdsTheTerminologyGate(t *testing.T) {
	root := writeProfileTermsProject(t, profileTermsFixture{binding: termsOnProfileStore, fixed: true, catalog: true})

	out := shipCheck(t, root, nil)
	g, ok := gateByName(out, gateTerms)
	require.True(t, ok, "%+v", out.Gates)
	assert.Equal(t, check.VerdictDidNotRun, g.Verdict)
	assert.Equal(t, check.CauseContentNotChecked, g.DidNotRunCause)
	assert.Contains(t, strings.Join(g.DidNotRun, "; "), "catalog")
	counts := coverageCounts(t, g)
	assert.Equal(t, 1, counts["files"])
	assert.Positive(t, counts["blocks"])
	assert.Equal(t, 1, counts["not_checked"])

	ship, ok := gateByName(out, gateShip)
	require.True(t, ok)
	assert.Equal(t, check.VerdictFailed, ship.Verdict)
	var messages []string
	for _, f := range ship.Findings {
		messages = append(messages, f.Message)
	}
	assert.Contains(t, strings.Join(messages, "; "), "no terminology result")
}

// TestShip_DefaultTermsGovernEveryPoint is the control for project-wide terms:
// both collections are governed, and nothing is counted as not governed.
func TestShip_DefaultTermsGovernEveryPoint(t *testing.T) {
	root := writeProfileTermsProject(t, profileTermsFixture{binding: termsOnDefaults, elsewhere: true})

	out := shipCheck(t, root, nil)
	g, ok := gateByName(out, gateTerms)
	require.True(t, ok, "%+v", out.Gates)
	assert.Equal(t, check.VerdictFailed, g.Verdict)
	assert.ElementsMatch(t, []string{"app/fr/app.json", "press/fr/app.json"}, findingFiles(g))
	counts := coverageCounts(t, g)
	assert.Equal(t, 2, counts["files"])
	assert.Zero(t, counts["not_governed"])

	status := profileTermsStatus(t, root)
	assert.Empty(t, scopeCoverage(t, status, "press-docs", "fr").NotGoverned)
	assert.Empty(t, scopeCoverage(t, status, "app", "fr").NotGoverned)
}

// TestShip_NoTermsAnywhere is the control for a project that binds terms
// nowhere: a default run has no terminology gate, and naming the gate reports
// the missing binding as before.
func TestShip_NoTermsAnywhere(t *testing.T) {
	root := writeProfileTermsProject(t, profileTermsFixture{binding: termsNowhere, elsewhere: true})

	out := shipCheck(t, root, nil)
	_, ok := gateByName(out, gateTerms)
	assert.False(t, ok, "a project binding no terms has no terminology gate: %+v", out.Gates)

	named := shipCheck(t, root, map[string]string{"gate": gateTerms})
	g, ok := gateByName(named, gateTerms)
	require.True(t, ok)
	assert.Equal(t, check.VerdictDidNotRun, g.Verdict)
	require.NotEmpty(t, g.Findings)
	assert.Contains(t, g.Findings[0].Message, "defaults.terms_source")

	status := profileTermsStatus(t, root)
	assert.Equal(t, []string{"terms"}, scopeCoverage(t, status, "press-docs", "fr").NotGoverned)
	assert.Equal(t, []string{"terms"}, profileTermsManifest(t, root)["fr"].NotGoverned)
}

// TestShip_ProfileTermsWithNoContentAtTheirPoint binds terms on a profile no
// collection sits at. Nothing in scope is governed, so a default run has no
// terminology gate, and naming the gate reports that it checked nothing.
func TestShip_ProfileTermsWithNoContentAtTheirPoint(t *testing.T) {
	root := writeProfileTermsProject(t, profileTermsFixture{binding: termsOnProfileStore, noDocument: true, elsewhere: true})

	out := shipCheck(t, root, nil)
	_, ok := gateByName(out, gateTerms)
	assert.False(t, ok, "terms that govern no content in scope schedule no gate: %+v", out.Gates)

	named := shipCheck(t, root, map[string]string{"gate": gateTerms})
	g, ok := gateByName(named, gateTerms)
	require.True(t, ok)
	assert.Equal(t, check.VerdictDidNotRun, g.Verdict)
	assert.False(t, g.Pass)
	assert.Contains(t, strings.Join(g.DidNotRun, "; "), "profiles.press")
}

// TestShip_ProfileTermsSettleAnIdenticalTarget binds a term the profile's
// vocabulary keeps unchanged in French. The press target keeps it unchanged,
// and the checks gate and the ship gate's checks read that as settled at the
// press point rather than as an untranslated unit.
func TestShip_ProfileTermsSettleAnIdenticalTarget(t *testing.T) {
	root := writeProfileTermsProject(t, profileTermsFixture{binding: termsOnProfileStore, fixed: true, brand: true})

	out := shipCheck(t, root, nil)
	checks, ok := gateByName(out, gateChecks)
	require.True(t, ok)
	for _, f := range checks.Findings {
		assert.NotContains(t, f.Message, "identical to source", "the profile's terms keep KapiMart unchanged")
	}
	assert.Equal(t, check.VerdictPassed, checks.Verdict, "%+v", checks.Findings)

	press := scopeCoverage(t, profileTermsStatus(t, root), "press-docs", "fr")
	assert.Zero(t, press.FailingChecks)
}
