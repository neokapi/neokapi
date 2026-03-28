package jobs

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/neokapi/neokapi/bowrain/billing"
	"github.com/neokapi/neokapi/bowrain/service"
	platagent "github.com/neokapi/neokapi/platform/agent"
	platauth "github.com/neokapi/neokapi/platform/auth"
)

// AgentWorkerDeps holds dependencies for the agent job worker.
type AgentWorkerDeps struct {
	Queue        Queue                // Service Bus queue for bravo-jobs
	AgentStore   platagent.AgentStore // conversations + messages
	Pool         *service.AgentPool   // container lifecycle
	PubSub       *service.AgentPubSub // Redis pub/sub for SSE relay
	JWTSecret    string               // Bowrain JWT secret for creating MCP auth tokens
	BillingHooks *billing.UsageHooks  // optional; nil disables billing credit deduction
	QuotaStore   *PgQuotaStore        // optional; nil disables runner usage recording
}

// RunAgentWorker runs the agent job processing loop.
// It dequeues agent job messages, spawns containers, streams responses,
// and publishes SSE events to Redis for the API server to relay.
func RunAgentWorker(ctx context.Context, deps *AgentWorkerDeps) error {
	log.Println("Agent worker started")
	for {
		rawMsg, ack, _, err := deps.Queue.Dequeue(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			log.Printf("Agent worker: dequeue error: %v", err)
			sleepCtx(ctx, 2*time.Second)
			continue
		}

		if err := processAgentJob(ctx, deps, rawMsg); err != nil {
			log.Printf("Agent worker: job failed: %v", err)
			// Publish error to Redis so the client sees it.
			convID := extractConversationID(rawMsg)
			errData, _ := json.Marshal(service.ErrorData{Error: err.Error()})
			_ = deps.PubSub.Publish(ctx, convID, service.SSEEvent{
				Event: service.SSEError,
				Data:  errData,
			})
		}
		ack()
	}
}

func processAgentJob(ctx context.Context, deps *AgentWorkerDeps, rawMessage string) error {
	var job AgentJobMessage
	if err := json.Unmarshal([]byte(rawMessage), &job); err != nil {
		return fmt.Errorf("unmarshal agent job: %w", err)
	}

	log.Printf("Agent worker: processing conversation=%s user=%s mode=%s", job.ConversationID, job.UserID, job.Mode)

	// Create a proper Bowrain JWT for MCP authentication.
	// The MCP server validates these using the same JWT secret as the REST API.
	agentJWT, err := platauth.GenerateToken(&platauth.User{
		ID:    job.UserID,
		Email: "bravo-agent@bowrain.internal",
		Name:  "@bravo",
	}, deps.JWTSecret, 30*time.Minute)
	if err != nil {
		return fmt.Errorf("create agent JWT: %w", err)
	}

	// Acquire a container from the pool.
	containerStart := time.Now()
	container, err := deps.Pool.Acquire(ctx, service.ContainerConfig{
		ConversationID: job.ConversationID,
		WorkspaceID:    job.WorkspaceID,
		UserID:         job.UserID,
		AgentToken:     agentJWT,
	})
	if err != nil {
		return fmt.Errorf("acquire container: %w", err)
	}

	// Stream from gateway, publishing SSE events to Redis.
	sink := &redisSinkWriter{
		pubsub:         deps.PubSub,
		conversationID: job.ConversationID,
		ctx:            ctx,
	}

	result, err := service.StreamFromGateway(ctx, container, deps.AgentStore, job.ConversationID, job.UserID, job.Content, job.Mode, nil, sink)
	containerDuration := time.Since(containerStart)
	if err != nil {
		return fmt.Errorf("gateway stream: %w", err)
	}

	// Record container time usage and deduct billing credits.
	if deps.QuotaStore != nil && job.WorkspaceID != "" {
		_ = deps.QuotaStore.RecordRunnerUsage(ctx, RunnerUsageRecord{
			WorkspaceID: job.WorkspaceID,
			Operation:   "bravo_container",
			DurationSec: containerDuration.Seconds(),
			ReferenceID: job.ConversationID,
		})
	}
	if deps.BillingHooks != nil && job.WorkspaceID != "" {
		deps.BillingHooks.DeductContainerTime(ctx, job.WorkspaceID, containerDuration, job.ConversationID)
	}

	// Record token usage.
	if result != nil && (result.InputTokens > 0 || result.OutputTokens > 0) {
		_ = deps.AgentStore.RecordUsage(ctx, &platagent.UsageRecord{
			WorkspaceID:    job.WorkspaceID,
			UserID:         job.UserID,
			ConversationID: job.ConversationID,
			MessageID:      result.MessageID,
			Kind:           "tokens",
			InputTokens:    result.InputTokens,
			OutputTokens:   result.OutputTokens,
		})
	}

	// Update conversation timestamp.
	conv, _ := deps.AgentStore.GetConversation(ctx, job.ConversationID)
	if conv != nil {
		_ = deps.AgentStore.UpdateConversation(ctx, conv)
	}

	return nil
}

// redisSinkWriter implements service.EventSink by publishing events to Redis.
type redisSinkWriter struct {
	pubsub         *service.AgentPubSub
	conversationID string
	ctx            context.Context
}

func (w *redisSinkWriter) WriteEvent(event string, data any) error {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return err
	}
	return w.pubsub.Publish(w.ctx, w.conversationID, service.SSEEvent{
		Event: event,
		Data:  jsonData,
	})
}

// extractConversationID tries to parse the conversation ID from a raw job message.
func extractConversationID(raw string) string {
	var job AgentJobMessage
	if json.Unmarshal([]byte(raw), &job) == nil && job.ConversationID != "" {
		return job.ConversationID
	}
	return "unknown"
}
