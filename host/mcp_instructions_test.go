package host

import (
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// The instructions are the only text some clients ever read about kapi: a host
// that loads no skill still gets them, ahead of the tool list. So they are held
// to the surface rather than to a reviewer's memory. A sentence naming a verb
// that does not exist is the most expensive kind of wrong, because a model acts
// on it before it can find out.

// TestMCPInstructionsNameOnlyTheWritingSet: every tool the instructions name is
// in the writing set, the set a server serves the instructions with.
func TestMCPInstructionsNameOnlyTheWritingSet(t *testing.T) {
	text := MCPInstructions()
	writing := MCPToolSetTools(MCPSetWriting)
	for _, name := range regexp.MustCompile(`\b(?:context|check)_[a-z_]+\b`).FindAllString(text, -1) {
		assert.Containsf(t, writing, name, "the instructions name %s, which the writing set does not serve", name)
	}
	for _, absent := range []string{"voice_guide", "term_lookup", "external-command", "kapi up"} {
		assert.NotContainsf(t, text, absent, "%s is not served here, so the instructions must not name it", absent)
	}
}

// TestMCPInstructionsCarryTheHabits holds the text to the habits the skill
// leads with. A client that loads no skill reads this and nothing else, so a
// habit missing here is a habit that surface does not have.
func TestMCPInstructionsCarryTheHabits(t *testing.T) {
	text := MCPInstructions()
	for habit, name := range map[string]string{
		"read what applies before writing":           "context://",
		"the same answer for a client without reads": "kapi context <path>",
		"record names while reading":                 "context_observe",
		"record the person's correction":             "context_correct",
		"take back what was recorded wrongly":        "context_withdraw",
		"check what you changed before saying done":  "check_file",
		"report what the session recorded":           "context_session_summary",
	} {
		assert.Containsf(t, text, name, "the instructions carry the habit: %s", habit)
	}
	assert.Contains(t, text, "A person decides what becomes a rule",
		"an agent that reports what it recorded as a rule in force is the failure this sentence prevents")
}

// TestMCPInstructionsReadAsInstructions keeps the text to the register the
// repository writes agent-facing prose in, and short: it is read into every
// session's context.
func TestMCPInstructionsReadAsInstructions(t *testing.T) {
	text := MCPInstructions()
	assert.NotContains(t, text, "—", "no em dash in agent-facing prose")
	assert.LessOrEqual(t, len(strings.Fields(text)), 110, "about a hundred words; a longer text is a decision rather than a drift")
}

// TestMCPInstructionsFollowTheSets: a server that does not serve the writing
// set names none of its tools.
func TestMCPInstructionsFollowTheSets(t *testing.T) {
	assert.Equal(t, MCPInstructions(), MCPInstructionsFor(nil))
	assert.Equal(t, MCPInstructions(), MCPInstructionsFor(map[string]bool{MCPSetWriting: true, MCPSetReview: true}))
	assert.Empty(t, MCPInstructionsFor(map[string]bool{MCPSetReview: true}))
}
