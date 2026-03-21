package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	platagent "github.com/neokapi/neokapi/platform/agent"
)

// webhookRequest is the request body for the ZeroClaw /webhook endpoint.
type webhookRequest struct {
	Message string `json:"message"`
}

// webhookResponse is the JSON response from the ZeroClaw /webhook endpoint.
type webhookResponse struct {
	Model    string `json:"model,omitempty"`
	Response string `json:"response"`
	Error    string `json:"error,omitempty"`
}

// GatewayResult holds data extracted from a gateway response.
type GatewayResult struct {
	MessageID    string
	InputTokens  int
	OutputTokens int
}

// StreamFromGateway sends a message to a ZeroClaw gateway container via the
// /webhook endpoint, converts the JSON response to SSE events, persists the
// assistant message, and writes events to the sink.
//
// Freshly spawned containers may not be ready immediately (Azure Container Apps
// returns 404/502/503 while the container is starting). The function retries
// transient errors for up to 60 seconds before giving up.
func StreamFromGateway(
	ctx context.Context,
	container *AgentContainer,
	store platagent.AgentStore,
	conversationID, userID, content, mode string,
	bravoCtx map[string]string,
	sink EventSink,
) (*GatewayResult, error) {
	// Prepend mode + context instructions to the message.
	message := contextPrefix(bravoCtx) + modePrefix(mode) + content

	client := &http.Client{Timeout: 5 * time.Minute}

	var body []byte
	deadline := time.Now().Add(60 * time.Second)
	backoff := 2 * time.Second
	for {
		payload, _ := json.Marshal(webhookRequest{Message: message})
		req, err := http.NewRequestWithContext(ctx, http.MethodPost,
			container.GatewayURL+"/webhook", bytes.NewReader(payload))
		if err != nil {
			return nil, fmt.Errorf("create gateway request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")

		resp, err := client.Do(req)
		if err != nil {
			if time.Now().Before(deadline) {
				log.Printf("Gateway request error (retrying): %v", err)
				if !sleepContext(ctx, backoff) {
					return nil, ctx.Err()
				}
				backoff = min(backoff*2, 10*time.Second)
				continue
			}
			return nil, fmt.Errorf("gateway request: %w", err)
		}

		body, err = io.ReadAll(io.LimitReader(resp.Body, 1<<20)) // 1MB limit
		resp.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("read gateway response: %w", err)
		}

		// 404/502/503 are transient during container cold-start — retry.
		if (resp.StatusCode == 404 || resp.StatusCode == 502 || resp.StatusCode == 503) && time.Now().Before(deadline) {
			log.Printf("Gateway returned %d (container starting, retrying)...", resp.StatusCode)
			if !sleepContext(ctx, backoff) {
				return nil, ctx.Err()
			}
			backoff = min(backoff*2, 10*time.Second)
			continue
		}

		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("gateway returned %d: %s", resp.StatusCode, string(body))
		}
		break
	}

	var webhookResp webhookResponse
	if err := json.Unmarshal(body, &webhookResp); err != nil {
		return nil, fmt.Errorf("parse gateway response: %w", err)
	}

	if webhookResp.Error != "" {
		return nil, fmt.Errorf("gateway error: %s", webhookResp.Error)
	}

	// Check for mode step-up markers in the response.
	if idx := strings.Index(webhookResp.Response, "[STEP_UP:"); idx >= 0 {
		if end := strings.Index(webhookResp.Response[idx:], "]"); end > 0 {
			markerJSON := webhookResp.Response[idx+9 : idx+end]
			var stepUp StepUpData
			if json.Unmarshal([]byte(markerJSON), &stepUp) == nil {
				stepUp.CurrentMode = mode
				_ = sink.WriteEvent(SSEStepUp, stepUp)
				// Remove the marker from the response text.
				webhookResp.Response = strings.Replace(webhookResp.Response, webhookResp.Response[idx:idx+end+1], "", 1)
				webhookResp.Response = strings.TrimSpace(webhookResp.Response)
			}
		}
	}

	// Persist the assistant message.
	msg := &platagent.Message{
		ConversationID: conversationID,
		Role:           platagent.RoleAssistant,
		Content:        webhookResp.Response,
	}
	if err := store.AddMessage(ctx, msg); err != nil {
		return nil, fmt.Errorf("persist assistant message: %w", err)
	}

	// Emit SSE events to the sink (same protocol the frontend expects).
	_ = sink.WriteEvent(SSEMessageStart, MessageStartData{ID: msg.ID, Role: "assistant"})
	_ = sink.WriteEvent(SSEContentDelta, ContentDeltaData{Delta: webhookResp.Response})
	_ = sink.WriteEvent(SSEMessageEnd, MessageEndData{ID: msg.ID})

	return &GatewayResult{MessageID: msg.ID}, nil
}

// streamFromGateway is the AgentService method that delegates to the
// standalone StreamFromGateway function.
func (s *AgentService) streamFromGateway(
	ctx context.Context,
	container *AgentContainer,
	conversationID, userID, content, mode string,
	bravoCtx map[string]string,
	sse SSEWriter,
) (*GatewayResult, error) {
	return StreamFromGateway(ctx, container, s.store, conversationID, userID, content, mode, bravoCtx, sse)
}

// contextPrefix returns a context instruction prefix describing the user's current location.
func contextPrefix(ctx map[string]string) string {
	if len(ctx) == 0 {
		return ""
	}
	parts := "[CONTEXT:"
	if v, ok := ctx["project_id"]; ok && v != "" {
		parts += " project=" + v
	}
	if v, ok := ctx["stream"]; ok && v != "" {
		parts += " stream=" + v
	}
	if v, ok := ctx["item_id"]; ok && v != "" {
		parts += " item=" + v
	}
	if parts == "[CONTEXT:" {
		return ""
	}
	return parts + "]\n"
}

// sleepContext sleeps for the given duration, returning false if the context
// is cancelled before the sleep completes.
func sleepContext(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return true
	case <-ctx.Done():
		return false
	}
}

// modePrefix returns a system instruction prefix based on the interaction mode.
func modePrefix(mode string) string {
	switch mode {
	case "ask":
		return `(Mode: Ask) You are answering questions only. Help the user understand their projects, TM, terminology, and formats. Explain what actions are possible but do not execute any changes.

If the user asks you to perform a mutating action (translate, edit, delete, run flows, manage files, push/pull), respond with a step-up marker followed by a helpful explanation:
[STEP_UP:{"required_mode":"coworker","action":"<brief description of what the user wants>"}]
Then explain what you can help with in Ask mode instead.

`
	case "coworker":
		return "(Mode: Co-worker) You can manage projects, run flows, push/pull content, and edit terminology. Confirm before any destructive operations like deletes or overwrites.\n\n"
	case "bravo":
		return `(Mode: Brand Voice) Focus on reviewing content for brand voice compliance, suggesting improvements, and running brand voice QA. Use check_vocabulary and get_voice_guide tools.

If the user asks you to perform actions beyond brand voice scope (translate, manage files, run non-brand flows), respond with a step-up marker:
[STEP_UP:{"required_mode":"coworker","action":"<brief description>"}]
Then explain what you can help with in Voice mode instead.

`
	default:
		return ""
	}
}
