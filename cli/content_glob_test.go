package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// #1468 item 2 does not live only in project.ResolveContent: `kapi ls` and
// `kapi content add` carry their own ExpandGlob calls, and both discarded the
// error. `ls` is the listing a user checks to confirm the recipe tracks what
// they think it does, and `add` is where a malformed pattern gets written into
// the recipe in the first place.

func writeGlobRecipe(t *testing.T, yaml string) string {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	recipe := filepath.Join(dir, "kapi.yaml")
	require.NoError(t, os.WriteFile(recipe, []byte(yaml), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "docs"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "store"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "docs", "guide.json"), []byte(`{"g":"Guide"}`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "store", "ui.json"), []byte(`{"a":"A"}`), 0o644))
	return recipe
}

func runLs(t *testing.T, a *App, recipe string, args ...string) (string, error) {
	t.Helper()
	cmd := NewLsCmd(a)
	cmd.SetArgs(append(args, "--project", recipe))
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	execErr := cmd.Execute()
	return out.String(), execErr
}

// TestLs_MalformedPatternFailsNamingTheCollection is the regression. What a user
// saw: `kapi ls` printing "1 file(s)" for a recipe that declares two
// collections, exit 0 — the broken pattern invisible.
func TestLs_MalformedPatternFailsNamingTheCollection(t *testing.T) {
	a := processOnlyApp(t)
	recipe := writeGlobRecipe(t, `version: v1
name: LsGlob
defaults:
  source_language: en-US
collections:
  - name: Docs
    content:
      - path: docs/*.json
        format:
          name: json
  - name: Store
    content:
      - path: "store/[a-.json"
`)
	_, err := runLs(t, a, recipe)
	require.Error(t, err, "a pattern that cannot be expanded must fail the listing, not shorten it")
	assert.Contains(t, err.Error(), `"Store"`, "the error must name the collection")
	assert.Contains(t, err.Error(), "store/[a-.json", "and the pattern to fix")
}

// TestLs_PatternMatchingNothingStillLists is the discriminating control: an
// empty collection is not a failed collection, so a well-formed pattern that
// matches nothing yet must still list the rest with exit 0.
func TestLs_PatternMatchingNothingStillLists(t *testing.T) {
	a := processOnlyApp(t)
	recipe := writeGlobRecipe(t, `version: v1
name: LsGlob
defaults:
  source_language: en-US
collections:
  - name: Docs
    content:
      - path: docs/*.json
        format:
          name: json
  - name: NotWrittenYet
    content:
      - path: locales/**/*.json
`)
	out, err := runLs(t, a, recipe)
	require.NoError(t, err, "an empty collection must not fail the listing")
	assert.Contains(t, out, "docs/guide.json")
	assert.Contains(t, out, "1 file(s)")
}

// TestAdd_RefusesAMalformedPattern stops the broken recipe being authored at
// all: the count used to be `matches, _ :=`, so `kapi content add` reported
// "0 files" — indistinguishable from "nothing matches yet" — and wrote the
// unusable pattern into the recipe.
func TestAdd_RefusesAMalformedPattern(t *testing.T) {
	a := processOnlyApp(t)
	recipe := writeGlobRecipe(t, `version: v1
name: AddGlob
defaults:
  source_language: en-US
`)
	cmd := NewAddCmd(a)
	cmd.SetArgs([]string{"store/[a-.json", "--project", recipe})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	err := cmd.Execute()
	require.Error(t, err, "a pattern that can never expand must not be written into the recipe")
	assert.Contains(t, err.Error(), "store/[a-.json")

	after, rerr := os.ReadFile(recipe)
	require.NoError(t, rerr)
	assert.NotContains(t, string(after), "store/[a-.json", "and the recipe must be left alone")
}

// TestAdd_AcceptsAPatternThatMatchesNothingYet is the control: adding content
// before it exists is ordinary (that is how a project is set up), so a
// well-formed pattern with no matches must still be added.
func TestAdd_AcceptsAPatternThatMatchesNothingYet(t *testing.T) {
	a := processOnlyApp(t)
	recipe := writeGlobRecipe(t, `version: v1
name: AddGlob
defaults:
  source_language: en-US
`)
	cmd := NewAddCmd(a)
	cmd.SetArgs([]string{"locales/**/*.json", "--project", recipe})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	require.NoError(t, cmd.Execute())

	after, err := os.ReadFile(recipe)
	require.NoError(t, err)
	assert.Contains(t, string(after), "locales/**/*.json")
}
