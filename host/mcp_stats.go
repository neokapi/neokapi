package host

import (
	"context"
	"errors"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// init registers the stats MCP tool on the shared `mcp` server: the sizing
// surface for agents. It pairs with extract_content (per-block detail) the way
// `kapi stats` pairs with `kapi inspect` — survey a document's volume and shape
// before deciding how to process it.
func init() {
	RegisterMCPToolFactory(registerStatsMCPTools)
}

// statsInput is the input to the stats MCP tool.
type statsInput struct {
	Files  []string `json:"files" jsonschema:"paths of the files to summarize"`
	Format string   `json:"format,omitempty" jsonschema:"input format override applied to every file (default: auto-detect by extension/content)"`
}

func registerStatsMCPTools(server *mcp.Server, a *App) {
	mcp.AddTool(server, &mcp.Tool{
		Name: "stats",
		Description: "Size files before processing them. Reports per-file and total content metrics: blocks (translatable " +
			"and not), words, characters (with and without spaces, plus the unique-character inventory), segments " +
			"when available, and a by-role breakdown. Works on any supported format (Word, PowerPoint, JSON, XLIFF, " +
			"Markdown, HTML, …) and returns the same JSON `kapi stats --json` emits.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in statsInput) (*mcp.CallToolResult, StatsOutput, error) {
		return a.statsMCP(ctx, in)
	})
}

// statsMCP computes the content metrics for the requested files via the same
// compute path as `kapi stats`. Any unreadable file fails the call — an agent
// sizing a job needs the whole answer or an error, not a partial total.
func (a *App) statsMCP(ctx context.Context, in statsInput) (*mcp.CallToolResult, StatsOutput, error) {
	a.InitRegistries()
	if len(in.Files) == 0 {
		return nil, StatsOutput{}, errors.New("files is required")
	}
	if in.Format != "" {
		prev := a.FormatFlag
		a.FormatFlag = in.Format
		defer func() { a.FormatFlag = prev }()
	}
	out, err := a.ComputeStats(ctx, in.Files, nil)
	if err != nil {
		return nil, StatsOutput{}, err
	}
	return nil, out, nil
}
