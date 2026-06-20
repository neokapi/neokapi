//go:build js && wasm

package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"syscall/js"

	"github.com/neokapi/neokapi/core/model"
	aiprovider "github.com/neokapi/neokapi/providers/ai"
)

// The browser Gemma provider bridges the in-WASM AI tools to a real Gemma 4
// model running in the page via transformers.js + WebGPU. It is the browser
// counterpart of the native kapi-llm plugin (cli/llm_plugin.go, //go:build !js):
// the page cannot spawn a subprocess, so instead of driving kapi-llm over
// stdin/stdout it calls out — via syscall/js — to a host function, exactly as
// core/segment/icu4xjs bridges sentence segmentation to ICU4X.
//
// # Host contract
//
// The page must define a global async function:
//
//	globalThis.kapiGemmaGenerate(payloadJSON: string)
//	  => Promise<string | {text: string, input_tokens?: number, output_tokens?: number}>
//
// payloadJSON is a JSON object: {messages, model, max_tokens, temperature, top_p,
// schema?}. Each message is {role, text, media?: [{kind, mime, data_url}]}. The
// JS glue (packages/kapi-playground/src/gemmaBridge.ts) wraps
// Gemma4ForConditionalGeneration (dtype "q4f16", device "webgpu"), applies the
// chat template, generates, and decodes. When the function is absent the
// provider returns a clear error so a page without the bridge degrades visibly
// (the lab falls back to the demo provider unless gemma is explicitly selected).
//
// Generation is async and can take seconds; this is safe because kapiRun runs
// each command in a goroutine and returns a Promise, so blocking the goroutine
// on the JS promise lets the event loop continue (no deadlock).
const gemmaJSFunc = "kapiGemmaGenerate"

// gemmaProviderID is the AI-provider id the in-browser host registers — the same
// "gemma" id the native cli host uses, kept out of the framework (providers/ai
// stays model-agnostic).
const gemmaProviderID aiprovider.ProviderID = "gemma"

func init() {
	aiprovider.RegisterProvider(
		aiprovider.ProviderInfo{Name: gemmaProviderID, Label: "Gemma (local, in-browser)", Local: true},
		func(cfg aiprovider.Config) aiprovider.LLMProvider { return newGemmaBrowserProvider(cfg) },
	)
}

const browserGemmaModel = "gemma-4-e2b"

type gemmaBrowserProvider struct{ cfg aiprovider.Config }

func newGemmaBrowserProvider(cfg aiprovider.Config) *gemmaBrowserProvider {
	if cfg.Model == "" {
		cfg.Model = browserGemmaModel
	}
	return &gemmaBrowserProvider{cfg: cfg}
}

func (p *gemmaBrowserProvider) Name() aiprovider.ProviderID { return gemmaProviderID }

func (p *gemmaBrowserProvider) InputModalities() []aiprovider.Modality {
	return []aiprovider.Modality{aiprovider.ModalityImage, aiprovider.ModalityAudio}
}

func (p *gemmaBrowserProvider) Translate(ctx context.Context, req aiprovider.TranslateRequest) (*aiprovider.TranslateResponse, error) {
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

func (p *gemmaBrowserProvider) Chat(ctx context.Context, messages []aiprovider.Message) (*aiprovider.ChatResponse, error) {
	return p.generate(ctx, messages, nil)
}

func (p *gemmaBrowserProvider) ChatStructured(ctx context.Context, messages []aiprovider.Message, schema aiprovider.JSONSchema) (*aiprovider.ChatResponse, error) {
	var raw json.RawMessage
	if schema.Schema != nil {
		b, err := json.Marshal(schema.Schema)
		if err != nil {
			return nil, fmt.Errorf("gemma: marshal schema: %w", err)
		}
		raw = b
	}
	return p.generate(ctx, messages, raw)
}

func (p *gemmaBrowserProvider) Close() error { return nil }

func (p *gemmaBrowserProvider) generate(ctx context.Context, messages []aiprovider.Message, schema json.RawMessage) (*aiprovider.ChatResponse, error) {
	fn := js.Global().Get(gemmaJSFunc)
	if !fn.Truthy() {
		return nil, errors.New("gemma (in-browser) not loaded: host did not define globalThis." + gemmaJSFunc + " — call installGemmaBridge() on the page")
	}
	payload := map[string]any{
		"messages":    toJSONMessages(messages),
		"model":       p.cfg.Model,
		"max_tokens":  p.cfg.MaxTokens,
		"temperature": p.cfg.Temperature,
		"top_p":       p.cfg.Temperature,
	}
	if len(schema) > 0 {
		payload["schema"] = json.RawMessage(schema)
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("gemma: marshal payload: %w", err)
	}

	res, err := awaitPromise(ctx, fn.Invoke(string(b)))
	if err != nil {
		return nil, err
	}
	text, inTok, outTok := parseGemmaResult(res)
	return &aiprovider.ChatResponse{
		Content: text,
		Model:   p.cfg.Model,
		Usage:   aiprovider.TokenUsage{InputTokens: inTok, OutputTokens: outTok},
	}, nil
}

// jsonMessage is the wire shape sent to the host bridge.
type jsonMessage struct {
	Role  string      `json:"role"`
	Text  string      `json:"text,omitempty"`
	Media []jsonMedia `json:"media,omitempty"`
}

type jsonMedia struct {
	Kind    string `json:"kind"`
	MIME    string `json:"mime,omitempty"`
	DataURL string `json:"data_url,omitempty"`
}

// toJSONMessages converts provider messages to the bridge wire shape, encoding
// media as data URLs (the browser-friendly form transformers.js accepts).
func toJSONMessages(messages []aiprovider.Message) []jsonMessage {
	out := make([]jsonMessage, 0, len(messages))
	for _, m := range messages {
		jm := jsonMessage{Role: m.Role, Text: m.Text()}
		for _, part := range m.Parts {
			if part.Kind == aiprovider.ContentText || part.Media == nil {
				continue
			}
			jm.Media = append(jm.Media, jsonMedia{
				Kind:    string(part.Kind),
				MIME:    part.Media.MimeType,
				DataURL: mediaDataURL(part.Media),
			})
		}
		out = append(out, jm)
	}
	return out
}

// mediaDataURL builds a base64 data: URL from inline media data, or returns the
// media's URI if it already is a URL.
func mediaDataURL(m *model.Media) string {
	if len(m.Data) > 0 {
		mime := m.MimeType
		if mime == "" {
			mime = "application/octet-stream"
		}
		return "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(m.Data)
	}
	return m.URI
}

// parseGemmaResult extracts the text and optional token counts from the host
// function's resolved value (either a string or an object).
func parseGemmaResult(res js.Value) (text string, inTok, outTok int) {
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
// is cancelled. Safe under kapiRun (which runs commands in a goroutine and
// returns a Promise, so the event loop keeps running while this parks).
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
		msg := "gemma (in-browser) generation failed"
		if len(args) > 0 && args[0].Truthy() {
			msg = "gemma: " + args[0].Call("toString").String()
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
