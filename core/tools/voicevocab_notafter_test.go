package tools

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/model"
	coreprofile "github.com/neokapi/neokapi/core/profile"
)

// A prohibited pattern whose not_after excludes the start of a text is still
// given a canary it catches, after a word of prose.
func TestVoiceVocabCanaryForAPatternWithNotAfter(t *testing.T) {
	p := &coreprofile.VoiceProfile{Name: "S", Style: coreprofile.StyleRules{ProhibitedPatterns: []coreprofile.Pattern{{
		Regex:    `(?i)\bused to\b`,
		NotAfter: `(?i)(?:(?:\b(?:is|are|was|were|be|been|being|get|gets|got|isn't|aren't|wasn't|weren't)|['’]s)\s+(?:\w+\s+)?|[^\w\s)\]"'\x60’”]\s*|^\s*)$`,
		Scope:    coreprofile.ScopeProse,
	}}}}
	canaries, uncheckable, err := NewVoiceVocabCheckTool(p, nil).Canaries(context.Background())
	require.NoError(t, err)
	require.Len(t, canaries, 1, uncheckable)
	text := model.RunsText(canaries[0].Block.SourceRuns())
	assert.NotEmpty(t, coreprofile.Findings(p, text, nil), "the canary %q is a violation", text)
}
