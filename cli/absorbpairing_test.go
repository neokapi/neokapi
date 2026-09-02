package cli

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/host"
)

// What the record absorber pairs a committed target with decides what the loop
// believes the project already says, because the plan and the pass read the
// project content memory and not the target files. Two ways of getting that
// wrong are held shut here, both whole-verb through `kapi up`:
//
//   - A source rewrite mispairs EVERY locale of the unit, not only the one that
//     holds a decision. The undecided locales went on serving the translation of
//     a deleted sentence, and the loop confirmed them as caught up.
//   - A translation legitimately identical to its source was dropped, so the
//     loop re-drafted it on every pass and overwrote the approval bound to it.

// writeThreeLocaleProject writes a project with three target locales, one unit
// per locale carrying a real translation and one carrying a product name that is
// the same word in all four languages.
func writeThreeLocaleProject(t *testing.T) string {
	t.Helper()
	// Dogfood isolation contract (CLAUDE.md): the repo root holds a live recipe
	// that an upward walk would otherwise find and act on.
	t.Setenv("KAPI_NO_PROJECT", "1")
	t.Setenv("KAPI_CONFIG_DIR", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("KAPI_PLUGINS_DIR_ONLY", "1")
	t.Setenv("KAPI_PLUGINS_DIR", t.TempDir())

	root := t.TempDir()
	recipe := `version: v1
name: three
defaults:
  source_language: en
  target_languages: [nb, de, nl]
collections:
  - path: en.json
    target: "{lang}.json"
ship_gate: { translated: 100, reviewed: 50 }
`
	require.NoError(t, os.WriteFile(filepath.Join(root, "kapi.yaml"), []byte(recipe), 0o644))
	write := func(name, body string) {
		require.NoError(t, os.WriteFile(filepath.Join(root, name), []byte(body), 0o644))
	}
	write("en.json", `{"a":"Apple","b":"Compass"}`)
	write("nb.json", `{"a":"Eple","b":"Compass"}`)
	write("de.json", `{"a":"Apfel","b":"Compass"}`)
	write("nl.json", `{"a":"Appel","b":"Compass"}`)
	return root
}

// localeTargets reads one materialized catalog.
func localeTargets(t *testing.T, root, locale string) map[string]string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, locale+".json"))
	require.NoError(t, err)
	var out map[string]string
	require.NoError(t, json.Unmarshal(data, &out))
	return out
}

// approveUnit approves (locale, source text) through the real review path, so
// the decision binds both halves of the pairing a live approval would.
func approveUnit(t *testing.T, root, locale, srcText string) {
	t.Helper()
	a := &App{}
	defer a.Shutdown()
	proj := filepath.Join(root, "kapi.yaml")
	rep, err := a.ProjectConvergence(context.Background(), proj, "en")
	require.NoError(t, err)
	for _, it := range rep.Review {
		if it.Locale != locale || it.Source != srcText {
			continue
		}
		ok, aerr := a.ApproveReviewUnit(context.Background(), proj, "en", locale, it.File, it.Key, "reviewed")
		require.NoError(t, aerr)
		require.True(t, ok)
		return
	}
	t.Fatalf("no %s review unit with source %q", locale, srcText)
}

// TestAbsorbPairing_RewrittenSourceIsWorkInEveryLocale is the whole verb for the
// defect: one locale holds a decision and two hold none, and a source rewrite
// must take the translation of the deleted sentence away from all three.
func TestAbsorbPairing_RewrittenSourceIsWorkInEveryLocale(t *testing.T) {
	root := writeThreeLocaleProject(t)
	proj := filepath.Join(root, "kapi.yaml")

	// A converged run first: it absorbs the committed catalogs, so the corpus
	// holds what each of them says and a second run recycles rather than drafts.
	first := runReviewUp(t, proj)
	assert.Zero(t, first.RedraftedUnits())
	for _, loc := range []string{"nb", "de", "nl"} {
		assert.Equal(t, localeApple[loc], localeTargets(t, root, loc)["a"],
			"a converged pass must not overwrite the wording it recycled from the record")
	}

	// Only de is reviewed. nb and nl have nothing on record for this unit.
	approveUnit(t, root, "de", "Apple")

	require.NoError(t, os.WriteFile(filepath.Join(root, "en.json"),
		[]byte(`{"a":"Apricot","b":"Compass"}`), 0o644))

	out := runReviewUp(t, proj)
	assert.Equal(t, 3, out.RedraftedUnits(),
		"the loop records what it translated in every locale, so the rewrite is drift in all three")
	assert.Equal(t, 1, out.StaleUnits(),
		"the re-draft settles the two undecided locales; de waits on a person to review the new pairing")

	for _, loc := range []string{"nb", "de", "nl"} {
		after := localeTargets(t, root, loc)
		assert.NotEqual(t, localeApple[loc], after["a"],
			"%s must not go on serving the translation of a sentence the project deleted", loc)
		assert.NotEmpty(t, after["a"], "%s is drafted against the source the project has now", loc)
	}

	// The undecided locales are not silently confirmed: they carry a fresh draft
	// of the current source, and the corpus answers that source rather than
	// claiming the old translation belongs to it.
	b := &App{}
	defer b.Shutdown()
	rep, rerr := b.ProjectConvergence(context.Background(), proj, "en")
	require.NoError(t, rerr)
	for _, loc := range []string{"nb", "de", "nl"} {
		found := false
		for _, it := range rep.Review {
			if it.Locale == loc && it.Source == "Apricot" {
				found = true
				assert.Equal(t, localeTargets(t, root, loc)["a"], it.Target)
			}
		}
		assert.True(t, found, "%s's rewritten unit is in the queue for a person to judge", loc)
	}
}

// localeApple is each locale's committed translation of "Apple" in the fixture.
var localeApple = map[string]string{"nb": "Eple", "de": "Apfel", "nl": "Appel"}

// TestAbsorbPairing_ApprovedIdenticalTranslationSurvives: a product name is the
// same word in every language. Approved, it is the project's own decision, and a
// pass that re-drafts it both mangles a correct string and discards the approval
// the unit ledger exists to keep.
func TestAbsorbPairing_ApprovedIdenticalTranslationSurvives(t *testing.T) {
	root := writeThreeLocaleProject(t)
	proj := filepath.Join(root, "kapi.yaml")

	// Approved before the first pass, which is where a committed record comes
	// from: the catalog and the decision arrive together in git.
	approveUnit(t, root, "nb", "Compass")

	first := runReviewUp(t, proj)
	require.NotNil(t, first)
	assert.Equal(t, "Compass", localeTargets(t, root, "nb")["b"],
		"the approved wording is recycled, so the AI step never sees the unit")

	// And it stays. A second and third pass reproduce it rather than drafting
	// over the approval each time.
	runReviewUp(t, proj)
	runReviewUp(t, proj)
	assert.Equal(t, "Compass", localeTargets(t, root, "nb")["b"])

	a := &App{}
	defer a.Shutdown()
	rep, err := a.ProjectConvergence(context.Background(), proj, "en")
	require.NoError(t, err)
	for _, it := range rep.Review {
		assert.False(t, it.Locale == "nb" && it.Source == "Compass",
			"the approval still describes the translation on disk, so the unit is decided")
	}

	unit, uerr := a.ReviewUnit(context.Background(), proj, "en", host.ReviewUnitRef{
		File: "nb.json", Key: "b", Locale: "nb",
	})
	require.NoError(t, uerr)
	assert.Equal(t, "approved", unit.ReviewState, "the decision is intact, not re-earned")

	// The locales that decided nothing keep the drop: an identical leaf with no
	// decision behind it is far more often a catalog carrying its untranslated
	// entries verbatim than a translation that happens to coincide.
	assert.NotEqual(t, "Compass", localeTargets(t, root, "de")["b"])
	assert.NotEqual(t, "Compass", localeTargets(t, root, "nl")["b"])
}
