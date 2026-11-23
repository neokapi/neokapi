//go:build !js

package host

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// locationWords matches an argument name or description that reads as a file
// system location.
var locationWords = regexp.MustCompile(`(?i)(path|file|dir|folder|uri\b|url\b|location)`)

// UndeclaredMCPPathArgs lists, as "tool.argument", every string argument of
// the tools a session's server lists whose name or description reads as a
// location and that is not declared (an x-path annotation or
// DeclareMCPPathArgs). A test of every surface that registers MCP tools calls
// it, so an argument added without a declaration fails a test.
func UndeclaredMCPPathArgs(ctx context.Context, cs *mcp.ClientSession) ([]string, error) {
	var out []string
	var cursor string
	for {
		res, err := cs.ListTools(ctx, &mcp.ListToolsParams{Cursor: cursor})
		if err != nil {
			return nil, err
		}
		for _, t := range res.Tools {
			props := schemaProperties(t.InputSchema)
			args, err := locationArgs(t.Name, props)
			if err != nil {
				return nil, err
			}
			declared := pathArgsOf(t.Name, props)
			for _, name := range args {
				if _, ok := declared[name]; !ok {
					out = append(out, t.Name+"."+name)
				}
			}
		}
		if res.NextCursor == "" {
			break
		}
		cursor = res.NextCursor
	}
	sort.Strings(out)
	return out, nil
}

// AuditMCPPathArgs serves every MCP tool factory registered in this binary,
// on the widest surface, to an in-process client, and returns
// UndeclaredMCPPathArgs for it. Each package that registers tools has a test
// that calls it.
func AuditMCPPathArgs(ctx context.Context, a *App) ([]string, error) {
	a.MCPSurface = MCPSurface{AllTools: true, AllFlows: true}
	server := mcp.NewServer(&mcp.Implementation{Name: "kapi", Version: "audit"}, nil)
	ApplyMCPToolFactories(server, a)
	st, ct := mcp.NewInMemoryTransports()
	ss, err := server.Connect(ctx, st, nil)
	if err != nil {
		return nil, err
	}
	defer ss.Close()
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "audit", Version: "audit"}, nil).Connect(ctx, ct, nil)
	if err != nil {
		return nil, err
	}
	defer cs.Close()
	return UndeclaredMCPPathArgs(ctx, cs)
}

// locationArgs returns the string arguments among props whose name or
// description reads as a location.
func locationArgs(tool string, props map[string]json.RawMessage) ([]string, error) {
	var out []string
	for name, raw := range props {
		var p struct {
			Type        any    `json:"type"`
			Description string `json:"description"`
			Items       *struct {
				Type any `json:"type"`
			} `json:"items"`
		}
		if err := json.Unmarshal(raw, &p); err != nil {
			return nil, fmt.Errorf("%s.%s: %w", tool, name, err)
		}
		if !isStringish(p.Type) && (p.Items == nil || !isStringish(p.Items.Type)) {
			continue
		}
		if locationWords.MatchString(name) || locationWords.MatchString(p.Description) {
			out = append(out, name)
		}
	}
	sort.Strings(out)
	return out, nil
}

// isStringish reports whether a JSON schema type is, or includes, string.
func isStringish(t any) bool {
	switch v := t.(type) {
	case string:
		return v == "string"
	case []any:
		for _, e := range v {
			if s, ok := e.(string); ok && s == "string" {
				return true
			}
		}
	}
	return false
}
