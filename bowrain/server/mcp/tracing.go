package mcp

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/neokapi/neokapi/bowrain/observe"
)

// tracingMiddleware puts a span on every MCP method.
//
// MCP is served over HTTP, so the Echo tracing middleware already opens a
// transaction for the request — but it names it by the HTTP route, and every
// MCP call arrives on the same one. A transaction called `POST /api/v1/mcp`
// says an agent did something and nothing about what, which for a surface whose
// whole point is that a model drives it is the least useful thing to know.
//
// So this adds the method inside the request's transaction rather than starting
// its own: one HTTP request may carry several MCP methods, and they are parts of
// that request, not separate ones.
//
// The tool name goes on as a TAG, never into the span description: an agent can
// call any registered tool and a description per tool would spread one surface
// across dozens of rows. `tools/call` is the shape; which tool is the detail.
func (s *MCPServer) tracingMiddleware() mcp.Middleware {
	return func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
			end := observe.StartSpan(ctx, "mcp.method", method)
			defer end()

			if name := toolName(req); name != "" {
				observe.TagTransaction(ctx, "mcp.tool", name)
			}
			// The same dimensions every other surface carries, read off the
			// token the agent presented — so "the MCP calls are slow" can be
			// narrowed to a customer the way an HTTP route can.
			observe.TagScope(ctx, observe.Scope{
				WorkspaceID: extractWorkspaceID(req),
				UserID:      mcpUserID(req),
				Feature:     "mcp",
			})
			return next(ctx, method, req)
		}
	}
}

// toolName is the tool a tools/call names, or empty for any other method.
//
// Read off the raw params rather than the decoded arguments: the name is all
// this needs, and the arguments carry whatever the caller sent — content,
// paths, ids — none of which belongs in telemetry.
func toolName(req mcp.Request) string {
	params := req.GetParams()
	if params == nil {
		return ""
	}
	raw, ok := params.(*mcp.CallToolParamsRaw)
	if !ok {
		return ""
	}
	return raw.Name
}

// mcpUserID is the acting user, or empty when the call is anonymous.
//
// extractUserID answers "anonymous" for an unauthenticated call, which is the
// right answer for a product event and the wrong one for a filter: a tag whose
// value is a literal "anonymous" reads as a user, and would put every
// unauthenticated call into one apparent account.
func mcpUserID(req mcp.Request) string {
	if id := extractUserID(req); id != "anonymous" {
		return id
	}
	return ""
}
