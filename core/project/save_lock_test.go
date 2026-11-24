package project

import (
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/model"
)

// saveLockRecipe writes a recipe to a temporary directory and returns its
// path.
func saveLockRecipe(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), RecipeFileName)
	require.NoError(t, Save(path, &KapiProject{Name: "app", Defaults: Defaults{SourceLanguage: "en"}}))
	return path
}

// TestSave_ASecondSaveOfAStaleRecipeIsRefused pins that two saves of one
// recipe, each made from a project loaded before either saved, never lose
// an edit without a word: the first lands, and the second is refused and
// writes nothing. Loading again and repeating the change then lands both.
func TestSave_ASecondSaveOfAStaleRecipeIsRefused(t *testing.T) {
	path := saveLockRecipe(t)
	first, err := Load(path)
	require.NoError(t, err)
	second, err := Load(path)
	require.NoError(t, err)

	first.Defaults.TargetLanguages = []model.LocaleID{"de"}
	require.NoError(t, Save(path, first))
	second.Name = "renamed"
	err = Save(path, second)
	require.ErrorIs(t, err, ErrRecipeChanged)
	assert.Contains(t, err.Error(), path, "the refusal names the recipe")

	now, err := Load(path)
	require.NoError(t, err)
	assert.Equal(t, "app", now.Name, "the refused save wrote nothing")
	assert.Equal(t, []model.LocaleID{"de"}, now.Defaults.TargetLanguages, "the first save landed")

	now.Name = "renamed"
	require.NoError(t, Save(path, now), "a save from the recipe as it now stands lands")
	again, err := Load(path)
	require.NoError(t, err)
	assert.Equal(t, "renamed", again.Name)
	assert.Equal(t, []model.LocaleID{"de"}, again.Defaults.TargetLanguages)

	require.NoError(t, Save(path, now), "a project that saved can save again")
}

// TestSave_TwoConcurrentSavesBothLandOrOneIsRefused races two saves of one
// recipe, each from its own load, many times. Each time exactly one lands
// and the other is refused, and the file holds the edit that landed.
func TestSave_TwoConcurrentSavesBothLandOrOneIsRefused(t *testing.T) {
	path := saveLockRecipe(t)
	for round := range 50 {
		a, err := Load(path)
		require.NoError(t, err)
		b, err := Load(path)
		require.NoError(t, err)
		a.Defaults.TargetLanguages = []model.LocaleID{"de"}
		b.Defaults.TargetLanguages = []model.LocaleID{"fr"}

		var wg sync.WaitGroup
		errs := make([]error, 2)
		for i, p := range []*KapiProject{a, b} {
			wg.Go(func() { errs[i] = Save(path, p) })
		}
		wg.Wait()

		landed := -1
		for i, err := range errs {
			if err == nil {
				require.Equal(t, -1, landed, "round %d: both saves landed from one basis, so one overwrote the other", round)
				landed = i
				continue
			}
			require.ErrorIs(t, err, ErrRecipeChanged, "round %d", round)
		}
		require.NotEqual(t, -1, landed, "round %d: neither save landed", round)
		now, err := Load(path)
		require.NoError(t, err)
		want := []model.LocaleID{"de", "fr"}[landed]
		require.Equal(t, []model.LocaleID{want}, now.Defaults.TargetLanguages, "round %d: the file holds the save that landed", round)

		// Reset for the next round from the file as it stands.
		now.Defaults.TargetLanguages = nil
		require.NoError(t, Save(path, now))
	}
	_, err := os.Stat(path + ".tmp")
	assert.ErrorIs(t, err, os.ErrNotExist, "no staged file is left beside the recipe")
}
