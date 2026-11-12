package host

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	aitools "github.com/neokapi/neokapi/core/ai/tools"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/tool"
	aiprovider "github.com/neokapi/neokapi/providers/ai"
)

// writePreReviewProject is writeReviewProject with the review tool preset to
// the demo provider, which answers locally.
func writePreReviewProject(t *testing.T) string {
	t.Helper()
	clearProviderEnv(t)
	t.Setenv("KAPI_CONFIG_DIR", t.TempDir())
	aiprovider.SetDemoNoticeWriter(nopWriter{})
	root := writeReviewProject(t)
	recipe := `version: v1
name: rev
defaults:
  source_language: en
  target_languages: [nb]
  tools:
    review:
      provider: demo
collections:
  - path: en.json
    target: "{lang}.json"
`
	require.NoError(t, os.WriteFile(filepath.Join(root, "kapi.yaml"), []byte(recipe), 0o644))
	return root
}

func TestRunUp_PreReviewScoresTheQueueAfterTheRun(t *testing.T) {
	root := writePreReviewProject(t)
	recipe := filepath.Join(root, "kapi.yaml")

	a := &App{}
	out, err := a.RunUp(context.Background(), recipe, "", UpOptions{MaxPasses: 1, NoChecks: true, PreReview: true})
	require.NoError(t, err)
	require.NotNil(t, out.PreReview)
	// The demo provider answers a review in prose, with no score to record,
	// so each block is counted as skipped and stays unscored.
	assert.Equal(t, 0, out.PreReview.Reviewed)
	assert.Equal(t, 2, out.PreReview.Skipped)
	assert.Equal(t, "demo", out.PreReview.Model, "the reviewer is the provider the recipe presets")

	var buf bytes.Buffer
	require.NoError(t, out.FormatText(&buf))
	assert.Contains(t, buf.String(), "AI pre-review: 0 blocks scored by demo, 2 without a usable score.")

	plain, err := a.RunUp(context.Background(), recipe, "", UpOptions{MaxPasses: 1, NoChecks: true})
	require.NoError(t, err)
	assert.Nil(t, plain.PreReview, "a run that asks for none reports none")
}

func TestExecuteUp_PreReviewIsRefusedWithPlan(t *testing.T) {
	root := writePreReviewProject(t)
	t.Chdir(root)

	a := &App{}
	cmd := NewEnvCommand(context.Background(), "up")
	AddProjectFlag(cmd)
	AddUpFlags(cmd)
	require.NoError(t, cmd.Flags().Set("plan", "true"))
	require.NoError(t, cmd.Flags().Set("pre-review", "true"))
	err := a.ExecuteUp(cmd, filepath.Join(root, "kapi.yaml"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--plan runs nothing")
}

func TestRunAIPreReview_ToolHookAndFilters(t *testing.T) {
	root := writePreReviewProject(t)
	recipe := filepath.Join(root, "kapi.yaml")

	mock := aiprovider.NewMockProvider()
	mock.ChatFunc = func(context.Context, []aiprovider.Message) (*aiprovider.ChatResponse, error) {
		return &aiprovider.ChatResponse{Content: `{"score": 81, "findings": [{"severity": "minor", "message": "literal"}]}`}, nil
	}
	var built []string
	var scoped, progress int
	opts := PreReviewOptions{
		Model: "judge",
		NewTool: func(name string, cfg map[string]any, targetLang string) (tool.Tool, error) {
			built = append(built, name+":"+targetLang)
			assert.Equal(t, "demo", cfg["provider"], "the hook receives the assembled config")
			return aitools.NewAIReviewTool(mock, aitools.AIReviewConfig{SourceLocale: "en", TargetLocale: model.LocaleID(targetLang)}), nil
		},
		UnitContext: func(ctx context.Context, _ ReviewQueueItem) context.Context { scoped++; return ctx },
		Progress:    func(_, _ int, _ ReviewQueueItem) { progress++ },
	}

	a := &App{}
	none, err := a.RunAIPreReview(context.Background(), recipe, "", PreReviewOptions{Model: "judge", Languages: []string{"de"}, NewTool: opts.NewTool})
	require.NoError(t, err)
	assert.Zero(t, none.Reviewed, "no block waits in a language outside the filter")
	assert.Empty(t, built)

	res, err := a.RunAIPreReview(context.Background(), recipe, "", opts)
	require.NoError(t, err)
	assert.Equal(t, 2, res.Reviewed)
	assert.Equal(t, "judge", res.Model)
	assert.Equal(t, []string{"review:nb"}, built, "one judge per file and language")
	assert.Equal(t, 2, scoped)
	assert.Equal(t, 2, progress)

	rep, err := a.ProjectConvergence(context.Background(), recipe, "")
	require.NoError(t, err)
	require.Len(t, rep.Review, 2)
	for _, it := range rep.Review {
		require.NotNil(t, it.AIScore)
		assert.Equal(t, 81, *it.AIScore)
		assert.Equal(t, "judge", it.AIModel)
	}
}
