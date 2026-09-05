package backend

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/terms"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// projectVoiceYAML is the voice at the project's own default point.
const projectVoiceYAML = `name: Northsea
description: How Northsea writes to everyone.
tone:
  personality: [clear, calm]
  formality: neutral
  emotion: neutral
  humor: none
  guidelines: Say the useful thing first.
style:
  active_voice: true
  sentence_length: medium
  person_pov: second
  contractions: sometimes
  prohibited_patterns:
    - regex: '\bsynergy\b'
      description: Corporate filler.
      severity: minor
vocabulary:
  preferred_terms:
    - term: log in
      replacement: sign in
      severity: major
      note: One spelling across the product.
  forbidden_terms:
    - term: bulletproof
      replacement: reliable
      severity: critical
examples:
  - before: Utilize the portal.
    after: Use the portal.
    explanation: Plain words carry further.
    category: vocabulary
locales:
  nb-NO:
    formality: informal
    cultural_notes: Norwegian readers expect direct address.
channels:
  docs:
    tone:
      formality: formal
personas:
  support-agent:
    tone:
      emotion: warm
`

// supportVoiceYAML sits at the support profile's conventional location, which
// is how a profile that binds no voice of its own still has one.
const supportVoiceYAML = `name: Northsea Support
description: How the support surfaces sound.
tone:
  personality: [patient]
  formality: informal
vocabulary:
  preferred_terms:
    - term: ticket
      replacement: request
      severity: minor
`

// campaignVoiceYAML belongs to a profile whose window has closed, so nothing
// resolves to it at the governance instant.
const campaignVoiceYAML = `name: Northsea Campaign
description: The seasonal campaign voice.
tone:
  personality: [energetic]
  formality: informal
`

// newContextProject scaffolds a project with a real context space on disk:
// three collections across three points, a declared brand axis, a profile whose
// window has closed, a voice profile at the project default and at a profile's
// conventional location, and a source file per collection. Everything the
// explorer answers comes from this recipe and the project's own store — no fake
// sources anywhere.
func newContextProject(t *testing.T, app *App) (*TabInfo, string) {
	t.Helper()
	root := t.TempDir()

	require.NoError(t, os.MkdirAll(filepath.Join(root, "docs", "help"), 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(root, "docs", "help", "billing.json"),
		[]byte(`{"intro":"Sign in to see your invoices.","cta":"Manage billing"}`),
		0o644,
	))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "app"), 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(root, "app", "en.json"),
		[]byte(`{"login":"Sign in"}`),
		0o644,
	))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "promo"), 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(root, "promo", "spring.json"),
		[]byte(`{"headline":"Spring offer"}`),
		0o644,
	))

	writeVoice(t, filepath.Join(root, project.RelStatePath("voice.yaml")), projectVoiceYAML)
	writeVoice(t, filepath.Join(root,
		project.RelStatePath(project.ProfilesDirName, "support", "voice.yaml")), supportVoiceYAML)
	writeVoice(t, filepath.Join(root,
		project.RelStatePath(project.ProfilesDirName, "campaign", "voice.yaml")), campaignVoiceYAML)

	proj := &project.KapiProject{
		Version: project.CurrentVersion,
		Name:    "Northsea",
		Defaults: project.Defaults{
			SourceLanguage:  "en-US",
			TargetLanguages: []model.LocaleID{"nb-NO"},
			// A declared axis every collection inherits, beside the structural
			// axes a collection's channel derives.
			Coordinates: map[string]string{project.BrandAxis: "northsea"},
			Voice:       &project.VoiceBinding{ProfileFile: project.RelStatePath("voice.yaml")},
		},
		Profiles: map[string]project.Profile{
			"support": {Channels: []project.Channel{{ID: "docs"}}},
			// A profile whose window closed yesterday: resolution must fall
			// through it and say so rather than governing with it.
			"campaign": {
				Channels: []project.Channel{{ID: "promo"}},
				ValidTo:  time.Now().AddDate(0, 0, -1).Format("2006-01-02"),
			},
		},
		Collections: []project.Collection{
			{
				Name:    "Docs",
				Channel: "support/docs",
				Content: []project.ContentItem{
					{Path: "docs/help/billing.json", Target: "docs/help/billing.{lang}.json"},
				},
			},
			{
				Name: "App",
				Content: []project.ContentItem{
					{Path: "app/en.json", Target: "app/{lang}.json"},
				},
			},
			{
				Name:    "Promo",
				Channel: "campaign/promo",
				Content: []project.ContentItem{
					{Path: "promo/spring.json", Target: "promo/spring.{lang}.json"},
				},
			},
		},
	}
	path := filepath.Join(root, "kapi.yaml")
	require.NoError(t, project.Save(path, proj))

	tab, err := app.OpenProject(path)
	require.NoError(t, err)
	t.Cleanup(func() { app.CloseProject(tab.ID) })
	return tab, root
}

// writeVoice puts a voice profile on disk at a location the resolution ladder
// consults, creating its directory.
func writeVoice(t *testing.T, path, body string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
}

// seedConcept writes one concept into the project's own terms store — the same
// schema inside the same store file the explorer reads back.
func seedConcept(t *testing.T, app *App, tab *TabInfo) {
	t.Helper()
	db, err := app.projectStore(app.getOpenProject(tab.ID))
	require.NoError(t, err)
	require.NoError(t, db.Terms().AddConcept(context.Background(), terms.Concept{
		ID:         "c-signin",
		Definition: "Authenticating into the product.",
		Terms: []terms.Term{
			{Text: "sign in", Locale: "en-US", Status: model.TermPreferred},
			{Text: "log in", Locale: "en-US", Status: model.TermForbidden},
		},
	}))
}

func TestContextGovernsResolvesTheFinestDeclaredPoint(t *testing.T) {
	app := NewApp()
	tab, _ := newContextProject(t, app)

	docs, err := app.ContextGoverns(tab.ID, "Docs", "", 0)
	require.NoError(t, err)
	assert.Equal(t, "support", docs.Point.Profile)
	assert.Equal(t, "docs", docs.Point.Channel)
	assert.False(t, docs.Point.Default)

	// A collection that binds nothing falls back to the project's default point.
	appColl, err := app.ContextGoverns(tab.ID, "App", "", 0)
	require.NoError(t, err)
	assert.Empty(t, appColl.Point.Profile)
	assert.True(t, appColl.Point.Default)
}

func TestContextAtAnswersThroughTheRetrievalSurface(t *testing.T) {
	app := NewApp()
	tab, _ := newContextProject(t, app)
	seedConcept(t, app, tab)

	res, err := app.ContextAt(tab.ID, "docs/help/billing.json", 10)
	require.NoError(t, err)
	assert.Equal(t, "docs/help/billing.json", res.Point.Path)
	assert.Equal(t, "support", res.Point.Profile)
	assert.Equal(t, "docs", res.Point.Channel)
	assert.False(t, res.Point.Default)

	// The terms come back ranked and typed by the retrieval surface, so the
	// desktop projects nothing of its own.
	require.NotEmpty(t, res.Terms)
	assert.Equal(t, "c-signin", res.Terms[0].ConceptID)
	assert.True(t, res.Terms[0].Discouraged, "a rule to act on ranks before a word to reuse")
}

func TestContextAtRefusesAPathlessRequest(t *testing.T) {
	app := NewApp()
	tab, _ := newContextProject(t, app)

	_, err := app.ContextAt(tab.ID, "", 10)
	assert.Error(t, err, "a collection has no location to resolve")
}

func TestContextGovernsReportsAProfileWindow(t *testing.T) {
	app := NewApp()
	tab, _ := newContextProject(t, app)

	res, err := app.ContextGoverns(tab.ID, "Docs", "", 0)
	require.NoError(t, err)

	states := map[string]string{}
	for _, p := range res.Profiles {
		states[p.Name] = p.State
	}
	assert.Equal(t, "expired", states["campaign"], "a closed window is reported, not hidden")
	assert.NotContains(t, states, "support", "an unbounded profile declares no window")
}

func TestContextGovernsCarriesTheProjectVocabulary(t *testing.T) {
	app := NewApp()
	tab, _ := newContextProject(t, app)
	seedConcept(t, app, tab)

	res, err := app.ContextGoverns(tab.ID, "Docs", "", 10)
	require.NoError(t, err)
	require.Len(t, res.Concepts, 1)
	assert.Equal(t, "c-signin", res.Concepts[0].ID)
	assert.Equal(t, 1, res.ConceptsTotal)

	statuses := map[string]string{}
	for _, term := range res.Concepts[0].Terms {
		statuses[term.Text] = term.Status
	}
	assert.Equal(t, "preferred", statuses["sign in"])
	assert.Equal(t, "forbidden", statuses["log in"])
}

func TestContextLivesPlacesContentAtThePoint(t *testing.T) {
	app := NewApp()
	tab, _ := newContextProject(t, app)
	_, err := app.RunExtract(tab.ID)
	require.NoError(t, err)

	res, err := app.ContextLives(tab.ID, "Docs", "", 50)
	require.NoError(t, err)
	require.Len(t, res.Collections, 1)
	assert.Equal(t, "Docs", res.Collections[0].Name)
	assert.Equal(t, "support", res.Collections[0].Profile)
	assert.Equal(t, 2, res.Collections[0].Blocks)

	require.Len(t, res.Coverage, 1)
	assert.Equal(t, "nb-NO", res.Coverage[0].Locale)
	assert.Equal(t, 2, res.Coverage[0].Units)
	assert.Equal(t, 0, res.Coverage[0].Covered)

	require.NotEmpty(t, res.Items)
	for _, item := range res.Items {
		assert.Equal(t, "docs/help/billing.json", item.Document)
	}
}

func TestContextLivesNarrowsToADocument(t *testing.T) {
	app := NewApp()
	tab, _ := newContextProject(t, app)
	_, err := app.RunExtract(tab.ID)
	require.NoError(t, err)

	// A directory is as legitimate a point as a file.
	dir, err := app.ContextLives(tab.ID, "Docs", "docs/help", 50)
	require.NoError(t, err)
	assert.Equal(t, 2, dir.ItemsTotal)

	// A path no content sits at answers empty rather than falling back to all.
	none, err := app.ContextLives(tab.ID, "Docs", "docs/help/other.json", 50)
	require.NoError(t, err)
	assert.Equal(t, 0, none.ItemsTotal)
	assert.Empty(t, none.Items)
}

func TestContextSearchGroupsByKind(t *testing.T) {
	app := NewApp()
	tab, _ := newContextProject(t, app)
	seedConcept(t, app, tab)
	_, err := app.RunExtract(tab.ID)
	require.NoError(t, err)

	res, err := app.ContextSearch(tab.ID, "sign in", "", 10)
	require.NoError(t, err)
	require.NotNil(t, res.Answer)
	assert.Equal(t, "sign in", res.Answer.Query)
	require.NotEmpty(t, res.Answer.Terms)
	assert.Equal(t, "c-signin", res.Answer.Terms[0].ConceptID)

	require.NotEmpty(t, res.Content, "the extracted content holds the phrase")
	assert.Contains(t, res.Content[0].Text, "Sign in")
}

func TestContextSearchRefusesAnEmptyQuery(t *testing.T) {
	app := NewApp()
	tab, _ := newContextProject(t, app)

	_, err := app.ContextSearch(tab.ID, "  ", "", 10)
	assert.Error(t, err)
}

func TestContextRelatesAnswersAtProjectReach(t *testing.T) {
	app := NewApp()
	tab, _ := newContextProject(t, app)
	seedConcept(t, app, tab)
	_, err := app.RunExtract(tab.ID)
	require.NoError(t, err)

	res, err := app.ContextRelates(tab.ID, "concept", "sign in", 20)
	require.NoError(t, err)
	assert.Equal(t, "project", res.Reach, "a local project holds its own subgraph only")
	require.NotEmpty(t, res.Occurrences)
	assert.Equal(t, "c-signin", res.Occurrences[0].ConceptID)
}

// Re-extract rebuilds the context graph with the blocks, so the usage count the
// explorer reads describes this extraction rather than the one before it. The
// count is the graph's, on every face; a Re-extract that refreshed the blocks
// and left the graph would have the desktop showing a number `kapi context
// search` no longer reports.
func TestRunExtractRebuildsTheContextGraph(t *testing.T) {
	app := NewApp()
	tab, _ := newContextProject(t, app)
	seedConcept(t, app, tab)

	usesOf := func(res *ContextSearchResult) (int, string) {
		t.Helper()
		notes := strings.Join(res.Answer.Notes, "\n")
		for _, h := range res.Answer.Terms {
			if h.Term == "sign in" {
				return h.Uses, notes
			}
		}
		t.Fatalf("no hit for %q in %+v", "sign in", res.Answer.Terms)
		return 0, notes
	}

	before, err := app.ContextSearch(tab.ID, "sign in", "", 10)
	require.NoError(t, err)
	uses, notes := usesOf(before)
	assert.Zero(t, uses)
	assert.Contains(t, notes, "has not been extracted yet", "nothing extracted is said, not shown as unused")

	_, err = app.RunExtract(tab.ID)
	require.NoError(t, err)

	after, err := app.ContextSearch(tab.ID, "sign in", "", 10)
	require.NoError(t, err)
	uses, notes = usesOf(after)
	assert.Equal(t, 2, uses, "billing.json and en.json each say it once")
	assert.NotContains(t, notes, "term usage", "a counted answer carries no usage caveat")

	// The relations pane reads the same edges, so it agrees with the search.
	rel, err := app.ContextRelates(tab.ID, "concept", "sign in", 20)
	require.NoError(t, err)
	assert.Equal(t, 2, rel.OccurrencesTotal)
	require.Len(t, rel.Occurrences, 2)
	for _, o := range rel.Occurrences {
		assert.Equal(t, "sign in", o.Term)
		assert.Equal(t, 1, o.Occurrences)
		assert.Contains(t, strings.ToLower(o.Snippet), "sign in", "the passage is read from the block the edge names")
	}
}

func TestContextRelatesSaysWhenTheSubjectIsUnknown(t *testing.T) {
	app := NewApp()
	tab, _ := newContextProject(t, app)
	seedConcept(t, app, tab)

	res, err := app.ContextRelates(tab.ID, "concept", "nothing-by-that-name", 20)
	require.NoError(t, err)
	assert.Empty(t, res.Occurrences)
	require.NotEmpty(t, res.Notes, "an unknown subject is reported, not answered as empty")
}

func TestContextOptionsOffersOnlyTheFreeDimensions(t *testing.T) {
	app := NewApp()
	tab, _ := newContextProject(t, app)

	collections, err := app.ContextOptions(tab.ID, "collection")
	require.NoError(t, err)
	values := make([]string, 0, len(collections))
	for _, o := range collections {
		values = append(values, o.Value)
	}
	assert.ElementsMatch(t, []string{"Docs", "App", "Promo"}, values)

	locales, err := app.ContextOptions(tab.ID, "locale")
	require.NoError(t, err)
	require.Len(t, locales, 1)
	assert.Equal(t, "nb-NO", locales[0].Value)

	coordinates, err := app.ContextOptions(tab.ID, "coordinate")
	require.NoError(t, err)
	found := make([]string, 0, len(coordinates))
	for _, o := range coordinates {
		found = append(found, o.Value)
	}
	assert.Contains(t, found, "support/docs")

	// A pinned dimension has no options: this mount cannot move it.
	projects, err := app.ContextOptions(tab.ID, "project")
	require.NoError(t, err)
	assert.Empty(t, projects)
}

func TestContextExplorerRejectsAnUnknownTab(t *testing.T) {
	app := NewApp()

	_, err := app.ContextGoverns("nope", "", "", 0)
	assert.Error(t, err)
	_, err = app.ContextLives("nope", "", "", 0)
	assert.Error(t, err)
	_, err = app.ContextRelates("nope", "concept", "x", 0)
	assert.Error(t, err)
	_, err = app.ContextSearch("nope", "x", "", 0)
	assert.Error(t, err)
	_, err = app.ContextOptions("nope", "collection")
	assert.Error(t, err)
}
