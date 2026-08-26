//go:build js && wasm

package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"syscall/js"

	"github.com/neokapi/neokapi/core/kbf"
)

// kbfDispatch is the browser entrypoint for the canonical Go KBF engine
// (core/kbf). Where labInspect drives a file through the generic format reader,
// this endpoint exposes the KBF spec operations directly — round-trip,
// validation, target placeholder-faithfulness, annotation anchor resolution,
// and Level-1 HTML preview — so the docs "KBF Lab" and "KBF Tests" pages can
// run the same code the CLI and server run, on KBF authored in the browser.
//
// Unlike labInspect, these operations are pure CPU work over an in-memory JSON
// payload (no filesystem), so the call returns its result synchronously as a
// JSON string the page JSON.parses. The argument is a JSON request string:
//
//	{ "op": "roundtrip"|"validateBlock"|"validateTarget"|"resolveAnchor"
//	      |"validateAnnotation"|"renderHtml", ... }
//
// Every response carries {"ok": bool}; on a usage/decode failure it also
// carries {"error": "..."}. Validation responses set ok:true and report the
// spec findings in "errors" (an empty list means the input is valid).
func kbfDispatch(_ js.Value, args []js.Value) any {
	if len(args) < 1 {
		return kbfError("kbf expects a JSON request string")
	}
	return doKBF(args[0].String())
}

func doKBF(reqJSON string) (out string) {
	defer func() {
		if r := recover(); r != nil {
			out = kbfError("internal error handling KBF request")
		}
	}()

	var req struct {
		Op string `json:"op"`
		// roundtrip
		KBF string `json:"kbf"`
		// validateBlock / validateTarget / resolveAnchor / renderHtml
		Block  json.RawMessage `json:"block"`
		Source json.RawMessage `json:"source"`
		Target json.RawMessage `json:"target"`
		Anchor json.RawMessage `json:"anchor"`
		// validateAnnotation
		Annotation json.RawMessage `json:"annotation"`
	}
	if err := json.Unmarshal([]byte(reqJSON), &req); err != nil {
		return kbfError("invalid request JSON: " + err.Error())
	}

	switch req.Op {
	case "roundtrip":
		return kbfRoundtrip(req.KBF)
	case "validateBlock":
		return kbfValidateBlock(req.Block)
	case "validateTarget":
		return kbfValidateTarget(req.Source, req.Target)
	case "resolveAnchor":
		return kbfResolveAnchor(req.Block, req.Anchor)
	case "validateAnnotation":
		return kbfValidateAnnotation(req.Block, req.Annotation)
	case "renderHtml":
		return kbfRenderHTML(req.Block)
	default:
		return kbfError("unknown op " + req.Op)
	}
}

// kbfRoundtrip decodes a .kbf.json payload and re-marshals it to the canonical
// deterministic form, returning the output text and its SHA-256. The docs
// Tests page compares this against the TypeScript mirror's output to prove the
// two implementations are byte-for-byte equivalent.
func kbfRoundtrip(src string) string {
	file, err := kbf.Unmarshal([]byte(src))
	if err != nil {
		return kbfResult(map[string]any{"ok": false, "error": err.Error()})
	}
	data, err := kbf.Marshal(file)
	if err != nil {
		return kbfResult(map[string]any{"ok": false, "error": err.Error()})
	}
	sum := sha256.Sum256(data)
	return kbfResult(map[string]any{
		"ok":     true,
		"output": string(data),
		"sha256": hex.EncodeToString(sum[:]),
	})
}

func kbfValidateBlock(raw json.RawMessage) string {
	var b kbf.Block
	if err := json.Unmarshal(raw, &b); err != nil {
		return kbfError("decode block: " + err.Error())
	}
	return kbfResult(map[string]any{
		"ok":     true,
		"errors": encodeValidationErrors(kbf.ValidateBlock(&b)),
	})
}

func kbfValidateTarget(srcRaw, targetRaw json.RawMessage) string {
	var src kbf.Block
	if err := json.Unmarshal(srcRaw, &src); err != nil {
		return kbfError("decode source block: " + err.Error())
	}
	var target []kbf.Run
	if err := json.Unmarshal(targetRaw, &target); err != nil {
		return kbfError("decode target runs: " + err.Error())
	}
	return kbfResult(map[string]any{
		"ok":     true,
		"errors": encodeValidationErrors(kbf.ValidateTargetAgainstSource(&src, target)),
	})
}

func kbfResolveAnchor(blockRaw, anchorRaw json.RawMessage) string {
	var b kbf.Block
	if err := json.Unmarshal(blockRaw, &b); err != nil {
		return kbfError("decode block: " + err.Error())
	}
	var anchor kbf.Anchor
	if err := json.Unmarshal(anchorRaw, &anchor); err != nil {
		return kbfError("decode anchor: " + err.Error())
	}
	res := kbf.ResolveAnchor(&b, anchor)
	resolution := map[string]any{
		"ok":   res.OK,
		"kind": string(res.Kind),
	}
	if !res.OK {
		resolution["reason"] = string(res.Err)
	}
	switch res.Kind {
	case kbf.AnchorRun:
		if res.RunTarget != nil {
			resolution["runId"] = res.RunTarget.RunID()
		}
	case kbf.AnchorRange:
		resolution["rangeText"] = res.RangeText
		resolution["rangeRunCount"] = len(res.RangeRuns)
	case kbf.AnchorForm:
		resolution["formRunCount"] = len(res.FormRuns)
	}
	return kbfResult(map[string]any{"ok": true, "resolution": resolution})
}

// kbfValidateAnnotation checks a whole annotation record against a block: the
// block it names, then where inside it the anchor points. A record naming
// another block is the case anchor resolution alone cannot see.
func kbfValidateAnnotation(blockRaw, annRaw json.RawMessage) string {
	var b kbf.Block
	if err := json.Unmarshal(blockRaw, &b); err != nil {
		return kbfError("decode block: " + err.Error())
	}
	var ann kbf.Annotation
	if err := json.Unmarshal(annRaw, &ann); err != nil {
		return kbfError("decode annotation: " + err.Error())
	}
	result := map[string]any{"valid": true}
	if verr := kbf.ValidateAnchor(&b, ann); verr != nil {
		result["valid"] = false
		result["reason"] = string(verr.Reason)
		result["message"] = verr.Message
	}
	return kbfResult(map[string]any{"ok": true, "validation": result})
}

func kbfRenderHTML(raw json.RawMessage) string {
	var b kbf.Block
	if err := json.Unmarshal(raw, &b); err != nil {
		return kbfError("decode block: " + err.Error())
	}
	return kbfResult(map[string]any{
		"ok":   true,
		"html": kbf.RenderBlockHTML(&b, nil),
	})
}

// encodeValidationErrors flattens core/kbf validation errors into the
// machine-readable shape the docs pages render (mirrors the TypeScript
// Diagnostic shape so both engines compare cleanly).
func encodeValidationErrors(errs []kbf.ValidationError) []map[string]any {
	out := make([]map[string]any, 0, len(errs))
	for _, e := range errs {
		out = append(out, map[string]any{
			"kind":        string(e.Kind),
			"blockId":     e.BlockID,
			"placeholder": e.Placeholder,
			"runId":       e.RunID,
			"message":     e.Message,
		})
	}
	return out
}

func kbfError(msg string) string {
	return kbfResult(map[string]any{"ok": false, "error": msg})
}

func kbfResult(v map[string]any) string {
	data, err := json.Marshal(v)
	if err != nil {
		return `{"ok":false,"error":"failed to encode KBF response"}`
	}
	return string(data)
}
