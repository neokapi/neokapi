package mcp

import (
	"context"
	"encoding/json"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// EventTracker captures product analytics events.
// Implementations should be safe to call concurrently and must not block.
type EventTracker interface {
	TrackEvent(userID, event string, properties map[string]any)
}

// WithEventTracker adds analytics event tracking for MCP activity.
func WithEventTracker(t EventTracker) Option {
	return func(s *MCPServer) { s.tracker = t }
}

// analyticsMiddleware returns an MCP receiving middleware that tracks
// mcp_session_start, mcp_tool_call, and mcp_resource_read events.
func (s *MCPServer) analyticsMiddleware() mcp.Middleware {
	return func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
			result, err := next(ctx, method, req)
			if err != nil {
				return result, err
			}

			// Fire-and-forget tracking after successful handling.
			switch method {
			case "initialize":
				s.trackSessionStart(req)
			case "tools/call":
				s.trackToolCall(req)
			case "resources/read":
				s.trackResourceRead(req)
			}

			return result, nil
		}
	}
}

// trackSessionStart emits an mcp_session_start event.
func (s *MCPServer) trackSessionStart(req mcp.Request) {
	userID := extractUserID(req)
	props := map[string]any{
		"transport": "streamable-http",
	}
	if wsID := extractWorkspaceID(req); wsID != "" {
		props["workspace_id"] = wsID
	}
	s.tracker.TrackEvent(userID, "mcp_session_start", props)
}

// trackToolCall emits an mcp_tool_call event.
func (s *MCPServer) trackToolCall(req mcp.Request) {
	userID := extractUserID(req)
	props := map[string]any{}

	if params := req.GetParams(); params != nil {
		if raw, ok := params.(*mcp.CallToolParamsRaw); ok {
			props["tool_name"] = raw.Name
			// Extract workspace_id or project_id from arguments if present.
			if len(raw.Arguments) > 0 {
				var args map[string]json.RawMessage
				if json.Unmarshal(raw.Arguments, &args) == nil {
					if wsRaw, ok := args["workspace_id"]; ok {
						var ws string
						if json.Unmarshal(wsRaw, &ws) == nil && ws != "" {
							props["workspace_id"] = ws
						}
					}
					if pidRaw, ok := args["project_id"]; ok {
						var pid string
						if json.Unmarshal(pidRaw, &pid) == nil && pid != "" {
							props["project_id"] = pid
						}
					}
				}
			}
		}
	}

	s.tracker.TrackEvent(userID, "mcp_tool_call", props)
}

// trackResourceRead emits an mcp_resource_read event.
func (s *MCPServer) trackResourceRead(req mcp.Request) {
	userID := extractUserID(req)
	props := map[string]any{}

	if params := req.GetParams(); params != nil {
		if rr, ok := params.(*mcp.ReadResourceParams); ok {
			props["resource_uri"] = rr.URI
			// Extract workspace_id from terminology URIs.
			if wsID := extractParam(rr.URI, "brand://terminology/"); wsID != "" {
				props["workspace_id"] = wsID
			}
		}
	}

	s.tracker.TrackEvent(userID, "mcp_resource_read", props)
}

// extractUserID returns the user ID from the request's bearer token info,
// or "anonymous" if not available.
func extractUserID(req mcp.Request) string {
	if extra := req.GetExtra(); extra != nil && extra.TokenInfo != nil {
		return extra.TokenInfo.UserID
	}
	return "anonymous"
}

// extractWorkspaceID tries to get a workspace_id from the request's token extra data.
func extractWorkspaceID(req mcp.Request) string {
	if extra := req.GetExtra(); extra != nil && extra.TokenInfo != nil {
		if ws, ok := extra.TokenInfo.Extra["workspace_id"].(string); ok {
			return ws
		}
	}
	return ""
}
