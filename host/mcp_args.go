package host

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"net/url"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Every MCP tool argument that names a location is declared. The roots
// middleware (mcp_roots.go) checks each one before the tool runs, and refuses
// an argument the tool's schema does not list.
//
// A framework tool declares one with the `path=` key of a config field's
// schema tag. A tool with a typed input calls DeclareMCPPathArgs where it is
// registered. TestEveryMCPPathArgumentIsDeclared fails on a string argument
// that reads as a location and has neither.

// contextURIScheme is the address space the by-location primitive lives in.
const contextURIScheme = "context://"

// MCPArgKind is how a declared argument names a location.
type MCPArgKind int

const (
	// MCPArgPath is a file system path, absolute or relative to the directory
	// the server was started in. It must lie under a directory the call may
	// reach (MCPReachable).
	MCPArgPath MCPArgKind = iota + 1
	// MCPArgProjectRelative is a path inside the project the call acts on. It
	// must be local (no root, no climbing), or, spelled absolute, lie under a
	// directory the call may reach.
	MCPArgProjectRelative
	// MCPArgNotLocation declares an argument whose name or description reads
	// as a location and that names none (a revision, a pack name, a
	// format), so the audit stops asking about it. It is never checked.
	MCPArgNotLocation
)

var (
	mcpPathArgsMu sync.RWMutex
	mcpPathArgs   = map[string]map[string]MCPArgKind{}
)

// DeclareMCPPathArgs declares the arguments of tool that name a location.
// Call it beside the tool's registration.
func DeclareMCPPathArgs(tool string, kind MCPArgKind, args ...string) {
	mcpPathArgsMu.Lock()
	defer mcpPathArgsMu.Unlock()
	m := mcpPathArgs[tool]
	if m == nil {
		m = map[string]MCPArgKind{}
		mcpPathArgs[tool] = m
	}
	for _, a := range args {
		m[a] = kind
	}
}

// declaredMCPPathArgs returns the arguments of tool declared with
// DeclareMCPPathArgs.
func declaredMCPPathArgs(tool string) map[string]MCPArgKind {
	mcpPathArgsMu.RLock()
	defer mcpPathArgsMu.RUnlock()
	out := make(map[string]MCPArgKind, len(mcpPathArgs[tool]))
	maps.Copy(out, mcpPathArgs[tool])
	return out
}

// mcpToolSchemas holds the input schema of every tool a server lists, read
// once from the server itself.
type mcpToolSchemas struct {
	once  sync.Once
	props map[string]map[string]json.RawMessage
	err   error
}

// load lists the server's tools through next, the server's own handler chain.
func (s *mcpToolSchemas) load(ctx context.Context, next mcp.MethodHandler, ss *mcp.ServerSession) {
	s.once.Do(func() {
		s.props = map[string]map[string]json.RawMessage{}
		var cursor string
		for {
			res, err := next(ctx, "tools/list", &mcp.ListToolsRequest{Session: ss, Params: &mcp.ListToolsParams{Cursor: cursor}})
			if err != nil {
				s.err = err
				return
			}
			list, ok := res.(*mcp.ListToolsResult)
			if !ok {
				s.err = fmt.Errorf("tools/list answered with %T", res)
				return
			}
			for _, t := range list.Tools {
				s.props[t.Name] = schemaProperties(t.InputSchema)
			}
			if list.NextCursor == "" {
				return
			}
			cursor = list.NextCursor
		}
	})
}

// schemaProperties returns the properties of a tool's input schema.
func schemaProperties(schema any) map[string]json.RawMessage {
	raw, err := json.Marshal(schema)
	if err != nil {
		return map[string]json.RawMessage{}
	}
	var s struct {
		Properties map[string]json.RawMessage `json:"properties"`
	}
	if err := json.Unmarshal(raw, &s); err != nil || s.Properties == nil {
		return map[string]json.RawMessage{}
	}
	return s.Properties
}

// pathArgsOf returns every argument of tool that names a location: those its
// schema annotates with x-path, those declared with DeclareMCPPathArgs, and
// `project`, which every project-scoped tool takes.
func pathArgsOf(tool string, props map[string]json.RawMessage) map[string]MCPArgKind {
	out := declaredMCPPathArgs(tool)
	for name, raw := range props {
		var p struct {
			XPath json.RawMessage `json:"x-path"`
		}
		if json.Unmarshal(raw, &p) == nil && len(p.XPath) > 0 && string(p.XPath) != "null" {
			if _, ok := out[name]; !ok {
				out[name] = MCPArgPath
			}
		}
	}
	if _, ok := props["project"]; ok {
		if _, declared := out["project"]; !declared {
			out["project"] = MCPArgPath
		}
	}
	return out
}

// checkMCPCallArgs refuses a call that passes an argument its tool's schema
// does not list, or a declared location the call may not reach.
func checkMCPCallArgs(ctx context.Context, tool string, props map[string]json.RawMessage, raw json.RawMessage) error {
	if len(raw) == 0 {
		return nil
	}
	var args map[string]any
	if err := json.Unmarshal(raw, &args); err != nil {
		return fmt.Errorf("decode arguments: %w", err)
	}
	var unknown []string
	for name := range args {
		if _, ok := props[name]; !ok {
			unknown = append(unknown, name)
		}
	}
	if len(unknown) > 0 {
		sort.Strings(unknown)
		return fmt.Errorf("%s takes no argument %s", tool, strings.Join(unknown, ", "))
	}
	for name, kind := range pathArgsOf(tool, props) {
		if kind == MCPArgNotLocation {
			continue
		}
		for _, v := range stringValues(args[name]) {
			if err := checkMCPLocation(ctx, name, kind, v); err != nil {
				return err
			}
		}
	}
	return nil
}

// stringValues returns the strings an argument holds: one for a string, each
// for an array of them.
func stringValues(v any) []string {
	switch t := v.(type) {
	case string:
		if t == "" {
			return nil
		}
		return []string{t}
	case []any:
		var out []string
		for _, e := range t {
			if s, ok := e.(string); ok && s != "" {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}

// checkMCPResourceURI checks the location a resources/read names: the path of
// a context:// address, and its project parameter.
func checkMCPResourceURI(ctx context.Context, uri string) error {
	if !strings.HasPrefix(uri, contextURIScheme) {
		return nil
	}
	rest := strings.TrimPrefix(uri, contextURIScheme)
	path, query, _ := strings.Cut(rest, "?")
	if err := checkMCPLocation(ctx, "uri", MCPArgProjectRelative, path); err != nil {
		return err
	}
	values, err := url.ParseQuery(query)
	if err != nil {
		return fmt.Errorf("uri: %w", err)
	}
	if p := values.Get("project"); p != "" {
		return MCPReachable(ctx, "project", p)
	}
	return nil
}

// checkMCPLocation checks one declared argument value.
func checkMCPLocation(ctx context.Context, arg string, kind MCPArgKind, v string) error {
	v = strings.TrimPrefix(v, contextURIScheme)
	if kind == MCPArgProjectRelative && !filepath.IsAbs(v) {
		if !filepath.IsLocal(filepath.FromSlash(v)) {
			return fmt.Errorf("%s: %q leaves the project. Name a path inside it", arg, v)
		}
		return nil
	}
	return MCPReachable(ctx, arg, v)
}
