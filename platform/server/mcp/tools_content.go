package mcp

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/neokapi/neokapi/platform/store"
)

// registerContentTools registers project and content management MCP tools.
func (s *MCPServer) registerContentTools() {
	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "list_projects",
		Description: "List workspace projects. Returns project IDs, names, source/target languages, and creation dates.",
	}, s.handleListProjects)

	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "get_project",
		Description: "Get detailed information about a project including languages, word counts, and settings.",
	}, s.handleGetProject)

	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "list_blocks",
		Description: "Search and filter translatable blocks within a project. Supports filtering by locale, item, translation status, and pagination.",
	}, s.handleListBlocks)

	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "get_block",
		Description: "Get a single block with its source text and all target translations.",
	}, s.handleGetBlock)

	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "create_version",
		Description: "Create a named version snapshot of the current project state.",
	}, s.handleCreateVersion)

	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "list_streams",
		Description: "List all streams (branches) in a project.",
	}, s.handleListStreams)

	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "diff_streams",
		Description: "Compare a stream against its parent to see what changed.",
	}, s.handleDiffStreams)

	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "merge_stream",
		Description: "Merge a stream's changes into its parent stream.",
	}, s.handleMergeStream)
}

// --- Input/Output types ---

type listProjectsInput struct {
	WorkspaceID string `json:"workspace_id" jsonschema:"the workspace to list projects for"`
}
type listProjectsOutput struct {
	Projects []projectSummaryContent `json:"projects"`
}
type projectSummaryContent struct {
	ID             string   `json:"id"`
	Name           string   `json:"name"`
	SourceLanguage string   `json:"source_language"`
	TargetLanguages []string `json:"target_languages"`
}

func (s *MCPServer) handleListProjects(ctx context.Context, req *mcp.CallToolRequest, input listProjectsInput) (*mcp.CallToolResult, listProjectsOutput, error) {
	projects, err := s.contentStore.ListProjects(ctx)
	if err != nil {
		return nil, listProjectsOutput{}, fmt.Errorf("list projects: %w", err)
	}
	var result []projectSummaryContent
	for _, p := range projects {
		if input.WorkspaceID != "" && p.WorkspaceID != input.WorkspaceID {
			continue
		}
		langs := make([]string, len(p.TargetLanguages))
		for i, l := range p.TargetLanguages {
			langs[i] = string(l)
		}
		result = append(result, projectSummaryContent{
			ID:              p.ID,
			Name:            p.Name,
			SourceLanguage:  string(p.DefaultSourceLanguage),
			TargetLanguages: langs,
		})
	}
	return nil, listProjectsOutput{Projects: result}, nil
}

type getProjectInput struct {
	ProjectID string `json:"project_id" jsonschema:"the project ID"`
}

func (s *MCPServer) handleGetProject(ctx context.Context, req *mcp.CallToolRequest, input getProjectInput) (*mcp.CallToolResult, projectSummaryContent, error) {
	p, err := s.contentStore.GetProject(ctx, input.ProjectID)
	if err != nil {
		return nil, projectSummaryContent{}, fmt.Errorf("get project: %w", err)
	}
	langs := make([]string, len(p.TargetLanguages))
	for i, l := range p.TargetLanguages {
		langs[i] = string(l)
	}
	return nil, projectSummaryContent{
		ID:              p.ID,
		Name:            p.Name,
		SourceLanguage:  string(p.DefaultSourceLanguage),
		TargetLanguages: langs,
	}, nil
}

type listBlocksInput struct {
	ProjectID string `json:"project_id" jsonschema:"the project ID"`
	Stream    string `json:"stream,omitempty" jsonschema:"stream name (defaults to main)"`
	ItemName  string `json:"item_name,omitempty" jsonschema:"filter by file/item name"`
	Locale    string `json:"locale,omitempty" jsonschema:"filter blocks with target for this locale"`
	Limit     int    `json:"limit,omitempty" jsonschema:"max results (default 100)"`
	Offset    int    `json:"offset,omitempty" jsonschema:"pagination offset"`
}
type listBlocksOutput struct {
	Blocks []blockSummary `json:"blocks"`
	Total  int            `json:"total"`
}
type blockSummary struct {
	ID       string `json:"id"`
	ItemName string `json:"item_name"`
	Source   string `json:"source"`
}

func (s *MCPServer) handleListBlocks(ctx context.Context, req *mcp.CallToolRequest, input listBlocksInput) (*mcp.CallToolResult, listBlocksOutput, error) {
	limit := input.Limit
	if limit <= 0 {
		limit = 100
	}
	q := store.BlockQuery{
		ProjectID: input.ProjectID,
		Stream:    input.Stream,
		ItemName:  input.ItemName,
		Limit:     limit,
		Offset:    input.Offset,
	}
	blocks, err := s.contentStore.GetBlocks(ctx, q)
	if err != nil {
		return nil, listBlocksOutput{}, fmt.Errorf("list blocks: %w", err)
	}
	var summaries []blockSummary
	for _, b := range blocks {
		src := ""
		if b.Block != nil && len(b.Block.Source) > 0 {
			src = b.Block.Source[0].Content.Text()
		}
		summaries = append(summaries, blockSummary{
			ID:       b.Block.ID,
			ItemName: b.ItemName,
			Source:   src,
		})
	}
	return nil, listBlocksOutput{Blocks: summaries, Total: len(summaries)}, nil
}

type getBlockInput struct {
	ProjectID string `json:"project_id" jsonschema:"the project ID"`
	BlockID   string `json:"block_id" jsonschema:"the block ID"`
	Stream    string `json:"stream,omitempty" jsonschema:"stream name (defaults to main)"`
}
type getBlockOutput struct {
	ID       string            `json:"id"`
	ItemName string            `json:"item_name"`
	Source   string            `json:"source"`
	Targets  map[string]string `json:"targets"`
}

func (s *MCPServer) handleGetBlock(ctx context.Context, req *mcp.CallToolRequest, input getBlockInput) (*mcp.CallToolResult, getBlockOutput, error) {
	b, err := s.contentStore.GetBlock(ctx, input.ProjectID, input.Stream, input.BlockID)
	if err != nil {
		return nil, getBlockOutput{}, fmt.Errorf("get block: %w", err)
	}
	src := ""
	if b.Block != nil && len(b.Block.Source) > 0 {
		src = b.Block.Source[0].Content.Text()
	}
	targets := make(map[string]string)
	if b.Block != nil {
		for locale, segs := range b.Block.Targets {
			if len(segs) > 0 {
				targets[string(locale)] = segs[0].Content.Text()
			}
		}
	}
	return nil, getBlockOutput{
		ID:       b.Block.ID,
		ItemName: b.ItemName,
		Source:   src,
		Targets:  targets,
	}, nil
}

type createVersionInput struct {
	ProjectID   string `json:"project_id" jsonschema:"the project ID"`
	Stream      string `json:"stream,omitempty" jsonschema:"stream name (defaults to main)"`
	Label       string `json:"label" jsonschema:"version label (e.g. v1.0)"`
	Description string `json:"description,omitempty" jsonschema:"version description"`
}
type createVersionOutput struct {
	ID         string `json:"id"`
	Label      string `json:"label"`
	BlockCount int    `json:"block_count"`
}

func (s *MCPServer) handleCreateVersion(ctx context.Context, req *mcp.CallToolRequest, input createVersionInput) (*mcp.CallToolResult, createVersionOutput, error) {
	v, err := s.contentStore.CreateVersion(ctx, input.ProjectID, input.Stream, input.Label, input.Description)
	if err != nil {
		return nil, createVersionOutput{}, fmt.Errorf("create version: %w", err)
	}
	return nil, createVersionOutput{
		ID:         v.ID,
		Label:      v.Label,
		BlockCount: v.BlockCount,
	}, nil
}

type listStreamsInput struct {
	ProjectID string `json:"project_id" jsonschema:"the project ID"`
}
type listStreamsOutput struct {
	Streams []streamSummary `json:"streams"`
}
type streamSummary struct {
	Name       string `json:"name"`
	Parent     string `json:"parent"`
	Visibility string `json:"visibility"`
}

func (s *MCPServer) handleListStreams(ctx context.Context, req *mcp.CallToolRequest, input listStreamsInput) (*mcp.CallToolResult, listStreamsOutput, error) {
	streams, err := s.contentStore.ListStreams(ctx, input.ProjectID, false)
	if err != nil {
		return nil, listStreamsOutput{}, fmt.Errorf("list streams: %w", err)
	}
	var result []streamSummary
	for _, st := range streams {
		result = append(result, streamSummary{
			Name:       st.Name,
			Parent:     st.Parent,
			Visibility: string(st.Visibility),
		})
	}
	return nil, listStreamsOutput{Streams: result}, nil
}

type diffStreamsInput struct {
	ProjectID  string `json:"project_id" jsonschema:"the project ID"`
	StreamName string `json:"stream_name" jsonschema:"the stream to diff against its parent"`
}

func (s *MCPServer) handleDiffStreams(ctx context.Context, req *mcp.CallToolRequest, input diffStreamsInput) (*mcp.CallToolResult, store.StreamDiff, error) {
	diff, err := s.contentStore.DiffStream(ctx, input.ProjectID, input.StreamName)
	if err != nil {
		return nil, store.StreamDiff{}, fmt.Errorf("diff stream: %w", err)
	}
	return nil, *diff, nil
}

type mergeStreamInput struct {
	ProjectID  string `json:"project_id" jsonschema:"the project ID"`
	StreamName string `json:"stream_name" jsonschema:"the stream to merge into its parent"`
	DryRun     bool   `json:"dry_run,omitempty" jsonschema:"preview merge without applying changes"`
}

func (s *MCPServer) handleMergeStream(ctx context.Context, req *mcp.CallToolRequest, input mergeStreamInput) (*mcp.CallToolResult, store.MergeResult, error) {
	result, err := s.contentStore.MergeStream(ctx, input.ProjectID, input.StreamName, store.MergeOptions{
		DryRun: input.DryRun,
	})
	if err != nil {
		return nil, store.MergeResult{}, fmt.Errorf("merge stream: %w", err)
	}
	return nil, *result, nil
}
