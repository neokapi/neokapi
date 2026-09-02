package host

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// One queue holds every language. A source unit and a translation are rows of
// the same list, and the row says which language it belongs to and whether that
// language is the project's source.

// writeUnifiedQueueProject writes a project with work waiting in three
// languages: two source units under an `approved` source gate, and two
// translated units in each of nb and fr with nothing approved.
//
// It runs under the dogfood isolation contract (CLAUDE.md): every root this run
// could otherwise inherit is pinned to a throwaway dir and project discovery is
// off, so the repo's own recipe can never be found.
func writeUnifiedQueueProject(t *testing.T, sourceGate string, targets string) string {
	t.Helper()
	root := t.TempDir()
	t.Setenv("KAPI_CONFIG_DIR", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("KAPI_PLUGINS_DIR_ONLY", "1")
	t.Setenv("KAPI_PLUGINS_DIR", t.TempDir())
	t.Setenv("KAPI_NO_PROJECT", "1")

	recipe := `version: v1
name: rev-unified
defaults:
  source_language: en
  target_languages: [` + targets + `]
  source_gate: ` + sourceGate + `
collections:
  - path: en.json
    target: "{lang}.json"
`
	require.NoError(t, os.WriteFile(filepath.Join(root, "kapi.yaml"), []byte(recipe), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "en.json"),
		[]byte(`{"a":"Apple","b":"Banana"}`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "nb.json"),
		[]byte(`{"a":"Eple","b":"Banan"}`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "fr.json"),
		[]byte(`{"a":"Pomme","b":"Banane"}`), 0o644))
	return root
}

func TestReviewQueue_ListsEveryLanguageWithASourceLane(t *testing.T) {
	root := writeUnifiedQueueProject(t, "approved", "nb, fr")
	recipe := filepath.Join(root, "kapi.yaml")

	cases := []struct {
		name      string
		languages []string
		wantLangs map[string]int // language → rows in Pending
	}{
		{
			name:      "no filter lists every language",
			wantLangs: map[string]int{"en": 2, "nb": 2, "fr": 2},
		},
		{
			name:      "the source language returns only source units",
			languages: []string{"en"},
			wantLangs: map[string]int{"en": 2},
		},
		{
			name:      "a target language returns only that target",
			languages: []string{"nb"},
			wantLangs: map[string]int{"nb": 2},
		},
		{
			name:      "several languages at once",
			languages: []string{"en", "fr"},
			wantLangs: map[string]int{"en": 2, "fr": 2},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a := &App{}
			queue, err := a.ReviewQueue(t.Context(), recipe, "en", ReviewQueueOptions{Languages: tc.languages})
			require.NoError(t, err)

			got := map[string]int{}
			for _, it := range queue.Pending {
				got[it.LanguageTag()]++
				assert.Equal(t, it.Locale, it.LanguageTag(), "language repeats locale on every row")
				if it.Language == "en" {
					assert.True(t, it.IsSource, "the source language's rows are marked")
					assert.Equal(t, "en.json", it.File, "a source row addresses the source file")
					assert.Empty(t, it.Target, "a source row has no translation half")
					assert.Equal(t, string(model.SourceStatusChecked), it.Status)
					assert.True(t, it.Held, "an approved gate holds a merely-checked unit")
				} else {
					assert.False(t, it.IsSource)
					assert.Equal(t, string(model.TargetStatusTranslated), it.Status)
					assert.NotEmpty(t, it.Target)
				}
			}
			assert.Equal(t, tc.wantLangs, got)

			// The summary answers for the whole queue however the listing is
			// filtered, so a surface narrowed to one language still offers the
			// others.
			summary := map[string]int{}
			for _, l := range queue.Languages {
				summary[l.Language] = l.Pending
				assert.Equal(t, l.Language == "en", l.Source, "%s", l.Language)
			}
			assert.Equal(t, map[string]int{"en": 2, "nb": 2, "fr": 2}, summary)

			total := 0
			for _, l := range queue.Languages {
				total += l.Pending
			}
			assert.Equal(t, 6, total, "the per-language counts add up to the whole queue")
		})
	}
}

// The source rows lead the queue: the source gate holds the fan-out, so the
// work that unblocks the rest is read first.
func TestReviewQueue_SourceUnitsSortFirst(t *testing.T) {
	root := writeUnifiedQueueProject(t, "approved", "nb")
	queue, err := (&App{}).ReviewQueue(t.Context(), filepath.Join(root, "kapi.yaml"), "en", ReviewQueueOptions{})
	require.NoError(t, err)
	require.Len(t, queue.Pending, 4)
	for i, it := range queue.Pending {
		assert.Equal(t, i < 2, it.IsSource, "row %d", i)
	}
	require.Len(t, queue.Languages, 2)
	assert.Equal(t, "en", queue.Languages[0].Language, "the source language heads the summary")
}

// Approving the source wording takes its rows out of the queue, and the
// language leaves the summary with them.
func TestReviewQueue_ApprovedSourceLeavesTheQueue(t *testing.T) {
	root := writeUnifiedQueueProject(t, "approved", "nb")
	recipe := filepath.Join(root, "kapi.yaml")
	a := &App{}

	for _, key := range []string{"a", "b"} {
		changed, err := a.ApproveSourceUnit(t.Context(), recipe, "en", SourceUnitRef{File: "en.json", Key: key})
		require.NoError(t, err)
		require.True(t, changed)
	}

	queue, err := a.ReviewQueue(t.Context(), recipe, "en", ReviewQueueOptions{})
	require.NoError(t, err)
	assert.Len(t, queue.Pending, 2, "only the two nb translations are left")
	require.Len(t, queue.Languages, 1)
	assert.Equal(t, "nb", queue.Languages[0].Language)
	assert.False(t, queue.Languages[0].Source)
}

// A project with nothing waiting returns an empty listing and an empty summary,
// rather than a summary of languages with no work in them.
func TestReviewQueue_EmptyWhenNothingIsPending(t *testing.T) {
	root := writeUnifiedQueueProject(t, "checked", "nb")
	// Drop the translations: an absent target is upstream of review, and the
	// default `checked` gate asks nobody to sign off a clean source.
	require.NoError(t, os.Remove(filepath.Join(root, "nb.json")))
	require.NoError(t, os.Remove(filepath.Join(root, "fr.json")))

	queue, err := (&App{}).ReviewQueue(t.Context(), filepath.Join(root, "kapi.yaml"), "en", ReviewQueueOptions{})
	require.NoError(t, err)
	assert.Empty(t, queue.Pending)
	assert.Empty(t, queue.Languages)
}

// review_unit and the desktop read one unit through ReviewUnitWithContext. A
// source-language unit is answered from its source file, with the authoring
// rung and the point that governs it.
func TestReviewUnit_AnswersASourceLanguageUnit(t *testing.T) {
	root := writeUnifiedQueueProject(t, "approved", "nb")
	recipe := filepath.Join(root, "kapi.yaml")
	a := &App{}

	info, err := a.ReviewUnitWithContext(t.Context(), recipe, "en", ReviewUnitRef{
		File: "en.json", Key: "a", Locale: "en",
	})
	require.NoError(t, err)
	assert.True(t, info.IsSource)
	assert.Equal(t, "en", info.Language)
	assert.Equal(t, "Apple", info.Source)
	assert.Empty(t, info.Target, "a source unit has no translation half")
	assert.Equal(t, string(model.SourceStatusChecked), info.Status)
	require.NotNil(t, info.Context)
	assert.Equal(t, "en.json", info.Context.Point.Path)
	assert.Equal(t, "en", info.Context.Point.Language)
	assert.True(t, info.Context.Point.IsSource)
	assert.Equal(t, "a", info.Context.Neighbourhood.Key)

	// The approval is readable back through the same call.
	_, err = a.ApproveSourceUnit(t.Context(), recipe, "en", SourceUnitRef{File: "en.json", Key: "a"})
	require.NoError(t, err)
	after, err := a.ReviewUnitWithContext(t.Context(), recipe, "en", ReviewUnitRef{
		File: "en.json", Key: "a", Locale: "en",
	})
	require.NoError(t, err)
	assert.Equal(t, string(model.SourceStatusApproved), after.Status)
	assert.Equal(t, "approved", after.ReviewState)
}

// runReviewStatus drives `kapi status --review` against one recipe, with the
// project bound explicitly so the run never walks up to a discovered project.
func runReviewStatus(t *testing.T, recipe string, flags map[string]string, langs []string) string {
	t.Helper()
	a := &App{}
	cmd := NewEnvCommand(t.Context(), "status")
	AddProjectFlag(cmd)
	AddStatusFlags(cmd)
	require.NoError(t, cmd.Flags().Set("project", recipe))
	require.NoError(t, cmd.Flags().Set("review", "true"))
	for k, v := range flags {
		require.NoError(t, cmd.Flags().Set(k, v))
	}
	for _, l := range langs {
		require.NoError(t, cmd.Flags().Set("lang", l))
	}
	out, err := captureStdout(t, func() error { return a.RunStatus(cmd, nil) })
	require.NoError(t, err)
	return out
}

// `kapi status --review` is the CLI's window on the one queue: source rows sit
// beside the translations, marked, and --lang narrows the listing.
func TestStatusReview_ListsSourceUnitsAndFiltersByLanguage(t *testing.T) {
	root := writeUnifiedQueueProject(t, "approved", "nb")
	recipe := filepath.Join(root, "kapi.yaml")

	var all reviewQueueOutput
	require.NoError(t, json.Unmarshal([]byte(runReviewStatus(t, recipe, map[string]string{"json": "true"}, nil)), &all))
	require.Len(t, all.Pending, 4)
	assert.Equal(t, []ReviewLanguage{
		{Language: "en", Pending: 2, Source: true},
		{Language: "nb", Pending: 2},
	}, all.Languages)

	var nbOnly reviewQueueOutput
	require.NoError(t, json.Unmarshal([]byte(runReviewStatus(t, recipe, map[string]string{"json": "true"}, []string{"nb"})), &nbOnly))
	require.Len(t, nbOnly.Pending, 2)
	for _, it := range nbOnly.Pending {
		assert.Equal(t, "nb", it.Language)
		assert.False(t, it.IsSource)
	}

	var enOnly reviewQueueOutput
	require.NoError(t, json.Unmarshal([]byte(runReviewStatus(t, recipe, map[string]string{"json": "true"}, []string{"en"})), &enOnly))
	require.Len(t, enOnly.Pending, 2)
	for _, it := range enOnly.Pending {
		assert.True(t, it.IsSource)
	}

	text := runReviewStatus(t, recipe, nil, nil)
	assert.Contains(t, text, "en · source", "the source language is marked in the table, as a tag")
	assert.Contains(t, text, "kapi apply", "the approval instruction stays")
	assert.Contains(t, text, "approve source wording in the Review page of Kapi Desktop",
		"the CLI records no source decision, and says so rather than naming a command that does not exist")
	assert.Contains(t, text, "rank below the project's source gate", "a held source unit says why the loop is waiting")
	// The table draws an em dash for an empty cell; the prose beside it carries
	// none.
	for line := range strings.SplitSeq(text, "\n") {
		if strings.HasPrefix(line, "Approve ") || strings.HasPrefix(line, "Units marked ") {
			assert.NotContains(t, line, "—", "CLI prose carries no em dash")
		}
	}
}

// A target unit keeps answering as it did, with the language fields filled in.
func TestReviewUnit_TargetUnitCarriesItsLanguage(t *testing.T) {
	root := writeUnifiedQueueProject(t, "checked", "nb")
	info, err := (&App{}).ReviewUnitWithContext(t.Context(), filepath.Join(root, "kapi.yaml"), "en",
		ReviewUnitRef{File: "nb.json", Key: "a", Locale: "nb"})
	require.NoError(t, err)
	assert.False(t, info.IsSource)
	assert.Equal(t, "nb", info.Language)
	assert.Equal(t, "Eple", info.Target)
	require.NotNil(t, info.Context)
	assert.Equal(t, "nb", info.Context.Point.Language)
	assert.False(t, info.Context.Point.IsSource)
}
