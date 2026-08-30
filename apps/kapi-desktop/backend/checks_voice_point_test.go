package backend

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	coreprofile "github.com/neokapi/neokapi/core/profile"
	"github.com/neokapi/neokapi/core/project"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The two-point fixture below is the whole point of the test: the SAME sentence
// sits in two collections at two points, and each point's voice forbids a word
// the other's allows. A run that resolved one profile for the project would
// report the same verdict on both files, and would report it against a point it
// had not used.

// supportPointVoice forbids "utilise" and says nothing about "cheap".
const supportPointVoice = `name: Support Voice
vocabulary:
  forbidden_terms:
    - term: utilise
      replacement: use
      severity: major
`

// promoPointVoice forbids "cheap" and says nothing about "utilise".
const promoPointVoice = `name: Promo Voice
vocabulary:
  forbidden_terms:
    - term: cheap
      replacement: affordable
      severity: major
`

// newTwoPointProject scaffolds two collections at two points, each with its own
// voice, and one file per collection carrying the same two words.
func newTwoPointProject(t *testing.T, app *App) (*TabInfo, string) {
	t.Helper()
	root := t.TempDir()

	body := `{"line":"Utilise the cheap plan."}`
	for _, rel := range []string{"docs/help.json", "promo/spring.json"} {
		path := filepath.Join(root, filepath.FromSlash(rel))
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
	}

	writeVoice(t, filepath.Join(root,
		project.RelStatePath(project.ProfilesDirName, "support", "voice.yaml")), supportPointVoice)
	writeVoice(t, filepath.Join(root,
		project.RelStatePath(project.ProfilesDirName, "promo", "voice.yaml")), promoPointVoice)

	proj := &project.KapiProject{
		Version: project.CurrentVersion,
		Name:    "Twopoint",
		Defaults: project.Defaults{
			SourceLanguage:  "en-US",
			TargetLanguages: []model.LocaleID{"nb-NO"},
		},
		Profiles: map[string]project.Profile{
			"support": {Channels: []project.Channel{{ID: "docs"}}},
			"promo":   {Channels: []project.Channel{{ID: "web"}}},
		},
		Collections: []project.Collection{
			{
				Name:    "Docs",
				Channel: "support/docs",
				Content: []project.ContentItem{{Path: "docs/help.json"}},
			},
			{
				Name:    "Promo",
				Channel: "promo/web",
				Content: []project.ContentItem{{Path: "promo/spring.json"}},
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

// findingsFor returns the vocabulary findings reported for one file — the ones
// a voice profile's term rules produce.
func findingsFor(res *CheckRunResult, suffix string) []DesktopFinding {
	var out []DesktopFinding
	for _, f := range res.Files {
		if !hasSuffix(filepath.ToSlash(f.Path), suffix) {
			continue
		}
		for _, fi := range f.Findings {
			if fi.Category == string(coreprofile.DimensionVocabulary) {
				out = append(out, fi)
			}
		}
	}
	return out
}

func hasSuffix(s, suffix string) bool {
	return len(s) >= len(suffix) && s[len(s)-len(suffix):] == suffix
}

func TestChecksJudgeEachFileByItsOwnPointsVoice(t *testing.T) {
	app := NewApp()
	tab, _ := newTwoPointProject(t, app)

	res, err := app.RunChecks(tab.ID, ProjectFilter{})
	require.NoError(t, err)

	docs := findingsFor(res, "docs/help.json")
	promo := findingsFor(res, "promo/spring.json")

	// The same sentence, two verdicts: each point forbids the word its own
	// voice forbids, and says nothing about the other's.
	require.Len(t, docs, 1, "support's voice forbids utilise only")
	assert.Equal(t, "utilise", docs[0].Rule)
	assert.Equal(t, "use", docs[0].Replacement)

	require.Len(t, promo, 1, "promo's voice forbids cheap only")
	assert.Equal(t, "cheap", promo[0].Rule)
	assert.Equal(t, "affordable", promo[0].Replacement)
}

func TestAFindingNamesThePointItWasJudgedAt(t *testing.T) {
	app := NewApp()
	tab, _ := newTwoPointProject(t, app)

	res, err := app.RunChecks(tab.ID, ProjectFilter{})
	require.NoError(t, err)

	// The point a finding reports is the point whose voice produced it, which
	// is the property that makes the click-through honest.
	docs := findingsFor(res, "docs/help.json")
	require.NotEmpty(t, docs)
	assert.Equal(t, "support/docs", docs[0].Point)
	assert.Equal(t, "Docs", docs[0].Collection)

	promo := findingsFor(res, "promo/spring.json")
	require.NotEmpty(t, promo)
	assert.Equal(t, "promo/web", promo[0].Point)
	assert.Equal(t, "Promo", promo[0].Collection)
}

func TestVoiceResolverResolvesEachPointOnce(t *testing.T) {
	app := NewApp()
	tab, _ := newTwoPointProject(t, app)
	op := app.getOpenProject(tab.ID)

	v := app.newVoiceResolver(op, false)
	require.NotNil(t, v)

	first := v.at(t.Context(), "Docs", "docs/help.json")
	require.NotNil(t, first)
	assert.Equal(t, "Support Voice", first.Name)

	// A second file at the same point reuses the resolution rather than
	// reading the profile again.
	assert.Len(t, v.seen, 1)
	again := v.at(t.Context(), "Docs", "docs/help.json")
	assert.Same(t, first, again)
	assert.Len(t, v.seen, 1)

	other := v.at(t.Context(), "Promo", "promo/spring.json")
	require.NotNil(t, other)
	assert.Equal(t, "Promo Voice", other.Name)
	assert.Len(t, v.seen, 2, "a different point is a different resolution")
}

func TestVoiceResolverFallsBackToTheProjectPoint(t *testing.T) {
	app := NewApp()
	tab, _ := newContextProject(t, app)
	op := app.getOpenProject(tab.ID)

	v := app.newVoiceResolver(op, false)
	require.NotNil(t, v)

	// App sits at the project's own point, so it is governed by the project's
	// voice rather than by a profile's.
	appVoice := v.at(t.Context(), "App", "app/en.json")
	require.NotNil(t, appVoice)
	assert.Equal(t, "Northsea", appVoice.Name)

	// Docs sits under the support profile, which keeps its own voice.
	docsVoice := v.at(t.Context(), "Docs", "docs/help/billing.json")
	require.NotNil(t, docsVoice)
	assert.Equal(t, "Northsea Support", docsVoice.Name)
}
