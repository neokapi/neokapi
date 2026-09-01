package backend

import (
	"testing"

	"github.com/neokapi/neokapi/core/check"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFindingNamesTheRuleThatFired(t *testing.T) {
	block := &model.Block{ID: "b1"}

	// The checker states the rule outright.
	named := toDesktopFinding(
		check.Finding{Category: "voice", Check: "voice-vocab-check"}, block, "source", "en",
		ContextPointDTO{},
	)
	assert.Equal(t, "voice-vocab-check", named.Rule)

	// Where it does not, the term the rule is about identifies it.
	byTerm := toDesktopFinding(
		check.Finding{Category: "voice", Metadata: map[string]string{"term": "log in"}},
		block, "source", "en", ContextPointDTO{},
	)
	assert.Equal(t, "log in", byTerm.Rule)

	// Failing that, the text objected to, then the category. A finding that
	// named nothing would leave a reader with a complaint and nowhere to go.
	byText := toDesktopFinding(
		check.Finding{Category: "placeholder", OriginalText: "{count}"}, block, "target", "de-DE",
		ContextPointDTO{},
	)
	assert.Equal(t, "{count}", byText.Rule)

	bare := toDesktopFinding(check.Finding{Category: "placeholder"}, block, "target", "de-DE", ContextPointDTO{})
	assert.Equal(t, "placeholder", bare.Rule)
}

func TestFindingCarriesThePointItIsScopedTo(t *testing.T) {
	block := &model.Block{ID: "b1"}

	at := toDesktopFinding(
		check.Finding{Category: "voice"}, block, "source", "en",
		ContextPointDTO{Profile: "support", Channel: "docs", Collection: "Docs"},
	)
	assert.Equal(t, "support/docs", at.Point)
	assert.Equal(t, "Docs", at.Collection)

	// The project's own point renders empty rather than as a guessed name.
	def := toDesktopFinding(
		check.Finding{Category: "voice"}, block, "source", "en",
		ContextPointDTO{Collection: "App", Default: true},
	)
	assert.Empty(t, def.Point)
	assert.Equal(t, "App", def.Collection)
}

func TestContextOptionsServesThePathDimension(t *testing.T) {
	app := NewApp()
	tab, _ := newContextProject(t, app)

	paths, err := app.ContextOptions(tab.ID, "path")
	require.NoError(t, err)
	require.NotEmpty(t, paths, "by-location browsing needs a picker, not a typed path")

	values := map[string]string{}
	for _, o := range paths {
		values[o.Value] = o.Label
	}
	assert.Contains(t, values, "docs/help/billing.json")
	assert.Equal(t, "Docs", values["docs/help/billing.json"],
		"a path is labelled by the collection that claims it")
	assert.Contains(t, values, "app/en.json")
}

func TestProjectStreamComesFromTheProject(t *testing.T) {
	app := NewApp()
	tab, _ := newContextProject(t, app)

	server, err := app.GetProjectServer(tab.ID)
	require.NoError(t, err)
	assert.False(t, server.Connected)
	assert.Equal(t, "main", server.Stream,
		"a project that binds no venue is on the framework's default stream")
}
