package main

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The corpus needs one task whose correct outcome is that nothing changes.
// Without it every task rewards an edit, and nothing measures whether kapi
// provokes one that should not happen.
func cleanPairedTask(t *testing.T) PairedTask {
	t.Helper()
	tasks := pairedTasks()
	index := slices.IndexFunc(tasks, func(task PairedTask) bool { return task.ID == "confirm-refund-window" })
	require.GreaterOrEqual(t, index, 0, "the corpus must retain a task that requires no edit")
	return tasks[index]
}

func TestPairedCleanTaskPassesWhenNothingChanges(t *testing.T) {
	task := cleanPairedTask(t)
	dir := t.TempDir()
	require.NoError(t, materializePairedTask(dir, task))

	result, err := validatePairedTask(dir, task)
	require.NoError(t, err)
	assert.True(t, result.ObjectivePassed, "an untouched workspace is the correct outcome, not a failure to act")
	for _, criterion := range result.Criteria {
		assert.True(t, criterion.Passed, "criterion %q failed: %s", criterion.ID, criterion.Detail)
	}
	// Every supplied file is asserted, so no path can drift out of scoring.
	files, err := pairedTaskFiles(task)
	require.NoError(t, err)
	for name := range files {
		assert.Contains(t, criterionIDs(result), "unchanged:"+name)
	}
	assert.True(t, result.HumanReviewRequired)
	assert.NotEmpty(t, result.HumanReviewRubric)
}

func TestPairedCleanTaskFailsOnAnyEdit(t *testing.T) {
	task := cleanPairedTask(t)
	edits := map[string]struct {
		path string
		body string
	}{
		"rewrites the already-correct page": {
			path: "content/en/page.json",
			body: `{"title":"Request a refund","refund_window":"Request a refund within 7 days of the appointment.",` +
				`"cancellation":"Cancel at least 24 hours before the appointment.","support":"help@harbor.example",` +
				`"explanation":"Send the request from the account that made the booking. Requests outside the refund window are not accepted."}`,
		},
		"edits the excluded archive": {
			path: "content/en/archive.json",
			body: `{"refund_window":"Request a refund within 14 days of the appointment."}`,
		},
		"reformats without changing a value": {
			path: "content/en/page.json",
			body: `{"title": "Request a refund", "refund_window": "Request a refund within 14 days of the appointment.", ` +
				`"cancellation": "Cancel at least 24 hours before the appointment.", "support": "help@harbor.example", ` +
				`"explanation": "Send the request from the account that made the booking. Requests outside the refund window are not accepted."}`,
		},
	}
	for name, edit := range edits {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			require.NoError(t, materializePairedTask(dir, task))
			require.NoError(t, os.WriteFile(filepath.Join(dir, filepath.FromSlash(edit.path)), []byte(edit.body), 0o600))

			result, err := validatePairedTask(dir, task)
			require.NoError(t, err)
			assert.False(t, result.ObjectivePassed)
			failed := failedCriterionIDs(result)
			assert.Contains(t, failed, "unchanged:"+edit.path)
		})
	}
}

// A new file is an edit too: the scope check must catch what byte comparison
// of the supplied files cannot see.
func TestPairedCleanTaskFailsOnAnAddedContentFile(t *testing.T) {
	task := cleanPairedTask(t)
	dir := t.TempDir()
	require.NoError(t, materializePairedTask(dir, task))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "content", "en", "notes.json"), []byte(`{"note":"added"}`), 0o600))

	result, err := validatePairedTask(dir, task)
	require.NoError(t, err)
	assert.False(t, result.ObjectivePassed)
	assert.Contains(t, failedCriterionIDs(result), "content-scope")
}

func criterionIDs(result PairedValidation) []string {
	ids := make([]string, 0, len(result.Criteria))
	for _, criterion := range result.Criteria {
		ids = append(ids, criterion.ID)
	}
	return ids
}

func failedCriterionIDs(result PairedValidation) []string {
	ids := []string{}
	for _, criterion := range result.Criteria {
		if !criterion.Passed {
			ids = append(ids, criterion.ID)
		}
	}
	return ids
}
