package tools_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/ai/tools"
	"github.com/neokapi/neokapi/core/model"
	aiprovider "github.com/neokapi/neokapi/providers/ai"
)

// inboxBlock is an ARB-style plural message with text around it.
func inboxBlock(id string) *model.Block {
	n := model.PhR(model.PlaceholderRun{ID: "p1", Type: "icu", Data: "#"})
	b := model.NewRunsBlock(id, []model.Run{
		model.TextR("You have "),
		{Plural: &model.PluralRun{Pivot: "count", Forms: map[model.PluralForm][]model.Run{
			"=0":              {model.TextR("no new messages")},
			model.PluralOne:   {n, model.TextR(" new message")},
			model.PluralOther: {n, model.TextR(" new messages")},
		}}},
		model.TextR(" in your inbox."),
	})
	b.SourceLocale = model.LocaleEnglish
	return b
}

func translateParts(t *testing.T, tl *tools.AITranslateTool, blocks ...*model.Block) []*model.Block {
	t.Helper()
	in := make(chan *model.Part, len(blocks))
	out := make(chan *model.Part, len(blocks))
	for _, b := range blocks {
		in <- &model.Part{Type: model.PartBlock, Resource: b}
	}
	close(in)
	require.NoError(t, tl.Process(t.Context(), in, out))
	close(out)
	var got []*model.Block
	for p := range out {
		got = append(got, p.Resource.(*model.Block))
	}
	return got
}

func assertInboxTranslated(t *testing.T, b *model.Block) {
	t.Helper()
	runs := b.TargetRuns(model.LocaleFrench)
	require.Len(t, runs, 3, "the target keeps the text, the plural and the text")
	// The text around the plural was translated with the plural in place.
	assert.Equal(t, "[fr] You have ", runs[0].Text.Text)
	assert.Equal(t, " in your inbox.", runs[2].Text.Text)
	require.NotNil(t, runs[1].Plural, "the plural is still a plural")
	forms := map[string]string{}
	for k, v := range runs[1].Plural.Forms {
		forms[string(k)] = model.RunsPlaceholderText(v)
	}
	assert.Equal(t, map[string]string{
		"=0":    "[fr] no new messages",
		"one":   `[fr] <x id="p1/"/> new message`,
		"other": `[fr] <x id="p1/"/> new messages`,
	}, forms)
	tgt, ok := b.Edition(model.Variant(model.LocaleFrench))
	require.True(t, ok)
	assert.Equal(t, model.Status(model.TargetStatusDraft), tgt.Status)
}

// A block holding a plural is translated a branch at a time, so every branch
// is translated and the target holds the plural, where translating the whole
// source as text would keep one branch.
func TestAITranslate_APluralKeepsEveryBranch(t *testing.T) {
	mock, calls := newTranslateMock(t)
	got := translateParts(t, tools.NewAITranslateTool(mock, singleBlockConfig()), inboxBlock("tu1"))
	require.Len(t, got, 1)
	assertInboxTranslated(t, got[0])
	assert.Equal(t, 4, *calls, "three branches and the text around the plural")
}

// In a batched run a block holding a plural is translated on its own, a
// branch at a time; the other blocks are batched as before.
func TestAITranslate_APluralIsNotBatched(t *testing.T) {
	mock := aiprovider.NewMockProvider()
	var mu sync.Mutex
	var batched []string
	mock.ChatStructuredFunc = func(_ context.Context, messages []aiprovider.Message, _ aiprovider.JSONSchema) (*aiprovider.ChatResponse, error) {
		data, err := json.Marshal(messages)
		if err != nil {
			return nil, err
		}
		mu.Lock()
		batched = append(batched, string(data))
		mu.Unlock()
		// No translations: each block falls back to a call of its own.
		return &aiprovider.ChatResponse{Content: `{"translations": []}`, Model: "test-model"}, nil
	}
	mock.TranslateFunc = func(_ context.Context, req aiprovider.TranslateRequest) (*aiprovider.TranslateResponse, error) {
		return &aiprovider.TranslateResponse{Translation: "[fr] " + req.Source, Model: "test-model"}, nil
	}
	cfg := singleBlockConfig()
	cfg.BatchSize = 10
	save, cancel := model.NewBlock("tu1", "Save"), model.NewBlock("tu2", "Cancel")
	save.SourceLocale, cancel.SourceLocale = model.LocaleEnglish, model.LocaleEnglish
	got := translateParts(t, tools.NewAITranslateTool(mock, cfg), save, cancel, inboxBlock("tu3"))
	require.Len(t, got, 3)
	require.NotEmpty(t, batched, "the plain blocks were batched")
	for _, b := range batched {
		assert.NotContains(t, b, "new message", "no batch carries the plural")
	}
	assert.Equal(t, "[fr] Save", got[0].TargetText(model.LocaleFrench))
	assertInboxTranslated(t, got[2])
}

func TestAITranslate_APluralFailsWhenABranchFails(t *testing.T) {
	mock := aiprovider.NewMockProvider()
	mock.TranslateFunc = func(_ context.Context, req aiprovider.TranslateRequest) (*aiprovider.TranslateResponse, error) {
		if strings.Contains(req.Source, "messages") {
			return nil, errors.New("provider offline")
		}
		return &aiprovider.TranslateResponse{Translation: "[fr] " + req.Source}, nil
	}
	in := make(chan *model.Part, 1)
	out := make(chan *model.Part, 1)
	in <- &model.Part{Type: model.PartBlock, Resource: inboxBlock("tu1")}
	close(in)
	err := tools.NewAITranslateTool(mock, singleBlockConfig()).Process(t.Context(), in, out)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "provider offline")
}
