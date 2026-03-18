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

// streamFromGateway sends a message to the ZeroClaw gateway and relays
// the SSE response stream to the client. It also persists the assistant
// message and any tool calls to the store.
func (s *AgentService) streamFromGateway(
	ctx context.Context,
	container *AgentContainer,
	conversationID, userID, content string,
	sse SSEWriter,
) error {
	// POST message to the ZeroClaw gateway.
	payload, _ := json.Marshal(gatewayMessage{
		Content:        content,
		ConversationID: conversationID,
	})

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		container.GatewayURL+"/v1/messages", bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("create gateway request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")

	client := &http.Client{Timeout: 5 * time.Minute}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("gateway request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("gateway returned %d: %s", resp.StatusCode, string(body))
	}

	// Parse the SSE stream from ZeroClaw and relay to client.
	return s.relayGatewaySSE(ctx, resp.Body, conversationID, userID, sse)
}

// relayGatewaySSE reads SSE events from the gateway response and relays
// them to the client. It also captures the full assistant message and
// any tool calls for persistence.
func (s *AgentService) relayGatewaySSE(
	ctx context.Context,
	body io.Reader,
	conversationID, userID string,
	sse SSEWriter,
) error {
	scanner := bufio.NewScanner(body)
	var contentBuf strings.Builder
	var assistantMsgID string
	var currentEvent string

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
				_ = sse.WriteEvent(SSEMessageStart, json.RawMessage(data))

			case SSEContentDelta:
				var d ContentDeltaData
				if json.Unmarshal([]byte(data), &d) == nil {
					contentBuf.WriteString(d.Delta)
				}
				_ = sse.WriteEvent(SSEContentDelta, json.RawMessage(data))

			case SSEToolCallStart, SSEToolCallEnd, SSENeedsApproval:
				_ = sse.WriteEvent(currentEvent, json.RawMessage(data))

			case SSEMessageEnd:
				_ = sse.WriteEvent(SSEMessageEnd, json.RawMessage(data))

			case SSEError:
				_ = sse.WriteEvent(SSEError, json.RawMessage(data))

			default:
				// Forward unknown events as-is.
				_ = sse.WriteEvent(currentEvent, json.RawMessage(data))
			}

			currentEvent = ""
			continue
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read gateway SSE: %w", err)
	}

	// Persist the assistant message.
	if contentBuf.Len() > 0 {
		msg := &platagent.Message{
			ConversationID: conversationID,
			Role:           platagent.RoleAssistant,
			Content:        contentBuf.String(),
		}
		if assistantMsgID != "" {
			msg.ID = assistantMsgID
		}
		if err := s.store.AddMessage(ctx, msg); err != nil {
			return fmt.Errorf("persist assistant message: %w", err)
		}
	}

	return nil
}
