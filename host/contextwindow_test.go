package host

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/contextop"
	"github.com/neokapi/neokapi/core/project"
)

// axesProject is a project that types its axes: a geo market with a nordics
// group and an audience age range. Each collection sits at one point, and
// every file uses the word a rule will be about.
func axesProject(t *testing.T, name string) string {
	t.Helper()
	root := t.TempDir()
	write := func(rel, body string) {
		t.Helper()
		path := filepath.Join(root, rel)
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
		require.NoError(t, os.WriteFile(path, []byte(body), 0o600))
	}
	collection := func(name, axis, value string) string {
		return `  - name: ` + name + `
    source_only: true
    coordinates:
      ` + axis + `: "` + value + `"
    content:
      - path: "` + name + `/*.md"
`
	}
	write("kapi.yaml", `version: v1
id: `+projectIDFor(name)+`
name: `+name+`
defaults:
  source_language: en
places:
  nordics:
    name: Nordics
    of: [no, se, dk, fi, is]
  oslo:
    name: Oslo
    type: locality
    within: no-03
axes:
  market:
    kind: geo
  age:
    kind: range
    unit: years
  audience:
    values: [general, specialists]
collections:
`+collection("nordic", "market", "nordics")+
		collection("norway", "market", "no")+
		collection("germany", "market", "de")+
		collection("oslo", "market", "oslo")+
		collection("teens", "age", "13-17")+
		collection("young", "age", "14-16")+
		collection("adults", "age", "18-")+`  - name: plain
    source_only: true
    content:
      - path: "plain/*.md"
`)
	for _, dir := range []string{"nordic", "norway", "germany", "oslo", "teens", "young", "adults", "plain"} {
		write(dir+"/a.md", "We utilise the widget.\n")
	}
	return root
}

// failingCollections runs a check and names the collection of each file that fails
// on the word under test.
func failingCollections(t *testing.T, app *App, root string) map[string]bool {
	t.Helper()
	out := map[string]bool{}
	for _, d := range vocabularyFindings(checkWith(t, app, root)) {
		if d.Fails {
			out[filepath.Base(filepath.Dir(d.Location.File))] = true
		}
	}
	return out
}

// advisedAt reports whether the suggestions resolved for a file name the word
// under test.
func advisedAt(t *testing.T, app *App, root, rel string) bool {
	t.Helper()
	res, err := app.ContextRulesAt(t.Context(), recipeOf(root), project.GovernancePoint{Path: rel, At: app.GovernanceInstant()})
	require.NoError(t, err)
	for _, r := range res.Advisory {
		if r.Term == "utilise" {
			return true
		}
	}
	return false
}

func observeUtilise(t *testing.T, app *App, root, rel string, req ContextObserveRequest) ContextOperation {
	t.Helper()
	req.Actor, req.Project = person, recipeOf(root)
	req.Term, req.InsteadOf = "use", []string{"utilise"}
	req.Evidence = []contextop.Evidence{{Path: rel}}
	op, err := app.RecordContextObservation(t.Context(), req)
	require.NoError(t, err)
	return op
}

// TestTypedAxes_AGeoRuleAnswersInsideItsRegion: a rule seen in the Nordic
// content answers for content written for Norway and stays silent for content
// written for Germany, as a suggestion and once kept.
func TestTypedAxes_AGeoRuleAnswersInsideItsRegion(t *testing.T) {
	app, _ := contextOpsApp(t)
	root := axesProject(t, "ctxops-geo")

	op := observeUtilise(t, app, root, "nordic/a.md", ContextObserveRequest{})
	assert.Equal(t, map[string]string{"market": "nordics"}, op.Scope.Coordinates)
	assert.True(t, advisedAt(t, app, root, "norway/a.md"), "a suggestion scoped to nordics advises at no")
	assert.False(t, advisedAt(t, app, root, "germany/a.md"), "and not at de")

	_, err := app.KeepContextOperation(t.Context(), ContextKeepRequest{Actor: person, Project: recipeOf(root), ID: op.ID})
	require.NoError(t, err)
	failing := failingCollections(t, app, root)
	assert.True(t, failing["nordic"])
	assert.True(t, failing["norway"], "the kept rule holds at a country inside its region")
	assert.False(t, failing["germany"], "and nowhere outside it")
	assert.False(t, failing["plain"], "content at no market sits outside every scoped rule")
}

// TestTypedAxes_AnAgeRuleAnswersInsideItsRange: a rule seen in content for
// readers aged 13 to 17 answers for 14 to 16 and not for 18 and over.
func TestTypedAxes_AnAgeRuleAnswersInsideItsRange(t *testing.T) {
	app, _ := contextOpsApp(t)
	root := axesProject(t, "ctxops-age")

	op := observeUtilise(t, app, root, "teens/a.md", ContextObserveRequest{})
	assert.True(t, advisedAt(t, app, root, "young/a.md"))
	assert.False(t, advisedAt(t, app, root, "adults/a.md"))

	_, err := app.KeepContextOperation(t.Context(), ContextKeepRequest{Actor: person, Project: recipeOf(root), ID: op.ID})
	require.NoError(t, err)
	failing := failingCollections(t, app, root)
	assert.True(t, failing["young"], "the kept rule holds at 14-16")
	assert.False(t, failing["adults"], "and not at 18-")
}

// TestRuleWindow_ARulePastItsEndStopsAnswering: a rule kept with a valid_to
// fails a check before that instant and says nothing from it on, as a
// suggestion and as a kept rule in the terms store.
func TestRuleWindow_ARulePastItsEndStopsAnswering(t *testing.T) {
	end := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	before, after := end.Add(-24*time.Hour), end.Add(24*time.Hour)

	app, _ := contextOpsApp(t)
	root := axesProject(t, "ctxops-window")
	app.governance = &governanceRun{at: before, noted: map[string]bool{}}

	op := observeUtilise(t, app, root, "plain/a.md", ContextObserveRequest{ValidTo: "2026-09-01"})
	require.NotNil(t, op.Scope.ValidTo)
	assert.True(t, op.Scope.ValidTo.Equal(end))
	assert.True(t, advisedAt(t, app, root, "plain/a.md"))

	_, err := app.KeepContextOperation(t.Context(), ContextKeepRequest{Actor: person, Project: recipeOf(root), ID: op.ID})
	require.NoError(t, err)
	assert.True(t, failingCollections(t, app, root)["plain"], "in force before valid_to")

	db, err := app.ProjectDB(t.Context(), root)
	require.NoError(t, err)
	concepts, err := db.Terms().Concepts(t.Context())
	require.NoError(t, err)
	var windowed bool
	for _, c := range concepts {
		for _, term := range c.Terms {
			if term.Text == "utilise" && term.Validity != nil && term.Validity.ValidTo != nil {
				windowed = term.Validity.ValidTo.Equal(end)
			}
		}
	}
	assert.True(t, windowed, "keeping writes the window onto the term")

	app.governance = &governanceRun{at: after, noted: map[string]bool{}}
	assert.False(t, failingCollections(t, app, root)["plain"], "past valid_to the rule stops answering")
}

// TestRuleWindow_KeepSetsALifetime: a person sets a lifetime while keeping, and
// the rule carries it from then on.
func TestRuleWindow_KeepSetsALifetime(t *testing.T) {
	app, _ := contextOpsApp(t)
	root := axesProject(t, "ctxops-keepwindow")
	op := observeUtilise(t, app, root, "plain/a.md", ContextObserveRequest{})

	_, err := app.KeepContextOperation(t.Context(), ContextKeepRequest{
		Actor: person, Project: recipeOf(root), ID: op.ID, ValidFrom: "2026-01-01", ValidTo: "2025-01-01",
	})
	require.ErrorContains(t, err, "is not after valid_from")

	_, err = app.KeepContextOperation(t.Context(), ContextKeepRequest{
		Actor: person, Project: recipeOf(root), ID: op.ID, ValidFrom: "2026-01-01", ValidTo: "2027-01-01",
	})
	require.NoError(t, err)
	kept, err := app.ContextSubject(t.Context(), recipeOf(root), op.ID)
	require.NoError(t, err)
	require.NotNil(t, kept.Scope.ValidFrom)
	require.NotNil(t, kept.Scope.ValidTo)
	assert.Equal(t, "2027-01-01", kept.Scope.ValidTo.Format(time.DateOnly))

	// A note recorded to start in June, kept with an end in March, would hold
	// at no instant: the bound given now is checked against the one recorded.
	june := observeUtilise(t, app, root, "plain/a.md", ContextObserveRequest{ValidFrom: "2026-06-01"})
	_, err = app.KeepContextOperation(t.Context(), ContextKeepRequest{
		Actor: person, Project: recipeOf(root), ID: june.ID, ValidTo: "2026-03-01",
	})
	require.ErrorContains(t, err, "which is no time at all")
}

// TestSubmittedPoint_Narrows: a submitted point narrows where a note holds,
// within the point its evidence resolves to, and every other use is refused
// with the fix named.
func TestSubmittedPoint_Narrows(t *testing.T) {
	app, _ := contextOpsApp(t)
	root := axesProject(t, "ctxops-narrow")

	narrowed := observeUtilise(t, app, root, "nordic/a.md", ContextObserveRequest{Point: map[string]string{"market": "no"}})
	assert.Equal(t, map[string]string{"market": "no"}, narrowed.Scope.Coordinates)
	city := observeUtilise(t, app, root, "nordic/a.md", ContextObserveRequest{Point: map[string]string{"market": "oslo"}})
	assert.Equal(t, map[string]string{"market": "oslo"}, city.Scope.Coordinates, "a declared city narrows the Nordics")

	added := observeUtilise(t, app, root, "plain/a.md", ContextObserveRequest{Point: map[string]string{"audience": "specialists", "age": "18-"}})
	assert.Equal(t, map[string]string{"audience": "specialists", "age": "18-"}, added.Scope.Coordinates)

	tests := []struct {
		name    string
		rel     string
		point   map[string]string
		wantErr string
	}{
		{"widening is refused", "norway/a.md", map[string]string{"market": "nordics"}, "a note may narrow where it holds, never widen it"},
		{"another country is refused", "nordic/a.md", map[string]string{"market": "de"}, "--widen-to market"},
		{"a value off a closed axis", "plain/a.md", map[string]string{"audience": "children"}, `"children" is not one of its values`},
		{"an unknown geo value", "plain/a.md", map[string]string{"market": "atlantis"}, "is not a place. A place is world"},
		{"a structural axis", "plain/a.md", map[string]string{"channel": "docs"}, "comes from the file the evidence was seen in"},
		{"an empty value", "plain/a.md", map[string]string{"market": ""}, "a point names an axis and a value"},
		{"an axis the recipe never names", "plain/a.md", map[string]string{"markt": "no"}, "the recipe declares no axis called markt"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := app.RecordContextObservation(t.Context(), ContextObserveRequest{
				Actor: person, Project: recipeOf(root), Term: "use", InsteadOf: []string{"utilise"},
				Evidence: []contextop.Evidence{{Path: tt.rel}}, Point: tt.point,
			})
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}

	_, err := app.RecordContextObservation(t.Context(), ContextObserveRequest{
		Actor: person, Project: recipeOf(root), Text: "the Nordic pages say use",
		Evidence: []contextop.Evidence{{Path: "plain/a.md"}}, ValidFrom: "soon",
	})
	require.ErrorContains(t, err, "valid_from")
}
