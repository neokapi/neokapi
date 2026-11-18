package model_test

import (
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// "You have {count, plural, one {# workspace} other {# workspaces}} left"
func workspaces() []model.Run {
	return []model.Run{
		model.TextR("You have "),
		model.PluralR(model.PluralRun{Pivot: "count", Forms: map[model.PluralForm][]model.Run{
			model.PluralOne:   {model.TextR("one workspace")},
			model.PluralOther: {model.TextR("many workspaces")},
		}}),
		model.TextR(" left"),
	}
}

func TestFormReadings_ReadEveryForm(t *testing.T) {
	t.Parallel()
	readings := model.FormReadings(workspaces())
	require.Len(t, readings, 2)
	assert.Equal(t, "You have many workspaces left", readings[0].Text)
	assert.Nil(t, readings[0].Varied)
	assert.Equal(t, "other", readings[0].Form())
	assert.Equal(t, "You have one workspace left", readings[1].Text)
	assert.Equal(t, "one", readings[1].Form())

	plain := model.FormReadings([]model.Run{model.TextR("Retry")})
	require.Len(t, plain, 1)
	assert.Equal(t, "Retry", plain[0].Text)
	assert.Empty(t, plain[0].Form())
}

func TestFormReading_AnchorNamesTheForm(t *testing.T) {
	t.Parallel()
	runs := workspaces()
	one := model.FormReadings(runs)[1]
	at := len("You have one ")
	a := one.Anchor(at, at+len("workspace"))
	assert.Equal(t, model.AnchorRange, a.Kind)
	require.Len(t, a.Path, 2)
	assert.Equal(t, model.PluralOne, a.Path[1].PluralForm)
	assert.Equal(t, "workspace", a.TextIn(runs))

	// Top-level text after the form keeps its own run's position, whatever
	// width the chosen form has.
	left := one.Anchor(len("You have one workspace "), len("You have one workspace left"))
	assert.Empty(t, left.Path)
	assert.Equal(t, model.RunPos{Run: 2, Offset: 1}, left.Start)
	assert.Equal(t, "left", left.TextIn(runs))

	// Across the edge of a form: the form itself.
	edge := one.Anchor(len("You "), len("You have one"))
	assert.Equal(t, model.AnchorForm, edge.Kind)
	assert.Equal(t, "one workspace", edge.TextIn(runs))
}

func TestFormReadingFor_PairsBySourcePivot(t *testing.T) {
	t.Parallel()
	r := model.FormReadingFor(workspaces(), "count", "one")
	assert.Equal(t, "You have one workspace left", r.Text)
	// A form the source lacks reads its default.
	assert.Equal(t, "You have many workspaces left", model.FormReadingFor(workspaces(), "count", "few").Text)
	assert.Equal(t, "You have many workspaces left", model.FormReadingFor(workspaces(), "n", "one").Text)
}
