package mcp

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/core/model"
)

// registerContentTools registers project and content management MCP tools.
func (s *MCPServer) registerContentTools() { //nolint:funlen // tool registration is inherently long
	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "create_project",
		Description: "Create a new project with source and target languages.",
	}, s.handleCreateProject)

	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "update_project",
		Description: "Update a project's name or target languages.",
	}, s.handleUpdateProject)

	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "update_block",
		Description: "Update a block's target translation for a specific locale.",
	}, s.handleUpdateBlock)

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
	// WorkspaceID is ignored — the MCP server is already scoped to the
	// authenticated workspace. Kept for backward compatibility but the
	// filter is no longer applied (agents were passing wrong values).
	WorkspaceID string `json:"workspace_id,omitempty" jsonschema:"deprecated — ignored, workspace is determined by auth token"`
}
type listProjectsOutput struct {
	Projects []projectSummaryContent `json:"projects"`
}
type projectSummaryContent struct {
	ID              string   `json:"id"`
	Name            string   `json:"name"`
	SourceLanguage  string   `json:"source_language"`
	TargetLanguages []string `json:"target_languages"`
}

func (s *MCPServer) handleListProjects(ctx context.Context, req *mcp.CallToolRequest, input listProjectsInput) (*mcp.CallToolResult, listProjectsOutput, error) {
	projects, err := s.contentStore.ListProjects(ctx)
	if err != nil {
		return nil, listProjectsOutput{}, fmt.Errorf("list projects: %w", err)
	}
	// No workspace_id filter — ContentStore is already scoped to the
	// authenticated workspace. Agents were passing wrong values (slugs,
	// project names) which caused zero results.
	var result []projectSummaryContent
	for _, p := range projects {
		if false { // workspace_id filter disabled
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
	p, err := s.contentStore.GetProject(ctx, s.resolveProjectID(ctx, input.ProjectID))
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
	// Resolve project_id if it looks like a name instead of a UUID.
	projectID := s.resolveProjectID(ctx, input.ProjectID)

	limit := input.Limit
	if limit <= 0 {
		limit = 100
	}
	q := store.BlockQuery{
		ProjectID: projectID,
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
		if b.Block != nil {
			src = b.Block.SourceText()
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
	b, err := s.contentStore.GetBlock(ctx, s.resolveProjectID(ctx, input.ProjectID), input.Stream, input.BlockID)
	if err != nil {
		return nil, getBlockOutput{}, fmt.Errorf("get block: %w", err)
	}
	src := ""
	if b.Block != nil {
		src = b.Block.SourceText()
	}
	targets := make(map[string]string)
	if b.Block != nil {
		for _, locale := range b.Block.TargetLocales() {
			targets[string(locale)] = model.RunsText(b.Block.TargetRuns(locale))
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

// --- create_project ---

type createProjectInput struct {
	WorkspaceID     string   `json:"workspace_id" jsonschema:"the workspace ID"`
	Name            string   `json:"name" jsonschema:"project name"`
	SourceLanguage  string   `json:"source_language" jsonschema:"source language code (e.g. en-US)"`
	TargetLanguages []string `json:"target_languages" jsonschema:"target language codes"`
}
type createProjectOutput struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func (s *MCPServer) handleCreateProject(ctx context.Context, req *mcp.CallToolRequest, input createProjectInput) (*mcp.CallToolResult, createProjectOutput, error) {
	if input.Name == "" {
		return nil, createProjectOutput{}, errors.New("name is required")
	}
	if input.SourceLanguage == "" {
		return nil, createProjectOutput{}, errors.New("source_language is required")
	}
	targets := make([]model.LocaleID, len(input.TargetLanguages))
	for i, l := range input.TargetLanguages {
		targets[i] = model.LocaleID(l)
	}
	p := &store.Project{
		WorkspaceID:           input.WorkspaceID,
		Name:                  input.Name,
		DefaultSourceLanguage: model.LocaleID(input.SourceLanguage),
		TargetLanguages:       targets,
	}
	if err := s.contentStore.CreateProject(ctx, p); err != nil {
		return nil, createProjectOutput{}, fmt.Errorf("create project: %w", err)
	}
	return nil, createProjectOutput{ID: p.ID, Name: p.Name}, nil
}

// --- update_project ---

type updateProjectInput struct {
	ProjectID       string   `json:"project_id" jsonschema:"the project ID"`
	Name            string   `json:"name,omitempty" jsonschema:"new project name"`
	TargetLanguages []string `json:"target_languages,omitempty" jsonschema:"new target language codes"`
}

func (s *MCPServer) handleUpdateProject(ctx context.Context, req *mcp.CallToolRequest, input updateProjectInput) (*mcp.CallToolResult, projectSummaryContent, error) {
	if input.ProjectID == "" {
		return nil, projectSummaryContent{}, errors.New("project_id is required")
	}
	p, err := s.contentStore.GetProject(ctx, input.ProjectID)
	if err != nil {
		return nil, projectSummaryContent{}, fmt.Errorf("get project: %w", err)
	}
	if input.Name != "" {
		p.Name = input.Name
	}
	if len(input.TargetLanguages) > 0 {
		targets := make([]model.LocaleID, len(input.TargetLanguages))
		for i, l := range input.TargetLanguages {
			targets[i] = model.LocaleID(l)
		}
		p.TargetLanguages = targets
	}
	if err := s.contentStore.UpdateProject(ctx, p); err != nil {
		return nil, projectSummaryContent{}, fmt.Errorf("update project: %w", err)
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

// --- update_block ---

type updateBlockInput struct {
	ProjectID    string `json:"project_id" jsonschema:"the project ID"`
	BlockID      string `json:"block_id" jsonschema:"the block ID"`
	Stream       string `json:"stream,omitempty" jsonschema:"stream name (defaults to main)"`
	TargetLocale string `json:"target_locale" jsonschema:"locale code for the translation"`
	TargetText   string `json:"target_text" jsonschema:"the translated text"`
}
type updateBlockOutput struct {
	ID           string `json:"id"`
	TargetLocale string `json:"target_locale"`
	Updated      bool   `json:"updated"`
}

func (s *MCPServer) handleUpdateBlock(ctx context.Context, req *mcp.CallToolRequest, input updateBlockInput) (*mcp.CallToolResult, updateBlockOutput, error) {
	if input.BlockID == "" {
		return nil, updateBlockOutput{}, errors.New("block_id is required")
	}
	if input.TargetLocale == "" {
		return nil, updateBlockOutput{}, errors.New("target_locale is required")
	}
	projectID := s.resolveProjectID(ctx, input.ProjectID)
	stream := input.Stream
	if stream == "" {
		stream = "main"
	}

	sb, err := s.contentStore.GetBlock(ctx, projectID, stream, input.BlockID)
	if err != nil {
		return nil, updateBlockOutput{}, fmt.Errorf("get block: %w", err)
	}

	sb.Block.SetTargetText(model.LocaleID(input.TargetLocale), input.TargetText)

	if err := s.contentStore.StoreBlocks(ctx, projectID, stream, []*model.Block{sb.Block}); err != nil {
		return nil, updateBlockOutput{}, fmt.Errorf("store block: %w", err)
	}

	return nil, updateBlockOutput{
		ID:           input.BlockID,
		TargetLocale: input.TargetLocale,
		Updated:      true,
	}, nil
}

// resolveProjectID resolves a project_id that might be a name instead of a UUID.
// If the value contains only hex digits and dashes (UUID-like), it's returned as-is.
// Otherwise, it's treated as a project name and looked up via ListProjects.
func (s *MCPServer) resolveProjectID(ctx context.Context, id string) string {
	if id == "" {
		return id
	}
	// Quick check: if it looks like a UUID or internal ID, use it directly.
	for _, c := range id {
		if (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F') || c == '-' || c == '_' {
			continue
		}
		// Contains non-hex chars — likely a name. Try to resolve.
		projects, err := s.contentStore.ListProjects(ctx)
		if err != nil {
			return id // can't resolve, return as-is
		}
		for _, p := range projects {
			if strings.EqualFold(p.Name, id) {
				return p.ID
			}
		}
		return id // not found, return as-is
	}
	return id
}
