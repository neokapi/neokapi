package main

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"testing"

	"github.com/neokapi/neokapi/core/profile"
	"github.com/neokapi/neokapi/core/project"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The WP5 families, one task each (docs/internals/edit-model.md, WP5).
var pairedWP5Families = []string{
	"bilingual-po", "gate-recovery", "insert-key", "new-edition", "plural-branch", "stale-recovery", "wording-link",
}

func TestPairedCorpusMaterialization(t *testing.T) {
	tasks := pairedTasks()
	require.Len(t, tasks, 7)
	families := []string{}
	seen := map[string]bool{}
	for _, task := range tasks {
		t.Run(task.ID, func(t *testing.T) {
			require.False(t, seen[task.ID], "task IDs must be unique")
			seen[task.ID] = true
			families = append(families, task.Family)
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
			assert.NotEmpty(t, task.spec.Scope)
			for _, name := range slices.Concat(task.spec.Editable, task.spec.Creates) {
				assert.Contains(t, task.spec.Scope, pairedDirOf(name), "%s lies outside every scoped directory", name)
			}
			for _, name := range task.spec.Editable {
				assert.Contains(t, files, name, "an editable file must be a fixture file")
			}
			for _, criterion := range task.spec.Criteria {
				if criterion.Kind == "no_override" {
					continue
				}
				assert.True(t, slices.Contains(task.spec.Editable, criterion.Path) || slices.Contains(task.spec.Creates, criterion.Path),
					"criterion %q names a path the task neither edits nor creates", criterion.ID)
			}
			if task.spec.Interference != nil {
				assert.Contains(t, task.spec.Editable, task.spec.Interference.Path)
				assert.Contains(t, string(files[task.spec.Interference.Path]), task.spec.Interference.Find)
			}
		})
	}
	slices.Sort(families)
	assert.Equal(t, pairedWP5Families, families)
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
		require.NoError(t, os.WriteFile(filepath.Join(dir, "STYLE.md"), []byte("keep"), 0o600))
		require.Error(t, materializePairedTask(dir, task))
		body, err := os.ReadFile(filepath.Join(dir, "STYLE.md"))
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

// This tests the authored fixture rules, not the product checker or prose
// quality: the voice every task's project binds forbids unsupported
// assurances, and its terms forbid "dashboard" in favour of "overview page",
// which STYLE.md states for an agent that reads files.
func TestPairedFixtureGovernanceMatchesItsStyleGuide(t *testing.T) {
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
		"Harbor Help is always safe.",
		"Harbor Help is risk-free.",
	} {
		assert.True(t, pattern.MatchString(statement), statement)
	}
	assert.False(t, pattern.MatchString("Harbor Help offers scheduled video appointments."))
	assert.Contains(t, string(body), "term: dashboard")
	assert.Contains(t, string(body), "replacement: overview page")
	style, err := pairedFixtures.ReadFile("testdata/paired/common/STYLE.md")
	require.NoError(t, err)
	assert.Contains(t, string(style), `write "overview page", never "dashboard"`)
}
