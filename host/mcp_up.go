package host

import (
	"context"
	"errors"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/neokapi/neokapi/core/project"
)

// init registers the kapi-loop MCP tools on the shared `mcp` server: `up`
// (bring the project up to date) and `up_plan` (the dry-run work plan with a
// token estimate). They drive exactly the CLI's `kapi up` engine at the venue
// the recipe binds, so an agent runs the same loop a human does — a connected
// project pushes, converges on the server and pulls the results back.
func init() {
	RegisterMCPToolFactory(registerUpMCPTools)
}

// upMCPInput is the input to the `up` MCP tool.
type upMCPInput struct {
	Project     string `json:"project,omitempty" jsonschema:"path to the kapi.yaml recipe (default: discovered upward from the working directory, like git)"`
	Passes      int    `json:"passes,omitempty" jsonschema:"maximum reconciliation passes (0 = loop until up to date or parked; 1 = single pass)"`
	Jobs        int    `json:"jobs,omitempty" jsonschema:"how many languages to catch up concurrently per pass (0 = project default, else 4)"`
	Materialize bool   `json:"materialize,omitempty" jsonschema:"after the loop, write the target-language files for every shippable locale (overrides the recipe's materialize policy)"`
	NoChecks    bool   `json:"no_checks,omitempty" jsonschema:"skip the project's bound checks inside the loop (failing units then count as translated)"`
	Local       bool   `json:"local,omitempty" jsonschema:"in a server-connected project, run the loop on this machine and push the results, instead of running it on the server"`
}

// upPlanMCPInput is the input to the `up_plan` MCP tool.
type upPlanMCPInput struct {
	Project string `json:"project,omitempty" jsonschema:"path to the kapi.yaml recipe (default: discovered upward from the working directory, like git)"`
}

func registerUpMCPTools(server *mcp.Server, a *App) {
	mcp.AddTool(server, &mcp.Tool{
		Name: "up",
		Description: "Bring a kapi project up to date against its ship gates: re-extract drifted sources, run the " +
			"default flow (content memory reuse then AI translate) over every target language, concurrently per language, " +
			"loop until every gated scope is shippable or parks for a human, and run the project's bound checks " +
			"each pass. In a project connected to a Bowrain server the run happens there (push, converge on the " +
			"org's keys and shared content memory, pull the results); pass local to run the loop on this machine " +
			"instead. Never fails on pending target work: parked units are reported, not thrown. Returns the " +
			"structured result (per-locale standing, parked scopes, materialized files). Use up_plan " +
			"first to see the pending work and token estimate.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in upMCPInput) (*mcp.CallToolResult, *ConvergeOutput, error) {
		path, err := a.mcpProjectPath(in.Project)
		if err != nil {
			return nil, nil, err
		}
		out, err := a.RunUpDispatch(ctx, path, "", UpOptions{
			UntilGate:   in.Passes != 1,
			MaxPasses:   in.Passes,
			Jobs:        in.Jobs,
			NoChecks:    in.NoChecks,
			Materialize: in.Materialize,
			Local:       in.Local,
		})
		if err != nil {
			return nil, nil, err
		}
		return nil, out, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name: "up_plan",
		Description: "Dry-run the catch-up work for a kapi project: per (collection, locale), the units " +
			"missing a target, exact-content-memory leverage, the remaining AI work, and a rough token estimate. No provider " +
			"calls, nothing written. The pre-flight for the up tool.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in upPlanMCPInput) (*mcp.CallToolResult, *UpPlanOutput, error) {
		path, err := a.mcpProjectPath(in.Project)
		if err != nil {
			return nil, nil, err
		}
		proj, err := project.LoadWithOptions(path, project.LoadOptions{SkipRequiresCheck: true})
		if err != nil {
			return nil, nil, err
		}
		a.InitRegistries()
		// One MCP server serves many projects over its lifetime, so this
		// project's language is bounded to this call (host/sourcelang.go).
		defer a.scopeSourceLang()()
		a.ResolveSourceLang(proj.Defaults.SourceLanguage)
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
		return "", errors.New("no kapi project found. Pass project, or run the MCP server inside a kapi project directory")
	}
	return path, nil
}
