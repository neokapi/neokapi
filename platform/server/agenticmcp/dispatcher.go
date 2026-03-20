package agenticmcp

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/neokapi/neokapi/bowrain/event"
)

// QueueDispatcher implements AgentDispatcher by publishing task messages
// to named queues via QueuePublisher (Service Bus or Redis).
type QueueDispatcher struct {
	publisher event.QueuePublisher
	prefix    string // e.g. "agentic:" for Redis, "" for Service Bus
}

// NewQueueDispatcher creates a dispatcher that publishes to the given backend.
func NewQueueDispatcher(publisher event.QueuePublisher, prefix string) *QueueDispatcher {
	return &QueueDispatcher{publisher: publisher, prefix: prefix}
}

// agentTaskMessage is the JSON payload placed on the queue for worker agents.
type agentTaskMessage struct {
	ExecutionID   string `json:"execution_id"`
	WorkspaceSlug string `json:"workspace_slug"`
	AgentRole     string `json:"agent_role"`
	Persona       string `json:"persona"`
	Task          string `json:"task"`
	Locale        string `json:"locale,omitempty"`
	Priority      string `json:"priority"`
	QueuedAt      string `json:"queued_at"`
}

// Dispatch publishes a task message to the agent's queue.
func (d *QueueDispatcher) Dispatch(ctx context.Context, req DispatchRequest) (*DispatchResult, error) {
	now := time.Now().UTC()
	execID := fmt.Sprintf("exec_%d", now.UnixMilli())

	msg := agentTaskMessage{
		ExecutionID:   execID,
		WorkspaceSlug: req.WorkspaceSlug,
		AgentRole:     req.AgentRole,
		Persona:       req.Persona,
		Task:          req.Task,
		Locale:        req.Locale,
		Priority:      req.Priority,
		QueuedAt:      now.Format(time.RFC3339),
	}

	data, err := json.Marshal(msg)
	if err != nil {
		return nil, fmt.Errorf("marshal task message: %w", err)
	}

	queue := d.prefix + "agent-" + req.Persona
	if err := d.publisher.PublishMessage(ctx, queue, data); err != nil {
		return nil, fmt.Errorf("publish to %s: %w", queue, err)
	}

	return &DispatchResult{
		ExecutionID: execID,
		QueuedAt:    msg.QueuedAt,
		Queue:       queue,
	}, nil
}
