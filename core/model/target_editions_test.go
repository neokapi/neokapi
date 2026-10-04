package model_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/neokapi/neokapi/core/model"
)

func TestBlockEachTargetEdition(t *testing.T) {
	b := editionBlock()
	b.SetTargetEdition(model.Variant("en"), model.Edition{Runs: []model.Run{model.TextR("Hello, same language")}})
	b.SetTargetEdition(model.EditionKey{}, model.Edition{Runs: []model.Run{model.TextR("No language")}, Status: model.Status(model.TargetStatusDraft)})
	b.Editions[model.Variant("de")] = nil

	got := map[string]model.Edition{}
	for k, e := range b.EachTargetEdition {
		text, _ := k.MarshalText()
		got[string(text)] = e
	}
	assert.Equal(t, map[string]model.Edition{
		"fr":               {Runs: []model.Run{model.TextR("Bonjour")}, Status: model.Status(model.TargetStatusTranslated), Origin: model.Origin{Kind: model.OriginHuman}, Score: 0.8},
		"en;channel=short": {Runs: []model.Run{model.TextR("Hi")}},
		"en":               {Runs: []model.Run{model.TextR("Hello, same language")}},
		"":                 {Runs: []model.Run{model.TextR("No language")}, Status: model.Status(model.TargetStatusDraft)},
	}, got, "every target, the zero key and the source language included, and never the edition the block was read in")

	n := 0
	for range b.EachTargetEdition {
		n++
		break
	}
	assert.Equal(t, 1, n, "the walk stops when the loop does")

	var none []model.EditionKey
	for k := range model.NewBlock("b2", "Hello").EachTargetEdition {
		none = append(none, k)
	}
	assert.Empty(t, none)
}
