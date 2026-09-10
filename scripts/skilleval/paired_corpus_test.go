package main

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/neokapi/neokapi/core/profile"
	"github.com/neokapi/neokapi/core/project"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPairedCorpusMaterialization(t *testing.T) {
	tasks := pairedTasks()
	require.Len(t, tasks, 7)
	families := map[string]int{}
	seen := map[string]bool{}
	for _, task := range tasks {
		t.Run(task.ID, func(t *testing.T) {
			require.False(t, seen[task.ID], "task IDs must be unique")
			seen[task.ID] = true
			families[task.Family]++
			dir := t.TempDir()
			require.NoError(t, materializePairedTask(dir, task))
			_, err := project.Load(filepath.Join(dir, "kapi.yaml"))
			require.NoError(t, err, "recipe must be usable by the shipped engine")
			voice, err := os.ReadFile(filepath.Join(dir, ".kapi", "voice.yaml"))
			require.NoError(t, err)
			_, err = profile.LoadProfileYAML(bytes.NewReader(voice))
			require.NoError(t, err)
			files, err := pairedTaskFiles(task)
			require.NoError(t, err)
			count := 0
			err = filepath.WalkDir(dir, func(name string, entry fs.DirEntry, walkErr error) error {
				require.NoError(t, walkErr)
				if entry.IsDir() {
					return nil
				}
				count++
				rel, err := filepath.Rel(dir, name)
				require.NoError(t, err)
				expected, exists := files[filepath.ToSlash(rel)]
				require.True(t, exists, "private scoring files must not leak into the workspace")
				actual, err := os.ReadFile(name)
				require.NoError(t, err)
				assert.Equal(t, expected, actual)
				return nil
			})
			require.NoError(t, err)
			assert.Len(t, files, count)
			assert.NotEmpty(t, task.spec.Criteria)
			assert.NotEmpty(t, task.spec.HumanReviewRubric)
		})
	}
	assert.Equal(t, map[string]int{"audience-adaptation": 2, "scoped-rename": 2, "guidance-revision": 2, "equipment-policy": 1}, families)
	hash, err := pairedCorpusHash()
	require.NoError(t, err)
	assert.Len(t, hash, 64)
	again, err := pairedCorpusHash()
	require.NoError(t, err)
	assert.Equal(t, hash, again)
}

func TestPairedMaterializationRejectsUnsafeDestinations(t *testing.T) {
	task := pairedTasks()[0]
	t.Run("existing file", func(t *testing.T) {
		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, "guidance.md"), []byte("keep"), 0o600))
		require.Error(t, materializePairedTask(dir, task))
		body, err := os.ReadFile(filepath.Join(dir, "guidance.md"))
		require.NoError(t, err)
		assert.Equal(t, "keep", string(body))
	})
	t.Run("linked root", func(t *testing.T) {
		link := filepath.Join(t.TempDir(), "workspace")
		require.NoError(t, os.Symlink(t.TempDir(), link))
		require.Error(t, materializePairedTask(link, task))
	})
	t.Run("linked parent", func(t *testing.T) {
		dir, outside := t.TempDir(), t.TempDir()
		require.NoError(t, os.Symlink(outside, filepath.Join(dir, ".kapi")))
		require.Error(t, materializePairedTask(dir, task))
		entries, err := os.ReadDir(outside)
		require.NoError(t, err)
		assert.Empty(t, entries)
	})
	t.Run("task traversal", func(t *testing.T) {
		task.ID = "../common"
		require.Error(t, materializePairedTask(t.TempDir(), task))
	})
}

// This tests the authored fixture rule, not the product checker or prose quality.
func TestPairedAssuranceFixtureMatchesItsGuidance(t *testing.T) {
	body, err := pairedFixtures.ReadFile("testdata/paired/common/.kapi/voice.yaml")
	require.NoError(t, err)
	voice, err := profile.LoadProfileYAML(bytes.NewReader(body))
	require.NoError(t, err)
	var expression string
	for _, constraint := range voice.Constraints {
		if constraint.ID == "harbor-help/no-unsupported-assurance" {
			expression = constraint.Regex
		}
	}
	require.NotEmpty(t, expression)
	pattern, err := regexp.Compile(expression)
	require.NoError(t, err)
	for _, statement := range []string{
		"Harbor Help is guaranteed to solve your problem.",
		"Harbor Help guarantees a result.",
		"We guarantee a result.",
		"Harbor Help is always safe.",
		"Harbor Help is risk-free.",
	} {
		assert.True(t, pattern.MatchString(statement), statement)
	}
	assert.False(t, pattern.MatchString("Harbor Help offers scheduled video appointments."))
}
