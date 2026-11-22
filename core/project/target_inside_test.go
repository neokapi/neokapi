package project

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A recipe target that lies outside the project is refused when the recipe is
// loaded, naming the key and the target, before any run plans work it cannot
// deliver. A target inside the project loads.
func TestRecipeTargetOutsideTheProjectIsRefused(t *testing.T) {
	recipe := func(c Collection) *KapiProject {
		return &KapiProject{Version: CurrentVersion, Collections: []Collection{c}}
	}
	for _, target := range []string{"../outside/{lang}.json", "/etc/cron.d/{lang}", "locales/../../x/{lang}.json", "{dir}/../../x/{lang}.json"} {
		err := recipe(Collection{Path: "src/en.json", Target: target}).Validate()
		require.Error(t, err, target)
		assert.Contains(t, err.Error(), "collections[0].target")
		assert.Contains(t, err.Error(), target)
		assert.Contains(t, err.Error(), "Move the recipe or the target")
	}

	err := recipe(Collection{Name: "app", Content: []ContentItem{{Path: "src/en.json", Target: "../x/{lang}.json"}}}).Validate()
	require.ErrorContains(t, err, "collections[0].content[0].target")

	// The collection's base is joined onto the target first.
	err = recipe(Collection{Name: "app", Base: "../shared", Content: []ContentItem{{Path: "en.json", Target: "{lang}.json"}}}).Validate()
	require.ErrorContains(t, err, "collections[0].content[0].target")

	for _, target := range []string{"locales/{lang}.json", "src/{lang}/", "./{lang}/{dir}/{name}.json"} {
		assert.NoError(t, recipe(Collection{Path: "src/en.json", Target: target}).Validate(), target)
	}
}
