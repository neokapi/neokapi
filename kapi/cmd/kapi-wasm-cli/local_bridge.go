//go:build js && wasm

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"syscall/js"

	aiprovider "github.com/neokapi/neokapi/providers/ai"
)

// The browser "local" provider bridges the in-WASM AI tools to a model running
// on-device in the page. It is the browser counterpart of the native CLI/desktop
// "local" story (where the local provider is an Ollama runtime): a web page
// cannot reach a local daemon, so it calls out — via syscall/js — to a host
// function, exactly as core/segment/icu4xjs bridges segmentation to ICU4X.
//
// The host (packages/kapi-playground/src/localLlmBridge.ts) runs the model with
// WebLLM (WebGPU) when available — MLC models that mirror the native Ollama
// lineup by name (Llama 3.2 3B, Qwen3 1.7B) — and falls back to transformers.js
// (WASM) otherwise. The provider itself is engine-agnostic: it just ships a chat
// request over the bridge and reads the text back.
//
// # Host contract
//
//	globalThis.kapiLocalGenerate(payloadJSON: string)
//	  => Promise<string | {text: string, input_tokens?: number, output_tokens?: number}>
//
// payloadJSON is {messages, model, max_tokens, temperature}. Each message is
// {role, text}. When the function is absent the provider returns a clear error so
// a page without the bridge degrades visibly (the lab falls back to the demo
// provider unless "local" is explicitly selected).
//
// Generation is async and can take seconds; this is safe because kapiRun runs
// each command in a goroutine and returns a Promise, so blocking the goroutine on
// the JS promise lets the event loop continue (no deadlock).
const localJSFunc = "kapiLocalGenerate"

// localProviderID is the AI-provider id the in-browser host registers. It is kept
// out of the framework (providers/ai stays model-agnostic); the native CLI uses
// the built-in "ollama" provider for the same on-device role.
const localProviderID aiprovider.ProviderID = "local"

func init() {
	aiprovider.RegisterProvider(
		aiprovider.ProviderInfo{Name: localProviderID, Label: "Local (in-browser)", Local: true, DefaultModel: defaultLocalModel},
		func(cfg aiprovider.Config) aiprovider.LLMProvider { return newLocalBrowserProvider(cfg) },
	)
}

// defaultLocalModel mirrors the WebLLM bridge's default (Llama 3.2 3B).
const defaultLocalModel = "llama-3.2-3b"

type localBrowserProvider struct{ cfg aiprovider.Config }

func newLocalBrowserProvider(cfg aiprovider.Config) *localBrowserProvider {
	if cfg.Model == "" {
		cfg.Model = defaultLocalModel
	}
	return &localBrowserProvider{cfg: cfg}
}

func (p *localBrowserProvider) Name() aiprovider.ProviderID { return localProviderID }

// InputModalities is text-only: the browser local engines (WebLLM / transformers.js
// text models) are LLMs, not multimodal. Image/audio understanding in the browser
// is handled separately (the vision OCR demo calls transformers.js directly).
func (p *localBrowserProvider) InputModalities() []aiprovider.Modality { return nil }

func (p *localBrowserProvider) Translate(ctx context.Context, req aiprovider.TranslateRequest) (*aiprovider.TranslateResponse, error) {
	prompt := fmt.Sprintf(
		"Translate the following text from %s to %s. Return ONLY the translation, no explanation.\n\nText: %s",
		req.SourceLanguage, req.TargetLocale, req.Source,
	) + req.Directives()
	resp, err := p.Chat(ctx, []aiprovider.Message{aiprovider.TextMessage("user", prompt)})
	if err != nil {
		return nil, err
	}
	return &aiprovider.TranslateResponse{
		Translation: resp.Content,
		Confidence:  0.7,
		Model:       resp.Model,
		Usage:       resp.Usage,
	}, nil
}

func (p *localBrowserProvider) Chat(ctx context.Context, messages []aiprovider.Message) (*aiprovider.ChatResponse, error) {
	return p.generate(ctx, messages, nil)
}

func (p *localBrowserProvider) ChatStructured(ctx context.Context, messages []aiprovider.Message, schema aiprovider.JSONSchema) (*aiprovider.ChatResponse, error) {
	var raw json.RawMessage
	if schema.Schema != nil {
		b, err := json.Marshal(schema.Schema)
		if err != nil {
			return nil, fmt.Errorf("local: marshal schema: %w", err)
		}
		raw = b
	}
	return p.generate(ctx, messages, raw)
}

func (p *localBrowserProvider) Close() error { return nil }

func (p *localBrowserProvider) generate(ctx context.Context, messages []aiprovider.Message, schema json.RawMessage) (*aiprovider.ChatResponse, error) {
	fn := js.Global().Get(localJSFunc)
	if !fn.Truthy() {
		return nil, errors.New("local model (in-browser) not loaded: host did not define globalThis." + localJSFunc + " — call installLocalLLMBridge() on the page")
	}
	payload := map[string]any{
		"messages":    toLocalMessages(messages),
		"model":       p.cfg.Model,
		"max_tokens":  p.cfg.MaxTokens,
		"temperature": p.cfg.Temperature,
	}
	if len(schema) > 0 {
		payload["schema"] = json.RawMessage(schema)
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("local: marshal payload: %w", err)
	}

	res, err := awaitPromise(ctx, fn.Invoke(string(b)))
	if err != nil {
		return nil, err
	}
	text, inTok, outTok := parseLocalResult(res)
	return &aiprovider.ChatResponse{
		Content: text,
		Model:   p.cfg.Model,
		Usage:   aiprovider.TokenUsage{InputTokens: inTok, OutputTokens: outTok},
	}, nil
}

// localMessage is the wire shape sent to the host bridge (text-only).
type localMessage struct {
	Role string `json:"role"`
	Text string `json:"text,omitempty"`
}

func toLocalMessages(messages []aiprovider.Message) []localMessage {
	out := make([]localMessage, 0, len(messages))
	for _, m := range messages {
		out = append(out, localMessage{Role: m.Role, Text: m.Text()})
	}
	return out
}

// parseLocalResult extracts the text and optional token counts from the host
// function's resolved value (either a string or an object).
func parseLocalResult(res js.Value) (text string, inTok, outTok int) {
	switch res.Type() {
	case js.TypeString:
		return res.String(), 0, 0
	case js.TypeObject:
		if t := res.Get("text"); t.Truthy() {
			text = t.String()
		}
		if v := res.Get("input_tokens"); v.Type() == js.TypeNumber {
			inTok = v.Int()
		}
		if v := res.Get("output_tokens"); v.Type() == js.TypeNumber {
			outTok = v.Int()
		}
		return text, inTok, outTok
	default:
		return "", 0, 0
	}
}

// awaitPromise blocks the calling goroutine until the JS promise settles or ctx
// is cancelled. Safe under kapiRun (which runs commands in a goroutine and returns
// a Promise, so the event loop keeps running while this parks). Shared with the
// browser MT bridge (same js+wasm package).
func awaitPromise(ctx context.Context, promise js.Value) (js.Value, error) {
	type result struct {
		v   js.Value
		err error
	}
	ch := make(chan result, 1)

	then := js.FuncOf(func(_ js.Value, args []js.Value) any {
		var v js.Value
		if len(args) > 0 {
			v = args[0]
		}
		ch <- result{v: v}
		return nil
	})
	catch := js.FuncOf(func(_ js.Value, args []js.Value) any {
		msg := "local model (in-browser) generation failed"
		if len(args) > 0 && args[0].Truthy() {
			msg = "local: " + args[0].Call("toString").String()
		}
		ch <- result{err: errors.New(msg)}
		return nil
	})
	promise.Call("then", then).Call("catch", catch)

	select {
	case <-ctx.Done():
		// Leave the callbacks un-released: a late settle must not invoke a freed
		// js.Func. The small per-cancel leak is acceptable (cancellation is rare).
		return js.Undefined(), ctx.Err()
	case r := <-ch:
		then.Release()
		catch.Release()
		return r.v, r.err
	}
}
