//go:build !js

package host

import (
	"regexp"
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

// TestMCPInstructionsNameOnlyTheWritingSet: every tool the instructions name is
// in the writing set, the set a server serves the instructions with.
func TestMCPInstructionsNameOnlyTheWritingSet(t *testing.T) {
	text := MCPInstructions()
	writing := MCPToolSetTools(MCPSetWriting)
	for _, name := range regexp.MustCompile(`\b(?:context|check)_[a-z_]+\b|\b(?:read_blocks|apply_edits|describe_format)\b`).FindAllString(text, -1) {
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
		"ask for the full answer where no rules file covers a file": "context_read",
		"rely on the rules files kapi writes":                       "AGENTS.md and CLAUDE.md",
		"record names while reading":                                "context_note",
		"record the person's correction":                            "from, to, path",
		"take back what was recorded wrongly":                       "withdraw",
		"check what you changed before saying done":                 "check_file",
		"report what the session recorded":                          "context_session_summary",
		"read a file's blocks before editing it":                    "read_blocks",
		"send an edit through the contract":                         "apply_edits",
		"learn what a format accepts":                               "describe_format",
	} {
		assert.Containsf(t, text, name, "the instructions carry the habit: %s", habit)
	}
	assert.Contains(t, text, "A person decides what becomes a rule",
		"an agent that reports what it recorded as a rule in force is the failure this sentence prevents")
	// The rules files already hold the answer. An agent told to ask before
	// every change pays a round trip and the tool schemas it loads for an
	// answer it has (the comparison eval measured 3.8 times the input tokens of
	// a hand-written rules file, for the same rule-following).
	assert.NotRegexp(t, `(?i)before (you change|you write|writing|every)[^.]*context_read`, text,
		"context_read is for a file no rules section covers, not a step before every change")
}

// TestMCPInstructionsReadAsInstructions keeps the text to the register the
// repository writes agent-facing prose in, and short: it is read into every
// session's context.
func TestMCPInstructionsReadAsInstructions(t *testing.T) {
	text := MCPInstructions()
	assert.NotContains(t, text, "—", "no em dash in agent-facing prose")
	assert.LessOrEqual(t, len(strings.Fields(text)), 145, "about a hundred and forty words, with the rules files first, the old names that stay correct and the path a note was seen in (R19.12, R19.13); a longer text is a decision rather than a drift")
	assert.NotContains(t, text, "portal", "the paired study's gate task turns on that word, so no agent-facing text uses it")
}

// TestMCPInstructionsFollowTheSets: a server that does not serve the writing
// set names none of its tools.
func TestMCPInstructionsFollowTheSets(t *testing.T) {
	assert.Equal(t, MCPInstructions(), MCPInstructionsFor(nil))
	assert.Equal(t, MCPInstructions(), MCPInstructionsFor(map[string]bool{MCPSetWriting: true, MCPSetReview: true}))
	assert.Empty(t, MCPInstructionsFor(map[string]bool{MCPSetReview: true}))
}

// TestRecordingGuidanceSaysWhatToRecordAndWhatToLeave holds every surface that
// asks an agent to record to both halves of the habit. An agent told only to
// record what it notices records what its own page touched and misses the
// names it read; an agent pushed to record more turns a word the project
// writes two ways into a rule. So each text names what to record, including
// what the agent's own text does not use, and names the word to leave alone.
func TestRecordingGuidanceSaysWhatToRecordAndWhatToLeave(t *testing.T) {
	app, _ := contextOpsApp(t)
	tools, err := growthSession(t, app).ListTools(t.Context(), nil)
	require.NoError(t, err)
	var observe string
	for _, tool := range tools.Tools {
		if tool.Name == "context_note" {
			observe = tool.Description
		}
	}
	require.NotEmpty(t, observe)

	for surface, text := range map[string]string{
		"the server instructions":       MCPInstructions(),
		"the answer with nothing in it": recordingAdvice,
		"the context_note description":  observe,
	} {
		for _, want := range []string{"name", "spelling variety"} {
			assert.Containsf(t, text, want, "%s says what to record", surface)
		}
		assert.Regexpf(t, `your (own )?text does not`, text,
			"%s asks for what the agent's own text does not use too", surface)
		assert.Truef(t, strings.Contains(text, "two ways") || strings.Contains(text, "more than one way"),
			"%s says to leave alone a word the project writes more than one way", surface)
		assert.NotContainsf(t, text, "—", "%s has no em dash", surface)
	}
}
