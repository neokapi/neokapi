package host

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// apply_edits' description is the reference a host shows once the tool is
// loaded. It names every content operation an agent reaches for, says what a
// gate refusal asks of it, names no tool the writing set does not serve, and
// asks for check_file only where the commit check does not already report.
func TestApplyEditsDescriptionNamesWhatAnAgentNeeds(t *testing.T) {
	session := editSession(t, &App{}, "describe-test")
	tools, err := session.ListTools(t.Context(), nil)
	require.NoError(t, err)
	desc := map[string]string{}
	for _, tool := range tools.Tools {
		desc[tool.Name] = tool.Description
	}
	apply := desc["apply_edits"]
	require.NotEmpty(t, apply)
	for _, want := range []string{"set_content", "replace_text", "set_attribute", "mark", "remove_edition", "insert_block",
		"gate_failed", "rewrite the wording they name", "only a person can override", "Several operations may name one block",
		`if_match "absent"`, "run check_file after a native write"} {
		assert.Contains(t, apply, want)
	}
	assert.NotContains(t, apply, "review_block", "the writing set does not serve review_block")
	assert.NotContains(t, apply, "on each changed file afterwards")
	assert.Contains(t, desc["describe_format"], "kapi apply --schema OP")
	for name, d := range desc {
		assert.NotContainsf(t, d, "—", "%s: no em dash in agent-facing prose", name)
		assert.NotContainsf(t, d, "portal", "%s: the paired study's gate task turns on that word", name)
	}
}
