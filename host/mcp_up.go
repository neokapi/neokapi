package host

import (
	"context"
	"errors"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/neokapi/neokapi/core/project"
)

// init registers the convergence MCP tools on the shared `mcp` server: `up`
// (reconcile the project toward its ship gates) and `up_plan` (the dry-run
// work plan with a token estimate). They drive exactly the CLI's `kapi up`
// engine, so an agent runs the same loop a human does.
func init() {
	RegisterMCPToolFactory(registerUpMCPTools)
}

// UpMCPInput is the input to the `up` MCP tool.
type UpMCPInput struct {
	Project     string `json:"project,omitempty" jsonschema:"path to the .kapi recipe (default: discovered upward from the working directory, like git)"`
	Passes      int    `json:"passes,omitempty" jsonschema:"maximum reconciliation passes (0 = loop until converged or parked; 1 = single pass)"`
	Jobs        int    `json:"jobs,omitempty" jsonschema:"how many languages to converge concurrently per pass (0 = project default, else 4)"`
	Materialize bool   `json:"materialize,omitempty" jsonschema:"after the loop, write localized files for every shippable locale (overrides the recipe's materialize policy)"`
	NoChecks    bool   `json:"no_checks,omitempty" jsonschema:"skip the project's bound checks inside the loop (failing units then count as translated)"`
}

// UpPlanMCPInput is the input to the `up_plan` MCP tool.
type UpPlanMCPInput struct {
	Project string `json:"project,omitempty" jsonschema:"path to the .kapi recipe (default: discovered upward from the working directory, like git)"`
}

func registerUpMCPTools(server *mcp.Server, a *App) {
	mcp.AddTool(server, &mcp.Tool{
		Name: "up",
		Description: "Reconcile a kapi project toward its ship gates: re-extract drifted sources, run the " +
			"default flow (TM reuse then AI translate) over every target language — concurrently per language — " +
			"loop until every gated scope is shippable or parks for a human, and run the project's bound checks " +
			"each pass. Never fails on pending target work: parked units are reported, not thrown. Returns the " +
			"structured convergence result (per-locale standing, parked scopes, materialized files). Use up_plan " +
			"first to see the pending work and token estimate.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in UpMCPInput) (*mcp.CallToolResult, *ConvergeOutput, error) {
		path, err := a.mcpProjectPath(in.Project)
		if err != nil {
			return nil, nil, err
		}
		out, err := a.RunUp(ctx, path, "", UpOptions{
			UntilGate:   in.Passes != 1,
			MaxPasses:   in.Passes,
			Jobs:        in.Jobs,
			NoChecks:    in.NoChecks,
			Materialize: in.Materialize,
		})
		if err != nil {
			return nil, nil, err
		}
		return nil, out, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name: "up_plan",
		Description: "Dry-run the convergence work for a kapi project: per (collection, locale), the units " +
			"missing a target, exact-TM leverage, the remaining AI work, and a rough token estimate. No provider " +
			"calls, nothing written. The pre-flight for the up tool.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in UpPlanMCPInput) (*mcp.CallToolResult, *UpPlanOutput, error) {
		path, err := a.mcpProjectPath(in.Project)
		if err != nil {
			return nil, nil, err
		}
		proj, err := project.LoadWithOptions(path, project.LoadOptions{SkipRequiresCheck: true})
		if err != nil {
			return nil, nil, err
		}
		a.InitRegistries()
		if a.SourceLang == "" && proj.Defaults.SourceLanguage != "" {
			a.SourceLang = string(proj.Defaults.SourceLanguage)
		}
		if a.SourceLang == "" {
			a.SourceLang = "en"
		}
		plan, err := a.computeProjectPlan(ctx, proj, path)
		if err != nil {
			return nil, nil, err
		}
		return nil, &plan, nil
	})
}

// mcpProjectPath resolves the project for an MCP call: the explicit input path
// when given, else the git-style upward discovery (honoring KAPI_NO_PROJECT).
func (a *App) mcpProjectPath(explicit string) (string, error) {
	if explicit != "" {
		return explicit, nil
	}
	path, err := ResolveProjectPath(nil)
	if err != nil {
		return "", err
	}
	if path == "" {
		return "", errors.New("no .kapi project found — pass project, or run the MCP server inside a kapi project directory")
	}
	return path, nil
}
