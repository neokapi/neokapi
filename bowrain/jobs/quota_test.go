package jobs

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRunnerUsageRecord_Fields(t *testing.T) {
	rec := RunnerUsageRecord{
		WorkspaceID: "ws-123",
		ProjectID:   "proj-456",
		Operation:   "bravo_container",
		DurationSec: 42.5,
		ReferenceID: "conv-789",
	}
	assert.Equal(t, "ws-123", rec.WorkspaceID)
	assert.Equal(t, "proj-456", rec.ProjectID)
	assert.Equal(t, "bravo_container", rec.Operation)
	assert.InDelta(t, 42.5, rec.DurationSec, 0.001)
	assert.Equal(t, "conv-789", rec.ReferenceID)
}

func TestAIUsageRecord_IncludesWorkspaceID(t *testing.T) {
	rec := AIUsageRecord{
		WorkspaceSlug: "my-ws",
		WorkspaceID:   "ws-uuid-123",
		ProjectID:     "proj-1",
		Model:         "claude-sonnet",
		Operation:     "translate",
		PromptTokens:  100,
		OutputTokens:  50,
		TotalTokens:   150,
	}
	assert.Equal(t, "ws-uuid-123", rec.WorkspaceID)
	assert.Equal(t, "translate", rec.Operation)
}

func TestModelUsage_Fields(t *testing.T) {
	mu := ModelUsage{
		Model:        "gpt-4o",
		Operation:    "qa_check",
		PromptTokens: 1000,
		OutputTokens: 200,
		TotalTokens:  1200,
		CallCount:    5,
	}
	assert.Equal(t, "gpt-4o", mu.Model)
	assert.Equal(t, int64(1200), mu.TotalTokens)
	assert.Equal(t, int64(5), mu.CallCount)
}

func TestRunnerUsageSummary_Fields(t *testing.T) {
	ru := RunnerUsageSummary{
		Operation:    "auto_translate",
		TotalSeconds: 845.3,
		Count:        15,
	}
	assert.Equal(t, "auto_translate", ru.Operation)
	assert.InDelta(t, 845.3, ru.TotalSeconds, 0.01)
	assert.Equal(t, int64(15), ru.Count)
}
