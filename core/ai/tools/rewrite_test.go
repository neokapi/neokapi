package tools

import (
	"context"
	"regexp"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/tool"
	aiprovider "github.com/neokapi/neokapi/providers/ai"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// lastUserText returns the text of the last user message — the placeholder text
// the rewrite tool sends. Mock ChatFuncs transform it and echo it back.
func lastUserText(msgs []aiprovider.Message) string {
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role == "user" {
			return msgs[i].Text()
		}
	}
	return ""
}

// chatReplace builds a mock whose Chat replaces old→new in the user text,
// leaving any placeholder tags untouched — a stand-in for a faithful rewrite.
func chatReplace(p *aiprovider.MockProvider, old, repl string) {
	p.ChatFunc = func(_ context.Context, msgs []aiprovider.Message) (*aiprovider.ChatResponse, error) {
		out := strings.ReplaceAll(lastUserText(msgs), old, repl)
		return &aiprovider.ChatResponse{Content: out, Model: "mock"}, nil
	}
}

// applyRewrite runs the rewrite tool over a single block and returns it.
func applyRewrite(t *testing.T, tl *tool.BaseTool, b *model.Block) {
	t.Helper()
	_, err := tl.Apply(&model.Part{Type: model.PartBlock, Resource: b})
	require.NoError(t, err)
}

// TestRewritePreservesInlineCodes proves the moat: a block with an inline code
// (a placeholder) survives the rewrite — the tool renders the runs with a
// placeholder tag, the model echoes the tag, and ParseRunsPlaceholderText
// reconstructs the run, so the code is still there after the text changed.
func TestRewritePreservesInlineCodes(t *testing.T) {
	mock := aiprovider.NewMockProvider()
	chatReplace(mock, "world", "planet")
	tl := NewRewriteTool(mock, RewriteConfig{Instruction: "swap words"})

	b := &model.Block{ID: "b1", Translatable: true, Source: []model.Run{
		{Text: &model.TextRun{Text: "Hello "}},
		{Ph: &model.PlaceholderRun{ID: "1", Type: "icon"}},
		{Text: &model.TextRun{Text: " world"}},
	}}
	applyRewrite(t, tl, b)

	// The placeholder run is still present and the text changed.
	require.Len(t, b.Source, 3)
	require.NotNil(t, b.Source[1].Ph, "the inline placeholder run must survive the rewrite")
	assert.Equal(t, "1", b.Source[1].Ph.ID)
	assert.Equal(t, "Hello  planet", model.RunsText(b.Source))
}

// TestRewriteRejectsCodeLoss proves the faithfulness guard: when the model
// drops an inline code it was told to preserve (here the placeholder tag), the
// rewrite would unbalance the document's markup — so the block is left exactly
// as it was rather than written back corrupted.
func TestRewriteRejectsCodeLoss(t *testing.T) {
	mock := aiprovider.NewMockProvider()
	tagRe := regexp.MustCompile(`<x[^>]*/>`)
	mock.ChatFunc = func(_ context.Context, msgs []aiprovider.Message) (*aiprovider.ChatResponse, error) {
		// Strip the placeholder tag — a model that fails to echo the codes.
		out := tagRe.ReplaceAllString(lastUserText(msgs), "")
		return &aiprovider.ChatResponse{Content: out, Model: "mock"}, nil
	}
	tl := NewRewriteTool(mock, RewriteConfig{Instruction: "drop the code"})

	b := &model.Block{ID: "b1", Translatable: true, Source: []model.Run{
		{Text: &model.TextRun{Text: "Hello "}},
		{Ph: &model.PlaceholderRun{ID: "1", Type: "icon"}},
		{Text: &model.TextRun{Text: " world"}},
	}}
	applyRewrite(t, tl, b)

	// The block is unchanged: the placeholder survives and the runs are intact.
	require.Len(t, b.Source, 3)
	require.NotNil(t, b.Source[1].Ph, "the dropped-code rewrite must be rejected, leaving the placeholder run")
	assert.Equal(t, "1", b.Source[1].Ph.ID)
	assert.Equal(t, "Hello  world", model.RunsText(b.Source))
}

// TestRewriteRejectsInjectedCode proves the guard rejects a hallucinated tag: a
// model that invents an inline code with no counterpart in the source would
// write an orphan code into the document, so the block is left unchanged.
func TestRewriteRejectsInjectedCode(t *testing.T) {
	mock := aiprovider.NewMockProvider()
	mock.ChatFunc = func(_ context.Context, _ []aiprovider.Message) (*aiprovider.ChatResponse, error) {
		// Inject a placeholder tag (id 99) absent from the source.
		return &aiprovider.ChatResponse{Content: `Hello <x id="99/"/>world`, Model: "mock"}, nil
	}
	tl := NewRewriteTool(mock, RewriteConfig{Instruction: "inject"})

	b := &model.Block{ID: "b1", Translatable: true, Source: []model.Run{
		{Text: &model.TextRun{Text: "Hello world"}},
	}}
	applyRewrite(t, tl, b)

	require.Len(t, b.Source, 1)
	assert.Equal(t, "Hello world", b.SourceText(), "an injected inline code must be rejected, leaving the source unchanged")
}

// TestRewriteRejectsReorderedCodes proves the guard rejects a model that keeps
// the same codes but reorders a paired close ahead of its open — same multiset,
// but unbalanced markup — so the block is left unchanged.
func TestRewriteRejectsReorderedCodes(t *testing.T) {
	mock := aiprovider.NewMockProvider()
	mock.ChatFunc = func(_ context.Context, _ []aiprovider.Message) (*aiprovider.ChatResponse, error) {
		// Source renders <x id="1"/>word<x id="/1"/>; swap the pair's order.
		return &aiprovider.ChatResponse{Content: `<x id="/1"/>word<x id="1"/>`, Model: "mock"}, nil
	}
	tl := NewRewriteTool(mock, RewriteConfig{Instruction: "swap codes"})

	b := &model.Block{ID: "b1", Translatable: true, Source: []model.Run{
		{PcOpen: &model.PcOpenRun{ID: "1"}},
		{Text: &model.TextRun{Text: "word"}},
		{PcClose: &model.PcCloseRun{ID: "1"}},
	}}
	applyRewrite(t, tl, b)

	require.Len(t, b.Source, 3)
	require.NotNil(t, b.Source[0].PcOpen, "open code must remain first")
	require.NotNil(t, b.Source[2].PcClose, "close code must remain last")
}

// TestRewriteStructuredFallback documents the opaque fallback: a block whose
// source carries a plural run has no linear text mapping, so the tool replaces
// the whole source with the rewritten plain text rather than corrupting the
// structure. The placeholder tags never leak into the result.
func TestRewriteStructuredFallback(t *testing.T) {
	mock := aiprovider.NewMockProvider()
	mock.ChatFunc = func(_ context.Context, msgs []aiprovider.Message) (*aiprovider.ChatResponse, error) {
		return &aiprovider.ChatResponse{Content: strings.ToUpper(lastUserText(msgs)), Model: "mock"}, nil
	}
	tl := NewRewriteTool(mock, RewriteConfig{Instruction: "shout"})

	b := &model.Block{ID: "b1", Translatable: true, Source: []model.Run{
		{Plural: &model.PluralRun{Pivot: "n", Forms: map[model.PluralForm][]model.Run{
			model.PluralOther: {{Text: &model.TextRun{Text: "world"}}},
		}}},
	}}
	applyRewrite(t, tl, b)

	// Whole-source replacement: the plural structure is gone, the text is plain.
	assert.Equal(t, "WORLD", b.SourceText())
	require.Len(t, b.Source, 1)
	require.NotNil(t, b.Source[0].Text)
	assert.Nil(t, b.Source[0].Plural)
}

// TestRewriteSkipsNonContent proves non-content blocks (!Translatable) and
// blank blocks are skipped with no provider call.
func TestRewriteSkipsNonContent(t *testing.T) {
	t.Run("non-translatable", func(t *testing.T) {
		mock := aiprovider.NewMockProvider()
		chatReplace(mock, "Hello", "Howdy")
		tl := NewRewriteTool(mock, RewriteConfig{Instruction: "x"})
		b := &model.Block{ID: "b1", Translatable: false, Source: []model.Run{
			{Text: &model.TextRun{Text: "Hello world"}},
		}}
		applyRewrite(t, tl, b)
		assert.Equal(t, "Hello world", b.SourceText(), "non-translatable source must not change")
		assert.Empty(t, mock.ChatCalls, "the provider must not be called for a non-translatable block")
	})

	t.Run("blank", func(t *testing.T) {
		mock := aiprovider.NewMockProvider()
		chatReplace(mock, "x", "y")
		tl := NewRewriteTool(mock, RewriteConfig{Instruction: "x"})
		b := &model.Block{ID: "b1", Translatable: true, Source: []model.Run{
			{Text: &model.TextRun{Text: "   "}},
		}}
		applyRewrite(t, tl, b)
		assert.Empty(t, mock.ChatCalls, "the provider must not be called for a blank block")
	})
}

// TestRewriteNoOpWhenUnchanged proves a model that echoes the input produces no
// edit — the source is left exactly as it was.
func TestRewriteNoOpWhenUnchanged(t *testing.T) {
	mock := aiprovider.NewMockProvider()
	mock.ChatFunc = func(_ context.Context, msgs []aiprovider.Message) (*aiprovider.ChatResponse, error) {
		// Echo the input verbatim (plus whitespace the tool trims).
		return &aiprovider.ChatResponse{Content: "  " + lastUserText(msgs) + "\n", Model: "mock"}, nil
	}
	tl := NewRewriteTool(mock, RewriteConfig{Instruction: "noop"})

	b := &model.Block{ID: "b1", Translatable: true, Source: []model.Run{
		{Text: &model.TextRun{Text: "Hello world"}},
	}}
	applyRewrite(t, tl, b)
	assert.Equal(t, "Hello world", b.SourceText())
	require.Len(t, b.Source, 1)
}

// TestRewriteIsSourceTransform asserts the tool reports the transform capability
// so the flow placement pass runs it in the leading source-transform stage.
func TestRewriteIsSourceTransform(t *testing.T) {
	tl := NewRewriteTool(aiprovider.NewMockProvider(), RewriteConfig{Instruction: "x"})
	assert.True(t, tool.IsSourceTransform(tl))
}
