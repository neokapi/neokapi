package tools_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/mt/tools"
	"github.com/neokapi/neokapi/core/profile"
	mtprovider "github.com/neokapi/neokapi/providers/mt"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockProvider is a simple mock MTProvider for unit testing.
type mockProvider struct {
	name        string
	translateFn func(ctx context.Context, req mtprovider.TranslateRequest) (*mtprovider.TranslateResponse, error)
	calls       int
	lastRequest mtprovider.TranslateRequest
}

func (m *mockProvider) Name() mtprovider.ProviderID { return mtprovider.ProviderID(m.name) }
func (m *mockProvider) Close() error                { return nil }
func (m *mockProvider) Translate(ctx context.Context, req mtprovider.TranslateRequest) (*mtprovider.TranslateResponse, error) {
	m.calls++
	m.lastRequest = req
	return m.translateFn(ctx, req)
}

func newMock(name string) *mockProvider {
	return &mockProvider{
		name: name,
		translateFn: func(_ context.Context, _ mtprovider.TranslateRequest) (*mtprovider.TranslateResponse, error) {
			return &mtprovider.TranslateResponse{Translation: "translated"}, nil
		},
	}
}

func processPart(t *testing.T, tl interface {
	Process(ctx context.Context, in <-chan *model.Part, out chan<- *model.Part) error
}, part *model.Part) *model.Part {
	t.Helper()
	in := make(chan *model.Part, 1)
	out := make(chan *model.Part, 1)
	in <- part
	close(in)

	err := tl.Process(t.Context(), in, out)
	close(out)
	require.NoError(t, err)

	result := <-out
	require.NotNil(t, result)
	return result
}

func TestMTTranslateToolSetsTarget(t *testing.T) {
	mock := newMock("test-mt")
	mock.translateFn = func(_ context.Context, req mtprovider.TranslateRequest) (*mtprovider.TranslateResponse, error) {
		return &mtprovider.TranslateResponse{Translation: "Bonjour le monde"}, nil
	}

	tl := tools.NewMTTranslateTool(mock, tools.MTTranslateConfig{
		SourceLocale: model.LocaleEnglish,
		TargetLocale: model.LocaleFrench,
	})

	assert.Equal(t, "test-mt-translate", tl.Name())
	assert.Contains(t, tl.Description(), "test-mt")

	block := model.NewBlock("tu1", "Hello World")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	assert.Equal(t, "Bonjour le monde", resultBlock.TargetText(model.LocaleFrench))
	assert.Equal(t, "Hello World", resultBlock.SourceText())
	assert.Equal(t, 1, mock.calls)
	assert.Equal(t, "Hello World", mock.lastRequest.Source)
	assert.Equal(t, model.LocaleEnglish, mock.lastRequest.SourceLocale)
	assert.Equal(t, model.LocaleFrench, mock.lastRequest.TargetLocale)
}

// TestMTTranslateToolStampsGovernance: an MT target under a governing profile
// carries the profile identity, its pinned version and a non-empty context
// fingerprint, the same governance half the AI and recycle producers stamp — so
// the target is attributable and can be told stale when the context moves.
func TestMTTranslateToolStampsGovernance(t *testing.T) {
	mock := newMock("test-mt")

	tl := tools.NewMTTranslateTool(mock, tools.MTTranslateConfig{
		SourceLocale:   model.LocaleEnglish,
		TargetLocale:   model.LocaleFrench,
		Profile:        &profile.VoiceProfile{ID: "end-user-help", Name: "End-user help", Version: 7},
		PreferredTerms: map[string]string{"cart": "panier"},
	})

	block := model.NewBlock("tu1", "Hello World")
	result := processPart(t, tl, &model.Part{Type: model.PartBlock, Resource: block})

	tgt := result.Resource.(*model.Block).Target(model.LocaleFrench)
	require.NotNil(t, tgt)
	assert.Equal(t, model.OriginMT, tgt.Origin.Kind)
	assert.Equal(t, "test-mt", tgt.Origin.Engine)
	assert.Equal(t, "end-user-help", tgt.Origin.Profile)
	assert.Equal(t, "7", tgt.Origin.ProfileVersion)
	assert.NotEmpty(t, tgt.Origin.ContextFingerprint)
}

// TestMTTranslateToolUngovernedHasNoFingerprint: with no profile and no
// terminology the target reads as ungoverned — empty profile fields and an
// empty fingerprint, not a constant that looks like a real one.
func TestMTTranslateToolUngovernedHasNoFingerprint(t *testing.T) {
	mock := newMock("test-mt")

	tl := tools.NewMTTranslateTool(mock, tools.MTTranslateConfig{
		SourceLocale: model.LocaleEnglish,
		TargetLocale: model.LocaleFrench,
	})

	block := model.NewBlock("tu1", "Hello World")
	result := processPart(t, tl, &model.Part{Type: model.PartBlock, Resource: block})

	tgt := result.Resource.(*model.Block).Target(model.LocaleFrench)
	require.NotNil(t, tgt)
	assert.Equal(t, model.OriginMT, tgt.Origin.Kind)
	assert.Empty(t, tgt.Origin.Profile)
	assert.Empty(t, tgt.Origin.ProfileVersion)
	assert.Empty(t, tgt.Origin.ContextFingerprint)
}

func TestMTTranslateToolSkipsNonTranslatable(t *testing.T) {
	mock := newMock("test-mt")

	tl := tools.NewMTTranslateTool(mock, tools.MTTranslateConfig{
		SourceLocale: model.LocaleEnglish,
		TargetLocale: model.LocaleFrench,
	})

	block := model.NewBlock("tu1", "Hello")
	block.Translatable = false
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	assert.False(t, resultBlock.HasTarget(model.LocaleFrench))
	assert.Equal(t, 0, mock.calls)
}

func TestMTTranslateToolSkipsEmptySource(t *testing.T) {
	mock := newMock("test-mt")

	tl := tools.NewMTTranslateTool(mock, tools.MTTranslateConfig{
		SourceLocale: model.LocaleEnglish,
		TargetLocale: model.LocaleFrench,
	})

	block := model.NewBlock("tu1", "")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	assert.False(t, resultBlock.HasTarget(model.LocaleFrench))
	assert.Equal(t, 0, mock.calls)
}

func TestMTTranslateToolPassesThroughNonBlock(t *testing.T) {
	mock := newMock("test-mt")

	tl := tools.NewMTTranslateTool(mock, tools.MTTranslateConfig{
		SourceLocale: model.LocaleEnglish,
		TargetLocale: model.LocaleFrench,
	})

	data := &model.Data{ID: "d1", Name: "skeleton"}
	part := &model.Part{Type: model.PartData, Resource: data}
	result := processPart(t, tl, part)

	assert.Equal(t, model.PartData, result.Type)
	assert.Equal(t, data, result.Resource)
	assert.Equal(t, 0, mock.calls)
}

func TestMTTranslateToolPropagatesProviderError(t *testing.T) {
	mock := newMock("test-mt")
	mock.translateFn = func(_ context.Context, _ mtprovider.TranslateRequest) (*mtprovider.TranslateResponse, error) {
		return nil, errors.New("API rate limit exceeded")
	}

	tl := tools.NewMTTranslateTool(mock, tools.MTTranslateConfig{
		SourceLocale: model.LocaleEnglish,
		TargetLocale: model.LocaleFrench,
	})

	block := model.NewBlock("tu1", "Hello")
	part := &model.Part{Type: model.PartBlock, Resource: block}

	in := make(chan *model.Part, 1)
	out := make(chan *model.Part, 1)
	in <- part
	close(in)

	err := tl.Process(t.Context(), in, out)
	close(out)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "test-mt-translate")
	assert.Contains(t, err.Error(), "API rate limit exceeded")
}

func TestMTTranslateToolMultipleBlocks(t *testing.T) {
	callCount := 0
	mock := newMock("test-mt")
	mock.translateFn = func(_ context.Context, req mtprovider.TranslateRequest) (*mtprovider.TranslateResponse, error) {
		callCount++
		return &mtprovider.TranslateResponse{Translation: fmt.Sprintf("translated-%d", callCount)}, nil
	}

	tl := tools.NewMTTranslateTool(mock, tools.MTTranslateConfig{
		SourceLocale: model.LocaleEnglish,
		TargetLocale: model.LocaleFrench,
	})

	in := make(chan *model.Part, 3)
	out := make(chan *model.Part, 3)

	in <- &model.Part{Type: model.PartBlock, Resource: model.NewBlock("tu1", "Hello")}
	in <- &model.Part{Type: model.PartBlock, Resource: model.NewBlock("tu2", "World")}
	in <- &model.Part{Type: model.PartBlock, Resource: model.NewBlock("tu3", "Test")}
	close(in)

	err := tl.Process(t.Context(), in, out)
	close(out)
	require.NoError(t, err)

	var results []*model.Part
	for p := range out {
		results = append(results, p)
	}
	require.Len(t, results, 3)
	assert.Equal(t, "translated-1", results[0].Resource.(*model.Block).TargetText(model.LocaleFrench))
	assert.Equal(t, "translated-2", results[1].Resource.(*model.Block).TargetText(model.LocaleFrench))
	assert.Equal(t, "translated-3", results[2].Resource.(*model.Block).TargetText(model.LocaleFrench))
	assert.Equal(t, 3, callCount)
}
