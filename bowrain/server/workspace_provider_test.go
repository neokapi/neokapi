package server

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/bowrain/jobs"
	bstore "github.com/neokapi/neokapi/bowrain/store"
	aiprovider "github.com/neokapi/neokapi/providers/ai"
)

// recordingJobStore keeps the jobs createTranslationJobs creates.
type recordingJobStore struct {
	jobs.JobStore
	created []*jobs.TranslationJob
}

func (r *recordingJobStore) CreateJob(_ context.Context, job *jobs.TranslationJob) error {
	r.created = append(r.created, job)
	return nil
}

// acceptQueue accepts every enqueue.
type acceptQueue struct{ jobs.Queue }

func (acceptQueue) Enqueue(context.Context, string) error { return nil }

func TestCreateTranslationJobsProviderChoice(t *testing.T) {
	openaiDefault, ok := aiprovider.ProviderInfoFor(aiprovider.OpenAI)
	require.True(t, ok)
	require.NotEmpty(t, openaiDefault.DefaultModel)

	tests := []struct {
		name         string
		saved        *bstore.ProviderConfig // nil: the workspace has saved none
		lookupErr    error
		zeroCredits  bool
		props        map[string]string
		wantErr      error
		wantConfigID string
		wantModel    string
	}{
		{
			name:         "no saved config runs on the platform",
			wantConfigID: "platform",
			wantModel:    platformJobModel,
		},
		{
			name:         "platform job takes the project's ai_model",
			props:        map[string]string{"ai_model": "gpt-5.6"},
			wantConfigID: "platform",
			wantModel:    "gpt-5.6",
		},
		{
			name:        "platform job on a zero-credit workspace is refused",
			zeroCredits: true,
			wantErr:     errStallNeedsCredits,
		},
		{
			name:         "saved config with a model runs on that config and model",
			saved:        &bstore.ProviderConfig{ID: "cfg-1", Type: "anthropic", Model: "claude-sonnet-5"},
			props:        map[string]string{"ai_model": "gpt-5.6"},
			wantConfigID: "cfg-1",
			wantModel:    "claude-sonnet-5",
		},
		{
			name:         "saved config without a model takes the provider's default",
			saved:        &bstore.ProviderConfig{ID: "cfg-2", Type: "openai"},
			wantConfigID: "cfg-2",
			wantModel:    openaiDefault.DefaultModel,
		},
		{
			name:         "saved config skips the credit pre-check",
			saved:        &bstore.ProviderConfig{ID: "cfg-3", Type: "gemini", Model: "gemini-3.5-flash"},
			zeroCredits:  true,
			wantConfigID: "cfg-3",
			wantModel:    "gemini-3.5-flash",
		},
		{
			name:         "a failed lookup falls back to the platform",
			lookupErr:    assert.AnError,
			wantConfigID: "platform",
			wantModel:    platformJobModel,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			store := &recordingJobStore{}
			s := &Server{
				JobStore: store,
				JobQueue: acceptQueue{},
				latestProviderConfig: func(_ context.Context, wsID string) (bstore.ProviderConfig, error) {
					assert.Equal(t, "ws-1", wsID)
					if tc.lookupErr != nil {
						return bstore.ProviderConfig{}, tc.lookupErr
					}
					if tc.saved == nil {
						return bstore.ProviderConfig{}, bstore.ErrProviderConfigNotFound
					}
					return *tc.saved, nil
				},
			}
			if tc.zeroCredits {
				s.BillingStore = zeroCreditBillingStore{}
			}
			proj := &platstore.Project{ID: "proj-1", WorkspaceID: "ws-1", Properties: tc.props}

			ids, err := s.createTranslationJobs(context.Background(), proj, "main",
				[]string{"en.json"}, []string{"nb", "de"}, "push-1", "acme", "")

			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
				assert.Empty(t, ids)
				assert.Empty(t, store.created)
				return
			}
			require.NoError(t, err)
			require.Len(t, ids, 2)
			require.Len(t, store.created, 2)
			for _, job := range store.created {
				assert.Equal(t, tc.wantConfigID, job.ProviderConfigID)
				assert.Equal(t, tc.wantModel, job.Model)
				assert.Equal(t, "ws-1", job.WorkspaceID)
			}
		})
	}
}

// A workspace with no id never looks up a saved config.
func TestWorkspaceProviderConfigNeedsAWorkspace(t *testing.T) {
	s := &Server{latestProviderConfig: func(context.Context, string) (bstore.ProviderConfig, error) {
		t.Fatal("lookup must not run without a workspace id")
		return bstore.ProviderConfig{}, nil
	}}
	_, ok := s.workspaceProviderConfig(context.Background(), "")
	assert.False(t, ok)

	_, ok = (&Server{}).workspaceProviderConfig(context.Background(), "ws-1")
	assert.False(t, ok, "no provider store means no saved config")
}
