package agenticmcp

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// capturingPublisher records published messages for testing.
type capturingPublisher struct {
	messages []publishedMessage
}

type publishedMessage struct {
	Queue string
	Data  []byte
}

func (p *capturingPublisher) PublishMessage(_ context.Context, queue string, data []byte) error {
	p.messages = append(p.messages, publishedMessage{Queue: queue, Data: data})
	return nil
}

func TestQueueDispatcher_Dispatch(t *testing.T) {
	pub := &capturingPublisher{}
	d := NewQueueDispatcher(pub, "")

	result, err := d.Dispatch(context.Background(), DispatchRequest{
		WorkspaceSlug: "excalidraw-l10n",
		AgentRole:     "translator",
		Persona:       "sophie-translator",
		Task:          "Translate 142 fr-FR blocks",
		Locale:        "fr-FR",
		Priority:      "normal",
	})
	require.NoError(t, err)

	assert.NotEmpty(t, result.ExecutionID)
	assert.Equal(t, "agent-sophie-translator", result.Queue)
	assert.NotEmpty(t, result.QueuedAt)

	// Verify the published message.
	require.Len(t, pub.messages, 1)
	assert.Equal(t, "agent-sophie-translator", pub.messages[0].Queue)

	var msg agentTaskMessage
	require.NoError(t, json.Unmarshal(pub.messages[0].Data, &msg))
	assert.Equal(t, "excalidraw-l10n", msg.WorkspaceSlug)
	assert.Equal(t, "translator", msg.AgentRole)
	assert.Equal(t, "sophie-translator", msg.Persona)
	assert.Equal(t, "Translate 142 fr-FR blocks", msg.Task)
	assert.Equal(t, "fr-FR", msg.Locale)
	assert.Equal(t, "normal", msg.Priority)
}

func TestQueueDispatcher_DispatchWithPrefix(t *testing.T) {
	pub := &capturingPublisher{}
	d := NewQueueDispatcher(pub, "agentic:")

	result, err := d.Dispatch(context.Background(), DispatchRequest{
		WorkspaceSlug: "excalidraw-l10n",
		AgentRole:     "developer",
		Persona:       "alex-developer",
		Task:          "Push v0.18.1",
		Priority:      "high",
	})
	require.NoError(t, err)

	assert.Equal(t, "agentic:agent-alex-developer", result.Queue)
	require.Len(t, pub.messages, 1)
	assert.Equal(t, "agentic:agent-alex-developer", pub.messages[0].Queue)
}
