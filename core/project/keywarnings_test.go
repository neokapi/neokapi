package project

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func loadWarningRecipe(t *testing.T, body string) *KapiProject {
	t.Helper()
	path := filepath.Join(t.TempDir(), "kapi.yaml")
	require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
	p, err := Load(path)
	require.NoError(t, err, "the recipe must load; the point is that it loads and is wrong")
	return p
}

// TestKeyWarningsCatchTheRecipeThatCostASweep.
//
// This is the recipe verbatim, near-miss keys and all. It loads, validates, and
// reports one collection with no content — the only thing the validator asks
// about explicitly. Everything else about it is wrong and was silent. See #2223.
func TestKeyWarningsCatchTheRecipeThatCostASweep(t *testing.T) {
	p := loadWarningRecipe(t, `version: v1
name: northwind
defaults:
  source: en
  targets: [nb]
collections:
  - name: app
    content:
      - path: src/locales/en.json
`)
	warnings := p.KeyWarnings()
	assert.Contains(t, warnings, "defaults.source is not a known field. Did you mean source_language?")
	assert.Contains(t, warnings, "defaults.targets is not a known field. Did you mean target_languages?")
}

// TestKeyWarningsLeaveExtensionsAlone.
//
// Extras exists so a platform layer can carry its own keys through a
// framework-only round trip. A warning that fired on those would train its
// reader to ignore it, and the mechanism would be worse than no mechanism.
func TestKeyWarningsLeaveExtensionsAlone(t *testing.T) {
	p := loadWarningRecipe(t, `version: v1
name: northwind
defaults:
  source_language: en
  telemetry_endpoint: https://example.invalid
collections:
  - name: app
    review_board: platform-team
    content:
      - path: a.json
`)
	assert.Empty(t, p.KeyWarnings(), "a key that resembles nothing is an extension, not a typo")
}

// TestKeyWarningsFindATypo: a wrong letter in a field that exists.
//
// The recipe still has to load. A typo that leaves a collection with no content
// is caught by the validator with a real error, which is the outcome that never
// needed a warning; these are the ones that load and quietly do the wrong thing.
func TestKeyWarningsFindATypo(t *testing.T) {
	p := loadWarningRecipe(t, `version: v1
name: northwind
defaults:
  sourcelanguage: en
collections:
  - name: app
    bse: src/locales
    content:
      - path: a.json
`)
	warnings := p.KeyWarnings()
	require.NotEmpty(t, warnings)
	assert.Contains(t, warnings, "collections[app].bse is not a known field. Did you mean base?")
	assert.Contains(t, warnings, "defaults.sourcelanguage is not a known field. Did you mean source_language?")
}

// TestKeyWarningsAreStable: the dataset a caller prints must not reorder
// between runs, and Extras is a map.
func TestKeyWarningsAreStable(t *testing.T) {
	body := `version: v1
name: northwind
defaults:
  source: en
  targets: [nb]
  materialise: manual
collections:
  - name: app
    content:
      - path: a.json
`
	first := loadWarningRecipe(t, body).KeyWarnings()
	for range 8 {
		assert.Equal(t, first, loadWarningRecipe(t, body).KeyWarnings())
	}
}

// TestKnownFieldsComeFromTheStruct: the list is reflected, not written down, so
// a field added to Defaults is known here without anyone remembering to add it.
func TestKnownFieldsComeFromTheStruct(t *testing.T) {
	fields := knownFields(reflect.TypeFor[Defaults]())
	assert.Contains(t, fields, "source_language")
	assert.Contains(t, fields, "target_languages")
	assert.Contains(t, fields, "materialize")
	assert.NotContains(t, fields, "", "an inline or skipped field contributes no key")
}
