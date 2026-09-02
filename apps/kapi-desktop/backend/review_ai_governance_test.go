package backend

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	aitools "github.com/neokapi/neokapi/core/ai/tools"
	"github.com/neokapi/neokapi/core/gate"
	"github.com/neokapi/neokapi/core/model"
	coreprofile "github.com/neokapi/neokapi/core/profile"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/tool"
	aiprovider "github.com/neokapi/neokapi/providers/ai"
	"github.com/neokapi/neokapi/terms"
	"github.com/neokapi/neokapi/terms/ktb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Review's AI actions used to hand-write their tool config: a source locale, an
// instruction, and nothing else. So a retranslation proposed in Review was
// off-voice by construction while the checks card above it judged that same
// unit against the voice, and the pre-review judge scored a bare pair. These
// tests assert the config each action actually builds with.

const reviewVoiceYAML = `id: house
name: House Style
tone:
  formality: formal
`

// capturedConfigs records the config every AI tool an action built was given,
// keyed by tool name.
type capturedConfigs struct {
	byTool map[string][]map[string]any
}

func (c *capturedConfigs) last(toolName string) map[string]any {
	got := c.byTool[toolName]
	if len(got) == 0 {
		return nil
	}
	return got[len(got)-1]
}

// newGovernedReviewApp is newAIReviewApp over a project that binds a voice
// profile, a terms source and a translate preset, with the config each tool is
// built with captured.
func newGovernedReviewApp(t *testing.T, mock *aiprovider.MockProvider) (*App, *capturedConfigs) {
	t.Helper()
	t.Setenv("KAPI_CONFIG_DIR", t.TempDir())
	app := NewApp()
	t.Cleanup(func() {
		if app.aiActivityStop != nil {
			app.aiActivityStop()
		}
	})
	captured := &capturedConfigs{byTool: map[string][]map[string]any{}}
	recorded := aiprovider.Recording(mock)
	app.aiToolFactory = func(name string, cfg map[string]any, targetLang string) (tool.Tool, error) {
		captured.byTool[name] = append(captured.byTool[name], cfg)
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
	return app, captured
}

// newGovernedReviewProject writes the review fixture with governance on it.
func newGovernedReviewProject(t *testing.T, app *App) (*TabInfo, string) {
	t.Helper()
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "locales"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "locales", "en.json"),
		[]byte(`{"greeting":"Hello {name}","farewell":"Goodbye"}`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "locales", "fr-FR.json"),
		[]byte(`{"greeting":"Bonjour {name}","farewell":"Au revoir"}`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "voice.yaml"), []byte(reviewVoiceYAML), 0o644))

	stamp := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	data, err := ktb.Marshal(ktb.FromConcepts([]terms.Concept{{
		ID:     "term:greeting",
		Source: terms.TermSourceTerminology,
		Terms: []terms.Term{
			{Text: "Hello", Locale: "en-US", Status: model.TermPreferred},
			{Text: "Bonjour", Locale: "fr-FR", Status: model.TermPreferred},
		},
		CreatedAt: stamp,
		UpdatedAt: stamp,
	}}))
	require.NoError(t, err)
	termsPath := filepath.Join(root, project.RelStatePath(ktb.ConventionalName))
	require.NoError(t, os.MkdirAll(filepath.Dir(termsPath), 0o755))
	require.NoError(t, os.WriteFile(termsPath, data, 0o644))

	proj := &project.KapiProject{
		Version: project.CurrentVersion,
		Name:    "GovernedReview",
		Defaults: project.Defaults{
			SourceLanguage:  "en-US",
			TargetLanguages: []model.LocaleID{"fr-FR"},
			Voice:           &project.VoiceBinding{ProfileFile: "voice.yaml"},
			TermsSource:     project.RelStatePath(ktb.ConventionalName),
			Tools: map[string]map[string]any{
				"translate": {"context": "neighbours", "contextWindow": 4},
			},
		},
		Collections: []project.Collection{{
			Name:    "App",
			Content: []project.ContentItem{{Path: "locales/en.json", Target: "locales/{lang}.json"}},
		}},
		ShipGate: gate.Gate{"translated": {Pct: 100}, "reviewed": {Pct: 50}},
	}
	path := filepath.Join(root, project.RecipeFileName)
	require.NoError(t, project.Save(path, proj))

	tab, oerr := app.OpenProject(path)
	require.NoError(t, oerr)
	t.Cleanup(func() { app.CloseProject(tab.ID) })
	return tab, root
}

// assertGoverned checks that a config carries what the project binds.
func assertGoverned(t *testing.T, cfg map[string]any, what string) {
	t.Helper()
	require.NotNil(t, cfg, "%s built no tool", what)

	profile, ok := cfg["profile"].(*coreprofile.VoiceProfile)
	require.True(t, ok, "%s: the governing voice profile reaches the tool", what)
	assert.Equal(t, "House Style", profile.Name)

	rules, ok := cfg["term_rules"].([]coreprofile.TermRule)
	require.True(t, ok, "%s: the governing term rules reach the tool", what)
	assert.Equal(t, map[string]string{"Hello": "Bonjour"}, coreprofile.TermRuleMap(rules),
		"%s: resolved for the unit's own locale", what)
}

func TestReviewAI_RetranslateIsGoverned(t *testing.T) {
	mock := aiprovider.NewMockProvider()
	mock.TranslateFunc = func(context.Context, aiprovider.TranslateRequest) (*aiprovider.TranslateResponse, error) {
		return &aiprovider.TranslateResponse{Translation: "Salut {name} !", Model: "mock-model"}, nil
	}
	app, captured := newGovernedReviewApp(t, mock)
	tab, _ := newGovernedReviewProject(t, app)

	_, err := app.ReviewAIAction(tab.ID, "fr-FR", filepath.Join("locales", "fr-FR.json"),
		"greeting", ReviewAIRetranslate, "make it informal")
	require.NoError(t, err)

	cfg := captured.last("translate")
	assertGoverned(t, cfg, "retranslate")
	assert.Equal(t, "neighbours", cfg["context"], "the recipe's translate preset reaches Review")
	assert.Equal(t, 4, cfg["contextWindow"])
	assert.Equal(t, "make it informal", cfg["instruction"], "the action's own keys survive")
	assert.Equal(t, 1, cfg["batchSize"])
}

func TestReviewAI_FixFindingsIsGoverned(t *testing.T) {
	mock := aiprovider.NewMockProvider()
	mock.TranslateFunc = func(context.Context, aiprovider.TranslateRequest) (*aiprovider.TranslateResponse, error) {
		return &aiprovider.TranslateResponse{Translation: "Bonjour {name}", Model: "mock-model"}, nil
	}
	app, captured := newGovernedReviewApp(t, mock)
	tab, _ := newGovernedReviewProject(t, app)

	_, err := app.ReviewAIAction(tab.ID, "fr-FR", filepath.Join("locales", "fr-FR.json"),
		"greeting", ReviewAIFixFindings, "")
	require.NoError(t, err)

	assertGoverned(t, captured.last("translate"), "fix-findings")
}

func TestReviewAI_ExplainIsGoverned(t *testing.T) {
	mock := aiprovider.NewMockProvider()
	mock.ChatFunc = func(context.Context, []aiprovider.Message) (*aiprovider.ChatResponse, error) {
		return &aiprovider.ChatResponse{
			Content: `{"score": 90, "findings": []}`,
			Model:   "mock-model",
		}, nil
	}
	app, captured := newGovernedReviewApp(t, mock)
	tab, _ := newGovernedReviewProject(t, app)

	_, err := app.ReviewAIAction(tab.ID, "fr-FR", filepath.Join("locales", "fr-FR.json"),
		"greeting", ReviewAIExplain, "")
	require.NoError(t, err)

	assertGoverned(t, captured.last("review"), "explain")
}

// TestReviewAI_PreReviewJudgeIsGoverned: the batch judge scores against the
// voice and vocabulary in force, so a target that reads well and says the wrong
// thing is not auto-approved on a clean score.
func TestReviewAI_PreReviewJudgeIsGoverned(t *testing.T) {
	mock := aiprovider.NewMockProvider()
	mock.ChatFunc = func(context.Context, []aiprovider.Message) (*aiprovider.ChatResponse, error) {
		return &aiprovider.ChatResponse{
			Content: `{"score": 95, "findings": []}`,
			Model:   "mock-model",
		}, nil
	}
	app, captured := newGovernedReviewApp(t, mock)
	tab, _ := newGovernedReviewProject(t, app)

	res, err := app.RunAIPreReview(tab.ID, "fr-FR", PreReviewScope{}, PreReviewPolicy{})
	require.NoError(t, err)
	require.Positive(t, res.Reviewed, "the queue had units to judge")

	assertGoverned(t, captured.last("review"), "pre-review")
}
