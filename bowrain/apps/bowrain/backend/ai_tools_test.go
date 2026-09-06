package backend

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// The desktop's tool list comes from the registry alone, so it names each AI
// tool once and carries the framework's own metadata for it. A second,
// hand-written list is what let the desktop drift from the schemas the server
// serves.
func TestListToolsCarriesEachAIToolOnce(t *testing.T) {
	tools := NewApp().ListTools()

	counts := make(map[string]int, len(tools))
	for _, tl := range tools {
		counts[tl.Name]++
	}
	for _, name := range []string{"translate", "qa", "review", "voice-check", "voice-infer", "term-extract"} {
		assert.Equal(t, 1, counts[name], "tool %q must appear exactly once", name)
	}

	byName := make(map[string]ToolInfo, len(tools))
	for _, tl := range tools {
		byName[tl.Name] = tl
	}
	translate := byName["translate"]
	assert.Equal(t, "bilingual", translate.Cardinality)
	assert.Contains(t, translate.Requires, "credentials")
	assert.NotEmpty(t, translate.SideEffects)
}
