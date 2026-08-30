package backend

import (
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

// The gap: the desktop runs its vocabulary gate with no terminology, so every
// finding that comes from the project's own terms is missing from the panel and
// present in `kapi check` and the check_file tool. Pinned rather than described,
// so closing it fails this test and the assertion is tightened to the record.
//
// See #2264.
func TestFaceParity_DesktopChecksMissTheTermsFindings(t *testing.T) {
	app, tab, p := openFixture(t)
	want := facetest.Golden(t)

	res, err := app.RunChecks(tab.ID, ProjectFilter{Glob: p.CheckPath})
	require.NoError(t, err)
	got := desktopCheckFacts(t, res)

	assert.NotEqual(t, want.Check, got,
		"the desktop now agrees with the record: replace this test with the full comparison")
	assert.Len(t, got.Findings, len(want.Findings())-1,
		"the panel is short exactly the terms-store findings")
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

// The gap: the two readings do not yet meet. A target file sitting in the
// working tree counts at the terminal and counts for nothing in the app until a
// run writes it into the store, so one project reads as partly translated in one
// place and untouched in the other. Pinned so the W3 status work fails here and
// tightens this to the record.
//
// See #2265.
func TestFaceParity_DesktopStatusCountsTheStoreNotTheTree(t *testing.T) {
	app, tab, _ := openFixture(t)
	want := facetest.Golden(t)

	_, err := app.RunExtract(tab.ID)
	require.NoError(t, err)
	status, err := app.GetProjectStatus(tab.ID)
	require.NoError(t, err)

	var recorded int
	for _, l := range want.Status.Locales {
		recorded += l.Translated
	}
	require.Positive(t, recorded, "the record stands on a partly translated locale")

	var counted int
	for _, c := range status.Collections {
		for _, n := range c.Coverage {
			counted += n
		}
	}
	assert.Zero(t, counted,
		"the desktop now counts the working tree: compare it against the record instead")
}
