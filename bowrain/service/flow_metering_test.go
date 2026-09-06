package service

import (
	"context"
	"errors"
	"testing"

	"github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/core/flow"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/registry"
	aiprovider "github.com/neokapi/neokapi/providers/ai"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// recordingAccountant is the platform's accounting seam under test: it answers
// admission from a fixed verdict and keeps every settlement it is handed.
type recordingAccountant struct {
	admitErr    error
	admitCalls  []string
	admitSource []SpendSource
	settlements []AISpend
}

func (a *recordingAccountant) Admit(_ context.Context, workspaceID string, source SpendSource) error {
	a.admitCalls = append(a.admitCalls, workspaceID)
	a.admitSource = append(a.admitSource, source)
	return a.admitErr
}

func (a *recordingAccountant) Record(_ context.Context, spend AISpend) {
	a.settlements = append(a.settlements, spend)
}

// A flow run's translate step spends the platform's key, so the run settles
// with the tokens it burned, under the operation that burned them and against
// the workspace the project belongs to.
func TestFlowRunRecordsWhatItsAIStepSpent(t *testing.T) {
	fs, _ := newAIFlowFixture(t)
	acct := &recordingAccountant{}
	fs.SetAIAccountant(acct)
	fs.SetAIProviderResolver(func(context.Context, string, string) (aiprovider.LLMProvider, error) {
		return aiprovider.NewMockProvider(), nil
	})

	_, err := fs.RunFlow(context.Background(), FlowRun{
		Definition:    translateFlow(),
		ProjectID:     "p1",
		TargetLocales: []string{"fr"},
		RunID:         "run-7",
		Source:        "test",
	})
	require.NoError(t, err)

	require.Len(t, acct.settlements, 1)
	spend := acct.settlements[0]
	assert.Equal(t, "ws-1", spend.WorkspaceID)
	assert.Equal(t, "p1", spend.ProjectID)
	assert.Equal(t, "run-7", spend.RunID)
	assert.Contains(t, spend.ReferenceID, "run-7", "the reference names the run it settles")
	assert.Positive(t, spend.Total.TotalTokens())
	require.Len(t, spend.ByOperation, 1)
	assert.Equal(t, "translate", spend.ByOperation[0].Operation)
	assert.Equal(t, "mock-model", spend.ByOperation[0].Model, "the model the provider answered with names the row")
	assert.Equal(t, spend.Total, spend.ByOperation[0].Usage)
}

// Two runs of the same flow over the same project settle under two references,
// so a meter that dedupes on the reference bills both.
func TestFlowRunSettlesUnderAReferenceUniquePerRun(t *testing.T) {
	fs, _ := newAIFlowFixture(t)
	acct := &recordingAccountant{}
	fs.SetAIAccountant(acct)
	fs.SetAIProviderResolver(func(context.Context, string, string) (aiprovider.LLMProvider, error) {
		return aiprovider.NewMockProvider(), nil
	})

	for range 2 {
		_, err := fs.RunFlow(context.Background(), FlowRun{
			Definition:    translateFlow(),
			ProjectID:     "p1",
			TargetLocales: []string{"fr"},
			RunID:         "run-7",
			Source:        "test",
		})
		require.NoError(t, err)
	}

	require.Len(t, acct.settlements, 2)
	assert.NotEqual(t, acct.settlements[0].ReferenceID, acct.settlements[1].ReferenceID)
}

// A workspace with nothing left to spend fails the step with the reason,
// before the provider is ever asked for.
func TestFlowRunRefusesAnExhaustedWorkspace(t *testing.T) {
	fs, cs := newAIFlowFixture(t)
	fs.SetAIAccountant(&recordingAccountant{admitErr: ErrOutOfCredits})
	granted := aiprovider.NewMockProvider()
	fs.SetAIProviderResolver(func(context.Context, string, string) (aiprovider.LLMProvider, error) {
		return granted, nil
	})

	_, err := fs.RunFlow(context.Background(), FlowRun{
		Definition:    translateFlow(),
		ProjectID:     "p1",
		TargetLocales: []string{"fr"},
		Source:        "test",
	})
	require.Error(t, err)
	require.ErrorIs(t, err, ErrOutOfCredits)
	assert.Contains(t, err.Error(), "translate", "the step that could not run is named")
	assert.Zero(t, providerCalls(granted), "an unadmitted run must call no model")

	stored, err := cs.GetBlocks(context.Background(), store.BlockQuery{ProjectID: "p1", Stream: "main", ItemName: "a.json"})
	require.NoError(t, err)
	require.Len(t, stored, 1)
	assert.Empty(t, stored[0].Block.TargetText("fr"), "a refused run wrote a target")
}

// A step that calls no model never reaches billing: nothing is admitted and
// nothing is settled.
func TestFlowRunWithNoAIStepMetersNothing(t *testing.T) {
	fs, _ := newAIFlowFixture(t)
	acct := &recordingAccountant{}
	fs.SetAIAccountant(acct)
	fs.SetAIProviderResolver(func(context.Context, string, string) (aiprovider.LLMProvider, error) {
		t.Fatal("a deterministic step must not ask for a provider")
		return nil, nil
	})

	_, err := fs.RunFlow(context.Background(), FlowRun{
		Definition: &flow.FlowDefinition{
			ID:    "rules-only",
			Name:  "Rules only",
			Nodes: []flow.FlowNode{{ID: "q", Type: flow.NodeTool, Name: "qa", Config: map[string]any{"mode": "rules"}}},
		},
		ProjectID:     "p1",
		TargetLocales: []string{"fr"},
		Source:        "test",
	})
	require.NoError(t, err)
	assert.Empty(t, acct.admitCalls)
	assert.Empty(t, acct.settlements)
}

// A flow whose steps span two items and two locales checks the balance once:
// the run is admitted, not each of its passes.
func TestFlowRunChecksTheBalanceOncePerRun(t *testing.T) {
	fs, cs := newAIFlowFixture(t)
	require.NoError(t, cs.StoreBlocksForItem(context.Background(), "p1", "main", "b.json", []*model.Block{
		translatableBlock("b1", "Another string."),
	}))
	acct := &recordingAccountant{}
	fs.SetAIAccountant(acct)
	fs.SetAIProviderResolver(func(context.Context, string, string) (aiprovider.LLMProvider, error) {
		return aiprovider.NewMockProvider(), nil
	})

	res, err := fs.RunFlow(context.Background(), FlowRun{
		Definition:    translateFlow(),
		ProjectID:     "p1",
		TargetLocales: []string{"fr", "de"},
		Source:        "test",
	})
	require.NoError(t, err)
	assert.Equal(t, 2, res.Items)
	assert.Len(t, acct.admitCalls, 1, "the balance was checked more than once")
	assert.Len(t, acct.settlements, 1, "the run settled more than once")
}

// A run that fails partway still settles the tokens it burned before it
// stopped: they are spent whatever the run's outcome.
func TestFlowRunSettlesWhatAFailedRunSpent(t *testing.T) {
	fs, _ := newAIFlowFixture(t)
	acct := &recordingAccountant{}
	fs.SetAIAccountant(acct)

	aiRun := fs.BeginAIRun(context.Background(), "p1", "run-9")
	aiRun.add("translate", "m1", SpendPlatformKey, aiprovider.TokenUsage{InputTokens: 12, OutputTokens: 8})
	aiRun.Settle(context.Background())

	require.Len(t, acct.settlements, 1)
	assert.Equal(t, 20, acct.settlements[0].Total.TotalTokens())
}

// A scope settled twice reports its spend once, so a caller that settles
// defensively cannot double-charge.
func TestAIRunSettlesOnce(t *testing.T) {
	fs, _ := newAIFlowFixture(t)
	acct := &recordingAccountant{}
	fs.SetAIAccountant(acct)

	aiRun := fs.BeginAIRun(context.Background(), "p1", "")
	aiRun.add("review", "m1", SpendPlatformKey, aiprovider.TokenUsage{InputTokens: 3, OutputTokens: 4})
	aiRun.Settle(context.Background())
	aiRun.Settle(context.Background())

	assert.Len(t, acct.settlements, 1)
}

// The spend splits by the operation that made it and the model that served it,
// in a stable order, and the total is their sum.
func TestAIRunSplitsSpendByOperationAndModel(t *testing.T) {
	fs, _ := newAIFlowFixture(t)
	acct := &recordingAccountant{}
	fs.SetAIAccountant(acct)

	aiRun := fs.BeginAIRun(context.Background(), "p1", "")
	aiRun.add("translate", "haiku", SpendPlatformKey, aiprovider.TokenUsage{InputTokens: 10, OutputTokens: 5})
	aiRun.add("review", "sonnet", SpendPlatformKey, aiprovider.TokenUsage{InputTokens: 4, OutputTokens: 1})
	aiRun.add("translate", "haiku", SpendPlatformKey, aiprovider.TokenUsage{InputTokens: 2, OutputTokens: 3})
	aiRun.Settle(context.Background())

	require.Len(t, acct.settlements, 1)
	spend := acct.settlements[0]
	require.Len(t, spend.ByOperation, 2)
	assert.Equal(t, OperationSpend{Operation: "review", Model: "sonnet", Source: SpendPlatformKey, Usage: aiprovider.TokenUsage{InputTokens: 4, OutputTokens: 1}}, spend.ByOperation[0])
	assert.Equal(t, OperationSpend{Operation: "translate", Model: "haiku", Source: SpendPlatformKey, Usage: aiprovider.TokenUsage{InputTokens: 12, OutputTokens: 8}}, spend.ByOperation[1])
	assert.Equal(t, 25, spend.Total.TotalTokens())
	assert.Equal(t, 25, spend.Billable.TotalTokens(), "every call was on the platform key")
}

// A service with no accountant meters nothing and admits everything, which is
// what a self-hosted instance runs as.
func TestFlowRunWithoutAnAccountantMetersNothing(t *testing.T) {
	fs, _ := newAIFlowFixture(t)
	granted := aiprovider.NewMockProvider()
	fs.SetAIProviderResolver(func(context.Context, string, string) (aiprovider.LLMProvider, error) {
		return granted, nil
	})

	_, err := fs.RunFlow(context.Background(), FlowRun{
		Definition:    translateFlow(),
		ProjectID:     "p1",
		TargetLocales: []string{"fr"},
		Source:        "test",
	})
	require.NoError(t, err)
	assert.NotZero(t, providerCalls(granted))
}

// The metered wrapper counts every entry point a tool can reach the model
// through, and keeps a streaming provider streaming: the translate tool
// type-asserts for it to surface live progress.
func TestMeteredProviderCountsEveryCall(t *testing.T) {
	fs, _ := newAIFlowFixture(t)
	fs.SetAIAccountant(&recordingAccountant{})
	aiRun := fs.BeginAIRun(context.Background(), "p1", "")

	inner := aiprovider.NewMockProvider()
	metered := aiRun.Meter(inner, "review", SpendPlatformKey)
	stream, ok := metered.(aiprovider.StreamingLLMProvider)
	require.True(t, ok, "the wrapper dropped streaming")

	ctx := context.Background()
	_, err := metered.Translate(ctx, aiprovider.TranslateRequest{Source: "Hello.", TargetLocale: "fr"})
	require.NoError(t, err)
	_, err = metered.Chat(ctx, []aiprovider.Message{aiprovider.TextMessage(aiprovider.RoleUser, "hi")})
	require.NoError(t, err)
	_, err = stream.ChatStream(ctx, []aiprovider.Message{aiprovider.TextMessage(aiprovider.RoleUser, "hi")}, func(aiprovider.ChatStreamEvent) {})
	require.NoError(t, err)

	spend, ok := aiRun.settlement()
	require.True(t, ok)
	require.Len(t, spend.ByOperation, 1)
	assert.Equal(t, "review", spend.ByOperation[0].Operation)
	assert.Equal(t, 90, spend.Total.TotalTokens(), "three mock calls at 30 tokens each")
	assert.Equal(t, inner.Name(), metered.Name())
	require.NoError(t, metered.Close())
}

// A provider that answers with an error contributes nothing, so a failed call
// is not billed.
func TestMeteredProviderRecordsNothingForAFailedCall(t *testing.T) {
	fs, _ := newAIFlowFixture(t)
	fs.SetAIAccountant(&recordingAccountant{})
	aiRun := fs.BeginAIRun(context.Background(), "p1", "")

	inner := aiprovider.NewMockProvider()
	inner.ChatFunc = func(context.Context, []aiprovider.Message) (*aiprovider.ChatResponse, error) {
		return nil, errors.New("upstream refused")
	}
	metered := aiRun.Meter(inner, "review", SpendPlatformKey)

	_, err := metered.Chat(context.Background(), []aiprovider.Message{aiprovider.TextMessage(aiprovider.RoleUser, "hi")})
	require.Error(t, err)
	_, ok := aiRun.settlement()
	assert.False(t, ok, "a failed call was billed")
}

// The operation a tool's calls are recorded under is the tool id, with the two
// names the editor and the worker already write kept as they are.
func TestAIOperationNames(t *testing.T) {
	tests := []struct {
		tool registry.ToolID
		want string
	}{
		{"translate", "translate"},
		{"qa", "qa_check"},
		{"review", "review"},
		{"voice-check", "voice_check"},
		{"voice-infer", "voice_infer"},
		{"term-extract", "term_extract"},
		{"entity-extract", "entity_extract"},
		{"media-refine", "media_refine"},
	}
	for _, tt := range tests {
		t.Run(string(tt.tool), func(t *testing.T) {
			assert.Equal(t, tt.want, aiOperation(tt.tool))
		})
	}
}
