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
// passes --all-tools to get a pipeline step back and silently grants
// shell and JS execution to any MCP client.
func TestExecutionToolsAreNeverAgentFacing(t *testing.T) {
	for _, name := range []string{"external-command", "script"} {
		assert.Truef(t, neverAgentFacing[name],
			"%q executes caller-supplied code and must never be exposed over MCP, not even under --all-tools — `kapi exec` still runs it",
			name)
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
		"voice_guide":      "the context:// resources",
		"recycle":          "up (recycling is invisible by design)",
		"diff-leverage":    "up",
	}
	for name, replacement := range porcelain {
		assert.Falsef(t, agentFacingTools[name],
			"%q duplicates %q on the agent surface — pick one", name, replacement)
	}
}

// TestByLocationIsAddressedNotCalled: the by-location primitive is a RESOURCE,
// not a tool. That is what lets its rendering be a property of the read — a
// mime type on the response — rather than a second tool per shape, which is the
// conflation that made `voice_guide` (voice only, by name only, markdown only)
// three narrow decisions wearing one name.
func TestByLocationIsAddressedNotCalled(t *testing.T) {
	for _, name := range []string{"voice_guide", "context_at", "context_location"} {
		assert.Falsef(t, agentFacingTools[name],
			"%q would re-open the by-location primitive as a tool; it is served at context://", name)
	}
}
