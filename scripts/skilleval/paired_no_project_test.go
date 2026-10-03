package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A cell that runs kapi with no project, the alias's or kapi's own with
// discovery off, holds no recipe and none of the project's context: what an
// agent there succeeds at owes nothing to files a project put beside it. A
// baseline cell keeps them, as an ordinary checkout would.
func TestPairedNoProjectCellsHoldNoProject(t *testing.T) {
	task := pairedTaskByID(t, "edit-po-context")
	for condition, project := range map[string]bool{
		"project-free": false, "kapi-no-project": false, "baseline": true, "skill-cli": true,
	} {
		t.Run(condition, func(t *testing.T) {
			dir := t.TempDir()
			require.NoError(t, materializePairedCell(dir, task, condition))
			for _, name := range []string{"kapi.yaml", "STYLE.md", ".kapi/voice.yaml"} {
				_, err := os.Stat(filepath.Join(dir, name))
				assert.Equal(t, project, err == nil, "%s in a %s cell", name, condition)
			}
			_, err := os.Stat(filepath.Join(dir, "locales", "nb", "messages.po"))
			require.NoError(t, err, "the task's own files are in every cell")

			// An untouched cell keeps every file it holds, with nothing
			// missing that it never held.
			result, err := validatePairedCell(dir, task, condition, &PairedAgentResult{})
			require.NoError(t, err)
			for _, c := range result.Criteria {
				if c.ID[:min(len(c.ID), 10)] == "unchanged:" {
					assert.True(t, c.Passed, "%s in a %s cell", c.ID, condition)
				}
			}
		})
	}
	arm, err := pairedArmFor("kapi-no-project")
	require.NoError(t, err)
	assert.Equal(t, pairedArm{Skill: "kapi", Executables: []string{"kapi"}}, arm)
	assert.True(t, arm.noProject())
}
