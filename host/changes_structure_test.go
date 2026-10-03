package host

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/project"
)

// In a project a message catalog's translations are files of their own, and a
// key added to or removed from the source catalog reaches them through the
// change service.
func TestChangeService_AddsAndRemovesKeysAcrossACatalogsTranslations(t *testing.T) {
	item := project.ContentItem{Path: "locales/en.json", Target: "locales/{lang}.json"}
	a, recipe := changeProject(t, item, map[string]string{
		"locales/en.json": "{\n  \"nav\": {\n    \"home\": \"Home\",\n    \"cart\": \"Cart\",\n    \"legacy\": \"Old\"\n  }\n}\n",
		"locales/de.json": "{\n  \"nav\": {\n    \"home\": \"Start\",\n    \"cart\": \"Warenkorb\",\n    \"legacy\": \"Alt\"\n  }\n}\n",
	})
	svc := changeService(t, a, recipe)
	ctx := context.Background()

	d, err := svc.Describe(ctx, change.DescribeRequest{Doc: "locales/en.json"})
	require.NoError(t, err)
	assert.NotNil(t, d.Ops.InsertBlock, "describe reports what the JSON writer writes")
	assert.NotNil(t, d.Ops.DeleteBlock)

	text := func(s string) *string { return &s }
	res, err := svc.Apply(ctx, change.Set{Ops: []change.Op{{
		Kind: change.KindInsertBlock, At: change.Ref{Doc: "locales/en.json"},
		Body: &change.InsertBlock{After: "nav.cart", Name: "nav.checkout", Editions: map[string]change.Content{
			"en": {Text: text("Checkout")}, "de": {Text: text("Kasse")}, "fr": {Text: text("Paiement")},
		}},
	}}}, changePerson)
	require.NoError(t, err)
	require.Equal(t, change.SetApplied, res.Status, "%+v", res.Ops)
	assert.Equal(t, "{\n  \"nav\": {\n    \"home\": \"Home\",\n    \"cart\": \"Cart\",\n    \"checkout\": \"Checkout\",\n    \"legacy\": \"Old\"\n  }\n}\n", readFile(t, recipe, "locales/en.json"))
	assert.Equal(t, "{\n  \"nav\": {\n    \"home\": \"Start\",\n    \"cart\": \"Warenkorb\",\n    \"checkout\": \"Kasse\",\n    \"legacy\": \"Alt\"\n  }\n}\n", readFile(t, recipe, "locales/de.json"))
	assert.Equal(t, "{\n  \"nav\": {\n    \"home\": \"Home\",\n    \"cart\": \"Cart\",\n    \"checkout\": \"Paiement\",\n    \"legacy\": \"Old\"\n  }\n}\n", readFile(t, recipe, "locales/fr.json"),
		"a translation with no file yet is written from the source, the new key translated")

	// The removal names the revision of the source catalog's own edition,
	// and the translations go with the key.
	page, err := svc.Read(ctx, change.ReadRequest{Doc: "locales/en.json", Blocks: []string{"nav.legacy"}})
	require.NoError(t, err)
	require.Len(t, page.Blocks, 1)
	legacy := page.Blocks[0]
	res, err = svc.Apply(ctx, change.Set{Ops: []change.Op{{
		Kind: change.KindDeleteBlock, At: legacy.Ref,
		Body: &change.DeleteBlock{IfMatch: map[string]string{"en": legacy.Rev}},
	}}}, changePerson)
	require.NoError(t, err)
	require.Equal(t, change.SetApplied, res.Status, "%+v", res.Ops)
	assert.Equal(t, "{\n  \"nav\": {\n    \"home\": \"Start\",\n    \"cart\": \"Warenkorb\",\n    \"checkout\": \"Kasse\"\n  }\n}\n", readFile(t, recipe, "locales/de.json"))
	assert.Equal(t, "{\n  \"nav\": {\n    \"home\": \"Home\",\n    \"cart\": \"Cart\",\n    \"checkout\": \"Paiement\"\n  }\n}\n", readFile(t, recipe, "locales/fr.json"))

	res, err = svc.Apply(ctx, change.Set{Ops: []change.Op{{
		Kind: change.KindInsertBlock, At: change.Ref{Doc: "locales/de.json"},
		Body: &change.InsertBlock{Name: "nav.x", Editions: map[string]change.Content{"de": {Text: text("X")}}},
	}}}, changePerson)
	require.NoError(t, err)
	assert.Equal(t, change.CodeInvalid, res.Ops[0].Error.Code, "a block is added through the source catalog: %s", res.Ops[0].Error.Message)
}

// A JSON catalog configured to read a block's note from the member beside it
// (noteRules, as a Chrome extension's messages.json is read) makes that member
// part of the block's shell. Its writer adds and removes no blocks, so
// describing the document reports neither operation and a removal is refused.
func TestChangeService_RefusesStructureWhereNotesAreReadFromBesideABlock(t *testing.T) {
	const catalog = "{\n  \"greeting\": {\n    \"message\": \"Hello\",\n    \"description\": \"Shown on the home page\"\n  },\n  \"cart\": {\n    \"message\": \"Cart\"\n  }\n}\n"
	item := project.ContentItem{Path: "locales/en.json", Format: &project.FormatSpec{Name: "json", Config: map[string]any{"noteRules": "^description$"}}}
	a, recipe := changeProject(t, item, map[string]string{"locales/en.json": catalog})
	svc := changeService(t, a, recipe)
	ctx := context.Background()

	d, err := svc.Describe(ctx, change.DescribeRequest{Doc: "locales/en.json"})
	require.NoError(t, err)
	assert.Nil(t, d.Ops.InsertBlock)
	assert.Nil(t, d.Ops.DeleteBlock)

	page, err := svc.Read(ctx, change.ReadRequest{Doc: "locales/en.json"})
	require.NoError(t, err)
	require.NotEmpty(t, page.Blocks)
	b := page.Blocks[0]
	assert.NotContains(t, b.Ops, change.KindDeleteBlock)
	res, err := svc.Apply(ctx, change.Set{Ops: []change.Op{{
		Kind: change.KindDeleteBlock, At: b.Ref, Body: &change.DeleteBlock{IfMatch: map[string]string{"en": b.Rev}},
	}}}, changePerson)
	require.NoError(t, err)
	require.NotNil(t, res.Ops[0].Error)
	assert.Equal(t, change.CodeUnsupported, res.Ops[0].Error.Code, res.Ops[0].Error.Message)
	assert.Equal(t, catalog, readFile(t, recipe, "locales/en.json"))
}
