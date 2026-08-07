package main

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"slices"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/neokapi/neokapi/cli"

	// The kapi-level MCP porcelain registers itself on import, exactly as it
	// does in the binary.
	_ "github.com/neokapi/neokapi/kapi/mcptools"
)

// Surfaces an MCP tool can belong to. `kapi mcp` serves the curated default;
// --all-tools and --all-flows widen it.
const (
	MCPSurfaceDefault  = "default"
	MCPSurfaceAllTools = "all-tools"
	MCPSurfaceAllFlows = "all-flows"
)

// MCPDataset is the generated MCP reference: the tools `kapi mcp` serves, with
// the descriptions and parameters an assistant actually receives.
//
// It is collected the way a client collects them — by connecting to a real
// server over an in-memory transport and issuing tools/list — so the published
// table is the registration source rendered, not a hand copy of it. The hand
// copy documented three tools that had been folded into `context_search`,
// omitted the context-retrieval tools entirely, and taught a change-set kind
// the write path rejects.
type MCPDataset struct {
	GeneratedAt string    `json:"generatedAt"`
	Tools       []MCPTool `json:"tools"`
}

// MCPTool is one tool as an MCP client sees it.
type MCPTool struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	// Surface is which server surface exposes the tool: the curated default, or
	// the set a --all-tools / --all-flows server adds.
	Surface string     `json:"surface"`
	Params  []MCPParam `json:"params,omitempty"`
}

// MCPParam is one top-level input-schema property.
type MCPParam struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Required    bool   `json:"required"`
	Description string `json:"description,omitempty"`
}

// collectMCPDataset enumerates each server surface and labels every tool with
// the narrowest surface that exposes it.
func collectMCPDataset(now string) (MCPDataset, error) {
	byName := map[string]MCPTool{}
	// Widest first: a tool the default surface also serves keeps that label.
	surfaces := []struct {
		label   string
		surface cli.MCPSurface
	}{
		{MCPSurfaceAllTools, cli.MCPSurface{AllTools: true}},
		{MCPSurfaceAllFlows, cli.MCPSurface{AllFlows: true}},
		{MCPSurfaceDefault, cli.MCPSurface{}},
	}
	for _, s := range surfaces {
		tools, err := listMCPTools(s.surface)
		if err != nil {
			return MCPDataset{}, fmt.Errorf("enumerate %s MCP surface: %w", s.label, err)
		}
		for _, t := range tools {
			t.Surface = s.label
			byName[t.Name] = t
		}
	}

	tools := make([]MCPTool, 0, len(byName))
	for _, t := range byName {
		tools = append(tools, t)
	}
	slices.SortFunc(tools, func(a, b MCPTool) int { return cmp.Compare(a.Name, b.Name) })
	return MCPDataset{GeneratedAt: now, Tools: tools}, nil
}

// listMCPTools starts a server with the given surface and reads its tool list
// over an in-memory transport — the same call an assistant makes on connect.
func listMCPTools(surface cli.MCPSurface) ([]MCPTool, error) {
	app := &cli.App{MCPSurface: surface}
	app.InitRegistries()

	server := mcp.NewServer(&mcp.Implementation{Name: "kapi", Version: "reference"}, nil)
	cli.ApplyMCPToolFactories(server, app)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(ctx, serverTransport, nil)
	if err != nil {
		return nil, fmt.Errorf("connect server: %w", err)
	}
	defer serverSession.Close()

	client := mcp.NewClient(&mcp.Implementation{Name: "gen-refs", Version: "reference"}, nil)
	session, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		return nil, fmt.Errorf("connect client: %w", err)
	}
	defer session.Close()

	var out []MCPTool
	params := &mcp.ListToolsParams{}
	for {
		res, err := session.ListTools(ctx, params)
		if err != nil {
			return nil, fmt.Errorf("tools/list: %w", err)
		}
		for _, t := range res.Tools {
			out = append(out, MCPTool{
				Name:        t.Name,
				Description: t.Description,
				Params:      mcpParams(t.InputSchema),
			})
		}
		if res.NextCursor == "" {
			break
		}
		params.Cursor = res.NextCursor
	}
	return out, nil
}

// mcpParams flattens a tool's input schema into its top-level properties,
// ordered required-first then alphabetically so the rendered table reads the
// way a caller fills the form.
func mcpParams(schema any) []MCPParam {
	raw, err := json.Marshal(schema)
	if err != nil {
		return nil
	}
	var decoded struct {
		Properties map[string]struct {
			// JSON Schema allows a type union ("type": ["null", "array"]), which
			// the SDK emits for pointer and slice fields — hence the raw decode.
			Type        json.RawMessage `json:"type"`
			Description string          `json:"description"`
		} `json:"properties"`
		Required []string `json:"required"`
	}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return nil
	}
	required := map[string]bool{}
	for _, r := range decoded.Required {
		required[r] = true
	}
	params := make([]MCPParam, 0, len(decoded.Properties))
	for name, prop := range decoded.Properties {
		params = append(params, MCPParam{
			Name:        name,
			Type:        schemaType(prop.Type),
			Required:    required[name],
			Description: prop.Description,
		})
	}
	slices.SortFunc(params, func(a, b MCPParam) int {
		if a.Required != b.Required {
			if a.Required {
				return -1
			}
			return 1
		}
		return cmp.Compare(a.Name, b.Name)
	})
	return params
}

// schemaType renders a JSON Schema `type` — a bare string, or a union such as
// ["null", "array"] for a pointer or slice field — as the one type a caller
// supplies.
func schemaType(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var single string
	if err := json.Unmarshal(raw, &single); err == nil {
		return single
	}
	var union []string
	if err := json.Unmarshal(raw, &union); err != nil {
		return ""
	}
	for _, t := range union {
		if t != "null" {
			return t
		}
	}
	return ""
}
