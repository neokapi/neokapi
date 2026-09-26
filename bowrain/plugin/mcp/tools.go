// Package bowrainmcp registers bowrain-specific MCP tools (project_status,
// project_push, project_pull, project_ls, project_config, list_flows) on
// the shared `mcp` command's MCP server. The host (kapi or bowrain CLI)
// blank-imports github.com/neokapi/neokapi/bowrain/plugin (which pulls in
// this package) and the tools become available alongside the kapi MCP
// tools registered by the host's own init().
package bowrainmcp

import (
	"context"
	"io"
	"path/filepath"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/neokapi/neokapi/bowrain/plugin/commands"
	"github.com/neokapi/neokapi/cli"
	"github.com/neokapi/neokapi/core/locale"
	"github.com/neokapi/neokapi/core/model"
	coreproj "github.com/neokapi/neokapi/core/project"
	bowrainconn "github.com/neokapi/neokapi/core/venue/connector"
	"github.com/neokapi/neokapi/host/venue/project"
	"github.com/neokapi/neokapi/host/venue/source"
	"github.com/neokapi/neokapi/host/venue/transfer"
	"github.com/spf13/cobra"
)

func init() {
	cli.RegisterMCPToolFactory(registerBowrainTools)
}

// registerBowrainTools registers all Bowrain CLI MCP tools on the given server.
func registerBowrainTools(server *mcp.Server, a *cli.App) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "project_status",
		Description: "Show project sync status including pending push/pull counts and server connection",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input MCPProjectInput) (*mcp.CallToolResult, MCPStatusOutput, error) {
		return handleProjectStatus(ctx, a, input)
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
	}, func(ctx context.Context, req *mcp.CallToolRequest, input MCPProjectInput) (*mcp.CallToolResult, MCPConfigOutput, error) {
		return handleProjectConfig(a, input)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_flows",
		Description: "List available processing flows (built-in and project-defined)",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input MCPProjectInput) (*mcp.CallToolResult, MCPListFlowsOutput, error) {
		return handleBowrainListFlows(a, input)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "concept_search",
		Description: "Search the workspace brand knowledge graph for governed concepts (terms, status, domain) matching a query",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input MCPConceptSearchInput) (*mcp.CallToolResult, MCPConceptSearchOutput, error) {
		return handleConceptSearch(ctx, a, input)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "concept_story",
		Description: "Show the chronological timeline of a governed concept (revisions, observations, comments, change-sets)",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input MCPConceptStoryInput) (*mcp.CallToolResult, MCPConceptStoryOutput, error) {
		return handleConceptStory(ctx, a, input)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "experiment_status",
		Description: "Report brand knowledge-graph change-sets; with a changeset_id, include its detail and a blast-radius summary",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input MCPExperimentStatusInput) (*mcp.CallToolResult, MCPExperimentStatusOutput, error) {
		return handleExperimentStatus(ctx, a, input)
	})
}

// --- Input/Output types ---

// MCPProjectInput is the input of a tool that takes nothing but the project it
// acts on.
type MCPProjectInput struct {
	Project string `json:"project,omitempty" jsonschema:"the project this call acts on: its kapi.yaml recipe, its root directory, or any path inside it (default: the project the MCP server started in)"`
}

// loadProject loads the project a call acts on, resolved the way every kapi MCP
// tool resolves it (App.RequireMCPCallProject): the call's `project`, else the
// project the server started in, else discovery from the working directory
// with KAPI_NO_PROJECT honoured. It refuses when none is in scope, before any
// server is contacted.
func loadProject(a *cli.App, named string) (*project.Project, error) {
	path, err := a.RequireMCPCallProject(named)
	if err != nil {
		return nil, err
	}
	return project.Load(path)
}

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
	Project string   `json:"project,omitempty" jsonschema:"the project this call acts on: its kapi.yaml recipe, its root directory, or any path inside it (default: the project the MCP server started in)"`
	Paths   []string `json:"paths,omitempty" jsonschema:"Specific file paths to push (default: all)"`
	Force   bool     `json:"force,omitempty" jsonschema:"Re-upload everything even if unchanged"`
	DryRun  bool     `json:"dry_run,omitempty" jsonschema:"Show what would be uploaded without sending"`
}

type MCPPushOutput struct {
	BlocksPushed int  `json:"blocks_pushed"`
	WordCount    int  `json:"word_count"`
	FilesScanned int  `json:"files_scanned"`
	DryRun       bool `json:"dry_run,omitempty"`
	UpToDate     bool `json:"up_to_date,omitempty"`
}

type MCPPullInput struct {
	Project string   `json:"project,omitempty" jsonschema:"the project this call acts on: its kapi.yaml recipe, its root directory, or any path inside it (default: the project the MCP server started in)"`
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
	Project string   `json:"project,omitempty" jsonschema:"the project this call acts on: its kapi.yaml recipe, its root directory, or any path inside it (default: the project the MCP server started in)"`
	Paths   []string `json:"paths,omitempty" jsonschema:"Filter by path prefixes"`
	Stats   bool     `json:"stats,omitempty" jsonschema:"Include block and word counts"`
	Dirty   bool     `json:"dirty,omitempty" jsonschema:"Show only files with local changes"`
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
	// Warning says why the project's own flows are missing, when its recipe
	// does not load.
	Warning string `json:"warning,omitempty"`
}

// --- Handlers ---

func handleProjectStatus(ctx context.Context, a *cli.App, input MCPProjectInput) (*mcp.CallToolResult, MCPStatusOutput, error) {
	proj, err := loadProject(a, input.Project)
	if err != nil {
		return nil, MCPStatusOutput{}, err
	}

	out := MCPStatusOutput{
		Project: MCPProjectInfo{
			Root:      proj.Root,
			ConfigDir: proj.RecipePath(),
		},
	}

	conn, err := source.NewSourceConnector(a, proj, a.FormatReg)
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
	proj, err := loadProject(a, input.Project)
	if err != nil {
		return nil, MCPPushOutput{}, err
	}

	conn, err := source.NewSourceConnector(a, proj, a.FormatReg)
	if err != nil {
		return nil, MCPPushOutput{}, err
	}
	defer conn.Close()

	// The push `kapi push` runs, automations and terminology included. The
	// tool answers with the structured result, so the printed report is
	// dropped.
	cmd := &cobra.Command{Use: "push"}
	cmd.SetContext(ctx)
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	rep, err := commands.PushProject(cmd, proj, conn, transfer.PushOptions{
		Paths:  input.Paths,
		Force:  input.Force,
		DryRun: input.DryRun,
	})
	if rep == nil {
		return nil, MCPPushOutput{}, err
	}
	out := MCPPushOutput{
		BlocksPushed: rep.BlocksPushed,
		WordCount:    rep.WordCount,
		FilesScanned: rep.FilesScanned,
		DryRun:       rep.DryRun,
		UpToDate:     rep.UpToDate,
	}
	return nil, out, err
}

func handleProjectPull(ctx context.Context, a *cli.App, input MCPPullInput) (*mcp.CallToolResult, MCPPullOutput, error) {
	proj, err := loadProject(a, input.Project)
	if err != nil {
		return nil, MCPPullOutput{}, err
	}

	conn, err := source.NewSourceConnector(a, proj, a.FormatReg)
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
	proj, err := loadProject(a, input.Project)
	if err != nil {
		return nil, MCPLsOutput{}, err
	}

	if input.Stats || input.Dirty {
		return handleProjectLsWithStats(ctx, a, proj, input)
	}
	return handleProjectLsFast(a, proj, input)
}

func handleProjectLsFast(a *cli.App, proj *project.Project, input MCPLsInput) (*mcp.CallToolResult, MCPLsOutput, error) {
	var out MCPLsOutput

	recipe := proj.Recipe
	// Content-aware detection over the recipe's embedded KapiProject, so a
	// compound suffix like .kbf.json resolves to the KBF reader rather than the
	// generic JSON reader its .json tail would select, matching the stats path.
	projCtx := coreproj.NewProjectContext(&recipe.KapiProject, filepath.Join(proj.Root, coreproj.RecipeFileName))
	// A file is claimed by the first item that matches it, so it is listed
	// once, with that item's format.
	seen := map[string]bool{}
	for _, it := range recipe.IterateContent() {
		lang := string(it.Item.ResolvedSourceLanguage(it.Collection, recipe.Defaults))
		pattern := coreproj.ResolvePathPattern(it.Item.Path, lang)
		relPaths, err := coreproj.ExpandGlob(proj.Root, pattern, recipe.Defaults.Exclude...)
		if err != nil {
			continue
		}
		for _, rp := range relPaths {
			if seen[rp] || !matchesMCPPathFilter(rp, input.Paths) {
				continue
			}
			seen[rp] = true

			formatName := ""
			if it.Item.Format != nil {
				formatName = coreproj.ResolveFormat(it.Item.Format.Name)
			}
			if formatName == "" {
				formatName = projCtx.DetectFormat(a.FormatReg, filepath.Join(proj.Root, rp))
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
	conn := source.NewLocalConnector(a, proj, a.FormatReg)

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

func handleProjectConfig(a *cli.App, input MCPProjectInput) (*mcp.CallToolResult, MCPConfigOutput, error) {
	proj, err := loadProject(a, input.Project)
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

// handleBowrainListFlows lists what `kapi flows` lists for the project in
// scope (cli.FlowListing): the composed built-in flows, then the recipe's own,
// inline and in its `flows_dir:`, each name once for the flow `kapi run`
// resolves it to. With no project in scope it lists the built-in flows. A
// recipe that does not load still lists them, and the warning says why the
// project's are missing.
func handleBowrainListFlows(a *cli.App, input MCPProjectInput) (*mcp.CallToolResult, MCPListFlowsOutput, error) {
	recipePath, err := a.ResolveMCPCallProject(input.Project)
	if err != nil {
		return nil, MCPListFlowsOutput{}, err
	}
	flows, err := cli.FlowListing(recipePath)

	out := MCPListFlowsOutput{Flows: make([]MCPFlowEntry, 0, len(flows))}
	if err != nil {
		out.Warning = err.Error()
	}
	for _, f := range flows {
		source := "builtin"
		if f.Path != "" {
			source = "project"
		}
		out.Flows = append(out.Flows, MCPFlowEntry{
			Name:        f.Name,
			Description: f.Description,
			Source:      source,
			Steps:       f.Steps,
		})
	}
	out.Total = len(out.Flows)
	return nil, out, nil
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
