package host

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A flow whose redact step detects entities, with no entity-extract step
// ahead of it, is refused before it reads a file: the step would withhold
// nothing, and the provider would read the name.
func TestFlowRun_AnEntitiesPolicyWithNoEntityStepIsRefused(t *testing.T) {
	isolateKapiEnv(t)
	onlyDemoProvider(t)
	recipe := redactProject(t, "  redaction:\n    enabled: true\n    detectors: [entities]\n"+
		"flows:\n  secure:\n    steps:\n      - tool: redact\n"+
		"      - tool: translate\n        config:\n          provider: demo\n      - tool: unredact\n")
	saw := providerSaw(t)
	_, _, err := runRecipeFlow(t, recipe, "secure", "target-lang", "fr")
	require.Error(t, err)
	assert.Contains(t, err.Error(), `the project's redaction policy (defaults.redaction) detects entities, which needs an entity-extract step that flow "secure" does not have ahead of its redact step`)
	assert.Contains(t, err.Error(), "set defaults.redaction.detectors to [rules]")
	assert.Empty(t, saw(), "the provider read nothing")
}
