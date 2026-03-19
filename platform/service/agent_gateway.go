package service

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	platagent "github.com/neokapi/neokapi/platform/agent"
)

// gatewayMessage is the request body sent to the ZeroClaw gateway.
type gatewayMessage struct {
	Content        string `json:"content"`
	ConversationID string `json:"conversation_id"`
}

// GatewayResult holds data extracted from a relayed gateway SSE stream.
type GatewayResult struct {
	MessageID    string
	InputTokens  int
	OutputTokens int
}

// StreamFromGateway sends a message to a ZeroClaw gateway container and relays
// the SSE response to the given sink. It also persists the assistant message
// to the store. This is a standalone function usable by both the API server
// (direct mode) and the worker (queue mode).
func StreamFromGateway(
	ctx context.Context,
	container *AgentContainer,
	store platagent.AgentStore,
	conversationID, userID, content string,
	sink EventSink,
) (*GatewayResult, error) {
	payload, _ := json.Marshal(gatewayMessage{
		Content:        content,
		ConversationID: conversationID,
	})

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		container.GatewayURL+"/v1/messages", bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("create gateway request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")

	client := &http.Client{Timeout: 5 * time.Minute}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("gateway request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("gateway returned %d: %s", resp.StatusCode, string(body))
	}

	return relayGatewaySSE(ctx, resp.Body, store, conversationID, sink)
}

// relayGatewaySSE reads SSE events from the gateway response and relays
// them to the sink. It captures the full assistant message for persistence.
func relayGatewaySSE(
	ctx context.Context,
	body io.Reader,
	store platagent.AgentStore,
	conversationID string,
	sink EventSink,
) (*GatewayResult, error) {
	scanner := bufio.NewScanner(body)
	var contentBuf strings.Builder
	var assistantMsgID string
	var currentEvent string
	var usage *MessageUsage

	for scanner.Scan() {
		line := scanner.Text()

		if strings.HasPrefix(line, "event: ") {
			currentEvent = strings.TrimPrefix(line, "event: ")
			continue
		}

		if strings.HasPrefix(line, "data: ") {
			data := strings.TrimPrefix(line, "data: ")

			switch currentEvent {
			case SSEMessageStart:
				var d MessageStartData
				if json.Unmarshal([]byte(data), &d) == nil {
					assistantMsgID = d.ID
				}
				_ = sink.WriteEvent(SSEMessageStart, json.RawMessage(data))

			case SSEContentDelta:
				var d ContentDeltaData
				if json.Unmarshal([]byte(data), &d) == nil {
					contentBuf.WriteString(d.Delta)
				}
				_ = sink.WriteEvent(SSEContentDelta, json.RawMessage(data))

			case SSEToolCallStart, SSEToolCallEnd, SSENeedsApproval:
				_ = sink.WriteEvent(currentEvent, json.RawMessage(data))

			case SSEMessageEnd:
				var d MessageEndData
				if json.Unmarshal([]byte(data), &d) == nil {
					usage = d.Usage
				}
				_ = sink.WriteEvent(SSEMessageEnd, json.RawMessage(data))

			case SSEError:
				_ = sink.WriteEvent(SSEError, json.RawMessage(data))

			default:
				_ = sink.WriteEvent(currentEvent, json.RawMessage(data))
			}

			currentEvent = ""
			continue
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read gateway SSE: %w", err)
	}

	result := &GatewayResult{MessageID: assistantMsgID}
	if usage != nil {
		result.InputTokens = usage.InputTokens
		result.OutputTokens = usage.OutputTokens
	}

	// Persist the assistant message.
	if contentBuf.Len() > 0 {
		msg := &platagent.Message{
			ConversationID: conversationID,
			Role:           platagent.RoleAssistant,
			Content:        contentBuf.String(),
			InputTokens:    result.InputTokens,
			OutputTokens:   result.OutputTokens,
		}
		if assistantMsgID != "" {
			msg.ID = assistantMsgID
		}
		if err := store.AddMessage(ctx, msg); err != nil {
			return nil, fmt.Errorf("persist assistant message: %w", err)
		}
		result.MessageID = msg.ID
	}

	return result, nil
}

// streamFromGateway is the AgentService method that delegates to the
// standalone StreamFromGateway function.
func (s *AgentService) streamFromGateway(
	ctx context.Context,
	container *AgentContainer,
	conversationID, userID, content string,
	sse SSEWriter,
) (*GatewayResult, error) {
	return StreamFromGateway(ctx, container, s.store, conversationID, userID, content, sse)
}
