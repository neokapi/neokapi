package main

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestScenariosStayDownstreamClean.
//
// This suite publishes its dataset into web/src, which the neokapi docs site
// serves and which scripts/check-docs-bowrain-clean.sh holds to zero mentions
// of the downstream platform. A scenario prompt or fixture path naming it puts
// that name into the committed JSON and fails a CI job three steps away from
// anything that looks like this file.
//
// It cost one red build. The rule is not arbitrary: the framework's docs
// describe the framework, and an eval about onboarding to the platform belongs
// in the platform's own suite.
func TestScenariosStayDownstreamClean(t *testing.T) {
	for _, sc := range scenarios {
		t.Run(sc.ID, func(t *testing.T) {
			assert.NotContains(t, strings.ToLower(sc.Prompt), "bowrain",
				"the prompt is copied verbatim into the published dataset")
			assert.NotContains(t, strings.ToLower(sc.Why), "bowrain")
			assert.NotContains(t, strings.ToLower(sc.Path), "bowrain")
			for _, f := range sc.Fixture {
				assert.NotContains(t, strings.ToLower(f.From), "bowrain",
					"a fixture path is recorded in the dataset, so its directory name ships too")
				assert.NotContains(t, strings.ToLower(f.Body), "bowrain")
				assert.NotContains(t, strings.ToLower(f.Note), "bowrain")
			}
		})
	}
}

// TestWithholdingClearsTheMentionAndKeepsTheNumbers.
//
// The transcript is the half this suite does not control. The shipped skill's
// own description names the platform, so an agent can repeat it in a closing
// message on any scenario, and the first version of the guard threw away the
// whole run when one did. Half an hour of metered sweeping, discarded because
// one agent said one word.
//
// Withholding is the trade: the prose goes, the numbers stay, and the omission
// is recorded where a reader will see it.
func TestWithholdingClearsTheMentionAndKeepsTheNumbers(t *testing.T) {
	r := &Report{
		Results: []Result{
			{
				Scenario: Scenario{ID: "clean"},
				Verdict:  "pass",
				Runs: []Run{{
					Messages: 12, Triggered: true, FinalText: "Translated the deck.",
					Gate: &GateResult{ExitCode: 0, Output: "ok"},
				}},
			},
			{
				Scenario: Scenario{ID: "dirty"},
				Verdict:  "fail",
				Runs: []Run{{
					Messages: 34, Triggered: true,
					FinalText:    "You could push this to Bowrain next.",
					KapiCommands: []string{"kapi status"},
					Gate:         &GateResult{ExitCode: 1, Output: "no"},
					Changed:      []FileChange{{Path: "a.md", Before: "x", After: "bowrain"}},
				}},
				Unaided: []Run{{Messages: 9, FinalText: "Used bowrain docs."}},
			},
		},
	}

	withholdDownstream(r)
	require.NoError(t, stillMentionsDownstream(r), "withholding must clear every mention it claims to")

	clean, dirty := r.Results[0], r.Results[1]
	assert.Empty(t, clean.Withheld, "a result that never mentioned it is untouched")
	assert.Equal(t, "Translated the deck.", clean.Runs[0].FinalText)

	assert.NotEmpty(t, dirty.Withheld, "the omission is recorded on the result")
	assert.Equal(t, 1, r.Summary.Withheld, "and counted on the report")

	// The numbers a reader judges the suite by survive.
	assert.Equal(t, "fail", dirty.Verdict)
	assert.Equal(t, 34, dirty.Runs[0].Messages)
	assert.Equal(t, 1, dirty.Runs[0].Gate.ExitCode)
	assert.Equal(t, "a.md", dirty.Runs[0].Changed[0].Path)
	assert.Equal(t, 9, dirty.Unaided[0].Messages)

	// The prose does not.
	assert.Empty(t, dirty.Runs[0].FinalText)
	assert.Empty(t, dirty.Runs[0].KapiCommands)
	assert.Empty(t, dirty.Runs[0].Gate.Output)
	assert.Empty(t, dirty.Runs[0].Changed[0].After)
	assert.Empty(t, dirty.Unaided[0].FinalText)
}

// TestWithholdingIsNotABlanketScrub: a run that never named the platform keeps
// everything. A guard that cleared every transcript would pass its own check
// and publish nothing worth reading.
func TestWithholdingIsNotABlanketScrub(t *testing.T) {
	r := &Report{Results: []Result{{
		Scenario: Scenario{ID: "clean"},
		Runs:     []Run{{FinalText: "Done.", KapiCommands: []string{"kapi kcat x.docx"}}},
	}}}
	withholdDownstream(r)
	assert.Zero(t, r.Summary.Withheld)
	assert.Equal(t, "Done.", r.Results[0].Runs[0].FinalText)
	assert.Equal(t, []string{"kapi kcat x.docx"}, r.Results[0].Runs[0].KapiCommands)
}

// TestPartitionFilesEachResultUnderItsOwnSurface.
//
// `make skill-eval` selects every scenario, MCP included, and a report with no
// Surface keys as skill. All 30 landed under `skill:trigger`: the skill card
// counted eight MCP scenarios as its own while the MCP card showed a separate
// older run of the same eight. The documented command mislabelled its output
// and double-counted a third of it.
func TestPartitionFilesEachResultUnderItsOwnSurface(t *testing.T) {
	r := &Report{Mode: modeTrigger, Results: []Result{
		{Scenario: Scenario{ID: "p01", Kind: positive}, Verdict: "pass"},
		{Scenario: Scenario{ID: "m01", Kind: positive, Surface: surfaceMCP}, Verdict: "pass"},
		{Scenario: Scenario{ID: "n01", Kind: negative}, Verdict: "pass"},
		{Scenario: Scenario{ID: "m02", Kind: positive, Surface: surfaceMCP}, Verdict: "fail"},
	}}

	parts := partitionBySurface(r)
	byKey := map[string]*Report{}
	total := 0
	for _, p := range parts {
		byKey[p.Key()] = p
		total += len(p.Results)
	}

	assert.Len(t, parts, 2)
	assert.Equal(t, len(r.Results), total, "every result appears once, in exactly one part")

	skill, mcp := byKey["skill:"+modeTrigger], byKey["mcp:"+modeTrigger]
	require.NotNil(t, skill)
	require.NotNil(t, mcp)
	assert.Len(t, skill.Results, 2)
	assert.Len(t, mcp.Results, 2)

	// Each part's summary counts its own results, not the whole run's.
	assert.Equal(t, 2, skill.Summary.Scenarios)
	assert.Equal(t, 2, mcp.Summary.Scenarios)
	assert.Equal(t, 1, mcp.Summary.Fail, "the failing MCP scenario is counted where it belongs")
	assert.Zero(t, skill.Summary.Fail)
}

// TestPartitionOfOneSurfaceIsOnePart: a run already filtered to one surface
// must not be split, or -surface mcp would write an empty skill report over a
// real one.
func TestPartitionOfOneSurfaceIsOnePart(t *testing.T) {
	r := &Report{Mode: modeTrigger, Surface: surfaceMCP, Results: []Result{
		{Scenario: Scenario{ID: "m01", Kind: positive, Surface: surfaceMCP}},
	}}
	parts := partitionBySurface(r)
	require.Len(t, parts, 1)
	assert.Equal(t, "mcp:"+modeTrigger, parts[0].Key())
}
