package main

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// A host failure that reported no token use is left out of every median
// alike: seconds and tool calls describe the attempts the tokens do.
func TestPairedMediansLeaveOutAnUnmeasuredAttemptAlike(t *testing.T) {
	tokens := func(n int64) *int64 { return &n }
	session := PairedSession{Task: "add-edition-markup", Agent: PairedAgentSpec{Host: "claude"}, Condition: "skill-cli"}
	rows := []pairedScoreRow{
		{Session: session, Phase: "main", Status: "completed", DurationMS: 100_000, ToolCalls: 30, InputTokens: tokens(1000), OutputTokens: tokens(100)},
		{Session: session, Phase: "main", Status: "completed", DurationMS: 200_000, ToolCalls: 40, InputTokens: tokens(3000), OutputTokens: tokens(300)},
		{Session: session, Phase: "main", Status: "process_failed", DurationMS: 900_000, ToolCalls: 90},
	}
	var markdown strings.Builder
	writePairedSummary(&markdown, rows)
	text := markdown.String()
	assert.Contains(t, text, "| add-edition-markup | claude | skill-cli | 3 | 0 | 2 | 150 | 2000 | 200 | 35 |",
		"seconds and tool calls are the medians of the two measured attempts")
	assert.Contains(t, text, "add-edition-markup claude skill-cli (1)")
}
