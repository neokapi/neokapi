package host

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/contextop"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
)

func atlasRecord(t *testing.T, atlas ContextAtlas, id string) AtlasRecord {
	t.Helper()
	for _, r := range atlas.Records {
		if r.ID == id {
			return r
		}
	}
	require.FailNow(t, "no record "+id)
	return AtlasRecord{}
}

func atlasAxis(atlas ContextAtlas, id string) (AtlasAxis, bool) {
	for _, ax := range atlas.Axes {
		if ax.ID == id {
			return ax, true
		}
	}
	return AtlasAxis{}, false
}

func valueIDs(ax AtlasAxis) []string {
	out := []string{}
	for _, v := range ax.Values {
		out = append(out, v.ID)
	}
	return out
}

// TestContextAtlas_ReadsTheLog: every rule and note the log holds is a record
// at its point on the project's axes, with the statuses it held over time, and
// every operation is an event; a window adds its bounds on the effective clock.
func TestContextAtlas_ReadsTheLog(t *testing.T) {
	app, _ := contextOpsApp(t)
	root := axesProject(t, "ctxops-atlas")
	ctx := t.Context()

	agent := agentIn("s-atlas")
	rule, err := app.RecordContextObservation(ctx, ContextObserveRequest{
		Actor: agent, Project: recipeOf(root), Term: "use", InsteadOf: []string{"utilise"},
		Evidence: []contextop.Evidence{{Path: "nordic/a.md", Quote: "We utilise the widget."}},
	})
	require.NoError(t, err)
	_, err = app.KeepContextOperation(ctx, ContextKeepRequest{
		Actor: person, Project: recipeOf(root), ID: rule.ID, ValidTo: "2027-01-01",
	})
	require.NoError(t, err)
	note, err := app.RecordContextObservation(ctx, ContextObserveRequest{
		Actor: agent, Project: recipeOf(root), Text: "the teen pages address the reader as you",
		Evidence: []contextop.Evidence{{Path: "teens/a.md"}}, Point: map[string]string{"age": "14-16"},
	})
	require.NoError(t, err)
	_, err = app.DropContextOperation(ctx, ContextDropRequest{Actor: person, Project: recipeOf(root), ID: note.ID})
	require.NoError(t, err)

	atlas, err := app.ContextAtlas(ctx, ContextAtlasRequest{Project: recipeOf(root)})
	require.NoError(t, err)
	assert.Equal(t, ContextAtlasFormat, atlas.Format)
	assert.Equal(t, 1, atlas.Version)
	assert.Equal(t, projectIDFor("ctxops-atlas"), atlas.Project)

	r := atlasRecord(t, atlas, rule.ID)
	assert.Equal(t, AtlasFamilyRule, r.Family)
	assert.Equal(t, `"use", not "utilise"`, r.Label[:len(`"use", not "utilise"`)])
	assert.Equal(t, "nordics", r.Coordinates["market"])
	assert.Equal(t, AtlasFamilyRule, r.Coordinates[AtlasAxisFamily])
	assert.Equal(t, "agent", r.Coordinates[AtlasAxisActor])
	assert.Equal(t, rule.Short, r.Short)
	require.NotNil(t, r.ValidTo)
	assert.Equal(t, "2027-01-01", r.ValidTo.Format("2006-01-02"))
	require.Len(t, r.History, 2)
	assert.Equal(t, "suggested", r.History[0].State)
	assert.Equal(t, "established", r.History[1].State)
	assert.Equal(t, r.Recorded, r.History[0].At)

	n := atlasRecord(t, atlas, note.ID)
	assert.Equal(t, AtlasFamilyNote, n.Family)
	assert.Equal(t, "the teen pages address the reader as you", n.Label)
	assert.Equal(t, "14-16", n.Coordinates["age"], "the submitted point narrowed the note")
	assert.Equal(t, "dropped", n.History[len(n.History)-1].State)

	kinds := map[string]int{}
	for _, e := range atlas.Events {
		kinds[e.Kind]++
		if e.Kind == AtlasEventWindowCloses {
			assert.True(t, e.Record == rule.ID || e.Record == "term:en:utilise", e.Record)
			require.NotNil(t, e.Effective)
			assert.Equal(t, "2027-01-01", e.Effective.Format("2006-01-02"))
		}
	}
	assert.Equal(t, 2, kinds["observe"])
	assert.Equal(t, 1, kinds["keep"])
	assert.Equal(t, 1, kinds["drop"])
	assert.Equal(t, 2, kinds[AtlasEventWindowCloses], "the rule's window and the window the keep wrote onto its term")

	market, ok := atlasAxis(atlas, "market")
	require.True(t, ok)
	assert.Equal(t, "geo", market.Kind)
	assert.Equal(t, AtlasSourceDeclared, market.Source)
	assert.Contains(t, valueIDs(market), "nordics", "a declared group is a value of its axis")
	age, ok := atlasAxis(atlas, "age")
	require.True(t, ok)
	assert.Equal(t, "range", age.Kind)
	assert.Equal(t, []string{"14-16"}, valueIDs(age), "an open axis lists the values its records use")
	audience, ok := atlasAxis(atlas, "audience")
	require.True(t, ok)
	assert.Equal(t, []string{"general", "specialists"}, valueIDs(audience), "a closed axis lists its declared values")
	family, ok := atlasAxis(atlas, AtlasAxisFamily)
	require.True(t, ok)
	assert.Equal(t, AtlasSourceDerived, family.Source)
	assert.Equal(t, []string{"rule", "note", "term"}, valueIDs(family))

	// The kept rule landed in the terms store, and the store's concept is a
	// term record carrying the window the keep wrote.
	var term *AtlasRecord
	for i := range atlas.Records {
		if atlas.Records[i].Family == AtlasFamilyTerm {
			term = &atlas.Records[i]
		}
	}
	require.NotNil(t, term)
	assert.Equal(t, "nordics", term.Coordinates["market"])
	assert.Equal(t, "en", term.Coordinates[AtlasAxisLocale])
	require.NotNil(t, term.ValidTo)

	assert.False(t, atlas.Span.From.IsZero())
	assert.False(t, atlas.Span.To.Before(*r.ValidTo), "the span reaches the window's end")

	// The JSON field names are the contract the Explore view reads.
	raw, err := json.Marshal(atlas)
	require.NoError(t, err)
	var doc map[string]any
	require.NoError(t, json.Unmarshal(raw, &doc))
	for _, key := range []string{"format", "version", "project", "generated_at", "span", "axes", "records", "events", "notes"} {
		assert.Contains(t, doc, key)
	}
	first := doc["records"].([]any)[0].(map[string]any)
	for _, key := range []string{"id", "family", "label", "weight", "coordinates", "recorded", "history"} {
		assert.Contains(t, first, key)
	}

	var text bytes.Buffer
	require.NoError(t, atlas.FormatText(&text))
	assert.Contains(t, text.String(), "market (geo)")
	assert.Contains(t, text.String(), "nordics 2")
}

// TestContextAtlas_ContentCells: with content asked for, the atlas counts the
// project's units by collection, language and state, and reads no block into
// a record of its own.
func TestContextAtlas_ContentCells(t *testing.T) {
	app, recipe := changeProject(t, project.ContentItem{Path: "docs/*.md", Target: "i18n/{lang}/*.md"}, map[string]string{
		"docs/guide.md":     "# Guide\n\nRead the guide first.\n\nThen read the rest.\n",
		"i18n/fr/guide.md":  "# Guide\n\nLisez d'abord le guide.\n\nPuis lisez le reste.\n",
		"i18n/nb-NO/.keep":  "",
		"docs/.kapiignored": "",
	}, func(p *project.KapiProject) {
		p.ID = projectIDFor("ctxops-atlas-content")
		p.Defaults.TargetLanguages = []model.LocaleID{"fr", "nb-NO"}
	})
	app.SetWorkspaceRoot(t.TempDir())
	t.Cleanup(app.Shutdown)

	atlas, err := app.ContextAtlas(t.Context(), ContextAtlasRequest{Project: recipe, Content: true})
	require.NoError(t, err)
	byLocale := map[string]int{}
	for _, r := range atlas.Records {
		if r.Family != AtlasFamilyContent {
			continue
		}
		byLocale[r.Coordinates[AtlasAxisLocale]] += r.Weight
		assert.Len(t, r.History, 1)
		assert.Equal(t, atlas.GeneratedAt, r.History[0].At)
	}
	assert.Equal(t, 3, byLocale["fr"])
	assert.Equal(t, 3, byLocale["nb-NO"])
	nb := atlasRecord(t, atlas, "content:docs:nb-NO:not-started")
	assert.Equal(t, 3, nb.Weight)
	assert.Equal(t, "no", nb.Coordinates[AtlasAxisRegion], "a locale's region subtag places the cell on the map")
	region, ok := atlasAxis(atlas, AtlasAxisRegion)
	require.True(t, ok)
	assert.Equal(t, "geo", region.Kind)
	assert.Equal(t, []AtlasValue{{ID: "no", Label: "Norway"}}, region.Values, "a geo value is its id and its name; fr names no region")
	no := atlasPlace(t, atlas, "no")
	assert.Equal(t, []string{"northern-europe"}, no.Within)
	require.NotNil(t, no.Centroid)
	assert.Contains(t, atlasPlaceIDs(atlas), "no-03", "a country in the atlas brings its subdivisions, so a map can drill down")

	without, err := app.ContextAtlas(t.Context(), ContextAtlasRequest{Project: recipe})
	require.NoError(t, err)
	for _, r := range without.Records {
		assert.NotEqual(t, AtlasFamilyContent, r.Family)
	}
}

// TestContextAtlas_Limit: a cap on records leaves the oldest out and says so.
func TestContextAtlas_Limit(t *testing.T) {
	app, _ := contextOpsApp(t)
	root := axesProject(t, "ctxops-atlas-limit")
	for _, text := range []string{"one", "two", "three"} {
		_, err := app.RecordContextObservation(t.Context(), ContextObserveRequest{
			Actor: person, Project: recipeOf(root), Text: text, Evidence: []contextop.Evidence{{Path: "plain/a.md"}},
		})
		require.NoError(t, err)
	}
	atlas, err := app.ContextAtlas(t.Context(), ContextAtlasRequest{Project: recipeOf(root), Limit: 2})
	require.NoError(t, err)
	labels := []string{}
	for _, r := range atlas.Records {
		labels = append(labels, r.Label)
	}
	assert.Equal(t, []string{"two", "three"}, labels)
	assert.Len(t, atlas.Events, 2)
	assert.Contains(t, atlas.Notes, "2 of 3 recorded rules, notes and pairs are in the atlas; the 1 oldest were left out.")
}

// TestContextAtlas_AllProjects reads every project's operations, each record
// placed on the project axis.
func TestContextAtlas_AllProjects(t *testing.T) {
	app, _ := contextOpsApp(t)
	for _, name := range []string{"ctxops-atlas-one", "ctxops-atlas-two"} {
		root := axesProject(t, name)
		_, err := app.RecordContextObservation(t.Context(), ContextObserveRequest{
			Actor: person, Project: recipeOf(root), Text: "fact about " + name, Evidence: []contextop.Evidence{{Path: "plain/a.md"}},
		})
		require.NoError(t, err)
	}
	atlas, err := app.ContextAtlas(t.Context(), ContextAtlasRequest{AllProjects: true, Content: true})
	require.NoError(t, err)
	assert.Empty(t, atlas.Project)
	projects, ok := atlasAxis(atlas, AtlasAxisProject)
	require.True(t, ok)
	assert.Len(t, projects.Values, 2)
	for _, r := range atlas.Records {
		assert.NotEmpty(t, r.Coordinates[AtlasAxisProject])
	}
	assert.Contains(t, atlas.Notes, "Content cells are measured for one project at a time.")
}
