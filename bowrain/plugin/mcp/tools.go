// Package bowrainmcp registers bowrain-specific MCP tools (project_status,
// project_push, project_pull, project_ls, project_config, list_flows) on
// the shared `mcp` command's MCP server. The host (kapi or bowrain CLI)
// blank-imports github.com/neokapi/neokapi/bowrain/plugin (which pulls in
// this package) and the tools become available alongside the kapi MCP
// tools registered by the host's own init().
package bowrainmcp

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	bowrainconn "github.com/neokapi/neokapi/bowrain/core/connector"
	"github.com/neokapi/neokapi/bowrain/core/project"
	"github.com/neokapi/neokapi/bowrain/plugin/connector"
	"github.com/neokapi/neokapi/bowrain/plugin/internal/projflow"
	"github.com/neokapi/neokapi/cli"
	clioutput "github.com/neokapi/neokapi/cli/output"
	"github.com/neokapi/neokapi/core/locale"
	"github.com/neokapi/neokapi/core/model"
	coreproj "github.com/neokapi/neokapi/core/project"
)

func init() {
	cli.RegisterMCPToolFactory(registerBowrainTools)
}

// registerBowrainTools registers all Bowrain CLI MCP tools on the given server.
func registerBowrainTools(server *mcp.Server, a *cli.App) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "project_status",
		Description: "Show project sync status including pending push/pull counts and server connection",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input struct{}) (*mcp.CallToolResult, MCPStatusOutput, error) {
		return handleProjectStatus(ctx, a)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "project_push",
		Description: "Upload local changes to the server",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input MCPPushInput) (*mcp.CallToolResult, MCPPushOutput, error) {
		return handleProjectPush(ctx, a, input)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "project_pull",
		Description: "Download translations from the server and update local files",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input MCPPullInput) (*mcp.CallToolResult, MCPPullOutput, error) {
		return handleProjectPull(ctx, a, input)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "project_ls",
		Description: "List files tracked by the project with optional stats and dirty detection",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input MCPLsInput) (*mcp.CallToolResult, MCPLsOutput, error) {
		return handleProjectLs(ctx, a, input)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "project_config",
		Description: "Read project configuration from the .kapi recipe",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input struct{}) (*mcp.CallToolResult, MCPConfigOutput, error) {
		return handleProjectConfig()
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_flows",
		Description: "List available processing flows (built-in and project-defined)",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input struct{}) (*mcp.CallToolResult, MCPListFlowsOutput, error) {
		return handleBowrainListFlows(a)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "concept_search",
		Description: "Search the workspace brand knowledge graph for governed concepts (terms, status, domain) matching a query",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input MCPConceptSearchInput) (*mcp.CallToolResult, MCPConceptSearchOutput, error) {
		return handleConceptSearch(ctx, input)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "concept_story",
		Description: "Show the chronological timeline of a governed concept (revisions, observations, comments, change-sets)",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input MCPConceptStoryInput) (*mcp.CallToolResult, MCPConceptStoryOutput, error) {
		return handleConceptStory(ctx, input)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "experiment_status",
		Description: "Report brand knowledge-graph change-sets; with a changeset_id, include its detail and a blast-radius summary",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input MCPExperimentStatusInput) (*mcp.CallToolResult, MCPExperimentStatusOutput, error) {
		return handleExperimentStatus(ctx, input)
	})
}

// --- Input/Output types ---

type MCPStatusOutput struct {
	Project     MCPProjectInfo `json:"project"`
	ItemCount   int            `json:"item_count"`
	FileCount   int            `json:"file_count"`
	WordCount   int            `json:"word_count"`
	PendingPush int            `json:"pending_push"`
	PendingPull int            `json:"pending_pull"`
	UpToDate    bool           `json:"up_to_date"`
	Errors      []string       `json:"errors,omitempty"`
}

type MCPProjectInfo struct {
	Root      string `json:"root"`
	ConfigDir string `json:"config_dir"`
	Server    string `json:"server,omitempty"`
	ProjectID string `json:"project_id,omitempty"`
}

type MCPPushInput struct {
	Paths  []string `json:"paths,omitempty" jsonschema:"Specific file paths to push (default: all)"`
	Force  bool     `json:"force,omitempty" jsonschema:"Re-upload everything even if unchanged"`
	DryRun bool     `json:"dry_run,omitempty" jsonschema:"Show what would be uploaded without sending"`
}

type MCPPushOutput struct {
	BlocksPushed int  `json:"blocks_pushed"`
	WordCount    int  `json:"word_count"`
	FilesScanned int  `json:"files_scanned"`
	DryRun       bool `json:"dry_run,omitempty"`
	UpToDate     bool `json:"up_to_date,omitempty"`
}

type MCPPullInput struct {
	Locales []string `json:"locales,omitempty" jsonschema:"Languages to download (e.g. fr or de)"`
	Force   bool     `json:"force,omitempty" jsonschema:"Re-download everything even if unchanged"`
	DryRun  bool     `json:"dry_run,omitempty" jsonschema:"Show what would change without writing files"`
}

type MCPPullOutput struct {
	BlocksPulled int  `json:"blocks_pulled"`
	LocalesCount int  `json:"locales_count"`
	FilesWritten int  `json:"files_written,omitempty"`
	DryRun       bool `json:"dry_run,omitempty"`
	UpToDate     bool `json:"up_to_date,omitempty"`
}

type MCPLsInput struct {
	Paths []string `json:"paths,omitempty" jsonschema:"Filter by path prefixes"`
	Stats bool     `json:"stats,omitempty" jsonschema:"Include block and word counts"`
	Dirty bool     `json:"dirty,omitempty" jsonschema:"Show only files with local changes"`
}

type MCPLsEntry struct {
	Path   string `json:"path"`
	Format string `json:"format"`
	Blocks int    `json:"blocks,omitempty"`
	Words  int    `json:"words,omitempty"`
	Dirty  int    `json:"dirty,omitempty"`
}

type MCPLsOutput struct {
	Files   []MCPLsEntry `json:"files"`
	Total   int          `json:"total"`
	Blocks  int          `json:"blocks,omitempty"`
	Words   int          `json:"words,omitempty"`
	Changed int          `json:"changed,omitempty"`
}

type MCPLocaleInfo struct {
	Code        string `json:"code"`
	DisplayName string `json:"display_name"`
}

type MCPConfigOutput struct {
	Root            string          `json:"root"`
	ConfigPath      string          `json:"config_path"`
	SourceLanguage  MCPLocaleInfo   `json:"source_language"`
	TargetLanguages []MCPLocaleInfo `json:"target_languages,omitempty"`
	ServerURL       string          `json:"server_url,omitempty"`
	ProjectID       string          `json:"project_id,omitempty"`
	ContentCount    int             `json:"content_count"`
}

type MCPFlowEntry struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Source      string `json:"source"`
	Steps       int    `json:"steps,omitempty"`
}

type MCPListFlowsOutput struct {
	Flows []MCPFlowEntry `json:"flows"`
	Total int            `json:"total"`
}

// --- Handlers ---

func handleProjectStatus(ctx context.Context, a *cli.App) (*mcp.CallToolResult, MCPStatusOutput, error) {
	proj, err := project.FindProject("")
	if err != nil {
		return nil, MCPStatusOutput{}, err
	}

	out := MCPStatusOutput{
		Project: MCPProjectInfo{
			Root:      proj.Root,
			ConfigDir: proj.RecipePath(),
		},
	}

	conn, err := connector.NewSourceConnector(proj, a.FormatReg)
	if err != nil {
		// No server configured — return local info only.
		return nil, out, nil
	}
	defer conn.Close()

	status, err := conn.Status(ctx)
	if err != nil {
		return nil, MCPStatusOutput{}, err
	}

	out.Project.Server = proj.Recipe.Server.ServerURL()
	out.Project.ProjectID = proj.Recipe.Server.ProjectID()
	out.ItemCount = status.ItemCount
	out.FileCount = status.FileCount
	out.WordCount = status.WordCount
	out.PendingPush = status.PendingPush
	out.PendingPull = status.PendingPull
	out.UpToDate = status.PendingPush == 0 && status.PendingPull == 0
	out.Errors = status.Errors

	return nil, out, nil
}

func handleProjectPush(ctx context.Context, a *cli.App, input MCPPushInput) (*mcp.CallToolResult, MCPPushOutput, error) {
	proj, err := project.FindProject("")
	if err != nil {
		return nil, MCPPushOutput{}, err
	}

	conn, err := connector.NewSourceConnector(proj, a.FormatReg)
	if err != nil {
		return nil, MCPPushOutput{}, err
	}
	defer conn.Close()

	result, err := conn.Push(ctx, bowrainconn.PushOptions{
		Paths:  input.Paths,
		Force:  input.Force,
		DryRun: input.DryRun,
	})
	if err != nil {
		return nil, MCPPushOutput{}, err
	}

	out := MCPPushOutput{
		BlocksPushed: result.BlocksPushed,
		WordCount:    result.WordCount,
		FilesScanned: result.FilesScanned,
	}
	if input.DryRun {
		out.DryRun = true
	} else if result.BlocksPushed == 0 {
		out.UpToDate = true
	}

	return nil, out, nil
}

func handleProjectPull(ctx context.Context, a *cli.App, input MCPPullInput) (*mcp.CallToolResult, MCPPullOutput, error) {
	proj, err := project.FindProject("")
	if err != nil {
		return nil, MCPPullOutput{}, err
	}

	conn, err := connector.NewSourceConnector(proj, a.FormatReg)
	if err != nil {
		return nil, MCPPullOutput{}, err
	}
	defer conn.Close()

	locales := make([]model.LocaleID, len(input.Locales))
	for i, l := range input.Locales {
		locales[i] = model.LocaleID(l)
	}

	result, err := conn.Pull(ctx, bowrainconn.PullOptions{
		Locales: locales,
		Force:   input.Force,
		DryRun:  input.DryRun,
	})
	if err != nil {
		return nil, MCPPullOutput{}, err
	}

	out := MCPPullOutput{
		BlocksPulled: result.BlocksPulled,
		LocalesCount: result.LocalesCount,
		FilesWritten: result.FilesWritten,
	}
	if input.DryRun {
		out.DryRun = true
	} else if result.BlocksPulled == 0 {
		out.UpToDate = true
	}

	return nil, out, nil
}

func handleProjectLs(ctx context.Context, a *cli.App, input MCPLsInput) (*mcp.CallToolResult, MCPLsOutput, error) {
	proj, err := project.FindProject("")
	if err != nil {
		return nil, MCPLsOutput{}, fmt.Errorf("no kapi project found (run 'kapi init' first): %w", err)
	}

	if input.Stats || input.Dirty {
		return handleProjectLsWithStats(ctx, a, proj, input)
	}
	return handleProjectLsFast(a, proj, input)
}

func handleProjectLsFast(a *cli.App, proj *project.Project, input MCPLsInput) (*mcp.CallToolResult, MCPLsOutput, error) {
	var out MCPLsOutput

	recipe := proj.Recipe
	for _, it := range recipe.IterateContent() {
		lang := string(it.Item.ResolvedSourceLanguage(it.Collection, recipe.Defaults))
		pattern := coreproj.ResolvePathPattern(it.Item.Path, lang)
		relPaths, err := coreproj.ExpandGlob(proj.Root, pattern, recipe.Defaults.Exclude...)
		if err != nil {
			continue
		}
		for _, rp := range relPaths {
			if !matchesMCPPathFilter(rp, input.Paths) {
				continue
			}

			formatName := ""
			if it.Item.Format != nil {
				formatName = coreproj.ResolveFormat(it.Item.Format.Name)
			}
			if formatName == "" {
				ext := filepath.Ext(rp)
				if ext != "" {
					formatName, _ = a.FormatReg.Detector().DetectByExtension(ext)
				}
			}
			if formatName == "" {
				continue
			}

			out.Files = append(out.Files, MCPLsEntry{
				Path:   rp,
				Format: formatName,
			})
		}
	}

	out.Total = len(out.Files)
	return nil, out, nil
}

func handleProjectLsWithStats(ctx context.Context, a *cli.App, proj *project.Project, input MCPLsInput) (*mcp.CallToolResult, MCPLsOutput, error) {
	conn := connector.NewLocalConnector(proj, a.FormatReg)

	files, err := conn.ListFiles(ctx, nil)
	if err != nil {
		return nil, MCPLsOutput{}, err
	}

	var out MCPLsOutput
	for _, f := range files {
		if !matchesMCPPathFilter(f.Path, input.Paths) {
			continue
		}
		if input.Dirty && f.DirtyCount == 0 {
			continue
		}

		out.Files = append(out.Files, MCPLsEntry{
			Path:   f.Path,
			Format: f.Format,
			Blocks: f.BlockCount,
			Words:  f.WordCount,
			Dirty:  f.DirtyCount,
		})
		out.Blocks += f.BlockCount
		out.Words += f.WordCount
		out.Changed += f.DirtyCount
	}
	out.Total = len(out.Files)

	return nil, out, nil
}

func handleProjectConfig() (*mcp.CallToolResult, MCPConfigOutput, error) {
	proj, err := project.FindProject("")
	if err != nil {
		return nil, MCPConfigOutput{}, err
	}

	recipe := proj.Recipe
	srcLang := recipe.Defaults.SourceLanguage
	// Count tracked content items by walking bare entries + collections.
	contentCount := 0
	for range recipe.IterateContent() {
		contentCount++
	}

	out := MCPConfigOutput{
		Root:       proj.Root,
		ConfigPath: proj.RecipePath(),
		SourceLanguage: MCPLocaleInfo{
			Code:        string(srcLang),
			DisplayName: locale.DisplayName(srcLang),
		},
		ContentCount: contentCount,
	}

	for _, l := range recipe.Defaults.TargetLanguages {
		out.TargetLanguages = append(out.TargetLanguages, MCPLocaleInfo{
			Code:        string(l),
			DisplayName: locale.DisplayName(l),
		})
	}

	if recipe.HasServer() {
		out.ServerURL = recipe.Server.ServerURL()
		out.ProjectID = recipe.Server.ProjectID()
	}

	return nil, out, nil
}

func handleBowrainListFlows(a *cli.App) (*mcp.CallToolResult, MCPListFlowsOutput, error) {
	builtinFlows := []clioutput.FlowInfo{
		{Name: "ai-translate", Description: "Translate content using AI/LLM"},
		{Name: "ai-translate-qa", Description: "Translate + quality check using AI/LLM"},
		{Name: "pseudo-translate", Description: "Generate pseudo-translations for testing"},
		{Name: "qa-check", Description: "Run rule-based quality checks on translations"},
		{Name: "tm-leverage", Description: "Pre-fill translations from translation memory"},
		{Name: "segmentation", Description: "Split source text into sentence segments"},
	}

	var entries []MCPFlowEntry
	for _, f := range builtinFlows {
		entries = append(entries, MCPFlowEntry{
			Name:        f.Name,
			Description: f.Description,
			Source:      "builtin",
		})
	}

	// Add project flows if available.
	projectFlows := projflow.List()
	for _, f := range projectFlows {
		entries = append(entries, MCPFlowEntry{
			Name:        f.Name,
			Description: f.Description,
			Source:      "project",
			Steps:       f.Steps,
		})
	}

	return nil, MCPListFlowsOutput{Flows: entries, Total: len(entries)}, nil
}

// matchesMCPPathFilter returns true if relPath matches any of the given path prefixes,
// or if no filter paths are specified.
func matchesMCPPathFilter(relPath string, filterPaths []string) bool {
	if len(filterPaths) == 0 {
		return true
	}
	for _, prefix := range filterPaths {
		if len(prefix) > 0 && prefix[len(prefix)-1] == '/' {
			prefix = prefix[:len(prefix)-1]
		}
		if len(relPath) >= len(prefix) && relPath[:len(prefix)] == prefix {
			return true
		}
	}
	return false
}
