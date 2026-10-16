package tools_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/check"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/tools"
)

// consistencyPair is one source/target block fed to the qa tool in order.
type consistencyPair struct {
	source, target []model.Run
}

// consistencyFindings runs one qa tool over the pairs in order and returns the
// findings on the last block. The tool keeps the cross-block state, so the
// same instance sees every pair.
func consistencyFindings(t *testing.T, cfg *tools.RuleCheckConfig, pairs []consistencyPair) []check.Finding {
	t.Helper()
	tl := tools.NewRuleCheckTool(cfg)

	var last *model.Block
	for i, p := range pairs {
		b := model.NewRunsBlock("u"+string(rune('1'+i)), p.source)
		b.SetTargetRuns(cfg.TargetLocale, p.target)
		out := processPart(t, tl, &model.Part{Type: model.PartBlock, Resource: b})
		blk, ok := out.Resource.(*model.Block)
		require.True(t, ok)
		last = blk
	}
	return checkFindings(last)
}

func inconsistencyMessages(findings []check.Finding) []string {
	var msgs []string
	for _, f := range findings {
		if f.Category == "inconsistency" {
			msgs = append(msgs, f.Message)
		}
	}
	return msgs
}

// TestRuleCheck_ConsistencyKeyKeepsCodes holds the consistency maps to a
// code-aware key (#1463). The plain text drops inline codes, so under it
// "Hello {name}" and "Hello {count}" were one source translated two ways, and
// every placeholder-only block keyed as "".
func TestRuleCheck_ConsistencyKeyKeepsCodes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		configure     func(cfg *tools.RuleCheckConfig)
		pairs         []consistencyPair
		wantReported  bool
		wantMentioned []string // readable alternatives the message must list
	}{
		{
			name: "same text with different placeholders is not a target inconsistency",
			configure: func(cfg *tools.RuleCheckConfig) {
				cfg.CheckTargetInconsistency = true
			},
			pairs: []consistencyPair{
				{source: []model.Run{checkTx("Hello "), checkPh("1", "name")}, target: []model.Run{checkTx("Bonjour "), checkPh("1", "name")}},
				{source: []model.Run{checkTx("Hello "), checkPh("1", "count")}, target: []model.Run{checkTx("Salut "), checkPh("1", "count")}},
			},
			wantReported: false,
		},
		{
			name: "different text with different placeholders sharing the words is not a source inconsistency",
			configure: func(cfg *tools.RuleCheckConfig) {
				cfg.CheckSourceInconsistency = true
			},
			pairs: []consistencyPair{
				{source: []model.Run{checkTx("Hello "), checkPh("1", "name")}, target: []model.Run{checkTx("Bonjour "), checkPh("1", "name")}},
				{source: []model.Run{checkTx("Hi "), checkPh("1", "count")}, target: []model.Run{checkTx("Bonjour "), checkPh("1", "count")}},
			},
			wantReported: false,
		},
		{
			name: "placeholder-only blocks with different placeholders are not an inconsistency",
			configure: func(cfg *tools.RuleCheckConfig) {
				cfg.CheckTargetInconsistency = true
				cfg.CheckSourceInconsistency = true
			},
			pairs: []consistencyPair{
				{source: []model.Run{checkPh("1", "count")}, target: []model.Run{checkPh("1", "count")}},
				{source: []model.Run{checkPh("1", "percent")}, target: []model.Run{checkPh("1", "percent"), checkTx(" %")}},
			},
			wantReported: false,
		},
		{
			name: "a code span with different contents is a different source",
			configure: func(cfg *tools.RuleCheckConfig) {
				cfg.CheckTargetInconsistency = true
			},
			pairs: []consistencyPair{
				{
					source: []model.Run{checkTx("Run "), checkCodeOpen("1"), checkTx("make build"), checkCodeClose("1")},
					target: []model.Run{checkTx("Lancez "), checkCodeOpen("1"), checkTx("make build"), checkCodeClose("1")},
				},
				{
					source: []model.Run{checkTx("Run "), checkCodeOpen("1"), checkTx("make test"), checkCodeClose("1")},
					target: []model.Run{checkTx("Exécutez "), checkCodeOpen("1"), checkTx("make test"), checkCodeClose("1")},
				},
			},
			wantReported: false,
		},

		// Genuine positives.
		{
			name: "the same source with the same code translated two ways is reported",
			configure: func(cfg *tools.RuleCheckConfig) {
				cfg.CheckTargetInconsistency = true
			},
			pairs: []consistencyPair{
				{source: []model.Run{checkTx("Hello "), checkPh("1", "name")}, target: []model.Run{checkTx("Bonjour "), checkPh("1", "name")}},
				{source: []model.Run{checkTx("Hello "), checkPh("1", "name")}, target: []model.Run{checkTx("Salut "), checkPh("1", "name")}},
			},
			wantReported:  true,
			wantMentioned: []string{"Bonjour {name}"},
		},
		{
			name: "different sources sharing one translation is reported",
			configure: func(cfg *tools.RuleCheckConfig) {
				cfg.CheckSourceInconsistency = true
			},
			pairs: []consistencyPair{
				{source: []model.Run{checkTx("Hello "), checkPh("1", "name")}, target: []model.Run{checkTx("Bonjour "), checkPh("1", "name")}},
				{source: []model.Run{checkTx("Hi "), checkPh("1", "name")}, target: []model.Run{checkTx("Bonjour "), checkPh("1", "name")}},
			},
			wantReported:  true,
			wantMentioned: []string{"Hello {name}"},
		},
		{
			name: "a placeholder-only source translated two ways is reported",
			configure: func(cfg *tools.RuleCheckConfig) {
				cfg.CheckTargetInconsistency = true
			},
			pairs: []consistencyPair{
				{source: []model.Run{checkPh("1", "percent")}, target: []model.Run{checkPh("1", "percent"), checkTx("%")}},
				{source: []model.Run{checkPh("1", "percent")}, target: []model.Run{checkPh("1", "percent"), checkTx(" %")}},
			},
			wantReported:  true,
			wantMentioned: []string{"{percent}%"},
		},
		{
			name: "case-insensitive comparison still folds the text",
			configure: func(cfg *tools.RuleCheckConfig) {
				cfg.CheckTargetInconsistency = true
				cfg.ConsistencyCaseSensitive = false
			},
			pairs: []consistencyPair{
				{source: []model.Run{checkTx("Hello "), checkPh("1", "name")}, target: []model.Run{checkTx("Bonjour "), checkPh("1", "name")}},
				{source: []model.Run{checkTx("hello "), checkPh("1", "name")}, target: []model.Run{checkTx("Salut "), checkPh("1", "name")}},
			},
			wantReported:  true,
			wantMentioned: []string{"Bonjour {name}"},
		},
		{
			name: "case-sensitive comparison keeps the cased sources apart",
			configure: func(cfg *tools.RuleCheckConfig) {
				cfg.CheckTargetInconsistency = true
			},
			pairs: []consistencyPair{
				{source: []model.Run{checkTx("Hello "), checkPh("1", "name")}, target: []model.Run{checkTx("Bonjour "), checkPh("1", "name")}},
				{source: []model.Run{checkTx("hello "), checkPh("1", "name")}, target: []model.Run{checkTx("Salut "), checkPh("1", "name")}},
			},
			wantReported: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			cfg := tools.NewRuleCheckConfig(model.LocaleFrench)
			tt.configure(cfg)

			msgs := inconsistencyMessages(consistencyFindings(t, cfg, tt.pairs))
			if !tt.wantReported {
				assert.Empty(t, msgs, "false positive")
				return
			}
			require.Len(t, msgs, 1)
			for _, want := range tt.wantMentioned {
				assert.Contains(t, msgs[0], want, "the message lists the alternative as readable text")
			}
			assert.NotContains(t, msgs[0], string(model.ObjectReplacement), "the key's sentinel must not reach the message")
			assert.NotContains(t, msgs[0], "ph:", "the key's code signature must not reach the message")
		})
	}
}

func checkCodeOpen(id string) model.Run {
	return model.Run{PcOpen: &model.PcOpenRun{ID: id, Type: model.TypeInlineCode, Data: "`"}}
}

func checkCodeClose(id string) model.Run {
	return model.Run{PcClose: &model.PcCloseRun{ID: id, Data: "`"}}
}
