package agent

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	platagent "github.com/neokapi/neokapi/bowrain/core/agent"
)

func TestSQLiteStoreRecordUsage(t *testing.T) {
	store, err := NewSQLiteStore(":memory:")
	require.NoError(t, err)
	defer store.Close()

	ctx := t.Context()

	// Record token usage.
	rec := &platagent.UsageRecord{
		WorkspaceID:    "ws1",
		UserID:         "user1",
		ConversationID: "conv1",
		MessageID:      "msg1",
		Kind:           "tokens",
		InputTokens:    1500,
		OutputTokens:   500,
	}
	require.NoError(t, store.RecordUsage(ctx, rec))
	assert.NotEmpty(t, rec.ID)

	// Record container time.
	rec2 := &platagent.UsageRecord{
		WorkspaceID:    "ws1",
		UserID:         "user1",
		ConversationID: "conv1",
		Kind:           "container_time",
		DurationSec:    120.5,
	}
	require.NoError(t, store.RecordUsage(ctx, rec2))
}

func TestSQLiteStoreGetUsageSummary(t *testing.T) {
	store, err := NewSQLiteStore(":memory:")
	require.NoError(t, err)
	defer store.Close()

	ctx := t.Context()
	now := time.Now().UTC()

	// Insert some usage records.
	records := []*platagent.UsageRecord{
		{WorkspaceID: "ws1", UserID: "user1", ConversationID: "conv1", MessageID: "msg1", Kind: "tokens", InputTokens: 100, OutputTokens: 50},
		{WorkspaceID: "ws1", UserID: "user1", ConversationID: "conv1", MessageID: "msg2", Kind: "tokens", InputTokens: 200, OutputTokens: 75},
		{WorkspaceID: "ws1", UserID: "user1", ConversationID: "conv1", Kind: "container_time", DurationSec: 60.0},
		{WorkspaceID: "ws2", UserID: "user2", ConversationID: "conv2", MessageID: "msg3", Kind: "tokens", InputTokens: 999, OutputTokens: 999},
	}
	for _, r := range records {
		require.NoError(t, store.RecordUsage(ctx, r))
	}

	// Query ws1.
	from := now.Add(-1 * time.Hour)
	to := now.Add(1 * time.Hour)
	summary, err := store.GetUsageSummary(ctx, "ws1", from, to)
	require.NoError(t, err)
	assert.Equal(t, "ws1", summary.WorkspaceID)
	assert.Equal(t, int64(300), summary.TotalInputTokens)
	assert.Equal(t, int64(125), summary.TotalOutputTokens)
	assert.InDelta(t, 60.0, summary.TotalContainerSec, 0.1)
	assert.Equal(t, int64(2), summary.MessageCount) // 2 token records

	// Query ws2.
	summary2, err := store.GetUsageSummary(ctx, "ws2", from, to)
	require.NoError(t, err)
	assert.Equal(t, int64(999), summary2.TotalInputTokens)
	assert.Equal(t, int64(999), summary2.TotalOutputTokens)

	// Query with time range that excludes records.
	summary3, err := store.GetUsageSummary(ctx, "ws1", now.Add(-2*time.Hour), now.Add(-1*time.Hour))
	require.NoError(t, err)
	assert.Equal(t, int64(0), summary3.TotalInputTokens)
}

func TestSQLiteStoreMessageTokenFields(t *testing.T) {
	store, err := NewSQLiteStore(":memory:")
	require.NoError(t, err)
	defer store.Close()

	ctx := t.Context()

	conv := &platagent.Conversation{WorkspaceID: "ws1", UserID: "user1"}
	require.NoError(t, store.CreateConversation(ctx, conv))

	msg := &platagent.Message{
		ConversationID: conv.ID,
		Role:           platagent.RoleAssistant,
		Content:        "Hello!",
		InputTokens:    500,
		OutputTokens:   200,
	}
	require.NoError(t, store.AddMessage(ctx, msg))

	msgs, err := store.ListMessages(ctx, conv.ID, 10, 0)
	require.NoError(t, err)
	require.Len(t, msgs, 1)
	assert.Equal(t, 500, msgs[0].InputTokens)
	assert.Equal(t, 200, msgs[0].OutputTokens)
}
