package tools_test

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/ai/tools"
	"github.com/neokapi/neokapi/core/blockstore"
	"github.com/neokapi/neokapi/core/blockstore/sqlitestore"
	"github.com/neokapi/neokapi/core/model"
	coreprofile "github.com/neokapi/neokapi/core/profile"
	aiprovider "github.com/neokapi/neokapi/providers/ai"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTranslateMock returns a mock provider whose Translate call count is
// tracked through the returned counter.
func newTranslateMock(t *testing.T) (*aiprovider.MockProvider, *int) {
	t.Helper()
	calls := new(int)
	mock := aiprovider.NewMockProvider()
	mock.TranslateFunc = func(_ context.Context, req aiprovider.TranslateRequest) (*aiprovider.TranslateResponse, error) {
		*calls++
		return &aiprovider.TranslateResponse{
			Translation: "[fr] " + req.Source,
			Confidence:  0.9,
			Model:       "test-model",
		}, nil
	}
	return mock, calls
}

// runSession pushes one block through SessionProcess and returns the block.
func runSession(t *testing.T, ctx context.Context, tl *tools.AITranslateTool, sess blockstore.Session, block *model.Block) *model.Block {
	t.Helper()
	in := make(chan *model.Part, 1)
	out := make(chan *model.Part, 1)
	in <- &model.Part{Type: model.PartBlock, Resource: block}
	close(in)
	require.NoError(t, tl.SessionProcess(ctx, sess, in, out))
	return (<-out).Resource.(*model.Block)
}

func singleBlockConfig() tools.AITranslateConfig {
	return tools.AITranslateConfig{
		SourceLocale: model.LocaleEnglish,
		TargetLocale: model.LocaleFrench,
		Provider:     "anthropic",
		Model:        "claude-x",
		BatchSize:    1, BatchConcurrency: 1,
	}
}

// TestAITranslate_SessionOverlayWrittenUnderStoreKey: with a source file in
// context (multi-file project runs), the target overlay is written under the
// globally-unique StoreKey — not the collision-prone file-local block id —
// and the produced target carries draft/AI provenance.
func TestAITranslate_SessionOverlayWrittenUnderStoreKey(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "blocks.db")
	store, err := sqlitestore.New(dbPath)
	require.NoError(t, err)
	defer store.Close()

	ctx := blockstore.WithSourceRel(context.Background(), "docs/a.md")
	mock, calls := newTranslateMock(t)
	tl := tools.NewAITranslateTool(mock, singleBlockConfig())

	sess, err := store.Begin(ctx)
	require.NoError(t, err)
	block := runSession(t, ctx, tl, sess, model.NewBlock("tu1", "Hello"))
	require.NoError(t, sess.Commit())

	assert.Equal(t, 1, *calls)
	assert.Equal(t, "[fr] Hello", block.TargetText(model.LocaleFrench))
	tgt := block.Target(model.LocaleFrench)
	require.NotNil(t, tgt)
	assert.Equal(t, model.TargetStatusDraft, tgt.Status)
	assert.Equal(t, model.OriginAI, tgt.Origin.Kind)

	// Reopen the store: the overlay must persist under the StoreKey…
	require.NoError(t, store.Close())
	store2, err := sqlitestore.New(dbPath)
	require.NoError(t, err)
	defer store2.Close()
	check, err := store2.Begin(context.Background())
	require.NoError(t, err)
	defer check.Close()

	storeKey := blockstore.StoreKey("docs/a.md", "tu1", "Hello")
	ov, err := check.GetOverlay("targets/fr", storeKey)
	require.NoError(t, err, "overlay must be addressed by the source-rel StoreKey")
	var cached struct {
		Text     string `json:"text"`
		Provider string `json:"provider"`
		Config   string `json:"config"`
	}
	require.NoError(t, json.Unmarshal(ov.Payload, &cached))
	assert.Equal(t, "[fr] Hello", cached.Text)
	assert.Equal(t, "mock", cached.Provider)
	assert.NotEmpty(t, cached.Config, "overlay must record the config fingerprint")

	// …and NOT under the raw file-local id (that key would collide across files).
	_, err = check.GetOverlay("targets/fr", "tu1")
	require.ErrorIs(t, err, blockstore.ErrNotFound)
}

// TestAITranslate_SessionOverlayRawIDFallback: without a source file in
// context (ad-hoc single-document runs) the overlay key falls back to the raw
// block id — a single document cannot collide with itself.
func TestAITranslate_SessionOverlayRawIDFallback(t *testing.T) {
	store, err := sqlitestore.New(filepath.Join(t.TempDir(), "blocks.db"))
	require.NoError(t, err)
	defer store.Close()

	ctx := context.Background() // no WithSourceRel
	mock, _ := newTranslateMock(t)
	tl := tools.NewAITranslateTool(mock, singleBlockConfig())

	sess, err := store.Begin(ctx)
	require.NoError(t, err)
	runSession(t, ctx, tl, sess, model.NewBlock("tu1", "Hello"))
	require.NoError(t, sess.Commit())

	check, err := store.Begin(ctx)
	require.NoError(t, err)
	defer check.Close()
	ov, err := check.GetOverlay("targets/fr", "tu1")
	require.NoError(t, err, "without source-rel context the overlay keys on the raw block id")
	assert.Contains(t, string(ov.Payload), "[fr] Hello")
}

// TestAITranslate_SessionCachedOverlaySkipsSecondPass: a re-run over the same
// (file, block, config) is hydrated from the cached overlay — no LLM call —
// and still stamps draft/AI provenance on the hydrated target. A different
// source file addresses different overlays, so it translates fresh.
func TestAITranslate_SessionCachedOverlaySkipsSecondPass(t *testing.T) {
	store, err := sqlitestore.New(filepath.Join(t.TempDir(), "blocks.db"))
	require.NoError(t, err)
	defer store.Close()

	ctx := blockstore.WithSourceRel(context.Background(), "docs/a.md")
	mock, calls := newTranslateMock(t)
	tl := tools.NewAITranslateTool(mock, singleBlockConfig())

	sess, err := store.Begin(ctx)
	require.NoError(t, err)
	runSession(t, ctx, tl, sess, model.NewBlock("tu1", "Hello"))
	require.NoError(t, sess.Commit())
	require.Equal(t, 1, *calls)

	// Second pass, same file: served from the overlay cache.
	sess2, err := store.Begin(ctx)
	require.NoError(t, err)
	hydrated := runSession(t, ctx, tl, sess2, model.NewBlock("tu1", "Hello"))
	require.NoError(t, sess2.Commit())

	assert.Equal(t, 1, *calls, "cached overlay must skip the LLM on the second pass")
	assert.Equal(t, "[fr] Hello", hydrated.TargetText(model.LocaleFrench))
	tgt := hydrated.Target(model.LocaleFrench)
	require.NotNil(t, tgt)
	assert.Equal(t, model.TargetStatusDraft, tgt.Status, "hydrated target keeps draft provenance")
	assert.Equal(t, model.OriginAI, tgt.Origin.Kind)

	// Same block id in a different source file keys differently → fresh call.
	ctxB := blockstore.WithSourceRel(context.Background(), "docs/b.md")
	sess3, err := store.Begin(ctxB)
	require.NoError(t, err)
	runSession(t, ctxB, tl, sess3, model.NewBlock("tu1", "Hello"))
	require.NoError(t, sess3.Commit())
	assert.Equal(t, 2, *calls, "a different source file must not reuse another file's overlay")
}

// Config-fingerprint invalidation (changed model/term rules re-translates
// instead of serving the stale cached target) is covered by
// TestAITranslate_SessionCacheIsConfigAware in session_cache_test.go.

// TestAITranslate_BatchSessionWritesAndReusesOverlays: the batched session
// path (batchSize > 1) writes overlays for every translated block and serves
// a full second pass from the cache without any provider call.
func TestAITranslate_BatchSessionWritesAndReusesOverlays(t *testing.T) {
	store, err := sqlitestore.New(filepath.Join(t.TempDir(), "blocks.db"))
	require.NoError(t, err)
	defer store.Close()

	ctx := blockstore.WithSourceRel(context.Background(), "docs/a.md")
	mock := aiprovider.NewMockProvider()
	mock.ChatStructuredFunc = func(context.Context, []aiprovider.Message, aiprovider.JSONSchema) (*aiprovider.ChatResponse, error) {
		return &aiprovider.ChatResponse{
			Content: batchJSON(map[int]string{1: "Bonjour", 2: "Monde"}),
			Model:   "test",
		}, nil
	}

	cfg := singleBlockConfig()
	cfg.BatchSize = 10
	tl := tools.NewAITranslateTool(mock, cfg)

	runBatch := func() []*model.Part {
		sess, err := store.Begin(ctx)
		require.NoError(t, err)
		in := make(chan *model.Part, 2)
		out := make(chan *model.Part, 2)
		in <- &model.Part{Type: model.PartBlock, Resource: model.NewBlock("tu1", "Hello")}
		in <- &model.Part{Type: model.PartBlock, Resource: model.NewBlock("tu2", "World")}
		close(in)
		require.NoError(t, tl.SessionProcess(ctx, sess, in, out))
		close(out)
		var parts []*model.Part
		for p := range out {
			parts = append(parts, p)
		}
		require.NoError(t, sess.Commit())
		return parts
	}

	parts := runBatch()
	require.Len(t, parts, 2)
	assert.Len(t, mock.ChatStructuredCalls, 1)

	// Overlays landed under the StoreKeys.
	check, err := store.Begin(ctx)
	require.NoError(t, err)
	for id, src := range map[string]string{"tu1": "Hello", "tu2": "World"} {
		_, err := check.GetOverlay("targets/fr", blockstore.StoreKey("docs/a.md", id, src))
		require.NoErrorf(t, err, "batch path must write the overlay for %s", id)
	}
	require.NoError(t, check.Close())

	// Second pass: everything hydrates from cache, no new provider calls.
	parts2 := runBatch()
	require.Len(t, parts2, 2)
	assert.Len(t, mock.ChatStructuredCalls, 1, "cached batch pass must not call the provider")
	assert.Empty(t, mock.TranslateCalls)
	for _, p := range parts2 {
		b := p.Resource.(*model.Block)
		assert.True(t, b.HasTarget(model.LocaleFrench), "block %s must hydrate from the overlay", b.ID)
		tgt := b.Target(model.LocaleFrench)
		require.NotNil(t, tgt)
		assert.Equal(t, model.TargetStatusDraft, tgt.Status)
		assert.Equal(t, model.OriginAI, tgt.Origin.Kind)
	}
}

// TestAITranslate_ErrorPropagation: provider failures surface from every
// processing path (single-block, batch, and session), wrapped with the tool
// name.
func TestAITranslate_ErrorPropagation(t *testing.T) {
	provErr := errors.New("provider exploded")

	t.Run("single-block path", func(t *testing.T) {
		mock := aiprovider.NewMockProvider()
		mock.TranslateFunc = func(context.Context, aiprovider.TranslateRequest) (*aiprovider.TranslateResponse, error) {
			return nil, provErr
		}
		tl := tools.NewAITranslateTool(mock, tools.AITranslateConfig{
			SourceLocale: model.LocaleEnglish, TargetLocale: model.LocaleFrench,
			BatchSize: 1, BatchConcurrency: 1,
		})
		in := make(chan *model.Part, 1)
		out := make(chan *model.Part, 1)
		in <- &model.Part{Type: model.PartBlock, Resource: model.NewBlock("tu1", "Hello")}
		close(in)
		err := tl.Process(t.Context(), in, out)
		require.Error(t, err)
		require.ErrorIs(t, err, provErr)
		assert.Contains(t, err.Error(), "translate:")
	})

	t.Run("batch path", func(t *testing.T) {
		mock := aiprovider.NewMockProvider()
		mock.ChatStructuredFunc = func(context.Context, []aiprovider.Message, aiprovider.JSONSchema) (*aiprovider.ChatResponse, error) {
			return nil, provErr
		}
		tl := tools.NewAITranslateTool(mock, tools.AITranslateConfig{
			SourceLocale: model.LocaleEnglish, TargetLocale: model.LocaleFrench,
			BatchSize: 10, BatchConcurrency: 1,
		})
		in := make(chan *model.Part, 2)
		out := make(chan *model.Part, 2)
		in <- &model.Part{Type: model.PartBlock, Resource: model.NewBlock("tu1", "Hello")}
		in <- &model.Part{Type: model.PartBlock, Resource: model.NewBlock("tu2", "World")}
		close(in)
		err := tl.Process(t.Context(), in, out)
		require.Error(t, err)
		require.ErrorIs(t, err, provErr)
		assert.Contains(t, err.Error(), "translate batch:")
	})

	t.Run("batch malformed response", func(t *testing.T) {
		mock := aiprovider.NewMockProvider()
		mock.ChatStructuredFunc = func(context.Context, []aiprovider.Message, aiprovider.JSONSchema) (*aiprovider.ChatResponse, error) {
			return &aiprovider.ChatResponse{Content: "not json at all", Model: "test"}, nil
		}
		tl := tools.NewAITranslateTool(mock, tools.AITranslateConfig{
			SourceLocale: model.LocaleEnglish, TargetLocale: model.LocaleFrench,
			BatchSize: 10, BatchConcurrency: 1,
		})
		in := make(chan *model.Part, 2)
		out := make(chan *model.Part, 2)
		in <- &model.Part{Type: model.PartBlock, Resource: model.NewBlock("tu1", "Hello")}
		in <- &model.Part{Type: model.PartBlock, Resource: model.NewBlock("tu2", "World")}
		close(in)
		err := tl.Process(t.Context(), in, out)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unmarshal response")
	})

	t.Run("session path", func(t *testing.T) {
		store, err := sqlitestore.New(filepath.Join(t.TempDir(), "blocks.db"))
		require.NoError(t, err)
		defer store.Close()

		mock := aiprovider.NewMockProvider()
		mock.TranslateFunc = func(context.Context, aiprovider.TranslateRequest) (*aiprovider.TranslateResponse, error) {
			return nil, provErr
		}
		tl := tools.NewAITranslateTool(mock, singleBlockConfig())

		ctx := context.Background()
		sess, err := store.Begin(ctx)
		require.NoError(t, err)
		defer sess.Close()
		in := make(chan *model.Part, 1)
		out := make(chan *model.Part, 1)
		in <- &model.Part{Type: model.PartBlock, Resource: model.NewBlock("tu1", "Hello")}
		close(in)
		err = tl.SessionProcess(ctx, sess, in, out)
		require.Error(t, err)
		require.ErrorIs(t, err, provErr)
	})
}

// TestAITranslate_SessionBlockWithoutID: a block with no id cannot be keyed
// in the overlay store; it still translates (pass-through of the produce
// path) but no overlay is written.
func TestAITranslate_SessionBlockWithoutID(t *testing.T) {
	store, err := sqlitestore.New(filepath.Join(t.TempDir(), "blocks.db"))
	require.NoError(t, err)
	defer store.Close()

	ctx := context.Background()
	mock, calls := newTranslateMock(t)
	tl := tools.NewAITranslateTool(mock, singleBlockConfig())

	sess, err := store.Begin(ctx)
	require.NoError(t, err)
	block := runSession(t, ctx, tl, sess, model.NewBlock("", "Hello"))
	require.NoError(t, sess.Commit())

	assert.Equal(t, 1, *calls)
	assert.Equal(t, "[fr] Hello", block.TargetText(model.LocaleFrench))

	check, err := store.Begin(ctx)
	require.NoError(t, err)
	defer check.Close()
	_, err = check.GetOverlay("targets/fr", "")
	require.ErrorIs(t, err, blockstore.ErrNotFound, "an id-less block writes no overlay")
}

// TestAITranslate_StampsGoverningProfile: a target produced under a resolved
// context profile records which profile and which version governed it, not just
// which engine made it. The profile is rendered into the prompt and then
// discarded, and profiles are edited in place — so an unstamped target's
// governing context is unrecoverable, and a timestamp is only a proxy for it.
func TestAITranslate_StampsGoverningProfile(t *testing.T) {
	store, err := sqlitestore.New(filepath.Join(t.TempDir(), "blocks.db"))
	require.NoError(t, err)
	defer store.Close()

	ctx := context.Background()
	mock, _ := newTranslateMock(t)
	cfg := singleBlockConfig()
	cfg.Profile = &coreprofile.VoiceProfile{ID: "end-user-help", Name: "End-user help", Version: 7}
	tl := tools.NewAITranslateTool(mock, cfg)

	sess, err := store.Begin(ctx)
	require.NoError(t, err)
	block := runSession(t, ctx, tl, sess, model.NewBlock("tu1", "Hello"))
	require.NoError(t, sess.Commit())

	tgt := block.Target(model.LocaleFrench)
	require.NotNil(t, tgt)
	assert.Equal(t, model.OriginAI, tgt.Origin.Kind, "how it was made")
	assert.Equal(t, "end-user-help", tgt.Origin.Profile, "what governed it")
	assert.Equal(t, "7", tgt.Origin.ProfileVersion, "which revision of it was in force")
}

// TestAITranslate_NoProfileLeavesStampEmpty: an ad-hoc run resolves no profile,
// and the stamp stays empty rather than inventing a default. An empty Profile
// means "no context was in force", which is a different fact from "some context
// was in force and we failed to record it".
func TestAITranslate_NoProfileLeavesStampEmpty(t *testing.T) {
	store, err := sqlitestore.New(filepath.Join(t.TempDir(), "blocks.db"))
	require.NoError(t, err)
	defer store.Close()

	ctx := context.Background()
	mock, _ := newTranslateMock(t)
	tl := tools.NewAITranslateTool(mock, singleBlockConfig())

	sess, err := store.Begin(ctx)
	require.NoError(t, err)
	block := runSession(t, ctx, tl, sess, model.NewBlock("tu1", "Hello"))
	require.NoError(t, sess.Commit())

	tgt := block.Target(model.LocaleFrench)
	require.NotNil(t, tgt)
	assert.Equal(t, model.OriginAI, tgt.Origin.Kind)
	assert.Empty(t, tgt.Origin.Profile)
	assert.Empty(t, tgt.Origin.ProfileVersion)
}

// TestAITranslate_UnversionedProfileStampsIDOnly: a profile that has never been
// versioned (Version 0) stamps its id with no version, rather than a misleading
// "0" that would read as a real revision.
func TestAITranslate_UnversionedProfileStampsIDOnly(t *testing.T) {
	store, err := sqlitestore.New(filepath.Join(t.TempDir(), "blocks.db"))
	require.NoError(t, err)
	defer store.Close()

	ctx := context.Background()
	mock, _ := newTranslateMock(t)
	cfg := singleBlockConfig()
	cfg.Profile = &coreprofile.VoiceProfile{ID: "draft-profile", Name: "Draft"}
	tl := tools.NewAITranslateTool(mock, cfg)

	sess, err := store.Begin(ctx)
	require.NoError(t, err)
	block := runSession(t, ctx, tl, sess, model.NewBlock("tu1", "Hello"))
	require.NoError(t, sess.Commit())

	tgt := block.Target(model.LocaleFrench)
	require.NotNil(t, tgt)
	assert.Equal(t, "draft-profile", tgt.Origin.Profile)
	assert.Empty(t, tgt.Origin.ProfileVersion)
}

// stampedFingerprint runs one block through the tool and returns the context
// fingerprint stamped on the produced target.
func stampedFingerprint(t *testing.T, cfg tools.AITranslateConfig) string {
	t.Helper()
	store, err := sqlitestore.New(filepath.Join(t.TempDir(), "blocks.db"))
	require.NoError(t, err)
	defer store.Close()

	ctx := context.Background()
	mock, _ := newTranslateMock(t)
	tl := tools.NewAITranslateTool(mock, cfg)

	sess, err := store.Begin(ctx)
	require.NoError(t, err)
	block := runSession(t, ctx, tl, sess, model.NewBlock("tu1", "Hello"))
	require.NoError(t, sess.Commit())

	tgt := block.Target(model.LocaleFrench)
	require.NotNil(t, tgt)
	return tgt.Origin.ContextFingerprint
}

// TestContextFingerprintTracksTheContextNotTheEngine is the property that makes
// the stamp mean something. Profile/ProfileVersion pin the profile, but
// terminology reaches the tool separately and carries no version — so the
// fingerprint is what notices the terms moving. It must move when the governing
// context moves, and must NOT move when only the engine does: a stamp that
// changed on a model swap would be useless as a statement about governance.
func TestContextFingerprintTracksTheContextNotTheEngine(t *testing.T) {
	profile := &coreprofile.VoiceProfile{ID: "help", Name: "Help", Version: 3}

	base := singleBlockConfig()
	base.Profile = profile
	base.TermRules = []coreprofile.TermRule{
		{Term: "widget", Replacement: "gadget"},
		{Term: "login", Replacement: "connexion"},
	}
	baseFP := stampedFingerprint(t, base)
	require.NotEmpty(t, baseFP)

	// The same rules in a different order → the same fingerprint. Two terms
	// stores can list one vocabulary in either order without that reading as a
	// governance change, so the projection sorts before it hashes.
	reordered := singleBlockConfig()
	reordered.Profile = profile
	reordered.TermRules = []coreprofile.TermRule{
		{Term: "login", Replacement: "connexion"},
		{Term: "widget", Replacement: "gadget"},
	}
	assert.Equal(t, baseFP, stampedFingerprint(t, reordered),
		"identical terminology must hash identically regardless of rule order")

	// A changed term → different fingerprint.
	changedTerms := singleBlockConfig()
	changedTerms.Profile = profile
	changedTerms.TermRules = []coreprofile.TermRule{
		{Term: "widget", Replacement: "doohickey"},
		{Term: "login", Replacement: "connexion"},
	}
	assert.NotEqual(t, baseFP, stampedFingerprint(t, changedTerms),
		"changed terminology must move the fingerprint")

	// A changed model → SAME fingerprint. This is what separates it from the
	// engine's config fingerprint, which must move here so the cache invalidates.
	changedModel := singleBlockConfig()
	changedModel.Profile = profile
	changedModel.TermRules = base.TermRules
	changedModel.Model = "claude-y"
	assert.Equal(t, baseFP, stampedFingerprint(t, changedModel),
		"swapping the model is not a governance change and must not move the fingerprint")
}

// TestContextFingerprintEmptyWhenUngoverned: no profile and no terminology means
// no context governed the run, which must read as ungoverned rather than as the
// hash of two empty strings — a constant that would look like a real value.
func TestContextFingerprintEmptyWhenUngoverned(t *testing.T) {
	assert.Empty(t, stampedFingerprint(t, singleBlockConfig()))
}
