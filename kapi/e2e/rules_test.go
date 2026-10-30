//go:build e2e

package e2e

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// `kapi rules` against the built binary: an assistant records a change with
// the rule it implies, a person keeps the rule by id, and the listing shows
// it in force.
func TestRulesKeepsWhatAnAssistantSuggested(t *testing.T) {
	root := contextActorProject(t, "rules-e2e")
	recipe := filepath.Join(root, "kapi.yaml")
	agent := []string{"KAPI_ACTOR=agent", "KAPI_AGENT_NAME=codex", "KAPI_AGENT_SESSION=s-e2e-rules"}
	person := []string{"KAPI_ACTOR=person"}

	kapiEnvOK(t, agent, "context", "note", "--from", "utilise", "--to", "use",
		"--seen-in", "docs/guide.md", "--suggest", "-p", recipe)

	var view struct {
		Decisions []struct {
			ID          string `json:"id"`
			Question    string `json:"question"`
			Consequence string `json:"consequence"`
		} `json:"decisions"`
	}
	out := kapiEnvOK(t, person, "rules", "--json", "-p", recipe)
	require.NoError(t, json.Unmarshal([]byte(out), &view), out)
	require.Len(t, view.Decisions, 1, out)
	d := view.Decisions[0]
	assert.Equal(t, "Keep this as a rule?", d.Question)
	assert.Equal(t, "If you keep it: \"utilise\" becomes a finding in 1 block. Applies to the whole project.", d.Consequence)

	kapiEnvOK(t, person, "rules", "--keep", d.ID, "-p", recipe)

	var listing struct {
		Counts struct {
			Rules     int `json:"rules"`
			Suggested int `json:"suggested"`
		} `json:"counts"`
		Groups []struct {
			Group string `json:"group"`
			Rules []struct {
				ID       string `json:"id"`
				Sentence string `json:"sentence"`
				Fails    bool   `json:"fails"`
			} `json:"rules"`
		} `json:"groups"`
	}
	out = kapiEnvOK(t, person, "rules", "list", "--json", "-p", recipe)
	require.NoError(t, json.Unmarshal([]byte(out), &listing), out)
	assert.Equal(t, 1, listing.Counts.Rules)
	assert.Equal(t, 0, listing.Counts.Suggested)
	require.Len(t, listing.Groups, 1, out)
	assert.Equal(t, "words", listing.Groups[0].Group)
	require.Len(t, listing.Groups[0].Rules, 1)
	assert.Equal(t, d.ID, listing.Groups[0].Rules[0].ID)
	assert.Equal(t, "Write use, not utilise.", listing.Groups[0].Rules[0].Sentence)
	assert.True(t, listing.Groups[0].Rules[0].Fails, "a kept word fails a check")
}
