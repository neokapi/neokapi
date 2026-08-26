package tools_test

import (
	"context"
	"strings"
	"testing"

	aitools "github.com/neokapi/neokapi/core/ai/tools"
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

func (p *recordingProvider) ChatStructured(ctx context.Context, msgs []aiprovider.Message, _ aiprovider.JSONSchema) (*aiprovider.ChatResponse, error) {
	return p.Chat(ctx, msgs)
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

func (s *stubPriors) PriorVersion(_ context.Context, unit, point string, _, _ model.LocaleID, fingerprint string) (string, string, bool) {
	s.calls++
	s.unitSeen, s.pointSeen, s.fingerprintSeen = unit, point, fingerprint
	if unit != s.unit || point != s.point {
		return "", "", false
	}
	return s.source, s.target, true
}

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
	cfg.PriorVersions = priors
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
	cfg.PriorVersions = priors
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
	withPrior.PriorVersions = &stubPriors{
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

// TestABlockWithHistoryIsTranslatedAlone: the batch prompt carries one shared
// context and a key per segment, with no per-segment slot for a block's own
// history. A prior version can only travel on the single-block path, so a block
// that has one is packed alone — the rule DNT already follows.
func TestABlockWithHistoryIsTranslatedAlone(t *testing.T) {
	t.Parallel()

	priors := &stubPriors{
		unit: "cta.start", point: "acme\x1fweb\x1fsite",
		source: "Get started", target: "Kom i gang",
	}
	cfg := baseConfig()
	cfg.PriorVersions = priors
	cfg.Point = priors.point

	p := &recordingProvider{}
	runTranslate(t, cfg, p,
		blockNamed("cta.start", "Get started today"),
		blockNamed("cta.other", "Something else"),
		blockNamed("cta.third", "A third string"),
	)

	// The block with history got its own call carrying the reference; had it
	// been packed with the others, the reference would have had nowhere to go.
	var carried int
	for _, msgs := range p.prompts {
		seen := flatten(msgs)
		if strings.Contains(seen, "Kom i gang") {
			carried++
			assert.Contains(t, seen, "Get started today",
				"the reference must travel with the block it belongs to, alone")
			assert.NotContains(t, seen, "Something else")
		}
	}
	assert.Equal(t, 1, carried, "exactly one call carries the reference")
}

// flatten renders a prompt to one searchable string.
func flatten(msgs []aiprovider.Message) string {
	var sb strings.Builder
	for _, m := range msgs {
		sb.WriteString(m.Text())
	}
	return sb.String()
}
