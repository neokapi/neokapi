package host

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/project"
)

// recipeOp is a recipe operation setting path to the JSON value.
func recipeOp(path, value string) change.Op {
	return change.Op{Kind: change.KindRecipe, Body: &change.Recipe{Path: path, Value: []byte(value)}}
}

// TestChangeSet_EditingTheRecipeAndARecipeFieldAppliesWhole pins a change set
// that edits kapi.yaml as a document and also sets a recipe field. The
// recipe's lock is taken with the document's, in one order, and the recipe
// field is written under it, so the change set lands whole: the document's
// edit and the field are both in the file.
func TestChangeSet_EditingTheRecipeAndARecipeFieldAppliesWhole(t *testing.T) {
	a, recipe := changeProject(t, project.ContentItem{Path: project.RecipeFileName}, nil)
	svc := changeService(t, a, recipe)
	ctx := t.Context()
	page, err := svc.Read(ctx, change.ReadRequest{Doc: project.RecipeFileName})
	require.NoError(t, err)
	var name change.BlockRead
	for _, b := range page.Blocks {
		if b.Text == "changes" {
			name = b
		}
	}
	require.NotEmpty(t, name.Rev, "the recipe's name is a block of the document")

	res, err := svc.Apply(ctx, change.Set{Ops: []change.Op{
		setTo(name.Ref, name.Rev, "renamed"),
		recipeOp("defaults.coordinates.brand", `"kapi"`),
	}}, changePerson)
	require.NoError(t, err)
	require.Equal(t, change.SetApplied, res.Status, "%+v", res.Ops)

	got, err := project.Load(recipe)
	require.NoError(t, err)
	assert.Equal(t, "renamed", got.Name, "the document's edit landed")
	assert.Contains(t, readFile(t, recipe, project.RecipeFileName), "brand: kapi", "and the recipe field beside it")
}

// TestChangeSet_ARecipeFieldThatFailsAfterTheContentLandedIsPartial pins what
// the change service guarantees when an asset operation fails once the
// content has landed: it does not undo the content; the result is partial,
// the asset operation is refused with its error, and the content operation
// is applied.
func TestChangeSet_ARecipeFieldThatFailsAfterTheContentLandedIsPartial(t *testing.T) {
	item := project.ContentItem{Path: "locales/en.json"}
	a, recipe := changeProject(t, item, map[string]string{"locales/en.json": `{"title": "Welcome"}` + "\n"})
	svc := changeService(t, a, recipe)
	ctx := t.Context()
	page, err := svc.Read(ctx, change.ReadRequest{Doc: "locales/en.json"})
	require.NoError(t, err)
	b := page.Blocks[0]

	res, err := svc.Apply(ctx, change.Set{Ops: []change.Op{
		setTo(b.Ref, b.Rev, "Welcome aboard"),
		recipeOp("no_such_field", `"x"`),
	}}, changePerson)
	require.NoError(t, err)
	assert.Equal(t, change.SetPartial, res.Status, "%+v", res.Ops)
	require.Len(t, res.Ops, 2)
	assert.Equal(t, change.OpApplied, res.Ops[0].Status, "the content landed")
	assert.Equal(t, change.OpRefused, res.Ops[1].Status, "the recipe field is refused")
	require.NotNil(t, res.Ops[1].Error)
	assert.Contains(t, readFile(t, recipe, "locales/en.json"), "Welcome aboard")
	assert.False(t, strings.Contains(readFile(t, recipe, filepath.Base(recipe)), "no_such_field"))
}
