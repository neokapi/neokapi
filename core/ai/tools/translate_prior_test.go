package tools_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	aitools "github.com/neokapi/neokapi/core/ai/tools"
	corememory "github.com/neokapi/neokapi/core/memory"
	"github.com/neokapi/neokapi/core/model"
	coreprofile "github.com/neokapi/neokapi/core/profile"
	aiprovider "github.com/neokapi/neokapi/providers/ai"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The prior version reaching a live translate run.
//
// Every piece of this existed and nothing called it: the gate, the version
// chain and the prompt section all worked, while the tool built its context
// from the block key and its neighbours and never asked what the block said
// before. These tests are the ones that would have failed then.

// recordingProvider captures what was actually sent, so an assertion can be
// about the prompt rather than about the config that was meant to produce it.
type recordingProvider struct {
	aiprovider.LLMProvider
	prompts [][]aiprovider.Message
}

func (p *recordingProvider) Name() aiprovider.ProviderID            { return "recording" }
func (p *recordingProvider) InputModalities() []aiprovider.Modality { return nil }
func (p *recordingProvider) Close() error                           { return nil }

func (p *recordingProvider) Chat(_ context.Context, msgs []aiprovider.Message) (*aiprovider.ChatResponse, error) {
	p.prompts = append(p.prompts, msgs)
	return &aiprovider.ChatResponse{Content: "oversatt"}, nil
}

// ChatStructured answers a batch the way the batch path expects: one
// translation per segment id it was actually sent. Echoing the ids back is what
// makes a dropped segment detectable, so a stub that ignored them would pass
// tests the real path would fail.
func (p *recordingProvider) ChatStructured(ctx context.Context, msgs []aiprovider.Message, _ aiprovider.JSONSchema) (*aiprovider.ChatResponse, error) {
	p.prompts = append(p.prompts, msgs)

	var payload struct {
		Segments []struct {
			ID string `json:"id"`
		} `json:"segments"`
	}
	for _, m := range msgs {
		if json.Unmarshal([]byte(m.Text()), &payload) == nil && len(payload.Segments) > 0 {
			break
		}
	}

	var reply struct {
		Translations []map[string]string `json:"translations"`
	}
	for _, seg := range payload.Segments {
		reply.Translations = append(reply.Translations, map[string]string{"id": seg.ID, "text": "oversatt"})
	}
	out, err := json.Marshal(reply)
	if err != nil {
		return nil, err
	}
	return &aiprovider.ChatResponse{Content: string(out)}, nil
}

func (p *recordingProvider) Translate(ctx context.Context, req aiprovider.TranslateRequest) (*aiprovider.TranslateResponse, error) {
	// SingleWithContext, matching what a real provider does (providers/ai:
	// standardTranslate). Rendering with Single would silently drop the block
	// context, and this test exists to assert what is in it.
	turns := req.Prompt().SingleWithContext(req.Source, req.PreserveTags, req.BlockContext)
	if _, err := p.Chat(ctx, aiprovider.MessagesFromTurns(turns)); err != nil {
		return nil, err
	}
	return &aiprovider.TranslateResponse{Translation: "oversatt", Confidence: 1}, nil
}

// stubPriors answers for one unit at one point, and records what it was asked.
type stubPriors struct {
	unit, point     string
	source, target  string
	fingerprintSeen string
	unitSeen        string
	pointSeen       string
	calls           int
}

func (s *stubPriors) PriorVersion(_ context.Context, req corememory.VersionRequest) (corememory.Version, bool) {
	s.calls++
	s.unitSeen, s.pointSeen, s.fingerprintSeen = req.Unit, req.Point, req.GovernedBy
	if req.Unit != s.unit || req.Point != s.point {
		return corememory.Version{}, false
	}
	return corememory.Version{Source: s.source, Target: s.target}, true
}

// Lookup answers nothing: this stub exists to be asked about history, and a
// provider that cannot answer a question says so rather than omitting it.
func (s *stubPriors) Lookup(context.Context, corememory.Request) (corememory.Match, bool) {
	return corememory.Match{}, false
}

var _ corememory.Provider = (*stubPriors)(nil)

func blockNamed(name, source string) *model.Block {
	b := model.NewBlock(name, source)
	// Name, not just ID: a chain is keyed on the block's durable identity, and
	// an ID is assigned per read.
	b.Name = name
	b.Translatable = true
	return b
}

func runTranslate(t *testing.T, cfg aitools.AITranslateConfig, p aiprovider.LLMProvider, blocks ...*model.Block) {
	t.Helper()

	tl := aitools.NewAITranslateTool(p, cfg)
	in := make(chan *model.Part, len(blocks))
	out := make(chan *model.Part, len(blocks))
	for _, b := range blocks {
		in <- &model.Part{Type: model.PartBlock, Resource: b}
	}
	close(in)
	require.NoError(t, tl.Process(t.Context(), in, out))
	close(out)
	for range out { //nolint:revive // draining; the assertions read the blocks
	}
}

func baseConfig() aitools.AITranslateConfig {
	return aitools.AITranslateConfig{
		SourceLocale: "en",
		TargetLocale: "nb",
		Profile: &coreprofile.VoiceProfile{
			ID: "acme", Name: "Acme", Version: 1,
			Tone: coreprofile.ToneProfile{Formality: "neutral"},
		},
	}
}

func TestTheApprovedTranslationReachesThePrompt(t *testing.T) {
	t.Parallel()

	priors := &stubPriors{
		unit: "cta.start", point: "acme\x1fweb\x1fsite",
		source: "Get started", target: "Kom i gang",
	}
	cfg := baseConfig()
	cfg.Memory = priors
	cfg.Point = priors.point

	p := &recordingProvider{}
	runTranslate(t, cfg, p, blockNamed("cta.start", "Get started today"))

	require.NotEmpty(t, p.prompts, "the tool must have called the model")
	seen := flatten(p.prompts[0])
	assert.Contains(t, seen, "Kom i gang", "the approved translation must reach the model")
	assert.Contains(t, seen, "Get started", "and the source it was approved for, so the model sees the edit")
}

// TestThePriorIsAskedForAtThisBlockAndThisPoint: asking with the wrong identity
// would return another block's history, which is worse than none.
func TestThePriorIsAskedForAtThisBlockAndThisPoint(t *testing.T) {
	t.Parallel()

	priors := &stubPriors{unit: "cta.start", point: "acme\x1fweb\x1fsite", source: "Get started", target: "Kom i gang"}
	cfg := baseConfig()
	cfg.Memory = priors
	cfg.Point = priors.point

	runTranslate(t, cfg, &recordingProvider{}, blockNamed("cta.start", "Get started today"))

	assert.Equal(t, "cta.start", priors.unitSeen, "the unit is the block's chain identity")
	assert.Equal(t, priors.point, priors.pointSeen, "and the point is where this run's content sits")
	assert.NotEmpty(t, priors.fingerprintSeen,
		"the governance about to reach the model is what the old answer is judged against")
}

// TestNoProviderTranslatesExactlyAsBefore: a project without a content memory
// must be unaffected, which is most runs.
func TestNoProviderTranslatesExactlyAsBefore(t *testing.T) {
	t.Parallel()

	p := &recordingProvider{}
	runTranslate(t, baseConfig(), p, blockNamed("cta.start", "Get started today"))

	require.NotEmpty(t, p.prompts)
	assert.NotContains(t, flatten(p.prompts[0]), "Kom i gang")
}

// TestThePriorMovesTheCacheKey is the failure that would be invisible: a
// reference the model receives without moving the cache key is right once and
// stale for every run after, because the cached target was produced under a
// version the chain has left behind.
func TestThePriorMovesTheCacheKey(t *testing.T) {
	t.Parallel()

	block := blockNamed("cta.start", "Get started today")

	bare := aitools.NewAITranslateTool(&recordingProvider{}, baseConfig())

	withPrior := baseConfig()
	withPrior.Memory = &stubPriors{
		unit: "cta.start", point: "acme\x1fweb\x1fsite",
		source: "Get started", target: "Kom i gang",
	}
	withPrior.Point = "acme\x1fweb\x1fsite"
	steered := aitools.NewAITranslateTool(&recordingProvider{}, withPrior)

	assert.NotEqual(t,
		aitools.ExportCacheFingerprint(bare, t.Context(), block),
		aitools.ExportCacheFingerprint(steered, t.Context(), block),
		"a prompt carrying a prior version must not share a cache entry with one that does not")
}

// TestABatchCarriesAReferencePerSegment.
//
// A prior version is per-block, so it rides in the batch payload beside each
// segment's key rather than in the shared preamble — which would offer one
// block's history to every other block in the call.
//
// This replaces a test that asserted the opposite. A block with history used to
// be packed alone, because the payload had nowhere to put a per-segment
// reference. That un-batched exactly the blocks the feature applied to, and on
// a model migration (where every approved block has a governed prior) it
// un-batched the entire corpus.
func TestABatchCarriesAReferencePerSegment(t *testing.T) {
	t.Parallel()

	priors := &stubPriors{
		unit: "cta.start", point: "acme\x1fweb\x1fsite",
		source: "Get started", target: "Kom i gang",
	}
	cfg := baseConfig()
	cfg.Memory = priors
	cfg.Point = priors.point
	// Force the batch path: several blocks, one call.
	cfg.BatchSize = 3
	cfg.BatchConcurrency = 1

	p := &recordingProvider{}
	runTranslate(t, cfg, p,
		blockNamed("cta.start", "Get started today"),
		blockNamed("cta.other", "Something else"),
		blockNamed("cta.third", "A third string"),
	)

	require.Len(t, p.prompts, 1, "three blocks, one call — the reference did not cost the batch")
	sent := flatten(p.prompts[0])

	assert.Contains(t, sent, "Kom i gang", "the reference reached the model")
	assert.Contains(t, sent, "Get started today", "beside the block it belongs to")
	assert.Contains(t, sent, "Something else", "in the same call as the blocks with no history")
	assert.Contains(t, sent, "prior", "carried in the payload, not the preamble")
}

// TestOnlyTheSegmentWithHistoryCarriesOne: a reference is scoped to its own
// segment. A payload attaching one block's history to another would steer an
// unrelated translation, and the model has no way to know it should not.
func TestOnlyTheSegmentWithHistoryCarriesOne(t *testing.T) {
	t.Parallel()

	priors := &stubPriors{
		unit: "cta.start", point: "acme\x1fweb\x1fsite",
		source: "Get started", target: "Kom i gang",
	}
	cfg := baseConfig()
	cfg.Memory = priors
	cfg.Point = priors.point
	cfg.BatchSize = 2
	cfg.BatchConcurrency = 1

	p := &recordingProvider{}
	runTranslate(t, cfg, p,
		blockNamed("cta.start", "Get started today"),
		blockNamed("cta.other", "Something else"),
	)

	require.Len(t, p.prompts, 1)
	assert.Equal(t, 1, strings.Count(flatten(p.prompts[0]), "Kom i gang"),
		"exactly one segment carries the reference")
}

// flatten renders a prompt to one searchable string.
func flatten(msgs []aiprovider.Message) string {
	var sb strings.Builder
	for _, m := range msgs {
		sb.WriteString(m.Text())
	}
	return sb.String()
}

// TestTheCorpusIsAskedOncePerBlock.
//
// One block asks for its prior version from five places in a run: the packer
// deciding whether to batch it, cacheFingerprint three times (the skip check,
// the hit comparison, the write-back), and the translation itself. Reading the
// corpus five times to answer one question is the obvious cost; the subtler one
// is that five reads can disagree, and a cache fingerprint computed from a
// different answer than the prompt carried is a key that describes nothing.
func TestTheCorpusIsAskedOncePerBlock(t *testing.T) {
	t.Parallel()

	priors := &stubPriors{
		unit: "cta.start", point: "acme\x1fweb\x1fsite",
		source: "Get started", target: "Kom i gang",
	}
	cfg := baseConfig()
	cfg.Memory = priors
	cfg.Point = priors.point

	block := blockNamed("cta.start", "Get started today")
	tl := aitools.NewAITranslateTool(&recordingProvider{}, cfg)

	// The three cacheFingerprint calls one block makes on the session path,
	// plus the translation's own. Without the memo this is four reads of the
	// same row for one answer.
	first := aitools.ExportCacheFingerprint(tl, t.Context(), block)
	assert.Equal(t, first, aitools.ExportCacheFingerprint(tl, t.Context(), block))
	assert.Equal(t, first, aitools.ExportCacheFingerprint(tl, t.Context(), block))

	assert.Equal(t, 1, priors.calls, "the corpus answers once and every asker gets that answer")
}

// TestABlockWithNoHistoryIsAlsoAskedOnce: the memo has to remember an absent
// answer too, or a corpus with no chain pays the full five reads per block to
// be told nothing five times.
func TestABlockWithNoHistoryIsAlsoAskedOnce(t *testing.T) {
	t.Parallel()

	priors := &stubPriors{unit: "somewhere.else", point: "acme\x1fweb\x1fsite"}
	cfg := baseConfig()
	cfg.Memory = priors
	cfg.Point = priors.point

	block := blockNamed("cta.start", "Get started today")
	tl := aitools.NewAITranslateTool(&recordingProvider{}, cfg)
	aitools.ExportCacheFingerprint(tl, t.Context(), block)
	aitools.ExportCacheFingerprint(tl, t.Context(), block)

	assert.Equal(t, 1, priors.calls, "a miss is memoized like a hit")
}

// TestACallSendsOnlyTheTermsItsTextCanUse.
//
// Every prompt used to carry every term rule at the coordinate. Tokens are the
// smaller cost; the larger is attention — a model handed four hundred rules
// attends less to the three that bite the sentence in front of it.
func TestACallSendsOnlyTheTermsItsTextCanUse(t *testing.T) {
	t.Parallel()

	cfg := baseConfig()
	cfg.TermRules = []coreprofile.TermRule{
		{Term: "cart", Replacement: "kurv"},
		{Term: "workspace", Replacement: "arbeidsomrade"},
		{Term: "invoice", Replacement: "faktura"},
	}

	p := &recordingProvider{}
	runTranslate(t, cfg, p, blockNamed("cta.cart", "Add this to your cart"))

	require.NotEmpty(t, p.prompts)
	sent := flatten(p.prompts[0])
	assert.Contains(t, sent, "kurv", "the rule the text can use")
	assert.NotContains(t, sent, "arbeidsomrade", "and not the ones it cannot")
	assert.NotContains(t, sent, "faktura")
}

// TestABatchSendsTheUnionItsSegmentsCanUse: the projection is per call, which is
// the grain that matters — twenty segments carry what those twenty need.
func TestABatchSendsTheUnionItsSegmentsCanUse(t *testing.T) {
	t.Parallel()

	cfg := baseConfig()
	cfg.BatchSize = 2
	cfg.BatchConcurrency = 1
	cfg.TermRules = []coreprofile.TermRule{
		{Term: "cart", Replacement: "kurv"},
		{Term: "workspace", Replacement: "arbeidsomrade"},
		{Term: "invoice", Replacement: "faktura"},
	}

	p := &recordingProvider{}
	runTranslate(t, cfg, p,
		blockNamed("a", "Add this to your cart"),
		blockNamed("b", "Open your workspace settings"),
	)

	require.Len(t, p.prompts, 1)
	sent := flatten(p.prompts[0])
	assert.Contains(t, sent, "kurv")
	assert.Contains(t, sent, "arbeidsomrade")
	assert.NotContains(t, sent, "faktura", "no segment in this call can use it")
}

// TestScopingTermsDoesNotMoveTheContextFingerprint.
//
// The fingerprint is computed over every rule at the coordinate and must stay
// that way: it is a staleness detector, and a rule added about words a block
// does not contain should still re-check that block, because the block's wording
// may have been chosen under the old set.
func TestScopingTermsDoesNotMoveTheContextFingerprint(t *testing.T) {
	t.Parallel()

	rules := []coreprofile.TermRule{
		{Term: "cart", Replacement: "kurv"},
		{Term: "workspace", Replacement: "arbeidsomrade"},
	}

	cfg := baseConfig()
	cfg.TermRules = rules
	withAll := aitools.NewAITranslateTool(&recordingProvider{}, cfg)

	cfg2 := baseConfig()
	cfg2.TermRules = rules[:1]
	withOne := aitools.NewAITranslateTool(&recordingProvider{}, cfg2)

	block := blockNamed("cta.cart", "Add this to your cart")
	assert.NotEqual(t,
		aitools.ExportCacheFingerprint(withAll, t.Context(), block),
		aitools.ExportCacheFingerprint(withOne, t.Context(), block),
		"a rule the text cannot use is still governance, and removing it is still a change")
}
