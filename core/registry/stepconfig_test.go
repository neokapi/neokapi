package registry

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStepConfigError(t *testing.T) {
	const at, keys = "flows.review.steps[1]", "flows.review.steps[1].config"
	const modes = "Write mode: rules for the rule checks, or mode: ai for the AI review"

	for _, key := range []string{"checkTerminology", "check_terminology", "CheckTerminology", "checkterminology"} {
		t.Run(key, func(t *testing.T) {
			err := StepConfigError("qa", map[string]any{"mode": "rules", key: true}, at, keys)
			require.EqualError(t, err, keys+"."+key+" is retired and nothing reads it. Remove the key")
		})
	}

	require.EqualError(t, StepConfigError("qa", map[string]any{"provider": "anthropic"}, at, keys),
		at+" (qa) sets provider: anthropic without mode, and a qa step that names a provider requires mode. "+modes)
	require.EqualError(t, StepConfigError("qa", map[string]any{"mode": "llm"}, at, keys),
		keys+".mode is llm, a value qa does not take. "+modes)

	for _, ok := range []map[string]any{
		nil,
		{},
		{"mode": "rules"},
		{"mode": "ai", "provider": "anthropic"},
		{"mode": "rules", "provider": "anthropic"},
		{"checkDoubleSpaces": false},
	} {
		assert.NoError(t, StepConfigError("qa", ok, at, keys), "%v", ok)
	}

	assert.NoError(t, StepConfigError("translate", map[string]any{"provider": "anthropic", "checkTerminology": true}, at, keys),
		"only qa's config is held to these keys")
}
