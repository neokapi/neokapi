//go:build js && wasm

package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"syscall/js"

	"github.com/neokapi/neokapi/core/klf"
)

// klfDispatch is the browser entrypoint for the canonical Go KLF engine
// (core/klf). Where labInspect drives a file through the generic format reader,
// this endpoint exposes the KLF spec operations directly — round-trip,
// validation, target placeholder-faithfulness, annotation anchor resolution,
// and Level-1 HTML preview — so the docs "KLF Lab" and "KLF Tests" pages can
// run the same code the CLI and server run, on KLF authored in the browser.
//
// Unlike labInspect, these operations are pure CPU work over an in-memory JSON
// payload (no filesystem), so the call returns its result synchronously as a
// JSON string the page JSON.parses. The argument is a JSON request string:
//
//	{ "op": "roundtrip"|"validateBlock"|"validateTarget"|"resolveAnchor"|"renderHtml", ... }
//
// Every response carries {"ok": bool}; on a usage/decode failure it also
// carries {"error": "..."}. Validation responses set ok:true and report the
// spec findings in "errors" (an empty list means the input is valid).
func klfDispatch(_ js.Value, args []js.Value) any {
	if len(args) < 1 {
		return klfError("klf expects a JSON request string")
	}
	return doKLF(args[0].String())
}

func doKLF(reqJSON string) (out string) {
	defer func() {
		if r := recover(); r != nil {
			out = klfError("internal error handling KLF request")
		}
	}()

	var req struct {
		Op string `json:"op"`
		// roundtrip
		KLF string `json:"klf"`
		// validateBlock / validateTarget / resolveAnchor / renderHtml
		Block  json.RawMessage `json:"block"`
		Source json.RawMessage `json:"source"`
		Target json.RawMessage `json:"target"`
		Anchor json.RawMessage `json:"anchor"`
	}
	if err := json.Unmarshal([]byte(reqJSON), &req); err != nil {
		return klfError("invalid request JSON: " + err.Error())
	}

	switch req.Op {
	case "roundtrip":
		return klfRoundtrip(req.KLF)
	case "validateBlock":
		return klfValidateBlock(req.Block)
	case "validateTarget":
		return klfValidateTarget(req.Source, req.Target)
	case "resolveAnchor":
		return klfResolveAnchor(req.Block, req.Anchor)
	case "renderHtml":
		return klfRenderHTML(req.Block)
	default:
		return klfError("unknown op " + req.Op)
	}
}

// klfRoundtrip decodes a .klf payload and re-marshals it to the canonical
// deterministic form, returning the output text and its SHA-256. The docs
// Tests page compares this against the TypeScript mirror's output to prove the
// two implementations are byte-for-byte equivalent.
func klfRoundtrip(src string) string {
	file, err := klf.Unmarshal([]byte(src))
	if err != nil {
		return klfResult(map[string]any{"ok": false, "error": err.Error()})
	}
	data, err := klf.Marshal(file)
	if err != nil {
		return klfResult(map[string]any{"ok": false, "error": err.Error()})
	}
	sum := sha256.Sum256(data)
	return klfResult(map[string]any{
		"ok":     true,
		"output": string(data),
		"sha256": hex.EncodeToString(sum[:]),
	})
}

func klfValidateBlock(raw json.RawMessage) string {
	var b klf.Block
	if err := json.Unmarshal(raw, &b); err != nil {
		return klfError("decode block: " + err.Error())
	}
	return klfResult(map[string]any{
		"ok":     true,
		"errors": encodeValidationErrors(klf.ValidateBlock(&b)),
	})
}

func klfValidateTarget(srcRaw, targetRaw json.RawMessage) string {
	var src klf.Block
	if err := json.Unmarshal(srcRaw, &src); err != nil {
		return klfError("decode source block: " + err.Error())
	}
	var target []klf.Run
	if err := json.Unmarshal(targetRaw, &target); err != nil {
		return klfError("decode target runs: " + err.Error())
	}
	return klfResult(map[string]any{
		"ok":     true,
		"errors": encodeValidationErrors(klf.ValidateTargetAgainstSource(&src, target)),
	})
}

func klfResolveAnchor(blockRaw, anchorRaw json.RawMessage) string {
	var b klf.Block
	if err := json.Unmarshal(blockRaw, &b); err != nil {
		return klfError("decode block: " + err.Error())
	}
	var anchor klf.AnnotationAnchor
	if err := json.Unmarshal(anchorRaw, &anchor); err != nil {
		return klfError("decode anchor: " + err.Error())
	}
	res := klf.ResolveAnchor(&b, anchor)
	resolution := map[string]any{
		"ok":   res.OK,
		"kind": string(res.Kind),
	}
	if !res.OK {
		resolution["reason"] = string(res.Err)
	}
	switch res.Kind {
	case klf.AnchorRun:
		if res.RunTarget != nil {
			resolution["runId"] = res.RunTarget.RunID()
		}
	case klf.AnchorRange:
		resolution["rangeText"] = res.RangeText
		resolution["rangeOffset"] = res.RangeOffset
		resolution["rangeLength"] = res.RangeLength
	case klf.AnchorForm:
		resolution["formRunCount"] = len(res.FormRuns)
	}
	return klfResult(map[string]any{"ok": true, "resolution": resolution})
}

func klfRenderHTML(raw json.RawMessage) string {
	var b klf.Block
	if err := json.Unmarshal(raw, &b); err != nil {
		return klfError("decode block: " + err.Error())
	}
	return klfResult(map[string]any{
		"ok":   true,
		"html": klf.RenderBlockHTML(&b, nil),
	})
}

// encodeValidationErrors flattens core/klf validation errors into the
// machine-readable shape the docs pages render (mirrors the TypeScript
// Diagnostic shape so both engines compare cleanly).
func encodeValidationErrors(errs []klf.ValidationError) []map[string]any {
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

func klfError(msg string) string {
	return klfResult(map[string]any{"ok": false, "error": msg})
}

func klfResult(v map[string]any) string {
	data, err := json.Marshal(v)
	if err != nil {
		return `{"ok":false,"error":"failed to encode KLF response"}`
	}
	return string(data)
}
