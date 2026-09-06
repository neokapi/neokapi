package service

import (
	"context"
	"errors"
	"testing"

	"github.com/neokapi/neokapi/core/flow"
	aiprovider "github.com/neokapi/neokapi/providers/ai"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ownKeyTranslateFlow is a stored flow definition whose translate node names a
// provider of its own, which is what the flow editor writes when somebody fills
// in a step's provider or API key.
func ownKeyTranslateFlow() *flow.FlowDefinition {
	return &flow.FlowDefinition{
		ID:   "byo-translate",
		Name: "Translate on the workspace key",
		Nodes: []flow.FlowNode{{
			ID: "t", Type: flow.NodeTool, Name: "translate",
			Config: map[string]any{"provider": string(aiprovider.Demo), "apiKey": "workspace-key"},
		}},
	}
}

// A step on the workspace's own key is still counted. The platform never holds
// that key, so the grant cannot reach the provider, but the abuse cap's whole
// contract is that it sees every model call in a run.
func TestFlowRunRecordsAStepOnItsOwnKey(t *testing.T) {
	fs, _ := newAIFlowFixture(t)
	acct := &recordingAccountant{}
	fs.SetAIAccountant(acct)
	fs.SetAIProviderResolver(func(context.Context, string, string) (aiprovider.LLMProvider, error) {
		return aiprovider.NewMockProvider(), nil
	})

	_, err := fs.RunFlow(context.Background(), FlowRun{
		Definition:    ownKeyTranslateFlow(),
		ProjectID:     "p1",
		TargetLocales: []string{"fr"},
		RunID:         "run-9",
		Source:        "test",
	})
	require.NoError(t, err)

	require.Len(t, acct.settlements, 1)
	spend := acct.settlements[0]
	assert.Equal(t, "ws-1", spend.WorkspaceID)
	assert.Positive(t, spend.Total.TotalTokens(), "the cap saw nothing of a run that called a model")
	assert.Zero(t, spend.Billable.TotalTokens(), "a key the workspace pays for burns no credits")
	require.Len(t, spend.ByOperation, 1)
	assert.Equal(t, "translate", spend.ByOperation[0].Operation)
	assert.Equal(t, SpendOwnKey, spend.ByOperation[0].Source)
}

// The step's own provider is the one that runs: observing a call must not
// replace the model an author chose.
func TestFlowRunKeepsTheStepsOwnProviderWhileObservingIt(t *testing.T) {
	fs, _ := newAIFlowFixture(t)
	acct := &recordingAccountant{}
	fs.SetAIAccountant(acct)
	granted := aiprovider.NewMockProvider()
	fs.SetAIProviderResolver(func(context.Context, string, string) (aiprovider.LLMProvider, error) {
		return granted, nil
	})

	_, err := fs.RunFlow(context.Background(), FlowRun{
		Definition:    ownKeyTranslateFlow(),
		ProjectID:     "p1",
		TargetLocales: []string{"fr"},
		Source:        "test",
	})
	require.NoError(t, err)
	assert.Zero(t, providerCalls(granted), "the platform's provider served a step that named its own")
	require.Len(t, acct.settlements, 1)
	assert.Positive(t, acct.settlements[0].Total.TotalTokens())
}

// Admission is asked for the workspace's own key too, and a refusal ends the
// run: the monthly ceiling bounds runaway usage whoever's key pays for it.
func TestFlowRunRefusesAnOwnKeyStepOverTheCap(t *testing.T) {
	fs, _ := newAIFlowFixture(t)
	refused := errors.New("workspace monthly AI quota exceeded")
	acct := &recordingAccountant{admitErr: refused}
	fs.SetAIAccountant(acct)

	_, err := fs.RunFlow(context.Background(), FlowRun{
		Definition:    ownKeyTranslateFlow(),
		ProjectID:     "p1",
		TargetLocales: []string{"fr"},
		Source:        "test",
	})
	require.ErrorIs(t, err, refused)
	require.Equal(t, []SpendSource{SpendOwnKey}, acct.admitSource,
		"a step on its own key was admitted as though the platform paid")
	assert.Empty(t, acct.settlements, "a refused run spent nothing to settle")
}

// A run that mixes a granted step with one on the workspace's own key settles
// both against the cap and deducts only for the platform's.
func TestFlowRunSplitsAMixedRunByWhoseKeyPaid(t *testing.T) {
	fs, _ := newAIFlowFixture(t)
	acct := &recordingAccountant{}
	fs.SetAIAccountant(acct)
	fs.SetAIProviderResolver(func(context.Context, string, string) (aiprovider.LLMProvider, error) {
		return aiprovider.NewMockProvider(), nil
	})

	_, err := fs.RunFlow(context.Background(), FlowRun{
		Definition: &flow.FlowDefinition{
			ID:   "mixed",
			Name: "One granted step, one on the workspace key",
			Nodes: []flow.FlowNode{
				{ID: "t", Type: flow.NodeTool, Name: "translate"},
				{
					ID: "r", Type: flow.NodeTool, Name: "review",
					Config: map[string]any{"provider": string(aiprovider.Demo)},
				},
			},
		},
		ProjectID:     "p1",
		TargetLocales: []string{"fr"},
		Source:        "test",
	})
	require.NoError(t, err)

	require.Len(t, acct.settlements, 1)
	spend := acct.settlements[0]
	assert.ElementsMatch(t, []SpendSource{SpendPlatformKey, SpendOwnKey}, acct.admitSource,
		"each key is admitted on its own terms")
	assert.Positive(t, spend.Billable.TotalTokens(), "the granted step deducted nothing")
	assert.Greater(t, spend.Total.TotalTokens(), spend.Billable.TotalTokens(),
		"the workspace's own key was left out of the cap")
}
