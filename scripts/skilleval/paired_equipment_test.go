package main

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/profile"
	"github.com/neokapi/neokapi/core/project"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func equipmentTask(t *testing.T) PairedTask {
	t.Helper()
	for _, task := range pairedTasks() {
		if task.ID == "revise-equipment-loan" {
			return task
		}
	}
	t.Fatal("equipment task missing")
	return PairedTask{}
}

func applyEquipmentFault(t *testing.T, dir, fault string) {
	t.Helper()
	prefix := "testdata/paired/faults/revise-equipment-loan/" + fault
	err := fs.WalkDir(pairedFixtures, prefix, func(name string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil || entry.IsDir() {
			return walkErr
		}
		body, err := pairedFixtures.ReadFile(name)
		require.NoError(t, err)
		output := filepath.Join(dir, filepath.FromSlash(strings.TrimPrefix(name, prefix+"/")))
		return os.WriteFile(output, body, 0o600)
	})
	require.NoError(t, err)
}

func TestPairedEquipmentObjectiveFailures(t *testing.T) {
	task := equipmentTask(t)
	for _, test := range []struct {
		fault string
		fails []string
	}{
		{fault: "missed-surface", fails: []string{"camera-desk-card-loan-label", "camera-desk-card-loan-days"}},
		{fault: "missed-schedule", fails: []string{"camera-reminder-send-after-days"}},
		{fault: "over-broad-exception", fails: []string{"unchanged:content/en/telescope.json"}},
		{fault: "changed-identifier", fails: []string{"page-policy-id"}},
	} {
		t.Run(test.fault, func(t *testing.T) {
			dir := t.TempDir()
			require.NoError(t, materializePairedTask(dir, task))
			repairPairedFixture(t, dir, task)
			applyEquipmentFault(t, dir, test.fault)
			result, err := validatePairedTask(dir, task)
			require.NoError(t, err)
			assert.False(t, result.ObjectivePassed)
			failed := []string{}
			for _, criterion := range result.Criteria {
				if !criterion.Passed {
					failed = append(failed, criterion.ID)
				}
			}
			assert.ElementsMatch(t, test.fails, failed)
			assert.True(t, result.HumanReviewRequired, "objective checks do not measure reviewer effort")
		})
	}
}

// Exercise the authored rules through the product's resolver and checker. The
// independent acceptance criteria also cover errors outside pattern coverage.
func TestPairedEquipmentCheckCoverage(t *testing.T) {
	task := equipmentTask(t)
	dir := t.TempDir()
	require.NoError(t, materializePairedTask(dir, task))
	recipe, err := project.Load(filepath.Join(dir, "kapi.yaml"))
	require.NoError(t, err)
	body, err := os.ReadFile(filepath.Join(dir, ".kapi/voice.yaml"))
	require.NoError(t, err)
	voice, err := profile.LoadProfileYAML(bytes.NewReader(body))
	require.NoError(t, err)
	require.Empty(t, profile.Blocking(profile.ValidateProfile(voice)))

	for _, test := range []struct {
		file    string
		channel string
		text    string
		wantID  string
	}{
		{file: "page.json", channel: "camera", text: "7-day loan", wantID: "alder-library/obsolete-loan-wording"},
		{file: "camera-reminder.json", channel: "camera", text: "Your camera loan lasts 7 days.",
			wantID: "alder-library/obsolete-loan-wording"},
		{file: "camera-desk-card.json", channel: "camera", text: "7-day loan",
			wantID: "alder-library/obsolete-loan-wording"},
		{file: "telescope.json", channel: "telescope", text: "7-day loan"},
		{file: "archive.json", channel: "archive", text: "Camera loans last 7 days."},
		{file: "api.json", channel: "integration", text: "camera_7_day"},
		{file: "telescope.json", channel: "telescope", text: "14-day loan",
			wantID: "alder-library/telescope-loan-limit"},
		{file: "page.json", channel: "camera", text: "14-day loan"},
		{file: "camera-reminder.json", channel: "camera", text: "5"},
	} {
		t.Run(test.file+"/"+test.text, func(t *testing.T) {
			governance, err := recipe.ResolveGovernanceForPath("content/en/" + test.file)
			require.NoError(t, err)
			require.Equal(t, test.channel, governance.Channel)
			resolved := profile.ResolveProfile(voice, "en", governance.Channel, "")
			findings := profile.Findings(resolved, test.text, nil)
			if test.wantID == "" {
				assert.Empty(t, findings)
				return
			}
			require.Len(t, findings, 1)
			assert.Equal(t, test.wantID, findings[0].Metadata["constraint_id"])
		})
	}
	resolved := profile.ResolveProfile(voice, "en", "telescope", "")
	assert.Equal(t, "excepted", profile.ConstraintResolutions(resolved)[0].Status)
	assert.Contains(t, profile.RenderVoiceGuide(resolved), "EQ-2026-09-T")
	assert.Contains(t, profile.RenderVoiceGuide(resolved), "semantic verification unsupported")
}

func TestPairedEquipmentManifestIsSeparate(t *testing.T) {
	manifest, err := readPairedManifest("testdata/paired-equipment-study.json")
	require.NoError(t, err)
	assert.Equal(t, []string{"revise-equipment-loan"}, manifest.Tasks)
	assert.Equal(t, "revise-equipment-loan", manifest.SmokeTask)
	assert.Len(t, pairedSchedule(manifest, "smoke"), 6)
	assert.Len(t, pairedSchedule(manifest, "pilot"), 6)
	original, err := readPairedManifest("testdata/paired-study.json")
	require.NoError(t, err)
	assert.Equal(t, "audience-child", original.SmokeTask)
	assert.NotContains(t, original.Tasks, "revise-equipment-loan")
}
