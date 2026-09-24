package aiprovider

import (
	"context"
)

// Translation batch sizing must account for output limits as well as input
// context. Output grows with the source text, and exceeding the response budget
// can truncate structured output mid-JSON. Providers expose these limits so the
// caller can size batches and request an appropriate output budget.

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

// ConservativeMaxOutputTokens is the fallback output budget for providers or
// models without declared limits, including plugins and self-hosted endpoints.
const ConservativeMaxOutputTokens = 4096

// NonStreamingMaxOutputTokens caps synchronous request budgets even when a
// model supports larger responses. Large outputs can hold an HTTP connection
// open for minutes; streaming and batch APIs have different constraints.
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

// LimitsForModel resolves a model ID by longest-prefix match in models.json.
// It returns ok=false for an unknown model or an entry without a declared output
// limit, allowing the caller to use the conservative fallback.
func LimitsForModel(model string) (Limits, bool) {
	m, ok := ModelForID(model)
	if !ok || m.MaxOutputTokens == 0 {
		return Limits{}, false
	}
	return Limits{MaxOutputTokens: m.MaxOutputTokens, ContextWindow: m.ContextWindow}, true
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

// MaxOutputTokensFrom reports the per-request output budget carried by ctx, if
// the caller set one. A provider outside this package — a plugin backend — reads
// its request budget here; without it, such a provider can only ever send its own
// fixed default and truncates the calls kapi sized deliberately.
func MaxOutputTokensFrom(ctx context.Context) (n int, ok bool) {
	n, ok = ctx.Value(maxTokensKey{}).(int)
	return n, ok && n > 0
}

// maxOutputTokensFrom returns the per-request budget, or fallback when the
// caller set none. The result is clamped to the model's ceiling by the provider.
func maxOutputTokensFrom(ctx context.Context, fallback int) int {
	if n, ok := MaxOutputTokensFrom(ctx); ok {
		return n
	}
	return fallback
}
