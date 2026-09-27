package commands

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/bowrain/plugin/commands/output"
	"github.com/neokapi/neokapi/core/venue"
	"github.com/neokapi/neokapi/host/venue/transfer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// settingsPush is a push that tightened one recipe setting and was refused a
// loosening of the other.
func settingsPush() *transfer.PushResult {
	return &transfer.PushResult{
		SettingsApplied: venue.ProjectSettings{venue.SettingConvergePolicy: "manual"},
		SettingsRefused: []venue.SettingRefusal{{
			Setting: venue.SettingTranslateAfter, Requested: "none", InForce: "established",
			Reason: venue.SettingLoosens, Requires: "manage_project",
		}},
	}
}

// TestReportPushSettings_TextMatchesPushFooter asserts up's push phase says
// what the push did with the recipe's settings, in the lines `kapi push`
// prints, naming who can apply a refused one.
func TestReportPushSettings_TextMatchesPushFooter(t *testing.T) {
	cmd, stdout, stderr := voiceReportCmd()
	require.NoError(t, reportPushSettings(cmd, nil, settingsPush(), false))

	got := stderr.String()
	assert.Contains(t, got, "Server setting converge_policy is now manual, as the recipe declares.\n")
	assert.Contains(t, got, "Server setting translate_after stays established: the recipe asks for none")
	assert.Contains(t, got, "owner or admin (manage_project)")
	assert.Empty(t, stdout.String())

	pr := settingsPush()
	var push strings.Builder
	require.NoError(t, output.PushOutput{
		UpToDate: true, SettingsApplied: pr.SettingsApplied, SettingsRefused: pr.SettingsRefused,
	}.FormatText(&push))
	assert.Contains(t, push.String(), got, "kapi push prints the same lines")
}

// TestReportPushSettings_JSONLine asserts --json carries the report as one
// NDJSON record of type "settings".
func TestReportPushSettings_JSONLine(t *testing.T) {
	cmd, stdout, stderr := voiceReportCmd()
	require.NoError(t, reportPushSettings(cmd, nil, settingsPush(), true))

	assert.Empty(t, stderr.String())
	var line struct {
		Type    string                 `json:"type"`
		Applied venue.ProjectSettings  `json:"settings_applied"`
		Refused []venue.SettingRefusal `json:"settings_refused"`
	}
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &line))
	assert.Equal(t, "settings", line.Type)
	assert.Equal(t, settingsPush().SettingsApplied, line.Applied)
	assert.Equal(t, settingsPush().SettingsRefused, line.Refused)
}

// TestReportPushSettings_SilentWhenNothingChanged asserts a push whose settings
// already matched prints nothing.
func TestReportPushSettings_SilentWhenNothingChanged(t *testing.T) {
	for _, jsonOut := range []bool{false, true} {
		cmd, stdout, stderr := voiceReportCmd()
		require.NoError(t, reportPushSettings(cmd, nil, &transfer.PushResult{BlocksPushed: 1}, jsonOut))
		assert.Empty(t, stdout.String())
		assert.Empty(t, stderr.String())
	}
}
