package backend

import (
	"math"
	"slices"
	"testing"

	"github.com/neokapi/neokapi/host/facetest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The desktop leg of the face parity contract.
//
// The desktop's backend is a third module and cannot be linked beside the CLI
// and MCP suites, so the three meet at the committed record in host/facetest:
// one fixture written from one description, one set of answers, three faces
// asked their own way. Agreeing with the record is agreeing with each other.

// openFixture writes the conformance project and opens it in a tab.
func openFixture(t *testing.T) (*App, *TabInfo, facetest.Project) {
	t.Helper()
	p := facetest.Write(t)
	app := NewApp()
	tab, err := app.OpenProject(p.Recipe)
	require.NoError(t, err)
	t.Cleanup(func() { app.CloseProject(tab.ID) })
	return app, tab, p
}

func TestFaceParity_DesktopContextAtMatchesTheRecord(t *testing.T) {
	app, tab, p := openFixture(t)
	want := facetest.Golden(t)

	answer, err := app.ContextAt(tab.ID, p.ContextPath, p.ContextLimit)
	require.NoError(t, err)
	assert.Equal(t, want.ContextAt, facetest.ContextFactsFrom(answer),
		"ContextAt answers what the record says applies there")
}

func TestFaceParity_DesktopContextSearchMatchesTheRecord(t *testing.T) {
	app, tab, p := openFixture(t)
	want := facetest.Golden(t)

	result, err := app.ContextSearch(tab.ID, p.SearchQuery, "", p.SearchLimit)
	require.NoError(t, err)
	require.NotNil(t, result.Answer)

	// The desktop wraps the shared result and adds a content group of its own.
	// The contract is about the answer inside; the extra group is the desktop
	// showing more, not the desktop answering differently.
	assert.Equal(t, want.ContextSearch, facetest.SearchFactsFrom(result.Answer),
		"ContextSearch answers what the record says is known")
}

// desktopCheckFacts projects what the desktop's checks panel objected to.
func desktopCheckFacts(t *testing.T, res *CheckRunResult) facetest.CheckFacts {
	t.Helper()
	var got facetest.CheckFacts
	for _, f := range res.Files {
		for _, d := range f.Findings {
			got.Findings = append(got.Findings, facetest.FindingFacts{
				Severity:   d.Severity,
				Message:    d.Message,
				Suggestion: d.Suggestion,
			})
		}
	}
	facetest.SortFindings(got.Findings)
	return got
}

// What the desktop does find, it finds the same way: the profile's vocabulary
// rules produce the same finding, with the same severity and the same
// suggestion, as the verb and the tool.
func TestFaceParity_DesktopChecksAgreeOnTheProfileFindings(t *testing.T) {
	app, tab, p := openFixture(t)
	want := facetest.Golden(t)

	res, err := app.RunChecks(tab.ID, ProjectFilter{Glob: p.CheckPath})
	require.NoError(t, err)
	got := desktopCheckFacts(t, res)

	require.NotEmpty(t, want.Findings(), "the record has findings to agree about")
	assert.Contains(t, got.Findings, want.Findings()[0],
		"the profile's vocabulary finding is the same finding at every face")
}

// The desktop runs its vocabulary gate against the vocabulary the project
// decided, at the point each file sits at, so a term the project retired is a
// finding in the panel exactly as it is at the terminal and over MCP.
//
// Closed #2264.
func TestFaceParity_DesktopChecksMatchTheRecord(t *testing.T) {
	app, tab, p := openFixture(t)
	want := facetest.Golden(t)

	res, err := app.RunChecks(tab.ID, ProjectFilter{Glob: p.CheckPath})
	require.NoError(t, err)
	got := desktopCheckFacts(t, res)

	assert.Equal(t, want.Check, got, "the three faces answer one question one way")
}

// Status is the one question the faces answer from different places: `kapi
// status` counts the working tree, and the desktop counts the block store the
// project extracted. What must hold either way is the project and the locales
// it is answering about.
func TestFaceParity_DesktopStatusNamesTheSameProjectAndLocales(t *testing.T) {
	app, tab, _ := openFixture(t)
	want := facetest.Golden(t)

	_, err := app.RunExtract(tab.ID)
	require.NoError(t, err)

	status, err := app.GetProjectStatus(tab.ID)
	require.NoError(t, err)
	require.True(t, status.HasData, "the fixture has content to count")

	assert.Equal(t, want.Status.Project, status.ProjectName)

	locales := map[string]bool{}
	for _, c := range status.Collections {
		for _, l := range c.TargetLanguages {
			locales[l] = true
		}
	}
	for _, l := range want.Status.Locales {
		assert.True(t, locales[l.Locale],
			"the desktop reports on %q, the locale the record stands on", l.Locale)
	}
}

// The two readings meet: the desktop counts the coverage the CLI computes, so a
// target file sitting in the working tree counts the same on both faces.
//
// Closed #2265.
func TestFaceParity_DesktopStatusCountsTheSameTranslatedUnits(t *testing.T) {
	app, tab, _ := openFixture(t)
	want := facetest.Golden(t)

	_, err := app.RunExtract(tab.ID)
	require.NoError(t, err)
	status, err := app.GetProjectStatus(tab.ID)
	require.NoError(t, err)

	require.NotEmpty(t, want.Status.Locales, "the record stands on a locale")
	for _, l := range want.Status.Locales {
		assert.Equal(t, l.Translated, desktopTranslatedPct(status, l.Locale, want.Status.Collections),
			"locale %q: the desktop stands where the record stands", l.Locale)
	}
}

// desktopTranslatedPct renders the desktop's counts as the percentage the
// record reports, over the collections the record reports on. Numerator and
// denominator both come from the working-tree derivation, so the ratio is the
// one `kapi status` prints.
//
// The two faces list different collections and both are defensible: `kapi
// status` prints the ones that have a target locale row, the desktop draws
// every declared collection so a reader sees the whole project. The percentage
// is the part that has to agree, so it is compared over the same collections.
func desktopTranslatedPct(status *ProjectStatus, loc string, collections []string) int {
	var counted, total int
	for _, c := range status.Collections {
		if !slices.Contains(collections, c.Name) || !slices.Contains(c.TargetLanguages, loc) {
			continue
		}
		counted += c.Coverage[loc]
		total += c.Units[loc]
	}
	if total == 0 {
		return 0
	}
	return int(math.Round(float64(counted) / float64(total) * 100))
}

// A target committed beside its source counts before any run has carried it
// into the block store. This is the shape of the gap that was: extraction reads
// sources, so a store-only reading saw the source and not the translation.
func TestFaceParity_DesktopStatusCountsACommittedTargetBeforeARun(t *testing.T) {
	app, tab, _ := openFixture(t)
	want := facetest.Golden(t)

	_, err := app.RunExtract(tab.ID)
	require.NoError(t, err)
	status, err := app.GetProjectStatus(tab.ID)
	require.NoError(t, err)

	var counted int
	for _, c := range status.Collections {
		for _, n := range c.Coverage {
			counted += n
		}
	}
	assert.Positive(t, counted, "the committed target in the tree is translated content")

	for _, l := range want.Status.Locales {
		assert.Positive(t, l.Translated, "the record stands on a partly translated locale")
		assert.Positive(t, desktopTranslatedPct(status, l.Locale, want.Status.Collections),
			"locale %q reads as touched in the app, as it does at the terminal", l.Locale)
	}
}
