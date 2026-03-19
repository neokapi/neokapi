package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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
func StreamFromGateway(
	ctx context.Context,
	container *AgentContainer,
	store platagent.AgentStore,
	conversationID, userID, content, mode string,
	sink EventSink,
) (*GatewayResult, error) {
	// Prepend mode instructions to the message.
	message := modePrefix(mode) + content
	payload, _ := json.Marshal(webhookRequest{Message: message})

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		container.GatewayURL+"/webhook", bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("create gateway request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 5 * time.Minute}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("gateway request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20)) // 1MB limit
	if err != nil {
		return nil, fmt.Errorf("read gateway response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("gateway returned %d: %s", resp.StatusCode, string(body))
	}

	var webhookResp webhookResponse
	if err := json.Unmarshal(body, &webhookResp); err != nil {
		return nil, fmt.Errorf("parse gateway response: %w", err)
	}

	if webhookResp.Error != "" {
		return nil, fmt.Errorf("gateway error: %s", webhookResp.Error)
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
	sse SSEWriter,
) (*GatewayResult, error) {
	return StreamFromGateway(ctx, container, s.store, conversationID, userID, content, mode, sse)
}

// modePrefix returns a system instruction prefix based on the interaction mode.
func modePrefix(mode string) string {
	switch mode {
	case "ask":
		return "[MODE: Ask — You are in expert Q&A mode. Answer questions about the Bowrain platform, localization, TM, terminology, and formats. Do NOT perform any mutable operations (no creating, updating, deleting, pushing, or pulling). If the user asks you to perform an action, explain what they could do but do not execute it.]\n\n"
	case "coworker":
		return "[MODE: Co-worker — You are in full assistant mode. You can manage projects, run flows, push/pull content, edit terminology, and perform any operation the user requests. Always confirm before destructive operations (deletes, overwrites, pushes).]\n\n"
	case "bravo":
		return "[MODE: Brand Voice — You are in brand voice review mode. Focus on reviewing content for brand voice compliance, suggesting improvements, and running brand voice QA flows. Use the check_vocabulary and get_voice_guide tools when relevant.]\n\n"
	default:
		return ""
	}
}
