package host

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/workspace"
)

// writeRecipe writes kapi.yaml with body into a fresh folder.
func writeRecipe(t *testing.T, body string) string {
	t.Helper()
	recipe := filepath.Join(t.TempDir(), project.RecipeFileName)
	require.NoError(t, os.WriteFile(recipe, []byte(body), 0o600))
	return recipe
}

// A bowrain backend names no location of its own: it shares the context with
// the project the recipe's bowrain: block connects to.
func TestResolveContextBackend_BowrainReadsTheBowrainBlock(t *testing.T) {
	recipe := writeRecipe(t, `version: v1
source_language: en
context:
  backend: bowrain
bowrain:
  url: https://bowrain.example/acme/p123
  stream: v2
`)
	info, err := ResolveContextBackend(recipe)
	require.NoError(t, err)
	assert.Equal(t, project.ContextBackendBowrain, info.Kind)
	assert.Equal(t, "recipe", info.From)
	assert.Equal(t, "https://bowrain.example/acme/p123", info.Location)
	require.NotNil(t, info.spec.Venue, "the opener reads the project from the resolved spec")
	assert.Equal(t, project.ContextVenue{URL: "https://bowrain.example/acme/p123", Stream: "v2"}, *info.spec.Venue)

	var b bytes.Buffer
	require.NoError(t, info.FormatText(&b))
	assert.Equal(t, "Context: Managed on Bowrain (https://bowrain.example/acme/p123), declared in the recipe.\n", b.String())
}

// A recipe that declares a bowrain backend and no bowrain: block is refused,
// with the key to add and the command that adds it.
func TestResolveContextBackend_BowrainWithoutTheBlockIsRefused(t *testing.T) {
	recipe := writeRecipe(t, `version: v1
source_language: en
context:
  backend: bowrain
`)
	_, err := ResolveContextBackend(recipe)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no `bowrain:` block")
	assert.Contains(t, err.Error(), "kapi share --via bowrain <server-url>")

	recipe = writeRecipe(t, `version: v1
source_language: en
context:
  backend: bowrain
bowrain:
  converge: manual
`)
	_, err = ResolveContextBackend(recipe)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "`bowrain.url`")
}

// The context: block of a bowrain backend takes no field another backend
// reads.
func TestResolveContextBackend_BowrainTakesNoLocationFields(t *testing.T) {
	recipe := writeRecipe(t, `version: v1
source_language: en
context:
  backend: bowrain
  bucket: acme
bowrain:
  url: https://bowrain.example/acme/p123
`)
	_, err := ResolveContextBackend(recipe)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "context.bucket: applies to the s3 backend, and this recipe names bowrain")
}

// A pull or a push through a bowrain backend names Bowrain and the project,
// never the backend kind.
func TestDescribeRemote_NamesBowrainForTheProduct(t *testing.T) {
	assert.Equal(t, "Bowrain (https://bowrain.example/acme/p123)",
		describeRemote(workspace.RemoteDescriptor{Kind: project.ContextBackendBowrain, Location: "https://bowrain.example/acme/p123"}))
	assert.Equal(t, "s3 s3://acme/site/", describeRemote(workspace.RemoteDescriptor{Kind: "s3", Location: "s3://acme/site/"}))
}

// A project whose context is shared on Bowrain stands on the ladder's top
// rung, and `kapi share` names it so.
func TestShare_BowrainBackendIsTheTopRung(t *testing.T) {
	recipe := writeRecipe(t, `version: v1
source_language: en
context:
  backend: bowrain
bowrain:
  url: https://bowrain.example/acme/p123
`)
	app, _ := contextOpsApp(t)
	res, err := app.Share(t.Context(), ShareRequest{Project: recipe, Status: true})
	require.NoError(t, err)
	assert.Equal(t, ShareRungBowrain, res.Rung)
	assert.Equal(t, "https://bowrain.example/acme/p123", res.Server)
	require.NotNil(t, res.Backend)
	assert.Equal(t, project.ContextBackendBowrain, res.Backend.Kind)
	assert.Contains(t, shareText(t, res), "Managed on Bowrain · https://bowrain.example/acme/p123\n")
}
