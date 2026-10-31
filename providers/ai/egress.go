package aiprovider

import (
	"context"
	"maps"
	"slices"

	"github.com/neokapi/neokapi/core/ai/prompt"
	"github.com/neokapi/neokapi/core/redaction"
)

// redactingProvider redacts every request whose context carries a
// redaction.Egress before the provider it wraps sends it: each text part and
// section of a chat's messages, and every field of a translate request. A
// redact step puts the Egress on the context of the tool that reads the block
// (redaction.SecretAnnotation.BindContext), so the term rules, voice profile,
// prior version and neighbouring blocks a request carries beside the block
// are redacted under the policy that redacted the block.
//
// The provider it wraps reads a context with the Egress cleared, so a call
// is redacted once however many wrappers it passes through, and the
// recording wrapper inside records the request as it is sent.
type redactingProvider struct {
	LLMProvider
}

// Unwrap returns the provider this one wraps.
func (w *redactingProvider) Unwrap() LLMProvider { return w.LLMProvider }

// Limits reports the limits of the provider this one wraps, so a caller
// sizing a batch by LimitsOf reads the model's limits and not the
// conservative fallback.
func (w *redactingProvider) Limits() Limits { return LimitsOf(w.LLMProvider) }

func (w *redactingProvider) Translate(ctx context.Context, req TranslateRequest) (*TranslateResponse, error) {
	ctx, req, err := redactTranslate(ctx, req)
	if err != nil {
		return nil, err
	}
	return w.LLMProvider.Translate(ctx, req)
}

func (w *redactingProvider) Chat(ctx context.Context, messages []Message) (*ChatResponse, error) {
	ctx, messages, err := redactMessages(ctx, messages)
	if err != nil {
		return nil, err
	}
	return w.LLMProvider.Chat(ctx, messages)
}

func (w *redactingProvider) ChatStructured(ctx context.Context, messages []Message, schema JSONSchema) (*ChatResponse, error) {
	ctx, messages, err := redactMessages(ctx, messages)
	if err != nil {
		return nil, err
	}
	return w.LLMProvider.ChatStructured(ctx, messages, schema)
}

// redactingStreamingProvider keeps the StreamingLLMProvider interface the
// translate tool type-asserts for.
type redactingStreamingProvider struct {
	*redactingProvider
	streaming StreamingLLMProvider
}

func (w *redactingStreamingProvider) ChatStream(ctx context.Context, messages []Message, onEvent func(ChatStreamEvent)) (*ChatResponse, error) {
	ctx, messages, err := redactMessages(ctx, messages)
	if err != nil {
		return nil, err
	}
	return w.streaming.ChatStream(ctx, messages, onEvent)
}

func (w *redactingStreamingProvider) ChatStructuredStream(ctx context.Context, messages []Message, schema JSONSchema, onEvent func(ChatStreamEvent)) (*ChatResponse, error) {
	ctx, messages, err := redactMessages(ctx, messages)
	if err != nil {
		return nil, err
	}
	return w.streaming.ChatStructuredStream(ctx, messages, schema, onEvent)
}

// Redacting wraps p so a request whose context carries a redaction.Egress is
// redacted before p sends it. Every AI tool calls its provider through it,
// whoever built the provider. A provider already wrapped is returned as it
// is.
func Redacting(p LLMProvider) LLMProvider {
	switch p.(type) {
	case nil:
		return nil
	case *redactingProvider, *redactingStreamingProvider:
		return p
	}
	base := &redactingProvider{LLMProvider: p}
	if s, ok := p.(StreamingLLMProvider); ok {
		return &redactingStreamingProvider{redactingProvider: base, streaming: s}
	}
	return base
}

// WithoutRedaction returns the provider a Redacting wrapper holds, or p when
// p is not one.
func WithoutRedaction(p LLMProvider) LLMProvider {
	switch w := p.(type) {
	case *redactingProvider:
		return w.LLMProvider
	case *redactingStreamingProvider:
		return w.LLMProvider
	}
	return p
}

// redactMessages returns messages with every text part and section redacted
// under the Egress ctx carries, and ctx with the Egress cleared. With none it
// returns both as they are.
func redactMessages(ctx context.Context, messages []Message) (context.Context, []Message, error) {
	e := redaction.EgressFrom(ctx)
	if e == nil {
		return ctx, messages, nil
	}
	var texts []string
	for _, m := range messages {
		for _, p := range m.Parts {
			if p.Kind == ContentText {
				texts = append(texts, p.Text)
			}
		}
		for _, s := range m.Sections {
			texts = append(texts, s.Text)
		}
	}
	red, err := e.Redact(ctx, texts)
	if err != nil {
		return ctx, nil, err
	}
	out := make([]Message, len(messages))
	i := 0
	for k, m := range messages {
		m.Parts = slices.Clone(m.Parts)
		for j := range m.Parts {
			if m.Parts[j].Kind == ContentText {
				m.Parts[j].Text = red[i]
				i++
			}
		}
		m.Sections = slices.Clone(m.Sections)
		for j := range m.Sections {
			m.Sections[j].Text = red[i]
			i++
		}
		out[k] = m
	}
	return redaction.WithEgress(ctx, nil), out, nil
}

// redactTranslate returns req with every field the prompt renders redacted
// under the Egress ctx carries: the source, the term renderings, the
// do-not-translate terms, the voice guide, the instruction and the block's
// context (its key, its neighbours and its prior version).
func redactTranslate(ctx context.Context, req TranslateRequest) (context.Context, TranslateRequest, error) {
	e := redaction.EgressFrom(ctx)
	if e == nil {
		return ctx, req, nil
	}
	terms := slices.Sorted(maps.Keys(req.PreferredTerms))
	bc := req.BlockContext
	texts := []string{req.Source, req.VoiceGuide, req.Instruction, bc.Key}
	for _, k := range terms {
		texts = append(texts, k, req.PreferredTerms[k])
	}
	texts = append(texts, req.DoNotTranslate...)
	texts = append(texts, bc.Before...)
	texts = append(texts, bc.After...)
	if bc.Prior != nil {
		texts = append(texts, bc.Prior.Source, bc.Prior.Target)
	}
	red, err := e.Redact(ctx, texts)
	if err != nil {
		return ctx, req, err
	}
	next := func() string { s := red[0]; red = red[1:]; return s }
	req.Source, req.VoiceGuide, req.Instruction, bc.Key = next(), next(), next(), next()
	if len(terms) > 0 {
		pt := make(map[string]string, len(terms))
		for range terms {
			k := next()
			pt[k] = next()
		}
		req.PreferredTerms = pt
	}
	if n := len(req.DoNotTranslate); n > 0 {
		dnt := make([]string, n)
		for i := range dnt {
			dnt[i] = next()
		}
		req.DoNotTranslate = dnt
	}
	if n := len(bc.Before); n > 0 {
		before := make([]string, n)
		for i := range before {
			before[i] = next()
		}
		bc.Before = before
	}
	if n := len(bc.After); n > 0 {
		after := make([]string, n)
		for i := range after {
			after[i] = next()
		}
		bc.After = after
	}
	if bc.Prior != nil {
		bc.Prior = &prompt.PriorVersion{Source: next(), Target: next()}
	}
	req.BlockContext = bc
	return redaction.WithEgress(ctx, nil), req, nil
}
