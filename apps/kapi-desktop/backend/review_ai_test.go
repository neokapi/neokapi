package backend

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	aitools "github.com/neokapi/neokapi/core/ai/tools"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/state"
	"github.com/neokapi/neokapi/core/tool"
	aiprovider "github.com/neokapi/neokapi/providers/ai"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newAIReviewApp builds an App whose review AI actions run against a
// MockProvider — the deterministic no-network pattern from core/ai/tools —
// with an isolated config dir so no developer defaults leak into identities.
func newAIReviewApp(t *testing.T, mock *aiprovider.MockProvider) *App {
	t.Helper()
	t.Setenv("KAPI_CONFIG_DIR", t.TempDir())
	app := NewApp()
	t.Cleanup(func() {
		if app.aiActivityStop != nil {
			app.aiActivityStop()
		}
	})
	// The production path builds its provider through aiprovider.NewProvider,
	// which applies the recording wrapper itself. A factory that hands a tool a
	// mock skips that, so wrap here: without it these tests would exercise the
	// AI actions with the activity log silently switched off.
	recorded := aiprovider.Recording(mock)
	app.aiToolFactory = func(name string, cfg map[string]any, targetLang string) (tool.Tool, error) {
		switch name {
		case "translate":
			c := aitools.AITranslateConfig{
				SourceLocale: "en-US",
				TargetLocale: model.LocaleID(targetLang),
				BatchSize:    1,
			}
			if ins, _ := cfg["instruction"].(string); ins != "" {
				c.Instruction = ins
			}
			return aitools.NewAITranslateTool(recorded, c), nil
		case "review":
			return aitools.NewAIReviewTool(recorded, aitools.AIReviewConfig{
				SourceLocale: "en-US",
				TargetLocale: model.LocaleID(targetLang),
			}), nil
		default:
			return nil, fmt.Errorf("unexpected tool %q", name)
		}
	}
	return app
}

func TestReviewAIAction_Retranslate(t *testing.T) {
	mock := aiprovider.NewMockProvider()
	mock.TranslateFunc = func(_ context.Context, req aiprovider.TranslateRequest) (*aiprovider.TranslateResponse, error) {
		return &aiprovider.TranslateResponse{Translation: "Salut {name} !", Model: "mock-model"}, nil
	}
	app := newAIReviewApp(t, mock)
	tab, root := newReviewProject(t, app)

	res, err := app.ReviewAIAction(tab.ID, "fr-FR", filepath.Join("locales", "fr-FR.json"),
		"greeting", ReviewAIRetranslate, "make it informal")
	require.NoError(t, err)
	assert.Equal(t, "Salut {name} !", res.ProposedTarget)
	assert.Empty(t, res.Explanation)

	// The reviewer's instruction reached the model verbatim.
	require.Len(t, mock.TranslateCalls, 1)
	assert.Equal(t, "make it informal", mock.TranslateCalls[0].Instruction)
	assert.Equal(t, "Hello {name}", mock.TranslateCalls[0].Source)

	// A proposal never writes the target file — Accept routes through
	// UpdateReviewTarget.
	data, rerr := os.ReadFile(filepath.Join(root, "locales", "fr-FR.json"))
	require.NoError(t, rerr)
	assert.Contains(t, string(data), "Bonjour {name}")
}

func TestReviewAIAction_RetranslateNeedsInstruction(t *testing.T) {
	app := newAIReviewApp(t, aiprovider.NewMockProvider())
	tab, _ := newReviewProject(t, app)
	_, err := app.ReviewAIAction(tab.ID, "fr-FR", filepath.Join("locales", "fr-FR.json"),
		"greeting", ReviewAIRetranslate, "  ")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "instruction")
}

func TestReviewAIAction_FixFindingsCarriesFindings(t *testing.T) {
	mock := aiprovider.NewMockProvider()
	mock.TranslateFunc = func(_ context.Context, req aiprovider.TranslateRequest) (*aiprovider.TranslateResponse, error) {
		return &aiprovider.TranslateResponse{Translation: "Hallo {name}", Model: "mock-model"}, nil
	}
	app := newAIReviewApp(t, mock)
	tab, _ := newReviewProject(t, app)

	// de-DE greeting dropped the {name} placeholder — a check finding.
	res, err := app.ReviewAIAction(tab.ID, "de-DE", filepath.Join("locales", "de-DE.json"),
		"greeting", ReviewAIFixFindings, "")
	require.NoError(t, err)
	assert.Equal(t, "Hallo {name}", res.ProposedTarget)

	require.Len(t, mock.TranslateCalls, 1)
	ins := mock.TranslateCalls[0].Instruction
	assert.Contains(t, ins, "Hallo", "the instruction carries the current translation")
	assert.Contains(t, ins, "{name}", "the placeholder finding is a constraint")
	assert.Contains(t, ins, "review findings")
}

func TestReviewAIAction_Explain(t *testing.T) {
	mock := aiprovider.NewMockProvider()
	mock.ChatFunc = func(context.Context, []aiprovider.Message) (*aiprovider.ChatResponse, error) {
		return &aiprovider.ChatResponse{
			Content: `{"score": 87, "findings": [{"severity": "minor", "message": "slightly literal", "suggestion": "Salut"}]}`,
			Model:   "mock-model",
		}, nil
	}
	app := newAIReviewApp(t, mock)
	tab, root := newReviewProject(t, app)

	res, err := app.ReviewAIAction(tab.ID, "fr-FR", filepath.Join("locales", "fr-FR.json"),
		"greeting", ReviewAIExplain, "")
	require.NoError(t, err)
	assert.Empty(t, res.ProposedTarget, "explain is read-only")
	assert.Contains(t, res.Explanation, "Score: 87/100")
	assert.Contains(t, res.Explanation, "slightly literal")
	assert.Contains(t, res.Explanation, "Salut")

	// Read-only: no state write, no file write. The decision record is the
	// committed shard set under `.kapi/state/`, so a write shows up there —
	// asserting on any other path makes this vacuously true.
	layout := project.LayoutAt(root)
	units, _ := os.ReadDir(layout.Export().UnitStateDir())
	assert.Empty(t, units, "explain must record no decision")
}

func TestReviewAIAction_UnknownAction(t *testing.T) {
	app := newAIReviewApp(t, aiprovider.NewMockProvider())
	tab, _ := newReviewProject(t, app)
	_, err := app.ReviewAIAction(tab.ID, "fr-FR", filepath.Join("locales", "fr-FR.json"),
		"greeting", "improvise", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown review AI action")
}

func TestReviewAIAction_NoProviderConfigured(t *testing.T) {
	t.Setenv("KAPI_CONFIG_DIR", t.TempDir())
	t.Setenv("ANTHROPIC_API_KEY", "")
	app := NewApp() // no factory injected — the real resolution path, empty store
	tab, _ := newReviewProject(t, app)

	_, err := app.ReviewAIAction(tab.ID, "fr-FR", filepath.Join("locales", "fr-FR.json"),
		"greeting", ReviewAIRetranslate, "shorter")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Settings", "the UI-facing message points at Settings")
}

// scriptedReviewScores returns a ChatFunc that scores by the reviewed
// translation found in the prompt.
func scriptedReviewScores(scores map[string]int) func(context.Context, []aiprovider.Message) (*aiprovider.ChatResponse, error) {
	return func(_ context.Context, msgs []aiprovider.Message) (*aiprovider.ChatResponse, error) {
		prompt := msgs[len(msgs)-1].Text()
		for needle, score := range scores {
			if needle != "" && strings.Contains(prompt, needle) {
				return &aiprovider.ChatResponse{
					Content: fmt.Sprintf(`{"score": %d, "findings": []}`, score),
					Model:   "mock-model",
				}, nil
			}
		}
		return &aiprovider.ChatResponse{Content: `{"score": 50, "findings": [{"severity":"major","message":"weak"}]}`, Model: "mock-model"}, nil
	}
}

func TestRunAIPreReview_AnnotateOnly(t *testing.T) {
	mock := aiprovider.NewMockProvider()
	mock.ChatFunc = scriptedReviewScores(map[string]int{"Bonjour": 95, "Au revoir": 40})
	app := newAIReviewApp(t, mock)
	tab, root := newReviewProject(t, app)

	res, err := app.RunAIPreReview(tab.ID, "fr-FR", PreReviewScope{})
	require.NoError(t, err)
	assert.Equal(t, 2, res.Reviewed)

	// The queue surfaces the stored scores; nothing left the queue.
	queue, err := app.ReviewQueue(tab.ID, ProjectFilter{})
	require.NoError(t, err)
	frScores := map[string]int{}
	for _, it := range queue.Pending {
		if it.Locale == "fr-FR" {
			require.NotNil(t, it.AIScore, "fr units carry their annotation")
			frScores[it.Key] = *it.AIScore
			assert.Equal(t, "mock", it.AIModel)
		} else {
			assert.Nil(t, it.AIScore, "de units were out of scope")
		}
	}
	assert.Equal(t, map[string]int{"greeting": 95, "farewell": 40}, frScores)

	// No decisions were recorded — only annotations.
	f := struct{ Units []state.UnitState }{Units: commitAndReadUnits(t, app, root)}
	for _, u := range f.Units {
		assert.Empty(t, u.Decision.ReviewState)
		assert.Empty(t, u.Status)
		require.NotNil(t, u.AIReview)
	}

	// The unit detail shows the annotation for the CONTEXT row.
	d, err := app.GetReviewUnit(tab.ID, "fr-FR", filepath.Join("locales", "fr-FR.json"), "greeting")
	require.NoError(t, err)
	require.NotNil(t, d.AIReviewScore)
	assert.Equal(t, 95, *d.AIReviewScore)
	assert.Equal(t, "mock", d.AIReviewModel)
}

func TestRunAIPreReview_ProseFallbackSkipped(t *testing.T) {
	mock := aiprovider.NewMockProvider()
	mock.ChatFunc = func(context.Context, []aiprovider.Message) (*aiprovider.ChatResponse, error) {
		return &aiprovider.ChatResponse{Content: "Looks fine to me!", Model: "mock-model"}, nil
	}
	app := newAIReviewApp(t, mock)
	tab, _ := newReviewProject(t, app)

	res, err := app.RunAIPreReview(tab.ID, "fr-FR", PreReviewScope{})
	require.NoError(t, err)
	assert.Equal(t, 0, res.Reviewed, "prose responses carry no usable score")
	assert.Equal(t, 2, res.Skipped)
}

func TestRunAIPreReview_EmptyScope(t *testing.T) {
	app := newAIReviewApp(t, aiprovider.NewMockProvider())
	tab, _ := newReviewProject(t, app)
	res, err := app.RunAIPreReview(tab.ID, "fr-FR", PreReviewScope{Collection: "nope"})
	require.NoError(t, err)
	assert.Zero(t, res.Reviewed)
}

// commitAndReadUnits reads the units the project's decision ledger holds.
// Recording puts a decision in the ledger, where it is durable at once.
//
// It reads through the app's own engine. The working store is a schema of the
// project's one store, so reading from an App of its own would be a second
// connection pool on the file the app is holding open.
func commitAndReadUnits(t *testing.T, app *App, root string) []state.UnitState {
	t.Helper()
	st, err := app.hostEngine().OpenProjectState(t.Context(), root)
	require.NoError(t, err)
	units, err := st.All(t.Context())
	require.NoError(t, err)
	return units
}
