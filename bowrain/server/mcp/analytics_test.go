package mcp

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockTracker records analytics events for testing.
type mockTracker struct {
	mu     sync.Mutex
	events []trackedEvent
}

type trackedEvent struct {
	UserID     string
	Event      string
	Properties map[string]any
}

func (m *mockTracker) TrackEvent(userID, event string, properties map[string]any) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.events = append(m.events, trackedEvent{
		UserID:     userID,
		Event:      event,
		Properties: properties,
	})
}

func (m *mockTracker) lastEvent() trackedEvent {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.events) == 0 {
		return trackedEvent{}
	}
	return m.events[len(m.events)-1]
}

func TestTrackSessionStart(t *testing.T) {
	tracker := &mockTracker{}
	ms := &MCPServer{tracker: tracker}

	req := &mcp.ServerRequest[*mcp.InitializeParams]{
		Extra: &mcp.RequestExtra{
			TokenInfo: &auth.TokenInfo{UserID: "user-1"},
		},
	}
	ms.trackSessionStart(req)

	got := tracker.lastEvent()
	assert.Equal(t, "mcp_session_start", got.Event)
	assert.Equal(t, "user-1", got.UserID)
	assert.Equal(t, "streamable-http", got.Properties["transport"])
}

func TestTrackSessionStart_Anonymous(t *testing.T) {
	tracker := &mockTracker{}
	ms := &MCPServer{tracker: tracker}

	req := &mcp.ServerRequest[*mcp.InitializeParams]{}
	ms.trackSessionStart(req)

	got := tracker.lastEvent()
	assert.Equal(t, "anonymous", got.UserID)
	assert.Equal(t, "mcp_session_start", got.Event)
}

func TestTrackToolCall(t *testing.T) {
	tracker := &mockTracker{}
	ms := &MCPServer{tracker: tracker}

	args, _ := json.Marshal(map[string]any{
		"workspace_id": "ws-123",
		"project_id":   "proj-456",
	})
	req := &mcp.ServerRequest[*mcp.CallToolParamsRaw]{
		Params: &mcp.CallToolParamsRaw{
			Name:      "list_projects",
			Arguments: args,
		},
		Extra: &mcp.RequestExtra{
			TokenInfo: &auth.TokenInfo{UserID: "user-2"},
		},
	}
	ms.trackToolCall(req)

	got := tracker.lastEvent()
	assert.Equal(t, "mcp_tool_call", got.Event)
	assert.Equal(t, "user-2", got.UserID)
	assert.Equal(t, "list_projects", got.Properties["tool_name"])
	assert.Equal(t, "ws-123", got.Properties["workspace_id"])
	assert.Equal(t, "proj-456", got.Properties["project_id"])
}

func TestTrackToolCall_NoArgs(t *testing.T) {
	tracker := &mockTracker{}
	ms := &MCPServer{tracker: tracker}

	req := &mcp.ServerRequest[*mcp.CallToolParamsRaw]{
		Params: &mcp.CallToolParamsRaw{
			Name: "list_flows",
		},
		Extra: &mcp.RequestExtra{
			TokenInfo: &auth.TokenInfo{UserID: "user-3"},
		},
	}
	ms.trackToolCall(req)

	got := tracker.lastEvent()
	assert.Equal(t, "mcp_tool_call", got.Event)
	assert.Equal(t, "list_flows", got.Properties["tool_name"])
	_, hasWS := got.Properties["workspace_id"]
	assert.False(t, hasWS)
}

func TestTrackResourceRead(t *testing.T) {
	tracker := &mockTracker{}
	ms := &MCPServer{tracker: tracker}

	req := &mcp.ServerRequest[*mcp.ReadResourceParams]{
		Params: &mcp.ReadResourceParams{
			URI: "brand://profiles/prof-1",
		},
		Extra: &mcp.RequestExtra{
			TokenInfo: &auth.TokenInfo{UserID: "user-4"},
		},
	}
	ms.trackResourceRead(req)

	got := tracker.lastEvent()
	assert.Equal(t, "mcp_resource_read", got.Event)
	assert.Equal(t, "user-4", got.UserID)
	assert.Equal(t, "brand://profiles/prof-1", got.Properties["resource_uri"])
}

func TestTrackResourceRead_Terminology(t *testing.T) {
	tracker := &mockTracker{}
	ms := &MCPServer{tracker: tracker}

	req := &mcp.ServerRequest[*mcp.ReadResourceParams]{
		Params: &mcp.ReadResourceParams{
			URI: "brand://terminology/ws-789",
		},
		Extra: &mcp.RequestExtra{
			TokenInfo: &auth.TokenInfo{UserID: "user-5"},
		},
	}
	ms.trackResourceRead(req)

	got := tracker.lastEvent()
	assert.Equal(t, "mcp_resource_read", got.Event)
	assert.Equal(t, "ws-789", got.Properties["workspace_id"])
	assert.Equal(t, "brand://terminology/ws-789", got.Properties["resource_uri"])
}

func TestAnalyticsMiddleware_OnlyTracksOnSuccess(t *testing.T) {
	tracker := &mockTracker{}
	ms := &MCPServer{tracker: tracker}

	mw := ms.analyticsMiddleware()

	// Simulate a failing handler — should NOT track.
	failHandler := mw(func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
		return nil, assert.AnError
	})

	req := &mcp.ServerRequest[*mcp.ReadResourceParams]{
		Params: &mcp.ReadResourceParams{URI: "brand://profiles/x"},
	}
	_, err := failHandler(t.Context(), "resources/read", req)
	require.Error(t, err)
	assert.Empty(t, tracker.events)
}

func TestAnalyticsMiddleware_TracksOnSuccess(t *testing.T) {
	tracker := &mockTracker{}
	ms := &MCPServer{tracker: tracker}

	mw := ms.analyticsMiddleware()

	successHandler := mw(func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
		return nil, nil
	})

	req := &mcp.ServerRequest[*mcp.CallToolParamsRaw]{
		Params: &mcp.CallToolParamsRaw{Name: "check_vocabulary"},
		Extra: &mcp.RequestExtra{
			TokenInfo: &auth.TokenInfo{UserID: "user-6"},
		},
	}
	_, err := successHandler(t.Context(), "tools/call", req)
	require.NoError(t, err)

	got := tracker.lastEvent()
	assert.Equal(t, "mcp_tool_call", got.Event)
	assert.Equal(t, "check_vocabulary", got.Properties["tool_name"])
}

func TestAnalyticsMiddleware_IgnoresUnknownMethods(t *testing.T) {
	tracker := &mockTracker{}
	ms := &MCPServer{tracker: tracker}

	mw := ms.analyticsMiddleware()
	handler := mw(func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
		return nil, nil
	})

	req := &mcp.ServerRequest[*mcp.ListToolsParams]{}
	_, err := handler(t.Context(), "tools/list", req)
	require.NoError(t, err)
	assert.Empty(t, tracker.events)
}

func TestExtractUserID_WithToken(t *testing.T) {
	req := &mcp.ServerRequest[*mcp.CallToolParamsRaw]{
		Extra: &mcp.RequestExtra{
			TokenInfo: &auth.TokenInfo{UserID: "user-99"},
		},
	}
	assert.Equal(t, "user-99", extractUserID(req))
}

func TestExtractUserID_NoExtra(t *testing.T) {
	req := &mcp.ServerRequest[*mcp.CallToolParamsRaw]{}
	assert.Equal(t, "anonymous", extractUserID(req))
}

func TestExtractUserID_NoTokenInfo(t *testing.T) {
	req := &mcp.ServerRequest[*mcp.CallToolParamsRaw]{
		Extra: &mcp.RequestExtra{
			Header: http.Header{},
		},
	}
	assert.Equal(t, "anonymous", extractUserID(req))
}
