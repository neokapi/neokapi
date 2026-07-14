package aiprovider

import (
	"context"
	"strings"
)

// What a model can actually emit — and why anyone should care.
//
// For translation, output size tracks input size: translating N tokens produces
// roughly N tokens. So the constraint on "how much can we translate in one call"
// is not the context window (1M on current frontier models) but the *max output*
// (128k at best, 64k on Gemini, and far less if you don't ask for it). The two
// differ by an order of magnitude, and the smaller one binds.
//
// kapi used to send a fixed 100 blocks per call with max_tokens defaulted to
// 4096. Anything longer than a UI string overflowed that, the reply came back
// truncated mid-JSON, and the run died with "unmarshal response" — a bug whose
// real cause was that nothing in the system knew what the model could emit.

// Limits describes the hard ceilings of a configured model.
type Limits struct {
	// MaxOutputTokens is the most the model will emit in one response. Zero
	// means unknown, and callers must then fall back to a conservative default
	// rather than assume the sky.
	MaxOutputTokens int
	// ContextWindow is the total input the model accepts. Recorded for
	// completeness; it is rarely the binding constraint for translation.
	ContextWindow int
}

// ConservativeMaxOutputTokens is what we assume when a model is unknown to us —
// a plugin provider, a self-hosted endpoint, a model newer than this table. Low
// enough that every current model honours it, so an unknown model produces a
// smaller batch rather than a truncated response.
const ConservativeMaxOutputTokens = 4096

// NonStreamingMaxOutputTokens caps what we will ask for on a non-streaming call.
//
// The model ceilings below are the *streaming* ceilings. Asking a synchronous
// request to produce 128k tokens is a request to hold an HTTP connection open
// for many minutes, and the provider SDKs warn against it above roughly this
// mark. So the ceiling that applies to a blocking call is this one, not the
// model's — the model's ceiling is only reachable by streaming or via a batch
// API.
const NonStreamingMaxOutputTokens = 16_000

// EffectiveMaxOutputTokens is the ceiling that actually applies to a blocking
// request against this model: the lower of what the model can emit and what a
// synchronous call should ask for.
func (l Limits) EffectiveMaxOutputTokens() int {
	if l.MaxOutputTokens <= 0 || l.MaxOutputTokens > NonStreamingMaxOutputTokens {
		return NonStreamingMaxOutputTokens
	}
	return l.MaxOutputTokens
}

// LimitedProvider is implemented by providers that know their model's ceilings.
// Optional: a provider that doesn't implement it is treated as unknown, which is
// safe rather than merely undefined.
type LimitedProvider interface {
	Limits() Limits
}

// LimitsOf reports what p can emit, falling back to the conservative default for
// a provider that does not declare its limits.
func LimitsOf(p LLMProvider) Limits {
	if lp, ok := p.(LimitedProvider); ok {
		if l := lp.Limits(); l.MaxOutputTokens > 0 {
			return l
		}
	}
	return Limits{MaxOutputTokens: ConservativeMaxOutputTokens}
}

// modelLimits maps a model-ID prefix to its ceilings. Prefix-matched, so a dated
// snapshot (claude-sonnet-5-20260115) resolves to its family without needing a
// row per release.
//
// Longest prefix wins, so a more specific entry can override a family default.
var modelLimits = map[string]Limits{
	// Anthropic — 1M context is standard, not a beta, and carries no long-context
	// surcharge.
	"claude-opus-4-8":  {MaxOutputTokens: 128_000, ContextWindow: 1_000_000},
	"claude-sonnet-5":  {MaxOutputTokens: 128_000, ContextWindow: 1_000_000},
	"claude-fable-5":   {MaxOutputTokens: 128_000, ContextWindow: 1_000_000},
	"claude-haiku-4-5": {MaxOutputTokens: 64_000, ContextWindow: 200_000},
	"claude-opus-4":    {MaxOutputTokens: 32_000, ContextWindow: 200_000},
	"claude-sonnet-4":  {MaxOutputTokens: 64_000, ContextWindow: 200_000},
	"claude-3-5":       {MaxOutputTokens: 8_192, ContextWindow: 200_000},

	// OpenAI
	"gpt-5.6": {MaxOutputTokens: 128_000, ContextWindow: 1_050_000},
	"gpt-5":   {MaxOutputTokens: 128_000, ContextWindow: 400_000},
	"gpt-4.1": {MaxOutputTokens: 32_768, ContextWindow: 1_047_576},
	"gpt-4o":  {MaxOutputTokens: 16_384, ContextWindow: 128_000},
	"o3":      {MaxOutputTokens: 100_000, ContextWindow: 200_000},
	"gpt-4-":  {MaxOutputTokens: 4_096, ContextWindow: 128_000},
	"gpt-3.5": {MaxOutputTokens: 4_096, ContextWindow: 16_385},
	// Gemini. 3.5 Flash is 65k; the rest of the Gemini 3 family is 64k. Longest
	// prefix wins, so the specific entry overrides the family.
	"gemini-3.5-flash": {MaxOutputTokens: 65_536, ContextWindow: 1_048_576},
	"gemini-3":         {MaxOutputTokens: 64_000, ContextWindow: 1_048_576},
	"gemini-2":         {MaxOutputTokens: 8_192, ContextWindow: 1_048_576},
	"gemini-1.5":       {MaxOutputTokens: 8_192, ContextWindow: 1_048_576},
}

// LimitsForModel resolves a model ID to its known ceilings by longest-prefix
// match, returning ok=false for a model this build has never heard of.
func LimitsForModel(model string) (Limits, bool) {
	m := strings.ToLower(strings.TrimSpace(model))
	if m == "" {
		return Limits{}, false
	}

	var best string
	for prefix := range modelLimits {
		if strings.HasPrefix(m, prefix) && len(prefix) > len(best) {
			best = prefix
		}
	}
	if best == "" {
		return Limits{}, false
	}
	return modelLimits[best], true
}

// maxTokensKey carries a per-request output budget.
type maxTokensKey struct{}

// WithMaxOutputTokens sets the output budget for the calls made with this
// context. It exists because the budget is a property of the *request* — a batch
// of 3 blocks needs a fraction of what a batch of 30 needs — while the provider
// is constructed once and reused. Asking for only what a call can need keeps a
// truncation from being a surprise, and keeps providers that meter or require
// streaming on large max_tokens out of trouble.
func WithMaxOutputTokens(ctx context.Context, n int) context.Context {
	if n <= 0 {
		return ctx
	}
	return context.WithValue(ctx, maxTokensKey{}, n)
}

// maxOutputTokensFrom returns the per-request budget, or fallback when the
// caller set none. The result is clamped to the model's ceiling by the provider.
func maxOutputTokensFrom(ctx context.Context, fallback int) int {
	if n, ok := ctx.Value(maxTokensKey{}).(int); ok && n > 0 {
		return n
	}
	return fallback
}
