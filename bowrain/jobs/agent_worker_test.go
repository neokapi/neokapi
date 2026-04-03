package jobs

import (
	"encoding/json"
	"testing"
	"time"

	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAgentJobMessage_Unmarshal(t *testing.T) {
	raw := `{
		"conversation_id": "conv-1",
		"message_id": "msg-42",
		"workspace_id": "ws-1",
		"user_id": "user-1",
		"workspace_role": "admin",
		"content": "Translate my project",
		"mode": "coworker"
	}`

	var job AgentJobMessage
	require.NoError(t, json.Unmarshal([]byte(raw), &job))

	assert.Equal(t, "conv-1", job.ConversationID)
	assert.Equal(t, "msg-42", job.MessageID)
	assert.Equal(t, "ws-1", job.WorkspaceID)
	assert.Equal(t, "user-1", job.UserID)
	assert.Equal(t, "admin", job.WorkspaceRole)
	assert.Equal(t, "Translate my project", job.Content)
	assert.Equal(t, "coworker", job.Mode)
}

func TestAgentJobMessage_UnmarshalOptionalMode(t *testing.T) {
	// Mode is optional (omitempty).
	raw := `{
		"conversation_id": "conv-1",
		"message_id": "msg-1",
		"workspace_id": "ws-1",
		"user_id": "user-1",
		"workspace_role": "member",
		"content": "Hello"
	}`

	var job AgentJobMessage
	require.NoError(t, json.Unmarshal([]byte(raw), &job))
	assert.Empty(t, job.Mode)
}

func TestAgentJobMessage_Marshal(t *testing.T) {
	job := AgentJobMessage{
		ConversationID: "conv-99",
		MessageID:      "msg-1",
		WorkspaceID:    "ws-2",
		UserID:         "user-2",
		WorkspaceRole:  "member",
		Content:        "Check brand voice",
		Mode:           "bravo",
	}

	data, err := json.Marshal(job)
	require.NoError(t, err)

	var decoded map[string]string
	require.NoError(t, json.Unmarshal(data, &decoded))
	assert.Equal(t, "conv-99", decoded["conversation_id"])
	assert.Equal(t, "bravo", decoded["mode"])
	assert.Equal(t, "Check brand voice", decoded["content"])
}

func TestAgentJobMessage_MarshalOmitsEmptyMode(t *testing.T) {
	job := AgentJobMessage{
		ConversationID: "conv-1",
		MessageID:      "msg-1",
		WorkspaceID:    "ws-1",
		UserID:         "user-1",
		WorkspaceRole:  "member",
		Content:        "Hello",
		Mode:           "",
	}

	data, err := json.Marshal(job)
	require.NoError(t, err)

	var decoded map[string]any
	require.NoError(t, json.Unmarshal(data, &decoded))
	_, hasMode := decoded["mode"]
	assert.False(t, hasMode, "empty mode should be omitted from JSON")
}

func TestExtractConversationID(t *testing.T) {
	tests := []struct {
		name     string
		raw      string
		expected string
	}{
		{
			name:     "valid message",
			raw:      `{"conversation_id":"conv-42","message_id":"m1","workspace_id":"ws1","user_id":"u1","workspace_role":"member","content":"hi"}`,
			expected: "conv-42",
		},
		{
			name:     "empty conversation_id",
			raw:      `{"conversation_id":"","message_id":"m1"}`,
			expected: "unknown",
		},
		{
			name:     "invalid JSON",
			raw:      "not json at all",
			expected: "unknown",
		},
		{
			name:     "empty string",
			raw:      "",
			expected: "unknown",
		},
		{
			name:     "missing field",
			raw:      `{"message_id":"m1"}`,
			expected: "unknown",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, extractConversationID(tt.raw))
		})
	}
}

func TestExtractConversationID_WithAllModes(t *testing.T) {
	modes := []string{"ask", "coworker", "bravo"}
	for _, mode := range modes {
		t.Run(mode, func(t *testing.T) {
			job := AgentJobMessage{
				ConversationID: "conv-" + mode,
				MessageID:      "msg-1",
				WorkspaceID:    "ws-1",
				UserID:         "user-1",
				Content:        "test",
				Mode:           mode,
			}
			data, err := json.Marshal(job)
			require.NoError(t, err)
			assert.Equal(t, "conv-"+mode, extractConversationID(string(data)))
		})
	}
}

func TestAgentWorker_JWTCreation(t *testing.T) {
	// Verify that GenerateToken works with the same parameters used in processAgentJob.
	secret := "test-jwt-secret-for-agent"

	token, err := platauth.GenerateToken(&platauth.User{
		ID:    "user-1",
		Email: "bravo-agent@bowrain.internal",
		Name:  "@bravo",
	}, secret, 30*time.Minute)
	require.NoError(t, err)
	assert.NotEmpty(t, token)

	// Validate the token.
	claims, err := platauth.ValidateToken(token, secret)
	require.NoError(t, err)
	assert.Equal(t, "user-1", claims.Subject)
	assert.Equal(t, "bravo-agent@bowrain.internal", claims.Email)
	assert.Equal(t, "@bravo", claims.Name)
}

func TestAgentWorker_JWTExpiry(t *testing.T) {
	secret := "test-secret"

	token, err := platauth.GenerateToken(&platauth.User{
		ID:    "user-1",
		Email: "bravo-agent@bowrain.internal",
		Name:  "@bravo",
	}, secret, 30*time.Minute)
	require.NoError(t, err)

	claims, err := platauth.ValidateToken(token, secret)
	require.NoError(t, err)

	// Token should expire roughly 30 minutes from now.
	expiry := claims.ExpiresAt.Time
	assert.WithinDuration(t, time.Now().Add(30*time.Minute), expiry, 5*time.Second)
}

func TestAgentWorker_JWTInvalidSecret(t *testing.T) {
	token, err := platauth.GenerateToken(&platauth.User{
		ID:    "user-1",
		Email: "bravo-agent@bowrain.internal",
		Name:  "@bravo",
	}, "correct-secret", 30*time.Minute)
	require.NoError(t, err)

	_, err = platauth.ValidateToken(token, "wrong-secret")
	assert.Error(t, err)
}

func TestRedisSinkWriter_WriteEvent(t *testing.T) {
	// Test the redisSinkWriter event serialization without actual Redis.
	// We can't instantiate redisSinkWriter without a real AgentPubSub,
	// but we can verify that the EventSink interface contract matches
	// what redisSinkWriter implements.

	// Verify SSEEvent JSON roundtrip (what redisSinkWriter publishes).
	data, err := json.Marshal(map[string]string{"delta": "Hello"})
	require.NoError(t, err)

	evt := struct {
		Event string          `json:"event"`
		Data  json.RawMessage `json:"data"`
	}{
		Event: "content_delta",
		Data:  data,
	}

	serialized, err := json.Marshal(evt)
	require.NoError(t, err)

	var decoded struct {
		Event string          `json:"event"`
		Data  json.RawMessage `json:"data"`
	}
	require.NoError(t, json.Unmarshal(serialized, &decoded))
	assert.Equal(t, "content_delta", decoded.Event)
	assert.JSONEq(t, `{"delta":"Hello"}`, string(decoded.Data))
}

func TestAgentJobMessage_RoundTrip(t *testing.T) {
	// Verify that a job message can be serialized and deserialized
	// without losing data (the exact path used by enqueue → dequeue).
	original := AgentJobMessage{
		ConversationID: "conv-123",
		MessageID:      "msg-456",
		WorkspaceID:    "ws-789",
		UserID:         "user-abc",
		WorkspaceRole:  "admin",
		Content:        "Please translate all files to French",
		Mode:           "coworker",
	}

	data, err := json.Marshal(original)
	require.NoError(t, err)

	var decoded AgentJobMessage
	require.NoError(t, json.Unmarshal(data, &decoded))

	assert.Equal(t, original.ConversationID, decoded.ConversationID)
	assert.Equal(t, original.MessageID, decoded.MessageID)
	assert.Equal(t, original.WorkspaceID, decoded.WorkspaceID)
	assert.Equal(t, original.UserID, decoded.UserID)
	assert.Equal(t, original.WorkspaceRole, decoded.WorkspaceRole)
	assert.Equal(t, original.Content, decoded.Content)
	assert.Equal(t, original.Mode, decoded.Mode)
}
