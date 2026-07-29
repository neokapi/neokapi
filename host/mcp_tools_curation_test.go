package host

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// The curation contract (AD-037). The MCP surface is a decision, not a
// consequence of a tool being CLI-visible — these pin the two properties that
// make that true, because both failed silently before.

// TestExecutionToolsAreNeverAgentFacing is the one that matters most. "Show me
// every tool" and "let a caller run arbitrary commands and JavaScript" are
// different classes of decision; if the second ever rides on the first, someone
// enables KAPI_MCP_ALL_TOOLS to get a pipeline step back and silently grants
// shell and JS execution to any MCP client.
func TestExecutionToolsAreNeverAgentFacing(t *testing.T) {
	for _, name := range []string{"external-command", "script"} {
		assert.Truef(t, neverAgentFacing[name],
			"%q executes caller-supplied code and must never be exposed over MCP, not even under %s — `kapi exec` still runs it",
			name, ExposeAllMCPToolsEnv)
		assert.Falsef(t, agentFacingTools[name],
			"%q must not also appear in the curated set", name)
	}
}

// TestCuratedToolsDoNotShadowPorcelain: every curated registry tool must be a
// job the hand-authored tools do not already do. Two names for one job means
// the caller picks wrong half the time — which is why `qa` (check_text /
// check_file) and `pseudo-translate` (pseudo_translate) are absent.
func TestCuratedToolsDoNotShadowPorcelain(t *testing.T) {
	// The porcelain names, as registered by the hand-authored factories.
	porcelain := map[string]string{
		"qa":               "check_text / check_file",
		"pseudo-translate": "pseudo_translate",
		"term_lookup":      "context_search",
		"tm_search":        "context_search",
		"brand_guide":      "context_search",
		"recycle":          "up (recycling is invisible by design)",
		"diff-leverage":    "up",
	}
	for name, replacement := range porcelain {
		assert.Falsef(t, agentFacingTools[name],
			"%q duplicates %q on the agent surface — pick one", name, replacement)
	}
}
