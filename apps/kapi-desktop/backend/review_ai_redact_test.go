package backend

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	aiprovider "github.com/neokapi/neokapi/providers/ai"
)

// newRedactedReviewProject opens a project whose default redaction policy
// withholds Ada Lovelace, with an English source and a French translation
// that name her.
func newRedactedReviewProject(t *testing.T, app *App) *TabInfo {
	t.Helper()
	root := t.TempDir()
	write := func(rel, body string) {
		t.Helper()
		require.NoError(t, os.MkdirAll(filepath.Dir(filepath.Join(root, rel)), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(root, rel), []byte(body), 0o600))
	}
	write("locales/en.json", `{"greeting":"Write to Ada Lovelace today."}`)
	write("locales/fr-FR.json", `{"greeting":"Écrivez à Ada Lovelace aujourd'hui."}`)
	write("redact.yaml", "version: v1\nrules:\n  - term: Ada Lovelace\n    category: person\n")
	proj := &project.KapiProject{
		Version: project.CurrentVersion,
		Name:    "Redacted review",
		Defaults: project.Defaults{
			SourceLanguage:  "en-US",
			TargetLanguages: []model.LocaleID{"fr-FR"},
			Redaction:       &project.RedactionSpec{Enabled: true, Rules: "redact.yaml", Detectors: []string{"rules"}},
		},
		Collections: []project.Collection{{
			Name:    "App",
			Content: []project.ContentItem{{Path: "locales/en.json", Target: "locales/{lang}.json"}},
		}},
	}
	path := filepath.Join(root, "project.kapi")
	require.NoError(t, project.Save(path, proj))
	tab, err := app.OpenProject(path)
	require.NoError(t, err)
	t.Cleanup(func() { app.CloseProject(tab.ID) })
	return tab
}

// The review actions run their tool between redact and unredact under the
// policy that covers the unit's file: the model never reads the name, and a
// proposal carries it again.
func TestReviewAIAction_AppliesTheRedactionPolicy(t *testing.T) {
	t.Run("retranslate", func(t *testing.T) {
		mock := aiprovider.NewMockProvider()
		mock.TranslateFunc = func(_ context.Context, req aiprovider.TranslateRequest) (*aiprovider.TranslateResponse, error) {
			return &aiprovider.TranslateResponse{Translation: "FR " + req.Source, Model: "mock-model"}, nil
		}
		app := newAIReviewApp(t, mock)
		tab := newRedactedReviewProject(t, app)
		res, err := app.ReviewAIAction(tab.ID, "fr-FR", filepath.Join("locales", "fr-FR.json"),
			"greeting", ReviewAIRetranslate, "Address Ada Lovelace formally.")
		require.NoError(t, err)
		require.Len(t, mock.TranslateCalls, 1)
		sent, err := json.Marshal(mock.TranslateCalls[0])
		require.NoError(t, err)
		assert.NotContains(t, string(sent), "Lovelace", "neither the block nor the instruction carries the name")
		assert.Equal(t, "FR Write to Ada Lovelace today.", res.ProposedTarget, "the proposal carries the name again")
	})
	t.Run("explain", func(t *testing.T) {
		mock := aiprovider.NewMockProvider()
		mock.ChatFunc = func(context.Context, []aiprovider.Message) (*aiprovider.ChatResponse, error) {
			return &aiprovider.ChatResponse{Content: `{"score": 90, "findings": []}`, Model: "mock-model"}, nil
		}
		app := newAIReviewApp(t, mock)
		tab := newRedactedReviewProject(t, app)
		_, err := app.ReviewAIAction(tab.ID, "fr-FR", filepath.Join("locales", "fr-FR.json"),
			"greeting", ReviewAIExplain, "")
		require.NoError(t, err)
		require.Len(t, mock.ChatCalls, 1)
		sent, err := json.Marshal(mock.ChatCalls[0])
		require.NoError(t, err)
		assert.NotContains(t, string(sent), "Lovelace", "neither the source nor the translation carries the name")
		assert.Contains(t, string(sent), "[REDACTED:Person]")
	})
}
