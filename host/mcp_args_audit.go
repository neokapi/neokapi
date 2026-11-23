//go:build !js

package host

import (
	"context"
	"sort"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// UndeclaredMCPPathArgs lists, as "tool.argument", every argument of the
// tools a session's server lists that can carry a string and is declared
// neither as a location nor as not a location (mcp_args.go), whatever its
// name. It also lists, as "tool.argument (declared, not in the schema)",
// every DeclareMCPPathArgs declaration naming an argument the tool's schema
// does not have, which is what a renamed or moved field leaves behind. A test
// of every surface that registers MCP tools calls it, so an argument added
// without a declaration fails a test.
func UndeclaredMCPPathArgs(ctx context.Context, cs *mcp.ClientSession) ([]string, error) {
	var out []string
	var cursor string
	for {
		res, err := cs.ListTools(ctx, &mcp.ListToolsParams{Cursor: cursor})
		if err != nil {
			return nil, err
		}
		for _, t := range res.Tools {
			kinds, args, err := mcpArgKinds(t.Name, readMCPInputSchema(t.InputSchema))
			if err != nil {
				return nil, err
			}
			present := make(map[string]bool, len(args))
			for _, a := range args {
				for p := a.Path; p != ""; p = parentMCPArgPath(p) {
					present[p] = true
				}
				if _, ok := mcpArgKindAt(kinds, a.Path); !ok {
					out = append(out, t.Name+"."+a.Path)
				}
			}
			for p := range declaredMCPPathArgs(t.Name) {
				if !present[p] {
					out = append(out, t.Name+"."+p+" (declared, not in the schema)")
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
