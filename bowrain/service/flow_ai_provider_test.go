package service

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/bowrain/store/sqlitestore"
	aitools "github.com/neokapi/neokapi/core/ai/tools"
	"github.com/neokapi/neokapi/core/flow"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/neokapi/neokapi/core/tool"
	libtools "github.com/neokapi/neokapi/core/tools"
	aiprovider "github.com/neokapi/neokapi/providers/ai"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newAIFlowFixture is a flow service over a project that belongs to a
// workspace, with the deterministic and the model-backed tools registered the
// way the platform server registers them.
func newAIFlowFixture(t *testing.T) (*FlowService, store.ContentStore) {
	t.Helper()
	ctx := context.Background()

	cs, err := sqlitestore.NewSQLiteStore(filepath.Join(t.TempDir(), "content.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = cs.Close() })

	require.NoError(t, cs.CreateProject(ctx, &store.Project{
		ID:                    "p1",
		Name:                  "AI flow",
		WorkspaceID:           "ws-1",
		DefaultSourceLanguage: "en",
		TargetLanguages:       []model.LocaleID{"fr"},
	}))
	require.NoError(t, cs.StoreBlocksForItem(ctx, "p1", "main", "a.json", []*model.Block{
		translatableBlock("a1", "Hello there."),
	}))

	reg := registry.NewToolRegistry()
	libtools.RegisterAll(reg)
	aitools.RegisterAll(reg)
	return NewFlowService(cs, nil, reg), cs
}

func translateFlow() *flow.FlowDefinition {
	return &flow.FlowDefinition{
		ID:    "ai-translate",
		Name:  "AI translate",
		Nodes: []flow.FlowNode{{ID: "t", Type: flow.NodeTool, Name: "translate"}},
	}
}

// A translate step calls the provider the platform granted, so a run reaches
// the workspace's model instead of the placeholder the registry entry carries
// as its zero-argument default.
func TestFlowRunGrantsThePlatformProvider(t *testing.T) {
	fs, cs := newAIFlowFixture(t)
	granted := aiprovider.NewMockProvider()
	var sawWorkspace string
	fs.SetAIProviderResolver(func(_ context.Context, workspaceID, _ string) (aiprovider.LLMProvider, error) {
		sawWorkspace = workspaceID
		return granted, nil
	})

	res, err := fs.RunFlow(context.Background(), FlowRun{
		Definition:    translateFlow(),
		ProjectID:     "p1",
		TargetLocales: []string{"fr"},
		Source:        "test",
	})
	require.NoError(t, err)
	assert.Equal(t, 1, res.Items)
	assert.Equal(t, "ws-1", sawWorkspace, "the grant is scoped to the project workspace")
	assert.NotZero(t, providerCalls(granted), "the granted provider was never called")

	stored, err := cs.GetBlocks(context.Background(), store.BlockQuery{ProjectID: "p1", Stream: "main", ItemName: "a.json"})
	require.NoError(t, err)
	require.Len(t, stored, 1)
	assert.NotEmpty(t, stored[0].Block.TargetText("fr"), "the pass wrote no target")
}

// providerCalls counts every call a mock provider recorded, whichever entry
// point a tool reached it through.
func providerCalls(m *aiprovider.MockProvider) int {
	return len(m.TranslateCalls) + len(m.ChatCalls) + len(m.ChatStructuredCalls)
}

// The workspace scoping the provider is the one the project belongs to.
func TestFlowRunResolvesTheProjectWorkspace(t *testing.T) {
	fs, _ := newAIFlowFixture(t)
	fs.SetAIProviderResolver(func(context.Context, string, string) (aiprovider.LLMProvider, error) {
		return aiprovider.NewMockProvider(), nil
	})
	assert.Equal(t, "ws-1", fs.workspaceFor(context.Background(), "p1"))
	assert.Empty(t, fs.workspaceFor(context.Background(), "no-such-project"))
}

// A step naming its own provider keeps it: the grant fills a gap.
func TestFlowRunKeepsAStepsOwnProvider(t *testing.T) {
	fs, _ := newAIFlowFixture(t)
	fs.SetAIProviderResolver(func(context.Context, string, string) (aiprovider.LLMProvider, error) {
		t.Fatal("a step naming its own provider must not ask for a grant")
		return nil, nil
	})

	nodes := translateFlow().Nodes
	nodes[0].Config = map[string]any{"provider": string(aiprovider.Demo)}
	tools, err := fs.buildFlowTools(context.Background(), "ws-1", nodes, "fr")
	require.NoError(t, err)
	assert.Len(t, tools, 1)
}

// A step that calls no model is never granted one.
func TestFlowRunGrantsNothingToADeterministicStep(t *testing.T) {
	fs, _ := newAIFlowFixture(t)
	fs.SetAIProviderResolver(func(context.Context, string, string) (aiprovider.LLMProvider, error) {
		t.Fatal("a deterministic step must not ask for a provider")
		return nil, nil
	})

	nodes := []flow.FlowNode{{ID: "q", Type: flow.NodeTool, Name: "qa", Config: map[string]any{"mode": "rules"}}}
	tools, err := fs.buildFlowTools(context.Background(), "ws-1", nodes, "fr")
	require.NoError(t, err)
	assert.Len(t, tools, 1)
}

// A resolver that cannot answer fails the step rather than letting it run
// against a provider nobody chose.
func TestFlowRunFailsWhenTheProviderCannotBeResolved(t *testing.T) {
	fs, _ := newAIFlowFixture(t)
	fs.SetAIProviderResolver(func(context.Context, string, string) (aiprovider.LLMProvider, error) {
		return nil, errors.New("platform provider unavailable")
	})

	_, err := fs.buildFlowTools(context.Background(), "ws-1", translateFlow().Nodes, "fr")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "platform provider unavailable")
}

// With no resolver the step builds exactly as it did before the platform
// registered the model-backed tools.
func TestFlowRunWithoutAResolverBuildsFromConfig(t *testing.T) {
	fs, _ := newAIFlowFixture(t)

	tools, err := fs.buildFlowTools(context.Background(), "", translateFlow().Nodes, "fr")
	require.NoError(t, err)
	assert.Len(t, tools, 1)
}

// The gRPC flow route builds through the same path, so a tool named there is
// granted the project workspace's provider too.
func TestNewToolForProjectGrantsTheProvider(t *testing.T) {
	fs, _ := newAIFlowFixture(t)
	granted := aiprovider.NewMockProvider()
	fs.SetAIProviderResolver(func(context.Context, string, string) (aiprovider.LLMProvider, error) {
		return granted, nil
	})

	built, err := fs.NewToolForProject(context.Background(), "p1", "translate", map[string]any{"target_locale": "fr"}, "fr")
	require.NoError(t, err)
	_, err = tool.RunOnParts(context.Background(), built, []*model.Part{
		{Type: model.PartBlock, Resource: translatableBlock("x1", "Hello there.")},
	})
	require.NoError(t, err)
	assert.NotZero(t, providerCalls(granted), "the granted provider was never called")
}

// The built-in translate flow validates against the platform's registry: the
// gates a run applies before a block is read all pass.
func TestBuiltInTranslateFlowValidatesOnThePlatformRegistry(t *testing.T) {
	fs, _ := newAIFlowFixture(t)

	nodes, err := fs.storeFlowToolNodes(builtInFlow(t, "translate"))
	require.NoError(t, err)
	require.NotEmpty(t, nodes)
	assert.Contains(t, toolNames(nodes), "translate")
}

func toolNames(nodes []flow.FlowNode) []string {
	out := make([]string, 0, len(nodes))
	for _, n := range nodes {
		out = append(out, n.Name)
	}
	return out
}
