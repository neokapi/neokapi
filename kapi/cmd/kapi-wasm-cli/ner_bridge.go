//go:build js && wasm

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"syscall/js"

	"github.com/neokapi/neokapi/core/ai/ner"
	"github.com/neokapi/neokapi/core/model"
)

// JS-bridged local NER (the on-device model path for `ai-entity-extract` with
// `engine: ner` in the browser). The page registers
//
//	globalThis.kapiLocalNER = async (reqJSON) => respJSON
//
// where reqJSON is {"text": string, "locale": string} and respJSON is
// {"entities": [{"text","type","confidence","offset","length"}]} with type a
// bare category (person, organization, location, date, …). The lab loads a
// GLiNER ONNX model with onnxruntime-web and registers this hook; until it
// does, detection fails with an actionable error rather than silently
// returning nothing — the engine never pretends a model ran.
//
// The provider is registered unconditionally (ner.SetLocalProvider) and probes
// the JS global per call, so load order between the wasm boot and the model
// download doesn't matter.

const jsLocalNERFunc = "kapiLocalNER"

type jsNERProvider struct{}

func (p *jsNERProvider) Name() string { return "local-js" }

func (p *jsNERProvider) DetectEntities(ctx context.Context, req ner.Request) (*ner.Response, error) {
	fn := js.Global().Get(jsLocalNERFunc)
	if fn.IsUndefined() || fn.IsNull() {
		return nil, fmt.Errorf("local NER model not loaded: define %s(req) on the page (the lab's “load local NER” action does this)", jsLocalNERFunc)
	}

	payload, err := json.Marshal(map[string]string{
		"text":   req.Text,
		"locale": string(req.Locale),
	})
	if err != nil {
		return nil, err
	}

	respJSON, err := awaitJS(ctx, fn.Invoke(string(payload)))
	if err != nil {
		return nil, fmt.Errorf("local NER: %w", err)
	}

	var resp struct {
		Entities []struct {
			Text       string  `json:"text"`
			Type       string  `json:"type"`
			Confidence float64 `json:"confidence"`
			Offset     int     `json:"offset"`
			Length     int     `json:"length"`
		} `json:"entities"`
	}
	if err := json.Unmarshal([]byte(respJSON), &resp); err != nil {
		return nil, fmt.Errorf("local NER: bad response: %w", err)
	}

	out := &ner.Response{Entities: make([]ner.DetectedEntity, 0, len(resp.Entities))}
	for _, e := range resp.Entities {
		out.Entities = append(out.Entities, ner.DetectedEntity{
			Text:       e.Text,
			Type:       model.EntityType(model.EntityPrefix + e.Type),
			Confidence: e.Confidence,
			Offset:     e.Offset,
			Length:     e.Length,
		})
	}
	return out, nil
}

func (p *jsNERProvider) DetectEntitiesBatch(ctx context.Context, reqs []ner.Request) ([]ner.Response, error) {
	out := make([]ner.Response, len(reqs))
	for i, req := range reqs {
		resp, err := p.DetectEntities(ctx, req)
		if err != nil {
			return nil, err
		}
		out[i] = *resp
	}
	return out, nil
}

func (p *jsNERProvider) SupportedLocales() []model.LocaleID { return nil }
func (p *jsNERProvider) Close() error                       { return nil }

// awaitJS resolves a JS Promise (or passes through a plain value) to its
// string result, honouring ctx cancellation. The caller must be on a
// goroutine (command execution is), so blocking on the channel is safe.
func awaitJS(ctx context.Context, v js.Value) (string, error) {
	if v.Type() != js.TypeObject || v.Get("then").Type() != js.TypeFunction {
		return v.String(), nil
	}
	done := make(chan struct{})
	var result string
	var failure error
	then := js.FuncOf(func(_ js.Value, args []js.Value) any {
		if len(args) > 0 {
			result = args[0].String()
		}
		close(done)
		return nil
	})
	defer then.Release()
	catch := js.FuncOf(func(_ js.Value, args []js.Value) any {
		msg := "rejected"
		if len(args) > 0 {
			msg = args[0].String()
		}
		failure = fmt.Errorf("%s", msg)
		close(done)
		return nil
	})
	defer catch.Release()
	v.Call("then", then, catch)
	select {
	case <-done:
		return result, failure
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

// registerLocalNER wires the JS-bridged provider as the process-wide local
// NER model.
func registerLocalNER() {
	ner.SetLocalProvider(&jsNERProvider{})
}
