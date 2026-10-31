//go:build !js

package host

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/registry"
	"github.com/neokapi/neokapi/core/schema"
	"github.com/neokapi/neokapi/core/tool"
)

// readingTool is a tool that records the source of every block it reads.
type readingTool struct {
	mu   sync.Mutex
	read []string
}

func (r *readingTool) build() tool.Tool {
	return &tool.BaseTool{ToolName: "reader", Annotate: func(v tool.BlockView) error {
		r.mu.Lock()
		defer r.mu.Unlock()
		r.read = append(r.read, v.SourceText())
		return nil
	}}
}

func (r *readingTool) sources() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return strings.Join(r.read, "\n")
}

// readerRun is `kapi exec <name> docs/page.json` in the project at recipe,
// with rt standing in for the tool the registry builds.
func readerRun(name, recipe string, rt *readingTool) ToolRunConfig {
	return ToolRunConfig{
		ToolName: name,
		Project:  recipe,
		Files:    []string{filepath.Join(filepath.Dir(recipe), "docs", "page.json")},
		NewTool:  func() (tool.Tool, error) { return rt.build(), nil },
	}
}

// The ways a tool can reach a run with no contract the registry holds: no
// entry at all, an entry with no schema, and a schema with no toolMeta.
var contractless = map[string]func(reg *registry.ToolRegistry, rt *readingTool){
	"no registry entry": func(*registry.ToolRegistry, *readingTool) {},
	"an entry with no schema": func(reg *registry.ToolRegistry, rt *readingTool) {
		reg.Register("contractless", rt.build)
	},
	"a schema with no toolMeta": func(reg *registry.ToolRegistry, rt *readingTool) {
		reg.RegisterWithSchema("contractless", rt.build, &schema.ComponentSchema{Title: "Contractless"})
	},
	"a metadata-only entry with no toolMeta": func(reg *registry.ToolRegistry, _ *readingTool) {
		reg.RegisterMetadata("contractless", &schema.ComponentSchema{Title: "Contractless"}, "test-plugin")
	},
}

const contractlessRefusal = "kapi refused to run contractless: the project declares defaults.redaction, and contractless has no contract that kapi knows, so kapi cannot tell whether it sends blocks to a provider. Declare its side effects in the toolMeta of the schema its plugin manifest names, or run it in a flow with a redact step ahead of it"

func TestToolRun_RefusesAToolWithNoContractUnderAPolicy(t *testing.T) {
	isolateKapiEnv(t)
	onlyDemoProvider(t)
	for name, register := range contractless {
		t.Run(name, func(t *testing.T) {
			recipe := redactProject(t, redactionYAML)
			a := &App{SourceLang: "en"}
			a.InitRegistries()
			rt := &readingTool{}
			register(a.ToolReg, rt)

			err := a.RunToolOnFiles(t.Context(), readerRun("contractless", recipe, rt))
			require.EqualError(t, err, contractlessRefusal)
			assert.Empty(t, rt.sources(), "the tool reads no block")
		})
	}
}

func TestToolRun_RunsAToolWithNoContractWithoutAPolicy(t *testing.T) {
	isolateKapiEnv(t)
	onlyDemoProvider(t)
	for name, register := range contractless {
		t.Run(name, func(t *testing.T) {
			for project, recipe := range map[string]func(t *testing.T) string{
				"a project with no policy": func(t *testing.T) string { return redactProject(t, "") },
				"a policy switched off": func(t *testing.T) string {
					return redactProject(t, "  redaction:\n    enabled: false\n    rules: redact.yaml\n")
				},
			} {
				t.Run(project, func(t *testing.T) {
					a := &App{SourceLang: "en"}
					a.InitRegistries()
					rt := &readingTool{}
					register(a.ToolReg, rt)
					require.NoError(t, a.RunToolOnFiles(t.Context(), readerRun("contractless", recipe(t), rt)))
					assert.Contains(t, rt.sources(), redactedName, "the tool reads the block as it is")
				})
			}
			t.Run("outside a project", func(t *testing.T) {
				a := &App{SourceLang: "en"}
				a.InitRegistries()
				rt := &readingTool{}
				register(a.ToolReg, rt)
				cfg := readerRun("contractless", redactProject(t, redactionYAML), rt)
				cfg.Project = ""
				require.NoError(t, a.RunToolOnFiles(t.Context(), cfg))
				assert.Contains(t, rt.sources(), redactedName)
			})
		})
	}
}

// A tool whose contract the registry holds and carries no API call runs under
// the policy, and reads the block unredacted, because nothing it reads leaves
// the machine.
func TestToolRun_RunsAToolWhoseContractCallsNoProviderUnderAPolicy(t *testing.T) {
	isolateKapiEnv(t)
	onlyDemoProvider(t)
	recipe := redactProject(t, redactionYAML)
	a := &App{SourceLang: "en"}
	a.InitRegistries()
	rt := &readingTool{}
	a.ToolReg.RegisterWithSchema("local-reader", rt.build, &schema.ComponentSchema{
		Title:    "Local reader",
		ToolMeta: &schema.ToolMeta{ID: "local-reader", SideEffects: []schema.SideEffect{schema.SideEffectMemoryRead}},
	})
	require.NoError(t, a.RunToolOnFiles(t.Context(), readerRun("local-reader", recipe, rt)))
	assert.Contains(t, rt.sources(), redactedName)
}

// The MCP tools that run one registry tool over a text resolve the policy
// through the same chain, so the refusal holds there too.
func TestFrameworkMCPHandler_RefusesAToolWithNoContractUnderAPolicy(t *testing.T) {
	isolateKapiEnv(t)
	onlyDemoProvider(t)
	a := &App{}
	a.InitRegistries()
	rt := &readingTool{}
	a.ToolReg.Register("contractless", rt.build)
	handler := a.frameworkMCPHandler("contractless", "", targetWithheld)
	call := func(project string) error {
		t.Helper()
		args, err := json.Marshal(map[string]any{"text": "Write to Ada Lovelace today.", "project": project})
		require.NoError(t, err)
		_, err = handler(context.Background(), &mcp.CallToolRequest{Params: &mcp.CallToolParamsRaw{Arguments: args}})
		return err
	}

	require.EqualError(t, call(redactProject(t, redactionYAML)), contractlessRefusal)
	assert.Empty(t, rt.sources(), "the tool reads no text")

	require.NoError(t, call(redactProject(t, "")), "a project with no policy runs the tool")
	assert.Contains(t, rt.sources(), redactedName)
}

// Every tool the registry holds at startup carries a contract, so the refusal
// above never stops a built-in tool.
func TestInitRegistries_EveryToolCarriesAContract(t *testing.T) {
	a := &App{}
	a.InitRegistries()
	names := a.ToolReg.Names()
	require.NotEmpty(t, names)
	for _, name := range names {
		assert.True(t, a.toolContractKnown(string(name)), "%s has no toolMeta", name)
	}
}
