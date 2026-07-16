package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCohenKappa(t *testing.T) {
	// Perfect agreement with mixed marginals.
	assert.InDelta(t, 1.0, cohenKappa(10, 5, 5, 10), 1e-9)
	// Chance-level agreement: judge and human each say yes half the time,
	// agreeing on half — kappa 0 while raw agreement reads 0.5.
	assert.InDelta(t, 0.0, cohenKappa(5, 5, 5, 10), 1e-9)
	// A judge that says yes to everything against a human that says yes to 8 of
	// 10: raw agreement flatters it at 0.8; kappa reports 0.
	assert.InDelta(t, 0.0, cohenKappa(8, 10, 8, 10), 1e-9)
	// Unanimous and identical.
	assert.InDelta(t, 1.0, cohenKappa(10, 10, 10, 10), 1e-9)
	// Unanimous judge, unanimous human, opposite verdicts.
	assert.InDelta(t, 0.0, cohenKappa(0, 10, 0, 10), 1e-9)
}

func TestModelFamilyFollowsTheWeights(t *testing.T) {
	// The model id decides, not the route: Bedrock serves Anthropic weights.
	assert.Equal(t, "anthropic", modelFamily("bedrock", "eu.anthropic.claude-sonnet-4-6"))
	assert.Equal(t, "anthropic", modelFamily("claude-code", "sonnet"))
	assert.Equal(t, "google", modelFamily("gemini", "gemini-3.5-flash"))
	assert.Equal(t, "openai", modelFamily("azureopenai", "gpt-4o"))
	// Provider fallback when the model is empty (provider default model).
	assert.Equal(t, "anthropic", modelFamily("anthropic", ""))
	assert.Empty(t, modelFamily("demo", ""))
}

func TestSameFamilyJudgingIsRefusedNotWarned(t *testing.T) {
	j := &judgeTarget{provider: "gemini", model: "gemini-3.5-pro"}
	rec := j.record(modelTarget{provider: "gemini", model: "gemini-3.5-flash"})
	assert.True(t, rec.SkippedSameFamily, "self-preference bias: a same-family verdict is not evidence")

	rec = j.record(modelTarget{provider: "anthropic", model: "claude-sonnet-5"})
	assert.False(t, rec.SkippedSameFamily)
}

func TestRubricDigestShape(t *testing.T) {
	assert.Len(t, rubricDigest(), 12)
}
