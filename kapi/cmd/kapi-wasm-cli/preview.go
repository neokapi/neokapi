//go:build js && wasm

package main

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"syscall/js"

	"github.com/neokapi/neokapi/core/model"
)

// previewBlockLimit caps how many blocks we return for the file preview UI.
const previewBlockLimit = 2000

// kapiPreview reads a file through the kapi format reader and returns its
// extracted translatable blocks, so the files panel can show what kapi sees
// (works for binary formats like .docx too, where a raw text view is useless).
//
// Returns a Promise — os.ReadFile and the reader are async under js/wasm and
// would deadlock if run synchronously inside the JS callback.
func kapiPreview(_ js.Value, args []js.Value) any {
	if len(args) < 1 {
		return errorResult("kapiPreview expects a path")
	}
	path := args[0].String()
	executor := js.FuncOf(func(_ js.Value, p []js.Value) any {
		resolve := p[0]
		go func() { resolve.Invoke(doPreview(path)) }()
		return js.Undefined()
	})
	return js.Global().Get("Promise").New(executor)
}

func errorResult(msg string) map[string]any {
	return map[string]any{"ok": false, "error": msg}
}

func doPreview(path string) (result any) {
	defer func() {
		if r := recover(); r != nil {
			result = errorResult("internal error reading file")
		}
	}()

	data, err := os.ReadFile(path)
	if err != nil {
		return errorResult(err.Error())
	}

	// Content-aware detection so an extension shared by several formats
	// (e.g. .xlf/.xliff → XLIFF 1.2 and 2.x) resolves to the reader that
	// actually matches the bytes, not the alphabetically-first claimant.
	fmtName, err := app.FormatReg.DetectFile(path, nil)
	if err != nil {
		return errorResult("unsupported format for " + filepath.Base(path))
	}
	reader, err := app.FormatReg.NewReader(fmtName)
	if err != nil {
		return errorResult(err.Error())
	}

	ctx := context.Background()
	doc := &model.RawDocument{
		URI:          path,
		SourceLocale: "en",
		Encoding:     "UTF-8",
		Reader:       io.NopCloser(bytes.NewReader(data)),
	}
	if err := reader.Open(ctx, doc); err != nil {
		reader.Close()
		return errorResult(err.Error())
	}

	blocks := make([]any, 0, 64)
	total := 0
	for res := range reader.Read(ctx) {
		if res.Error != nil {
			reader.Close()
			return errorResult(res.Error.Error())
		}
		if res.Part == nil || res.Part.Type != model.PartBlock {
			continue
		}
		b, ok := res.Part.Resource.(*model.Block)
		if !ok {
			continue
		}
		total++
		if len(blocks) < previewBlockLimit {
			blocks = append(blocks, map[string]any{"id": b.ID, "text": b.SourceText()})
		}
	}
	reader.Close()

	return map[string]any{
		"ok":     true,
		"format": string(fmtName),
		"blocks": blocks,
		"total":  total,
		"bytes":  len(data),
	}
}
