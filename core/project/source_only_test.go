package project

import (
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func sourceOnlyProject(c Collection) *KapiProject {
	return &KapiProject{Version: CurrentVersion, Collections: []Collection{c}}
}

// A collection with no target and a collection whose target was forgotten are
// the same recipe. source_only is what separates them, so the flag has to be
// checked: an unchecked one drifts away from the items under it and becomes a
// second, wrong answer to the question the items already answer.
func TestSourceOnlyAcceptsACollectionWithNoTarget(t *testing.T) {
	p := sourceOnlyProject(Collection{
		Name:       "desktop-cask",
		SourceOnly: true,
		Content:    []ContentItem{{Path: "*.rb"}},
	})
	require.NoError(t, p.Validate())
}

func TestSourceOnlyRejectsAnItemTarget(t *testing.T) {
	p := sourceOnlyProject(Collection{
		Name:       "desktop-cask",
		SourceOnly: true,
		Content:    []ContentItem{{Path: "*.rb", Target: "out/{lang}.rb"}},
	})
	err := p.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "source_only")
	assert.Contains(t, err.Error(), "content[0]")
	assert.Contains(t, err.Error(), "out/{lang}.rb", "the error names the target it found")
}

func TestSourceOnlyRejectsCollectionTargetLanguages(t *testing.T) {
	p := sourceOnlyProject(Collection{
		Name:            "desktop-cask",
		SourceOnly:      true,
		TargetLanguages: []model.LocaleID{"nb"},
		Content:         []ContentItem{{Path: "*.rb"}},
	})
	err := p.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "target_languages must be empty")
}

// Both spellings of a target are covered, because either alone would leave a
// way to declare the contradiction.
func TestSourceOnlyRejectsItemTargetLanguages(t *testing.T) {
	p := sourceOnlyProject(Collection{
		Name:       "desktop-cask",
		SourceOnly: true,
		Content:    []ContentItem{{Path: "*.rb", TargetLanguages: []model.LocaleID{"nb"}}},
	})
	err := p.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "content[0]")
}

// A bare entry promotes its target to the collection, so it needs the check on
// the promoted field rather than on Content.
func TestSourceOnlyRejectsABareEntryTarget(t *testing.T) {
	p := sourceOnlyProject(Collection{
		SourceOnly: true,
		Path:       "packaging/nfpm.yaml",
		Target:     "out/{lang}.yaml",
	})
	err := p.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot have a target")
}

// Nothing changes for a collection that does not set the flag: a target is
// still ordinary, and this is not a new way to fail an existing recipe.
func TestWithoutTheFlagATargetIsStillOrdinary(t *testing.T) {
	p := sourceOnlyProject(Collection{
		Name:            "docs",
		TargetLanguages: []model.LocaleID{"nb"},
		Content:         []ContentItem{{Path: "*.md", Target: "i18n/{lang}/{name}.md"}},
	})
	require.NoError(t, p.Validate())
}

// The message has to be findable in a recipe with many collections, so it
// carries the index and the name.
func TestSourceOnlyErrorLocatesTheCollection(t *testing.T) {
	p := &KapiProject{
		Version: CurrentVersion,
		Collections: []Collection{
			{Name: "docs", Content: []ContentItem{{Path: "a.md"}}},
			{Name: "desktop-cask", SourceOnly: true, Content: []ContentItem{{Path: "*.rb", Target: "x"}}},
		},
	}
	err := p.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "collections[1]")
	assert.Contains(t, err.Error(), `"desktop-cask"`)
}
