package host

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/check"
)

// projectCheckOf runs ComputeProjectCheck over the one source file of a
// writeDoNotTranslateProject project, with its French translation.
func projectCheckOf(t *testing.T, root string) ProjectCheck {
	t.Helper()
	a := &App{}
	run, err := a.ComputeProjectCheck(t.Context(), ProjectCheckRequest{
		Recipe: filepath.Join(root, "kapi.yaml"),
		Files: []ProjectCheckFileRef{{
			Path:    filepath.Join(root, "en", "app.json"),
			Targets: []ProjectCheckTargetRef{{Locale: "fr", Path: filepath.Join(root, "fr", "app.json")}},
		}},
	})
	require.NoError(t, err)
	return run
}

// A project check holds a translation to the terms in force for its language,
// so a terms-store concept marked do-not-translate fails a translation that
// renders it, and the finding sits on the translation it was raised on.
func TestComputeProjectCheckHoldsTranslationsToTheirTerms(t *testing.T) {
	t.Run("a translated protected term fails", func(t *testing.T) {
		root := writeDoNotTranslateProject(t, `{"open": "Open kapi to begin."}`, `{"open": "Ouvrez capi pour commencer."}`)
		run := projectCheckOf(t, root)
		assert.Equal(t, check.VerdictFailed, run.Report.Verdict)
		require.Len(t, run.Files, 1)
		require.Len(t, run.Files[0].Targets, 1)
		target := run.Files[0].Targets[0]
		assert.Equal(t, "fr", target.Locale)
		require.NotEmpty(t, target.Findings)
		assert.Equal(t, "terms", target.Findings[0].Check)
		assert.Contains(t, target.Findings[0].Message, `do-not-translate term "kapi"`)
		assert.NotEmpty(t, target.Blocks, "the blocks the translation was checked over")
		assert.Empty(t, run.Files[0].Findings, "the source is clean")
	})

	t.Run("a verbatim protected term passes", func(t *testing.T) {
		root := writeDoNotTranslateProject(t, `{"open": "Open kapi to begin."}`, `{"open": "Ouvrez kapi pour commencer."}`)
		run := projectCheckOf(t, root)
		assert.Equal(t, check.VerdictPassed, run.Report.Verdict)
		require.Len(t, run.Files, 1)
		require.Len(t, run.Files[0].Targets, 1)
		assert.Empty(t, run.Files[0].Targets[0].Findings)
	})

	t.Run("a missing translation is pending work", func(t *testing.T) {
		root := writeDoNotTranslateProject(t, `{"open": "Open kapi to begin."}`, `{}`)
		a := &App{}
		run, err := a.ComputeProjectCheck(t.Context(), ProjectCheckRequest{
			Recipe: filepath.Join(root, "kapi.yaml"),
			Files: []ProjectCheckFileRef{{
				Path:    filepath.Join(root, "en", "app.json"),
				Targets: []ProjectCheckTargetRef{{Locale: "de", Path: filepath.Join(root, "de", "app.json")}},
			}},
		})
		require.NoError(t, err)
		require.Len(t, run.Files, 1)
		assert.Empty(t, run.Files[0].Targets, "no file, nothing translated yet")
	})
}

// A review queue listing asked to mark findings marks each translation row by
// the checkset a reviewer is shown for the unit (UnitFindings).
func TestReviewQueueMarksFindings(t *testing.T) {
	root := writeDoNotTranslateProject(t,
		`{"open": "Open kapi to begin.", "close": "Close the window."}`,
		`{"open": "Ouvrez capi pour commencer.", "close": "Fermez la fenêtre."}`)
	a := &App{}
	recipe := filepath.Join(root, "kapi.yaml")

	unmarked, err := a.ReviewQueue(t.Context(), recipe, "", ReviewQueueOptions{Languages: []string{"fr"}})
	require.NoError(t, err)
	for _, it := range unmarked.Pending {
		assert.Nil(t, it.HasFindings, "a listing that asks for no marks carries none")
	}

	queue, err := a.ReviewQueue(t.Context(), recipe, "", ReviewQueueOptions{Languages: []string{"fr"}, MarkFindings: true})
	require.NoError(t, err)
	marks := map[string]bool{}
	for _, it := range queue.Pending {
		require.NotNil(t, it.HasFindings, "%s carries the marker", it.Key)
		marks[it.Key] = *it.HasFindings
	}
	assert.True(t, marks["open"], "the protected term was translated")
	assert.False(t, marks["close"], "a clean translation")
}
