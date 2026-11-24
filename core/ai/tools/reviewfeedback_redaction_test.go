package tools_test

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/ai/tools"
	"github.com/neokapi/neokapi/core/blockstore/sqlitestore"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/redaction"
	"github.com/neokapi/neokapi/core/tool"
	libtools "github.com/neokapi/neokapi/core/tools"
	aiprovider "github.com/neokapi/neokapi/providers/ai"
)

// A value a redact step withheld from a block is withheld from the review
// feedback sent with it too: the rejected wording and the reviewer's note go
// out under the policy that redacted the block, on the per-block route and on
// the batch route.
func TestAITranslate_RedactsTheReviewFeedbackSentWithABlock(t *testing.T) {
	for _, route := range []struct {
		name      string
		batchSize int
		blocks    []string
	}{
		{"one block per request", 1, []string{"b1"}},
		{"a batch", 2, []string{"b1", "b2"}},
	} {
		t.Run(route.name, func(t *testing.T) {
			var mu sync.Mutex
			var sent []string
			mock := aiprovider.NewMockProvider()
			mock.TranslateFunc = func(_ context.Context, req aiprovider.TranslateRequest) (*aiprovider.TranslateResponse, error) {
				raw, _ := json.Marshal(req)
				mu.Lock()
				sent = append(sent, string(raw))
				mu.Unlock()
				return &aiprovider.TranslateResponse{Translation: "Merci.", Model: "m"}, nil
			}
			mock.ChatStructuredFunc = func(_ context.Context, messages []aiprovider.Message, _ aiprovider.JSONSchema) (*aiprovider.ChatResponse, error) {
				raw, _ := json.Marshal(messages)
				mu.Lock()
				sent = append(sent, string(raw))
				mu.Unlock()
				var in struct {
					Segments []struct {
						ID string `json:"id"`
					} `json:"segments"`
				}
				_ = json.Unmarshal([]byte(messages[len(messages)-1].Text()), &in)
				type entry struct {
					ID   string `json:"id"`
					Text string `json:"text"`
				}
				var out struct {
					Translations []entry `json:"translations"`
				}
				for _, s := range in.Segments {
					out.Translations = append(out.Translations, entry{ID: s.ID, Text: "Merci."})
				}
				b, _ := json.Marshal(out)
				return &aiprovider.ChatResponse{Content: string(b), Model: "m"}, nil
			}

			units := tool.ReviewFeedback{}
			for _, id := range route.blocks {
				units[id] = tool.UnitReviewFeedback{Rejections: []tool.ReviewRejection{{
					Text:    "Merci à Ada Lovelace.",
					Reasons: []string{"tone"},
					Note:    "Ada Lovelace is addressed formally.",
				}}}
			}
			cfg := tools.AITranslateConfig{
				SourceLocale: "en", TargetLocale: "fr", Provider: "anthropic", Model: "claude-x",
				BatchSize: route.batchSize, BatchConcurrency: 1,
				ReviewFeedback: tool.ReviewFeedbackGrant{Units: units},
			}
			translateRedacted(t, cfg, mock, route.blocks)

			all := strings.Join(sent, "\n")
			require.NotEmpty(t, sent, "the model was asked")
			assert.Contains(t, all, tool.RejectionReasonPhrase("tone"), "the feedback was sent")
			assert.NotContains(t, all, "Merci à Ada", "the rejected wording is redacted")
			assert.NotContains(t, all, "Ada Lovelace is addressed", "the note is redacted")
			assert.NotContains(t, all, "Lovelace")
		})
	}
}

// translateRedacted redacts a block per id ("Thank Ada Lovelace.") and runs the
// translate tool over them, as a flow with a redact step does.
func translateRedacted(t *testing.T, cfg tools.AITranslateConfig, p aiprovider.LLMProvider, ids []string) {
	t.Helper()
	ctx := context.Background()
	store, err := sqlitestore.New(filepath.Join(t.TempDir(), "blocks.db"))
	require.NoError(t, err)
	defer store.Close()
	redact, err := libtools.NewRedactTool(&libtools.RedactConfig{
		Detectors: []string{libtools.DetectRules},
		Rules:     []redaction.Rule{{Term: "Ada Lovelace", Category: redaction.CategoryPerson}},
	})
	require.NoError(t, err)

	in := make(chan *model.Part, len(ids))
	out := make(chan *model.Part, len(ids))
	for _, id := range ids {
		b := model.NewBlock(id, "Thank Ada Lovelace.")
		b.Translatable = true
		part := &model.Part{Type: model.PartBlock, Resource: b}
		_, err = redact.ApplyContext(ctx, part)
		require.NoError(t, err)
		in <- part
	}
	close(in)
	sess, err := store.Begin(ctx)
	require.NoError(t, err)
	require.NoError(t, tools.NewAITranslateTool(p, cfg).SessionProcess(ctx, sess, in, out))
	close(out)
	for range out {
	}
	require.NoError(t, sess.Commit())
}
