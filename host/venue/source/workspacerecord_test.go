package source

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/neokapi/neokapi/host/venue/schema"
)

// A sync cache records the workspace of the recipe's own connection only when
// that connection names the server project the push reached; a push an
// override sent elsewhere records none.
func TestRecipeWorkspaceForRecordsOnlyTheRecipesOwnConnection(t *testing.T) {
	spec := &schema.ServerSpec{URL: "https://app.example/ws-a/app"}
	assert.Equal(t, "ws-a", recipeWorkspaceFor(spec, "https://APP.example:443", "app"))
	assert.Empty(t, recipeWorkspaceFor(spec, "https://other.example", "app"), "an override to another server")
	assert.Empty(t, recipeWorkspaceFor(spec, "https://app.example", "other"), "an override to another project")
	assert.Empty(t, recipeWorkspaceFor(&schema.ServerSpec{URL: "https://app.example/projects/app"}, "https://app.example", "app"), "a direct project names no workspace")
	assert.Empty(t, recipeWorkspaceFor(nil, "https://app.example", "app"))
}
