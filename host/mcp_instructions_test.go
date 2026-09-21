package host

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The instructions are the only text some clients ever read about kapi: a host
// that loads no skill still gets them, ahead of the tool list. So they are held
// to the surface rather than to a reviewer's memory. A sentence naming a verb
// that does not exist is the most expensive kind of wrong, because a model acts
// on it before it can find out.

func TestMCPInstructionsNameWhatTheServerServes(t *testing.T) {
	text := MCPInstructions()
	require.NotEmpty(t, text)

	// Every name the instructions tell a client to call.
	for _, name := range []string{"context_search", "check_file"} {
		assert.Containsf(t, text, name, "the instructions ask a client to call %s", name)
	}
	assert.Contains(t, text, "context://", "the by-location primitive is addressed, so the instructions carry its scheme")

	// And nothing the surface withholds. A tool the instructions promise and
	// the server does not serve reads to a model as a capability it has.
	for _, absent := range []string{"voice_guide", "term_lookup", "tm_search", "external-command", "script"} {
		assert.NotContainsf(t, text, absent, "%s is not served here, so the instructions must not name it", absent)
	}
}

func TestMCPInstructionsSayWhatAnEmptyAnswerMeans(t *testing.T) {
	text := strings.ToLower(MCPInstructions())
	assert.Contains(t, text, "empty answer",
		"an agent that reads an empty answer as nothing to do is the failure these instructions exist to prevent")
	assert.Contains(t, text, "project", "every call takes a project, and the instructions say so")
	assert.Contains(t, text, "revision", "every answer carries the revision it was read at")
}

// TestMCPInstructionsReadAsInstructions keeps the text to the register the
// repository writes agent-facing prose in.
func TestMCPInstructionsReadAsInstructions(t *testing.T) {
	text := MCPInstructions()
	assert.NotContains(t, text, "—", "no em dash in agent-facing prose")
	assert.Less(t, len(text), 1400, "the instructions are read into every session's context; keep them short")
}
