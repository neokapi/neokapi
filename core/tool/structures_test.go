package tool_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/tool"
)

// upper is a translator of one flat sequence: it upper-cases the text, keeps
// every code, and records what it was given as placeholder text.
type upper struct{ seen []string }

func (u *upper) translate(seq []model.Run) ([]model.Run, error) {
	for _, r := range seq {
		if r.Plural != nil || r.Select != nil {
			return nil, errors.New("a structure reached the translator")
		}
	}
	u.seen = append(u.seen, model.RunsPlaceholderText(seq))
	out := make([]model.Run, len(seq))
	for i, r := range seq {
		if r.Text != nil {
			out[i] = model.TextR(strings.ToUpper(r.Text.Text))
			continue
		}
		out[i] = r
	}
	return out, nil
}

func hashPh() model.Run { return model.PhR(model.PlaceholderRun{ID: "p1", Type: "icu", Data: "#"}) }

func basket() []model.Run {
	return []model.Run{model.TextR("You have "), {Plural: &model.PluralRun{Pivot: "count", Forms: map[model.PluralForm][]model.Run{
		model.PluralOne:   {hashPh(), model.TextR(" item")},
		model.PluralOther: {hashPh(), model.TextR(" items")},
	}}}, model.TextR(" in your basket.")}
}

func TestTranslateStructuresKeepsEveryBranch(t *testing.T) {
	var u upper
	out, err := tool.TranslateStructures(basket(), u.translate)
	require.NoError(t, err)
	require.Len(t, out, 3)
	assert.Equal(t, "YOU HAVE ", out[0].Text.Text)
	require.NotNil(t, out[1].Plural, "the plural stays a plural")
	assert.Equal(t, "count", out[1].Plural.Pivot)
	assert.Equal(t, []model.Run{hashPh(), model.TextR(" ITEM")}, out[1].Plural.Forms[model.PluralOne])
	assert.Equal(t, []model.Run{hashPh(), model.TextR(" ITEMS")}, out[1].Plural.Forms[model.PluralOther])
	assert.Equal(t, " IN YOUR BASKET.", out[2].Text.Text)
	// Each branch, then the text around the structure with a stand-in for it.
	assert.Equal(t, []string{
		`<x id="p1/"/> item`, `<x id="p1/"/> items`,
		`You have <x id="s1/"/> in your basket.`,
	}, u.seen)
}

func TestTranslateStructuresNested(t *testing.T) {
	runs := []model.Run{{Select: &model.SelectRun{Pivot: "gender", Cases: map[string][]model.Run{
		"female": append([]model.Run{model.TextR("She has ")}, basket()[1]),
		"other":  append([]model.Run{model.TextR("They have ")}, basket()[1]),
	}}}}
	var u upper
	out, err := tool.TranslateStructures(runs, u.translate)
	require.NoError(t, err)
	require.Len(t, out, 1)
	female := out[0].Select.Cases["female"]
	assert.Equal(t, "SHE HAS ", female[0].Text.Text)
	assert.Equal(t, " ITEMS", female[1].Plural.Forms[model.PluralOther][1].Text.Text)
	// The message is the select alone: nothing around it to translate.
	assert.NotContains(t, u.seen, `<x id="s1/"/>`)
}

// A translation that drops or doubles a stand-in keeps the text around the
// structures as it was, and every structure, translated, where it was.
func TestTranslateStructuresKeepsAStructureATranslatorDropped(t *testing.T) {
	for name, frame := range map[string]func(seq []model.Run) []model.Run{
		"dropped": func(seq []model.Run) []model.Run { return []model.Run{model.TextR("VOUS AVEZ")} },
		"doubled": func(seq []model.Run) []model.Run { return append(seq, seq[1]) },
	} {
		t.Run(name, func(t *testing.T) {
			var u upper
			out, err := tool.TranslateStructures(basket(), func(seq []model.Run) ([]model.Run, error) {
				if model.RunsPlaceholderText(seq) == `You have <x id="s1/"/> in your basket.` {
					return frame(seq), nil
				}
				return u.translate(seq)
			})
			require.NoError(t, err)
			require.Len(t, out, 3)
			assert.Equal(t, "You have ", out[0].Text.Text)
			assert.Equal(t, " ITEMS", out[1].Plural.Forms[model.PluralOther][1].Text.Text)
		})
	}
}

func TestTranslateStructuresStandInAvoidsTheSequencesIDs(t *testing.T) {
	runs := append([]model.Run{model.PhR(model.PlaceholderRun{ID: "s1", Type: "icu", Data: "{name}"}), model.TextR(" has ")}, basket()[1])
	var u upper
	_, err := tool.TranslateStructures(runs, u.translate)
	require.NoError(t, err)
	assert.Contains(t, u.seen, `<x id="s1/"/> has <x id="s1_/"/>`)
}

func TestTranslateStructuresWithoutStructure(t *testing.T) {
	var u upper
	out, err := tool.TranslateStructures([]model.Run{model.TextR("Save")}, u.translate)
	require.NoError(t, err)
	assert.Equal(t, []model.Run{model.TextR("SAVE")}, out)

	_, err = tool.TranslateStructures(basket(), func([]model.Run) ([]model.Run, error) { return nil, errors.New("offline") })
	assert.EqualError(t, err, "offline")
}
