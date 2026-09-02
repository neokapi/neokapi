package host

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/projectdb"
	"github.com/neokapi/neokapi/terms"
)

// A target identical to its source is a heuristic's guess at "nobody translated
// this", and the project has two ways of answering it outright: its terms, and a
// decision bound to that exact pairing. Both surfaces that feed the ship gate
// have to take the same answer — the checks gate `kapi check` fails on, and the
// exclusion set that demotes a unit below `translated` during `kapi up`. They
// did not: the gate suppressed the finding for do-not-translate terms and the
// coverage did not, so one project, one term and one gate got two verdicts.

// identicalProject writes a project whose target catalog holds one legitimately
// identical string (a product name), one real translation, and one unit that
// drops its source's placeholder. dnt binds a terms concept keeping the product
// name unchanged in the target locale.
func identicalProject(t *testing.T, dnt bool) string {
	t.Helper()
	// The dogfood recipe at the repo root is found by an upward walk from any
	// cwd; an empty value does NOT disable discovery, so the fixture below is
	// what these Apps must bind to.
	t.Setenv("KAPI_NO_PROJECT", "")
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "locales", "en"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "locales", "nb"), 0o755))

	recipe := `version: v1
name: identical
defaults:
  source_language: en
  target_languages: [nb]
collections:
  - path: "locales/en/*.json"
    target: "locales/{lang}/*.json"
`
	require.NoError(t, os.WriteFile(filepath.Join(root, "kapi.yaml"), []byte(recipe), 0o644))
	write := func(rel, body string) {
		require.NoError(t, os.WriteFile(filepath.Join(root, rel), []byte(body), 0o644))
	}
	write("locales/en/app.json", `{"brand":"Compass","greet":"Hello","code":"Hi {name}"}`)
	write("locales/nb/app.json", `{"brand":"Compass","greet":"Hei","code":"Hei"}`)

	if dnt {
		db, err := projectdb.Open(t.Context(), project.LayoutAt(root))
		require.NoError(t, err)
		require.NoError(t, db.Terms().AddConcept(t.Context(), terms.Concept{
			ID: "brand",
			Terms: []terms.Term{
				{Text: "Compass", Locale: model.LocaleEnglish, Status: model.TermPreferred},
				{Text: "Compass", Locale: model.LocaleID("nb"), Status: model.TermPreferred},
			},
		}))
		require.NoError(t, db.Close())
	}
	return root
}

// identicalApp builds an App bound to the fixture's source language.
func identicalApp() *App {
	a := &App{}
	a.InitRegistries()
	a.SourceLang = "en"
	return a
}

// gateFindings runs the checks gate over the fixture and returns the messages it
// reported, plus whether the gate passed.
func gateFindings(t *testing.T, root string) (msgs []string, pass bool) {
	t.Helper()
	a := identicalApp()
	defer a.Shutdown()
	proj, err := project.LoadWithOptions(filepath.Join(root, "kapi.yaml"), project.LoadOptions{SkipRequiresCheck: true})
	require.NoError(t, err)
	cmd := NewEnvCommand(context.Background(), "check")
	AddProjectFlag(cmd)
	require.NoError(t, cmd.Flags().Set("project", filepath.Join(root, "kapi.yaml")))
	units, err := a.UnitsFromProject(proj, root, "")
	require.NoError(t, err)

	gate, err := a.verifyQA(cmd, proj, root, units)
	require.NoError(t, err)
	for _, f := range gate.Findings {
		msgs = append(msgs, f.Message)
	}
	return msgs, gate.Pass
}

// excludedUnits runs the loop's check exclusions over the fixture and returns
// how many units it would demote below `translated`.
func excludedUnits(t *testing.T, root string) int {
	t.Helper()
	a := identicalApp()
	defer a.Shutdown()
	proj, err := project.LoadWithOptions(filepath.Join(root, "kapi.yaml"), project.LoadOptions{SkipRequiresCheck: true})
	require.NoError(t, err)
	cmd := NewEnvCommand(context.Background(), "up")
	AddProjectFlag(cmd)
	require.NoError(t, cmd.Flags().Set("project", filepath.Join(root, "kapi.yaml")))
	units, err := a.UnitsFromProject(proj, root, "")
	require.NoError(t, err)

	excl, err := a.computeLoopCheckExclusions(context.Background(), cmd, proj, root, units)
	require.NoError(t, err)
	return excl.totalFailing()
}

// approveIdentical approves the product-name unit for nb through the real review
// path, so the decision binds both halves of the pairing a live approval would.
func approveIdentical(t *testing.T, root, key string) {
	t.Helper()
	a := identicalApp()
	defer a.Shutdown()
	ok, err := a.ApproveReviewUnit(context.Background(), filepath.Join(root, "kapi.yaml"), "en", "nb",
		filepath.Join("locales", "nb", "app.json"), key, "reviewed")
	require.NoError(t, err)
	require.True(t, ok, "the fixture unit must be approvable")
}

// TestIdenticalTarget_BothGateSurfacesReadTheSameRule holds the converged rule
// shut on both surfaces at once: whatever settles the identity settles it for
// the gate and for the coverage, and whatever does not is reported by both.
func TestIdenticalTarget_BothGateSurfacesReadTheSameRule(t *testing.T) {
	tests := []struct {
		name string
		// setup prepares the project and returns its root.
		setup func(t *testing.T) string
		// identicalReported is whether the product name is still reported as
		// untranslated; excluded is how many units the loop demotes.
		identicalReported bool
		excluded          int
	}{
		{
			name:              "an identical target nobody decided is still suspicious",
			setup:             func(t *testing.T) string { return identicalProject(t, false) },
			identicalReported: true,
			// The identical unit and the placeholder-dropping one.
			excluded: 2,
		},
		{
			name: "an approval settles the question the rule asks",
			setup: func(t *testing.T) string {
				root := identicalProject(t, false)
				approveIdentical(t, root, "brand")
				return root
			},
			identicalReported: false,
			excluded:          1,
		},
		{
			name:              "the project's terms settle it without any decision",
			setup:             func(t *testing.T) string { return identicalProject(t, true) },
			identicalReported: false,
			excluded:          1,
		},
		{
			name: "an approval on a unit that drops a placeholder licenses nothing",
			setup: func(t *testing.T) string {
				root := identicalProject(t, false)
				approveIdentical(t, root, "code")
				return root
			},
			identicalReported: true,
			excluded:          2,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			root := tc.setup(t)

			msgs, pass := gateFindings(t, root)
			identical := false
			for _, m := range msgs {
				if m == "Target is identical to source" {
					identical = true
				}
			}
			assert.Equal(t, tc.identicalReported, identical,
				"the checks gate's reading of the identical target: %v", msgs)
			// The placeholder drop fails the gate in every case, so a passing gate
			// would mean the fixture stopped exercising it.
			assert.False(t, pass, "the placeholder-dropping unit fails the gate throughout")

			assert.Equal(t, tc.excluded, excludedUnits(t, root),
				"the loop's exclusions must reach the same verdict as the gate")
		})
	}
}

// TestIdenticalTarget_ApprovalDoesNotSurviveAnEdit: the approval settles the
// pairing it read, not the string. Editing the translation retires it, and the
// identity is suspicious again on both surfaces.
func TestIdenticalTarget_ApprovalDoesNotSurviveAnEdit(t *testing.T) {
	root := identicalProject(t, false)
	approveIdentical(t, root, "brand")

	msgs, _ := gateFindings(t, root)
	assert.NotContains(t, msgs, "Target is identical to source")
	assert.Equal(t, 1, excludedUnits(t, root))

	// A person rewrites the approved translation to something else identical to
	// the source — same shape, a pairing nobody read.
	require.NoError(t, os.WriteFile(filepath.Join(root, "locales", "en", "app.json"),
		[]byte(`{"brand":"Compass Pro","greet":"Hello","code":"Hi {name}"}`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "locales", "nb", "app.json"),
		[]byte(`{"brand":"Compass Pro","greet":"Hei","code":"Hei"}`), 0o644))

	msgs, _ = gateFindings(t, root)
	assert.Contains(t, msgs, "Target is identical to source",
		"the decision judged another pairing; it settles nothing about this one")
	assert.Equal(t, 2, excludedUnits(t, root))
}
