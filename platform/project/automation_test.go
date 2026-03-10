package project

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAutomationConfigRoundTrip(t *testing.T) {
	dir := t.TempDir()
	configDir := filepath.Join(dir, ".bowrain")
	require.NoError(t, os.MkdirAll(configDir, 0755))

	enabled := true
	disabled := false

	cfg := &Config{
		Defaults: Defaults{SourceLanguage: "en"},
		Automations: []AutomationConfig{
			{
				Name:    "qa-before-push",
				Trigger: "pre-push",
				Actions: []ActionConfig{
					{Type: "run_flow", Config: map[string]string{"flow": "qa-check"}},
				},
			},
			{
				Name:    "sync-on-push",
				Trigger: "post-push",
				Enabled: &enabled,
				Actions: []ActionConfig{
					{Type: "wait_translate", Config: map[string]string{"timeout": "5m"}},
					{Type: "pull"},
				},
			},
			{
				Name:    "disabled-rule",
				Trigger: "pre-pull",
				Enabled: &disabled,
				Actions: []ActionConfig{
					{Type: "run_flow", Config: map[string]string{"flow": "unused"}},
				},
			},
		},
	}

	err := SaveConfig(configDir, cfg)
	require.NoError(t, err)

	loaded, err := LoadConfig(configDir)
	require.NoError(t, err)

	require.Len(t, loaded.Automations, 3)

	// Rule 1: qa-before-push
	r1 := loaded.Automations[0]
	assert.Equal(t, "qa-before-push", r1.Name)
	assert.Equal(t, "pre-push", r1.Trigger)
	assert.True(t, r1.IsEnabled())
	require.Len(t, r1.Actions, 1)
	assert.Equal(t, "run_flow", r1.Actions[0].Type)
	assert.Equal(t, "qa-check", r1.Actions[0].Config["flow"])

	// Rule 2: sync-on-push
	r2 := loaded.Automations[1]
	assert.Equal(t, "sync-on-push", r2.Name)
	assert.Equal(t, "post-push", r2.Trigger)
	assert.True(t, r2.IsEnabled())
	require.Len(t, r2.Actions, 2)
	assert.Equal(t, "wait_translate", r2.Actions[0].Type)
	assert.Equal(t, "pull", r2.Actions[1].Type)

	// Rule 3: disabled
	r3 := loaded.Automations[2]
	assert.False(t, r3.IsEnabled())
}

func TestAutomationIsEnabled(t *testing.T) {
	tests := []struct {
		name     string
		enabled  *bool
		expected bool
	}{
		{"nil means enabled", nil, true},
		{"true means enabled", boolPtr(true), true},
		{"false means disabled", boolPtr(false), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := AutomationConfig{Enabled: tt.enabled}
			assert.Equal(t, tt.expected, a.IsEnabled())
		})
	}
}

func boolPtr(b bool) *bool { return &b }
