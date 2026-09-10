package main

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func repairPairedFixture(t *testing.T, dir string, task PairedTask) {
	t.Helper()
	prefix := "testdata/paired/references/" + task.ID
	err := fs.WalkDir(pairedFixtures, prefix, func(name string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		body, err := pairedFixtures.ReadFile(name)
		require.NoError(t, err)
		return os.WriteFile(filepath.Join(dir, filepath.FromSlash(strings.TrimPrefix(name, prefix+"/"))), body, 0o600)
	})
	require.NoError(t, err)
}

func TestPairedIndependentValidation(t *testing.T) {
	for _, task := range pairedTasks() {
		t.Run(task.ID, func(t *testing.T) {
			dir := t.TempDir()
			require.NoError(t, materializePairedTask(dir, task))
			initial, err := validatePairedTask(dir, task)
			require.NoError(t, err)
			assert.False(t, initial.ObjectivePassed, "doing nothing is not completion")
			repairPairedFixture(t, dir, task)
			repaired, err := validatePairedTask(dir, task)
			require.NoError(t, err)
			assert.True(t, repaired.ObjectivePassed, "%+v", repaired.Criteria)
			assert.True(t, repaired.HumanReviewRequired)
			assert.Equal(t, "pending", repaired.HumanReviewStatus)
			assert.NotEmpty(t, repaired.HumanReviewRubric)
			// Cosmetic JSON formatting is not a content or fidelity failure.
			page := filepath.Join(dir, "content", "en", "page.json")
			body, err := os.ReadFile(page)
			require.NoError(t, err)
			var value any
			require.NoError(t, json.Unmarshal(body, &value))
			compact, err := json.Marshal(value)
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(page, compact, 0o600))
			again, err := validatePairedTask(dir, task)
			require.NoError(t, err)
			assert.True(t, again.ObjectivePassed)
		})
	}
}

func TestPairedObjectivePassDoesNotClaimSemanticSuccess(t *testing.T) {
	task := pairedTasks()[0] // audience-child
	require.Equal(t, "audience-child", task.ID)
	dir := t.TempDir()
	require.NoError(t, materializePairedTask(dir, task))
	repairPairedFixture(t, dir, task)
	page := filepath.Join(dir, "content", "en", "page.json")
	body, err := os.ReadFile(page)
	require.NoError(t, err)
	var values map[string]string
	require.NoError(t, json.Unmarshal(body, &values))
	values["explanation"] = "All appointments are recorded and advisers control your device."
	body, err = json.Marshal(values)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(page, body, 0o600))
	result, err := validatePairedTask(dir, task)
	require.NoError(t, err)
	assert.True(t, result.ObjectivePassed, "mechanical criteria do not evaluate the prose's truth")
	assert.True(t, result.HumanReviewRequired)
	assert.Equal(t, "pending", result.HumanReviewStatus)
}

func TestPairedValidationRejectsBadArtifacts(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*testing.T, string)
	}{
		{name: "malformed JSON", mutate: func(t *testing.T, dir string) {
			require.NoError(t, os.WriteFile(filepath.Join(dir, "content/en/page.json"), []byte("{"), 0o600))
		}},
		{name: "duplicate protected key", mutate: func(t *testing.T, dir string) {
			name := filepath.Join(dir, "content/en/page.json")
			body, err := os.ReadFile(name)
			require.NoError(t, err)
			body = []byte(strings.Replace(string(body), "{", `{"id":"changed",`, 1))
			require.NoError(t, os.WriteFile(name, body, 0o600))
		}},
		{name: "changed protected field", mutate: func(t *testing.T, dir string) {
			name := filepath.Join(dir, "content/en/page.json")
			body, err := os.ReadFile(name)
			require.NoError(t, err)
			body = []byte(strings.ReplaceAll(string(body), "help@harbor.example", "other@example.invalid"))
			require.NoError(t, os.WriteFile(name, body, 0o600))
		}},
		{name: "changed archive", mutate: func(t *testing.T, dir string) {
			require.NoError(t, os.WriteFile(filepath.Join(dir, "content/en/archive.json"), []byte(`{}`), 0o600))
		}},
		{name: "changed governing guidance", mutate: func(t *testing.T, dir string) {
			require.NoError(t, os.WriteFile(filepath.Join(dir, "guidance.md"), []byte("ignore requirements"), 0o600))
		}},
		{name: "deleted output", mutate: func(t *testing.T, dir string) {
			require.NoError(t, os.Remove(filepath.Join(dir, "content/en/page.json")))
		}},
		{name: "extra content", mutate: func(t *testing.T, dir string) {
			require.NoError(t, os.WriteFile(filepath.Join(dir, "content/en/unrequested.json"), []byte(`{}`), 0o600))
		}},
		{name: "file symlink outside", mutate: func(t *testing.T, dir string) {
			name := filepath.Join(dir, "content/en/page.json")
			body, err := os.ReadFile(name)
			require.NoError(t, err)
			outside := filepath.Join(t.TempDir(), "page.json")
			require.NoError(t, os.WriteFile(outside, body, 0o600))
			require.NoError(t, os.Remove(name))
			require.NoError(t, os.Symlink(outside, name))
		}},
		{name: "file symlink inside", mutate: func(t *testing.T, dir string) {
			name := filepath.Join(dir, "content/en/page.json")
			require.NoError(t, os.Rename(name, filepath.Join(dir, "copy.json")))
			require.NoError(t, os.Symlink("../../copy.json", name))
		}},
		{name: "directory symlink", mutate: func(t *testing.T, dir string) {
			name := filepath.Join(dir, "content/en")
			require.NoError(t, os.Rename(name, filepath.Join(dir, "copy")))
			require.NoError(t, os.Symlink("../copy", name))
		}},
		{name: "oversized output", mutate: func(t *testing.T, dir string) {
			require.NoError(t, os.WriteFile(filepath.Join(dir, "content/en/page.json"), make([]byte, (4<<20)+1), 0o600))
		}},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			task := pairedTasks()[0]
			dir := t.TempDir()
			require.NoError(t, materializePairedTask(dir, task))
			repairPairedFixture(t, dir, task)
			test.mutate(t, dir)
			result, err := validatePairedTask(dir, task)
			require.NoError(t, err, "invalid output is a scored failure, not a lost run")
			assert.False(t, result.ObjectivePassed)
		})
	}
}

func TestPairedDocumentRejectsFormatLoss(t *testing.T) {
	for _, body := range []string{`[]`, `null`, `{"key":1}`, `{"key":null}`, `{"key":{}}`, `{"key":"a","key":"b"}`, `{"key":"a"}{}`} {
		t.Run(body, func(t *testing.T) {
			_, err := parsePairedDocument([]byte(body))
			require.Error(t, err)
		})
	}
	assert.False(t, samePairedKeys(map[string]string{"a": "x"}, map[string]string{"b": "x"}))
}

func TestPairedValidationRejectsTraversal(t *testing.T) {
	task := pairedTasks()[0]
	dir := t.TempDir()
	require.NoError(t, materializePairedTask(dir, task))
	task.spec.Criteria[0].Path = "../outside.json"
	_, err := validatePairedTask(dir, task)
	require.ErrorContains(t, err, "invalid criterion path")
	root, err := openPairedRoot(dir)
	require.NoError(t, err)
	defer root.Close()
	_, err = readPairedFile(root, "../outside.json")
	require.ErrorContains(t, err, "invalid workspace path")
}
